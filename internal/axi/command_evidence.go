package axi

import (
	"context"
	"strings"
	"sync"
	"time"
)

func previewEvidence(item Object, full bool, output *Response) Object {
	field, source, limit := "details", "tests", 2000
	if _, ok := item["description"]; ok {
		field, source, limit = "description", "problems", 1200
	}
	if full {
		limit = 32768
	}
	preview, truncated := boundedText(Str(item, field), limit)
	if !truncated {
		return item
	}
	output.Meta["truncated"] = true
	if full {
		output.Status = "partial"
		output.Meta["complete"] = false
		notes := commandNotes(output.Meta["limitations"])
		output.Meta["limitations"] = append(notes, Limitation{Code: "TEXT_HARD_LIMIT", Message: "Selected text exceeds the bounded full-text ceiling", Source: source, RunID: Str(item, "runId")})
	}
	result := projectObject(item, keysOf(item))
	result[field] = preview
	return result
}

func projectTest(item Object, parsed Parsed) Object {
	if !flagPresent(parsed, "fields") {
		return item
	}
	keep := []string{"id", "runId", "name", "result", "muted", "ignored"}
	if Str(item, "result") == "unknown" {
		keep = append(keep, "rawStatus")
	}
	return fieldProjection(item, parsed.String("fields"), keep...)
}

func boundedLog(log Object, parsed Parsed, output *Response) Object {
	retained := Objects(log["messages"])
	messages := []Object{}
	limit := 2000
	if parsed.Bool("full") {
		limit = 32768
	}
	for _, m := range retained {
		if flagPresent(parsed, "contains") && !strings.Contains(Str(m, "text"), parsed.String("contains")) {
			continue
		}
		text, truncated := boundedText(Str(m, "text"), limit)
		if truncated {
			output.Meta["truncated"] = true
			if parsed.Bool("full") {
				output.Status = "partial"
				output.Meta["complete"] = false
				output.Meta["limitations"] = append(commandNotes(output.Meta["limitations"]), Limitation{Code: "TEXT_HARD_LIMIT", Message: "Log message exceeds the bounded full-text ceiling", Source: "log", RunID: Str(log, "runId")})
			}
		}
		item := projectObject(m, keysOf(m))
		item["text"] = text
		item["runId"] = log["runId"]
		messages = append(messages, item)
	}
	window := Object{"kind": "tail", "requested": parsed.Int("tail", 80), "providerReturned": log["providerReturned"], "retained": len(retained), "firstMessageId": nil, "lastMessageId": nil, "omittedProviderMessages": Int(log, "providerReturned") - len(retained), "matched": len(messages)}
	if len(retained) > 0 {
		window["firstMessageId"] = retained[0]["id"]
		window["lastMessageId"] = retained[len(retained)-1]["id"]
	}
	result := Object{"messages": messages, "window": window}
	if flagPresent(parsed, "contains") {
		window["contains"] = parsed.String("contains")
		if len(messages) == 0 {
			result["emptyReason"] = "No literal matches in the declared retained tail window"
		}
	}
	return result
}

func ReadEvidence(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	command, runID := parsed.Descriptor.Name, parsed.Positional
	now := time.Now().UnixMilli()
	kind := "tests"
	if command == "run.problems" {
		kind = "problems"
	}
	selected := parsed.String("problem")
	if selected == "" {
		selected = parsed.String("test")
	}
	selectedDefined := flagPresent(parsed, "problem") || flagPresent(parsed, "test")
	if selectedDefined {
		if _, err := OccurrenceLocator(kind, selected, runID); err != nil {
			return Response{}, err
		}
	}
	cursor, err := commandCursor(parsed, now)
	if err != nil {
		return Response{}, err
	}
	query := ReadRequest{Kind: kind + ".page", RunID: runID, Count: parsed.Int("limit", 20), ScanLimit: 5000}
	if cursor != nil {
		query.Start = cursor.Position
	}
	if command == "run.tests" && (parsed.Bool("failed") || parsed.Bool("muted")) {
		query.Failed = true
		query.FailedSet = true
	}
	if command == "run.tests" && parsed.Bool("muted") {
		query.Muted = true
		query.MutedSet = true
	} else if command == "run.tests" && parsed.Bool("failed") && !parsed.Bool("include-muted") {
		query.Muted = false
		query.MutedSet = true
	}
	if command != "run.log" {
		if _, err = EvidenceRequest(kind, query); err != nil {
			return Response{}, err
		}
	}
	session, err := openCommandSession(ctx, ec, "simple")
	if err != nil {
		return Response{}, err
	}
	defer session.Close()
	read, err := commandRun(ctx, parsed, ec, session)
	if err != nil {
		return Response{}, err
	}
	output := NewResponse(command, Object{"run": commandPrimaryRun(read.Value)})
	output.Context = commandRunContext(ec, read)
	output.Meta["limitations"] = append([]Limitation{}, read.Provenance.Limitations...)
	scopeArgs := commandScopeArgs(ec, read)
	if command == "run.log" {
		providerTruncated := false
		if parsed.Bool("failed") {
			requests := []ReadRequest{{Kind: "problems.page", RunID: runID, Count: 20, ScanLimit: 5000}, {Kind: "tests.page", RunID: runID, Count: 20, ScanLimit: 5000, Failed: true, FailedSet: true, Muted: false, MutedSet: true}, {Kind: "log.tail", ID: runID, Tail: 80}}
			results := make([]ReadResult, len(requests))
			var wg sync.WaitGroup
			for i, r := range requests {
				wg.Add(1)
				go func(i int, r ReadRequest) {
					defer wg.Done()
					results[i] = session.Reader.Read(ctx, r, Budget{Deadline: ec.Deadline, MaxChildProcesses: -1})
				}(i, r)
			}
			wg.Wait()
			sources := Object{}
			output.Data["mode"] = "failed"
			for i, r := range results {
				name := []string{"problems", "tests", "log"}[i]
				if r.State == "unavailable" {
					if r.Error.Code == "INTERRUPTED" {
						return Response{}, r.Error
					}
					output.Status = "partial"
					output.Meta["complete"] = false
					output.Meta["limitations"] = append(commandNotes(output.Meta["limitations"]), Limitation{Code: r.Error.Code, Message: "Independent source is unavailable", Source: name, RunID: runID})
					sources[name] = Object{"availability": "unavailable", "complete": false, "coverage": "unavailable", "errorCode": r.Error.Code}
					continue
				}
				limits := r.Provenance.Limitations
				coverage := "tail_window"
				if name != "log" {
					limits = pageLimitations(r.Value)
					coverage = "bounded_page"
				}
				output.Meta["limitations"] = append(commandNotes(output.Meta["limitations"]), limits...)
				sources[name] = Object{"availability": "available", "complete": len(limits) == 0, "coverage": coverage}
				if name == "log" {
					for k, v := range boundedLog(r.Value, parsed, &output) {
						output.Data[k] = v
					}
					providerTruncated = Bool(r.Value, "truncated")
				} else {
					items := []Object{}
					for _, item := range Objects(r.Value["items"]) {
						items = append(items, previewEvidence(item, parsed.Bool("full"), &output))
					}
					output.Data[name] = items
				}
			}
			output.Data["sources"] = sources
		} else {
			log, err := commandRead(ctx, session, ec, ReadRequest{Kind: "log.tail", ID: runID, Tail: parsed.Int("tail", 80)})
			if err != nil {
				if containsString([]string{"INTERRUPTED", "DEADLINE_EXCEEDED", "INPUT_LIMIT_EXCEEDED", "CONTEXT_MISMATCH", "UPSTREAM_SCHEMA_MISMATCH"}, AsDomainError(err).Code) {
					return Response{}, err
				}
				return Response{}, NewError("CAPABILITY_UNAVAILABLE", "Bounded structured log messages are unavailable", 1)
			}
			output.Meta["limitations"] = append(commandNotes(output.Meta["limitations"]), log.Provenance.Limitations...)
			output.Data["mode"] = "tail"
			for k, v := range boundedLog(log.Value, parsed, &output) {
				output.Data[k] = v
			}
			providerTruncated = Bool(log.Value, "truncated")
		}
		if Bool(output.Meta, "truncated") && !parsed.Bool("full") {
			argv := append([]string{"teamcity-axi", "run", "log", runID}, scopeArgs...)
			if parsed.Bool("failed") {
				argv = append(argv, "--failed")
			} else {
				argv = append(argv, "--tail", numberArg(parsed.Int("tail", 80)))
				if flagPresent(parsed, "contains") {
					argv = append(argv, "--contains", parsed.String("contains"))
				}
			}
			argv = append(argv, "--full")
			output.Next = []Object{commandHint("Expand bounded log text", argv)}
		}
		if providerTruncated {
			output.Meta["truncated"] = true
		}
	} else {
		request, err := EvidenceRequest(kind, query)
		if err != nil {
			return Response{}, err
		}
		bound := CursorBinding{Command: command, Server: ec.Server, Count: query.Count, FilterHash: commandHash(Object{"serverUrl": ec.ServerURL, "runId": runID, "jobId": read.Value["jobId"], "projectId": read.Provenance.ProjectID, "request": request.Filters, "allowedProjects": sortedAllowed(ec)})}
		if cursor != nil {
			if err = AssertCursor(*cursor, bound); err != nil {
				return Response{}, err
			}
		}
		items := []Object{}
		var page Object
		if selectedDefined {
			detail, err := commandRead(ctx, session, ec, ReadRequest{Kind: strings.TrimSuffix(kind, "s") + ".detail", RunID: runID, ID: selected})
			if err != nil {
				return Response{}, err
			}
			items = append(items, detail.Value)
			output.Meta["limitations"] = append(commandNotes(output.Meta["limitations"]), detail.Provenance.Limitations...)
		} else {
			p, err := commandRead(ctx, session, ec, query)
			if err != nil {
				return Response{}, err
			}
			page = p.Value
			items = Objects(page["items"])
			output.Meta["limitations"] = append(commandNotes(output.Meta["limitations"]), pageLimitations(page)...)
		}
		var token any
		if page != nil {
			var note *Limitation
			token, note, err = continuation(bound, cursor, page, now)
			if err != nil {
				return Response{}, err
			}
			if note != nil {
				note.Source = kind
				note.RunID = runID
				note.Message = "Cursor expired during acquisition; useful rows are retained"
				output.Meta["limitations"] = append(commandNotes(output.Meta["limitations"]), *note)
			}
		}
		displayed := []Object{}
		var previewTarget Object
		for _, item := range items {
			if kind == "tests" {
				item = projectTest(item, parsed)
			}
			field, limit := "details", 2000
			if kind == "problems" {
				field, limit = "description", 1200
			}
			if previewTarget == nil && len([]rune(Str(item, field))) > limit {
				previewTarget = item
			}
			displayed = append(displayed, previewEvidence(item, parsed.Bool("full"), &output))
		}
		selection := Object{"kind": "page", "pageSize": query.Count, "providerReturned": valueOrNull(page, "providerReturned"), "position": query.Start, "scanLimit": 5000, "consistency": "best_effort_offset"}
		pageOut := commandPage(displayed, page, token)
		if selectedDefined {
			selection["kind"] = "occurrence"
			selection["pageSize"] = 1
			selection["providerReturned"] = 1
			selection["occurrenceId"] = selected
			pageOut["total"] = 1
			pageOut["totalKind"] = "exact"
			pageOut["hasMore"] = false
		}
		if kind == "tests" {
			filter := "all"
			if parsed.Bool("muted") {
				filter = "muted_failures"
			} else if parsed.Bool("failed") {
				filter = "unmuted_failures"
				if parsed.Bool("include-muted") {
					filter = "failures_including_muted"
				}
			}
			selection["filter"] = filter
		}
		output.Data[kind] = displayed
		output.Data["selection"] = selection
		output.Data["page"] = pageOut
		if token != nil {
			argv := append([]string{"teamcity-axi", "run", kind, runID}, scopeArgs...)
			argv = append(argv, "--limit", numberArg(query.Count), "--cursor", Text(token))
			for _, flag := range []string{"failed", "muted", "include-muted"} {
				if parsed.Bool(flag) {
					argv = append(argv, "--"+flag)
				}
			}
			if flagPresent(parsed, "fields") {
				argv = append(argv, "--fields", parsed.String("fields"))
			}
			if parsed.Bool("full") {
				argv = append(argv, "--full")
			}
			output.Next = append(output.Next, commandHint("Read the next bounded occurrence page", argv))
		}
		if previewTarget != nil && !parsed.Bool("full") {
			argv := append([]string{"teamcity-axi", "run", kind, runID}, scopeArgs...)
			flag := "--test"
			if kind == "problems" {
				flag = "--problem"
			}
			argv = append(argv, flag, Str(previewTarget, "id"), "--full")
			output.Next = append(output.Next, commandHint("Expand an exact occurrence", argv))
		}
	}
	output.Meta["observedAt"] = ObservedAt()
	output = commandFinish(output, parsed, ec, session, 0, commandNotes(output.Meta["limitations"]), "UNVERIFIED_VERSION", "UNKNOWN_LIFECYCLE", "UNKNOWN_RESULT")
	if len(commandNotes(output.Meta["limitations"])) == 0 {
		delete(output.Meta, "limitations")
	}
	return output, nil
}

func ReadChanges(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	runID := parsed.Positional
	now := time.Now().UnixMilli()
	cursor, err := commandCursor(parsed, now)
	if err != nil {
		return Response{}, err
	}
	query := ReadRequest{Kind: "changes.page", RunID: runID, Count: parsed.Int("limit", 10), ScanLimit: 5000, Files: parsed.Bool("files")}
	if cursor != nil {
		query.Start = cursor.Position
	}
	request, err := RelatedRequest("changes", query, "")
	if err != nil {
		return Response{}, err
	}
	session, err := openCommandSession(ctx, ec, "simple")
	if err != nil {
		return Response{}, err
	}
	defer session.Close()
	read, err := commandRun(ctx, parsed, ec, session)
	if err != nil {
		return Response{}, err
	}
	bound := CursorBinding{Command: "run.changes", Server: ec.Server, Count: query.Count, FilterHash: commandHash(Object{"serverUrl": ec.ServerURL, "runId": runID, "jobId": read.Value["jobId"], "projectId": read.Provenance.ProjectID, "filters": request.Filters, "files": query.Files, "allowedProjects": sortedAllowed(ec)})}
	if cursor != nil {
		if err = AssertCursor(*cursor, bound); err != nil {
			return Response{}, err
		}
	}
	p, err := commandRead(ctx, session, ec, query)
	if err != nil {
		return Response{}, err
	}
	page := p.Value
	output := NewResponse("run.changes", Object{})
	output.Context = commandRunContext(ec, read)
	output.Meta["limitations"] = append(append([]Limitation{}, read.Provenance.Limitations...), pageLimitations(page)...)
	changes := []Object{}
	previewed := false
	roots := Objects(read.Value["revisions"])
	for _, change := range Objects(page["items"]) {
		if _, known := read.Value["revisions"]; known {
			found := false
			for _, root := range roots {
				if root["vcsRootId"] == change["vcsRootId"] {
					found = true
				}
			}
			if !found {
				return Response{}, NewError("CONTEXT_MISMATCH", "Change belongs to a VCS root absent from the selected run", 1)
			}
		}
		message := Str(change, "message")
		limit := 200
		display := strings.SplitN(message, "\n", 2)[0]
		if parsed.Bool("full") {
			display = message
			limit = 32768
		}
		display, _ = boundedText(display, limit)
		if message != display {
			output.Meta["truncated"] = true
			previewed = true
			if parsed.Bool("full") {
				output.Meta["limitations"] = append(commandNotes(output.Meta["limitations"]), Limitation{Code: "TEXT_HARD_LIMIT", Message: "Change message exceeds the bounded full-text ceiling", Source: "changes", RunID: runID})
			}
		}
		if Int(Obj(change["fileCoverage"]), "omitted") > 0 {
			output.Meta["truncated"] = true
		}
		item := projectObject(change, keysOf(change))
		item["message"] = display
		if flagPresent(parsed, "fields") {
			keep := []string{"id", "version", "vcsRootId", "message"}
			if containsString(strings.Split(parsed.String("fields"), ","), "files") {
				keep = append(keep, "fileCoverage")
			}
			item = fieldProjection(item, parsed.String("fields"), keep...)
		}
		changes = append(changes, item)
	}
	token, note, err := continuation(bound, cursor, page, now)
	if err != nil {
		return Response{}, err
	}
	if note != nil {
		note.Source = "changes"
		note.RunID = runID
		note.Message = "Cursor expired during acquisition; useful rows retained"
		output.Meta["limitations"] = append(commandNotes(output.Meta["limitations"]), *note)
	}
	output.Data = Object{"run": commandPrimaryRun(read.Value), "changes": changes, "selection": Object{"runId": runID, "pageSize": query.Count, "providerReturned": page["providerReturned"], "position": query.Start, "scanLimit": 5000, "filesRequested": query.Files, "consistency": "best_effort_offset", "meaning": "changes_associated_with_run"}, "page": commandPage(changes, page, token)}
	args := append([]string{"teamcity-axi", "run", "changes", runID}, commandScopeArgs(ec, read)...)
	args = append(args, "--limit", numberArg(query.Count))
	if query.Files {
		args = append(args, "--files")
	}
	if flagPresent(parsed, "fields") {
		args = append(args, "--fields", parsed.String("fields"))
	}
	if token != nil {
		argv := append(append([]string{}, args...), "--cursor", Text(token))
		if parsed.Bool("full") {
			argv = append(argv, "--full")
		}
		output.Next = append(output.Next, commandHint("Read the next bounded change page", argv))
	}
	if previewed && !parsed.Bool("full") {
		argv := append([]string{}, args...)
		if flagPresent(parsed, "cursor") {
			argv = append(argv, "--cursor", parsed.String("cursor"))
		}
		argv = append(argv, "--full")
		output.Next = append(output.Next, commandHint("Expand messages in this bounded change page", argv))
	}
	return commandFinish(output, parsed, ec, session, 0, commandNotes(output.Meta["limitations"]), "UNVERIFIED_VERSION", "UNKNOWN_LIFECYCLE", "UNKNOWN_RESULT"), nil
}
