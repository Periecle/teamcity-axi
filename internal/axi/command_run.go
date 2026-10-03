package axi

import (
	"context"
	"strings"
	"time"
)

func ViewRun(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	session, err := openCommandSession(ctx, ec, "simple")
	if err != nil {
		return Response{}, err
	}
	defer session.Close()
	read, err := commandRun(ctx, parsed, ec, session)
	if err != nil {
		return Response{}, err
	}
	run := read.Value
	output := NewResponse("run.view", Object{"run": run})
	output.Context = commandRunContext(ec, read)
	output.Meta["observedAt"] = read.Provenance.ObservedAt
	if text := Str(run, "statusText"); text != "" && !parsed.Bool("full") {
		if preview, truncated := boundedText(text, 1200); truncated {
			run = projectObject(run, keysOf(run))
			run["statusText"] = preview
			output.Meta["truncated"] = true
			argv := []string{"teamcity-axi", "run", "view", Str(run, "id"), "--full", "--server", ec.Server}
			for _, name := range []string{"job", "project"} {
				if parsed.String(name) != "" {
					argv = append(argv, "--"+name, parsed.String(name))
				}
			}
			output.Next = []Object{commandHint("Expand the execution summary", argv)}
		}
	}
	if flagPresent(parsed, "fields") {
		keep := []string{"id", "jobId", "state", "result"}
		if Str(run, "result") == "unknown" {
			keep = append(keep, "rawStatus")
		}
		run = fieldProjection(run, parsed.String("fields"), keep...)
	}
	output.Data = Object{"run": run}
	return commandFinish(output, parsed, ec, session, 0, read.Provenance.Limitations, "UNKNOWN_LIFECYCLE", "UNKNOWN_RESULT", "UNVERIFIED_VERSION"), nil
}

func keysOf(o Object) []string {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	return keys
}

func ListRuns(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, error) {
	if ec.Job == "" && ec.Project == "" {
		return Response{}, NewError("CONTEXT_REQUIRED", "Select a job or project for run list", 2)
	}
	result := parsed.String("result")
	if result != "" && !containsString([]string{"success", "failure", "error", "canceled", "failed_to_start", "unknown"}, result) {
		return Response{}, NewError("DEPENDENCY_UNSUPPORTED", "This outcome filter requires explicit metadata not yet verified by the adapter", 1)
	}
	if flagPresent(parsed, "revision") && ec.VCSRootID == "" {
		return Response{}, NewError("CONTEXT_REQUIRED", "Revision filtering requires an exact VCS root or unambiguous repository mapping", 2)
	}
	now := time.Now().UnixMilli()
	cursor, err := commandCursor(parsed, now)
	if err != nil {
		return Response{}, err
	}
	state := parsed.String("state")
	if state == "" {
		state = "finished"
	}
	until := time.UnixMilli(now / 1000 * 1000).UTC().Format("2006-01-02T15:04:05.000Z")
	if cursor != nil && cursor.Window != nil {
		until = cursor.Window.Until
	}
	if flagPresent(parsed, "until") {
		until, err = CanonicalTimestamp(parsed.String("until"))
		if err != nil {
			return Response{}, err
		}
	}
	since, err := ShiftTimestamp(until, -7*86400)
	if err != nil {
		return Response{}, err
	}
	if cursor != nil && cursor.Window != nil {
		since = cursor.Window.Since
	}
	if flagPresent(parsed, "since") {
		since, err = CanonicalTimestamp(parsed.String("since"))
		if err != nil {
			return Response{}, err
		}
	}
	query := ReadRequest{Kind: "run.page", JobID: ec.Job, ProjectID: ec.Project, Branch: ec.Branch, BranchSet: ec.BranchDefined || ec.Branch != "", State: state, Result: result, Count: parsed.Int("limit", 20), ScanLimit: 5000, AllowedProjects: commandAllowed(ec)}
	if cursor != nil {
		query.Start = cursor.Position
	}
	if state == "finished" {
		query.Window = &TimeWindow{Since: since, Until: until}
	}
	if flagPresent(parsed, "revision") {
		query.Revision = ec.Revision
		query.VCSRootID = ec.VCSRootID
	}
	if _, err = RunFilters(query); err != nil {
		return Response{}, err
	}
	session, err := openCommandSession(ctx, ec, "simple")
	if err != nil {
		return Response{}, err
	}
	defer session.Close()
	query, err = commandOwnedScope(ctx, session, ec, query, "Selected job belongs to another project", true)
	if err != nil {
		return Response{}, err
	}
	if commandAllowed(ec) != nil {
		query.AllowedProjects = []string{query.ProjectID}
	}
	filters, err := RunFilters(query)
	if err != nil {
		return Response{}, err
	}
	bound := CursorBinding{Command: "run.list", Server: ec.Server, Count: query.Count, Window: query.Window, FilterHash: commandHash(Object{"serverUrl": ec.ServerURL, "filters": filters, "result": nullString(result), "revision": nullString(query.Revision), "vcsRootId": nullString(query.VCSRootID), "allowedProjects": sortedAllowed(ec), "window": query.Window})}
	if cursor != nil {
		if err = AssertCursor(*cursor, bound); err != nil {
			return Response{}, err
		}
	}
	read, err := commandRead(ctx, session, ec, query)
	if err != nil {
		return Response{}, err
	}
	page := read.Value
	limits := append(pageLimitations(page), Limitation{Code: "BEST_EFFORT_PAGINATION", Message: "Offset pages can change when executions are inserted or deleted", Source: "run"})
	limits = commandVersion(limits, session)
	if flagPresent(parsed, "revision") && ec.Dirty != nil && *ec.Dirty {
		limits = append(limits, Limitation{Code: "DIRTY_WORKTREE", Message: "Remote revision matches do not cover uncommitted local changes", Source: "context"})
	}
	token, note, err := continuation(bound, cursor, page, now)
	if err != nil {
		limits = append(limits, Limitation{Code: "CURSOR_LIMIT_EXCEEDED", Message: "Exact query bounds exceed the cursor budget; continuation is unavailable", Source: "run"})
		token = nil
	}
	if note != nil {
		note.Source = "run"
		limits = append(limits, *note)
	}
	keep := []string{"id", "jobId", "state", "result"}
	if !query.BranchSet {
		keep = append(keep, "branch")
	}
	if flagPresent(parsed, "revision") {
		keep = append(keep, "revisions")
	}
	fields := parsed.String("fields")
	if fields == "" {
		fields = "branch"
	}
	keep = append(keep, strings.Split(fields, ",")...)
	runs := []Object{}
	rawRuns := Objects(page["runs"])
	if rawRuns == nil {
		rawRuns = Objects(page["items"])
	}
	for _, r := range rawRuns {
		keys := keep
		if Str(r, "result") == "unknown" {
			keys = append(append([]string{}, keep...), "rawStatus")
		}
		runs = append(runs, projectObject(r, keys))
	}
	exact := query.Start == 0 && page["hasMore"] == false
	for _, l := range limits {
		if l.Code != "BEST_EFFORT_PAGINATION" {
			exact = false
		}
	}
	pageOut := commandPage(runs, page, token)
	if exact {
		pageOut["total"] = len(runs)
		pageOut["totalKind"] = "exact"
	}
	exhaustion := "unverified"
	if page["hasMore"] == false {
		exhaustion = "verified_server_pagination"
	} else if page["hasMore"] == true {
		exhaustion = "provider_continuation"
	}
	selection := Object{"pageSize": query.Count, "providerReturned": page["providerReturned"], "position": query.Start, "scanLimit": 5000, "consistency": "best_effort_offset", "exhaustionBasis": exhaustion, "timestampBasis": nil, "result": nullString(result), "resultBasis": nil}
	if result != "" {
		selection["resultBasis"] = "provider"
		if result == "unknown" {
			selection["resultBasis"] = "normalized_candidates"
		}
	}
	if query.Window != nil {
		selection["timestampBasis"] = "finishTime"
		selection["timestampMembership"] = "provider"
		selection["reportedTimestampPrecision"] = "second"
		selection["filterTimestampPrecision"] = "millisecond"
		selection["window"] = Object{"since": since, "until": until, "bounds": "exclusive"}
	}
	aggregates := Object{"scope": "returnedPage"}
	for _, kind := range []string{"failure", "success", "error", "canceled", "failed_to_start", "unknown"} {
		n := 0
		for _, r := range runs {
			if Str(r, "result") == kind {
				n++
			}
		}
		aggregates[kind] = n
	}
	data := Object{"runs": runs, "page": pageOut, "selection": selection, "aggregates": aggregates}
	if len(runs) == 0 {
		reason := "No rows returned; bounded search exhaustion is unverified"
		if page["hasMore"] == true {
			reason = "No matches in this page; bounded continuation remains"
		} else if page["hasMore"] == false {
			reason = "No retained matches in this page; the query total remains unknown"
			if exact {
				reason = "No matching runs in the exhausted query scope"
			}
		}
		data["emptyReason"] = reason
	}
	output := NewResponse("run.list", data)
	output.Context = Object{"server": ec.Server}
	if query.ProjectID != "" {
		output.Context["project"] = query.ProjectID
	}
	if query.JobID != "" {
		output.Context["job"] = query.JobID
	}
	if query.BranchSet {
		output.Context["branch"] = query.Branch
	}
	if query.Revision != "" {
		output.Context["revision"] = query.Revision
		output.Context["vcsRootId"] = query.VCSRootID
	}
	output.Meta["observedAt"] = read.Provenance.ObservedAt
	commandCounts(&output, session)
	commandLimitations(&output, limits, "BEST_EFFORT_PAGINATION", "UNVERIFIED_VERSION", "UNKNOWN_LIFECYCLE", "UNKNOWN_RESULT")
	if token != nil && !parsed.Bool("no-hints") {
		argv := []string{"teamcity-axi", "run", "list", "--server", ec.Server}
		if query.JobID != "" {
			argv = append(argv, "--job", query.JobID)
		}
		if query.ProjectID != "" {
			argv = append(argv, "--project", query.ProjectID)
		}
		if query.BranchSet {
			argv = append(argv, "--literal-branch", query.Branch)
		} else {
			argv = append(argv, "--all-branches")
		}
		argv = append(argv, "--state", state)
		if result != "" {
			argv = append(argv, "--result", result)
		}
		if query.Revision != "" {
			argv = append(argv, "--revision", query.Revision, "--vcs-root", query.VCSRootID)
		}
		if flagPresent(parsed, "fields") {
			argv = append(argv, "--fields", parsed.String("fields"))
		}
		argv = append(argv, "--limit", numberArg(query.Count), "--cursor", Text(token))
		output.Next = []Object{commandHint("Read the next bounded page of this query", argv)}
	}
	return output, nil
}

func commandOwnedScope(ctx context.Context, session *ReadSession, ec ExecutionContext, query ReadRequest, mismatch string, required bool) (ReadRequest, error) {
	policy := commandPolicy(session, ec, Budget{Deadline: ec.Deadline, MaxChildProcesses: -1})
	if query.JobID != "" {
		read, err := commandRead(ctx, session, ec, ReadRequest{Kind: "job.detail", ID: query.JobID})
		if err != nil {
			return query, err
		}
		owner := Str(read.Value, "projectId")
		if query.ProjectID != "" && query.ProjectID != owner {
			return query, NewError("CONTEXT_MISMATCH", mismatch, 1)
		}
		query.ProjectID = owner
	} else if query.ProjectID != "" {
		if _, err := policy.Project(ctx, query.ProjectID); err != nil {
			return query, err
		}
	}
	if query.ProjectID != "" || required {
		if err := policy.Assert(ctx, query.ProjectID); err != nil {
			return query, err
		}
	}
	return query, nil
}
