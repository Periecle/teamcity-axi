package axi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type FailureOptions struct {
	Primary           *ReadResult
	Budget            Budget
	MaxChildProcesses int
	ChildProcesses    func() int
	Depth             int
	MaxNodes          int
	MaxDiagnosedRuns  int
	Full              bool
	Secrets           []string
	AssertProject     func(context.Context, string, Budget) error
}

func failureResult(result string) bool {
	return result == "failure" || result == "error" || result == "canceled" || result == "failed_to_start"
}

// SelectDiagnosedRuns prioritizes explicit references, observed failures, and then unknown outcomes.
func SelectDiagnosedRuns(graph Object, depths map[string]int, cap int, referenced map[string]bool) ([]Object, int) {
	var root Object
	candidates := []Object{}
	for _, node := range Objects(graph["nodes"]) {
		run := Obj(node["run"])
		if Str(run, "id") == Str(graph, "rootRunId") {
			root = node
		} else if failureResult(Str(run, "result")) || Str(run, "state") == "unknown" || Str(run, "result") == "unknown" {
			candidates = append(candidates, node)
		}
	}
	priority := func(node Object) int {
		run := Obj(node["run"])
		if referenced[Str(run, "id")] {
			return 0
		}
		if failureResult(Str(run, "result")) {
			return 1
		}
		return 2
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if priority(a) != priority(b) {
			return priority(a) < priority(b)
		}
		left, right := Str(Obj(a["run"]), "id"), Str(Obj(b["run"]), "id")
		if depths[left] != depths[right] {
			return depths[left] < depths[right]
		}
		return numericRunLess(left, right)
	})
	retained := min(max(0, cap-1), len(candidates))
	return append([]Object{root}, candidates[:retained]...), max(0, len(candidates)-cap+1)
}

func boundedText(value string, limit int) (string, bool) {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]), true
	}
	return value, false
}

func firstLine(value string) string {
	if end := strings.IndexByte(value, '\n'); end >= 0 {
		return strings.TrimSuffix(value[:end], "\r")
	}
	return value
}

var failureLogSignal = regexp.MustCompile(`(?i)connection refused|timeout|exception|\berror\b`)

// InvestigateFailure records independent observations without claiming a cause from graph ancestry.
func InvestigateFailure(ctx context.Context, reader Reader, ec ExecutionContext, runID string, options FailureOptions) (Object, error) {
	if options.MaxDiagnosedRuns < 1 || options.MaxDiagnosedRuns > 10 {
		return nil, Usage("Invalid diagnosis bound")
	}
	options.Budget = plannerBudget(ec, options.Budget)
	var primaryRead ReadResult
	if options.Primary != nil {
		primaryRead = *options.Primary
	} else {
		primaryRead = reader.Read(ctx, ReadRequest{Kind: "run.detail", ID: runID}, options.Budget)
	}
	if primaryRead.State == "unavailable" {
		return nil, readFailure(primaryRead)
	}
	initial := primaryRead.Value
	rootID := Str(initial, "id")
	reserve := 0
	if Str(initial, "state") != "finished" {
		reserve = 1
	}
	budget := options.Budget
	budget.MaxChildProcesses = max(0, options.MaxChildProcesses-reserve)
	// A runner supplies the authoritative launch count. The fallback counts successful planned reads.
	localLaunches := 0
	var countMu sync.Mutex
	childProcesses := func() int {
		if options.ChildProcesses != nil {
			return options.ChildProcesses()
		}
		countMu.Lock()
		defer countMu.Unlock()
		return localLaunches
	}
	countedRead := func(request ReadRequest, readBudget Budget) ReadResult {
		if options.ChildProcesses == nil {
			countMu.Lock()
			localLaunches++
			countMu.Unlock()
		}
		return reader.Read(ctx, request, readBudget)
	}
	sources, findings := []Object{}, []Object{}
	limitations := append([]Limitation{}, primaryRead.Provenance.Limitations...)
	var limitsMu sync.Mutex
	appendLimitations := func(notes ...Limitation) {
		limitsMu.Lock()
		limitations = append(limitations, notes...)
		limitsMu.Unlock()
	}
	truncated, hardTextLimit, finalFailed, changed := false, false, false, false
	primary := initial
	var optionalChanges []Object
	coverage := func(id, kind string, required bool, suffix string) Object {
		scope := "bounded_page"
		switch kind {
		case "log":
			scope = "tail_window"
		case "run":
			scope = "execution"
		case "dependencies":
			scope = "immediate_dependencies"
		}
		source := Object{"id": kind + ":" + id + suffix, "runId": id, "kind": kind, "required": required, "scope": scope, "state": "not_requested", "returned": nil, "total": nil, "reasonCode": "NOT_REQUESTED"}
		sources = append(sources, source)
		return source
	}
	primarySource := coverage(rootID, "run", true, "")
	primarySource["state"], primarySource["returned"], primarySource["total"], primarySource["observedAt"] = "complete", 1, 1, primaryRead.Provenance.ObservedAt
	delete(primarySource, "reasonCode")
	projects := map[string]string{rootID: primaryRead.Provenance.ProjectID}
	scopedArgs := func(run Object, project string) []string {
		args := []string{"--server", ec.Server, "--job", Str(run, "jobId")}
		if project != "" {
			args = append(args, "--project", project)
		}
		return args
	}
	excerpt := func(value, kind, id string) string {
		limit := 2000
		if options.Full {
			limit = 8192
		} else if kind == "problems" {
			limit = 1200
		} else if kind == "changes" {
			limit = 200
		}
		text, cut := boundedText(SanitizeText(value, options.Secrets), limit)
		if cut {
			truncated = true
			if options.Full {
				hardTextLimit = true
				appendLimitations(Limitation{Code: "TEXT_HARD_LIMIT", Message: "Evidence excerpt reached its bounded full-text ceiling", Source: kind, RunID: id})
			}
		}
		return text
	}
	finding := func(run, source Object, kind, itemID, summary, text string, argv []string) {
		safeText := SanitizeText(text, options.Secrets)
		components := []string{Str(run, "id"), Str(run, "jobId"), kind, Str(source, "id"), itemID, strings.Join(strings.Fields(safeText), " ")}
		for i := range components {
			components[i] = SanitizeText(components[i], options.Secrets)
		}
		var fingerprint bytes.Buffer
		encoder := json.NewEncoder(&fingerprint)
		encoder.SetEscapeHTML(false)
		_ = encoder.Encode(components)
		hashBytes := sha256.Sum256(bytes.TrimSuffix(fingerprint.Bytes(), []byte{'\n'}))
		hash := hex.EncodeToString(hashBytes[:])
		evidenceKind, reason := "run", "Read the exact supporting evidence"
		switch kind {
		case "build_problem":
			evidenceKind = "problem"
		case "failed_test":
			evidenceKind = "test"
		case "log_signal":
			evidenceKind, reason = "log", "Reinspect the declared source tail window"
		}
		safeSummary, cut := boundedText(SanitizeText(summary, options.Secrets), 1200)
		truncated = truncated || cut
		id := Str(run, "id")
		reference := Object{"id": "ev:" + id + ":" + hash, "sourceRef": source["id"], "runId": id, "kind": evidenceKind, "itemId": itemID, "excerpt": excerpt(safeText, Str(source, "kind"), id), "observedAt": source["observedAt"], "retrieve": Object{"reason": reason, "argv": argv}}
		findings = append(findings, Object{"id": "finding:" + id + ":" + hash, "runId": id, "kind": kind, "claim": "observation", "summary": safeSummary, "evidence": []Object{reference}})
	}
	collect := func(source Object, request ReadRequest) (*ReadResult, error) {
		var domain *DomainError
		var read ReadResult
		if ctx.Err() != nil {
			domain = NewError("INTERRUPTED", "Evidence collection interrupted", 130)
		} else if time.Now().UnixMilli() >= budget.Deadline {
			domain = NewError("DEADLINE_EXCEEDED", "Shared investigation deadline exhausted", 1)
		} else if childProcesses() >= budget.MaxChildProcesses {
			domain = NewError("CALL_LIMIT_EXCEEDED", "Reserved evidence capacity exhausted", 1)
		} else {
			read = countedRead(request, budget)
			if read.State == "unavailable" {
				domain = readFailure(read)
			}
		}
		if domain == nil {
			source["observedAt"], source["state"] = read.Provenance.ObservedAt, "complete"
			delete(source, "reasonCode")
			return &read, nil
		}
		if domain.Code == "INTERRUPTED" {
			return nil, domain
		}
		state := "unavailable"
		if domain.Code == "CALL_LIMIT_EXCEEDED" || domain.Code == "DEADLINE_EXCEEDED" {
			state = "budget_exhausted"
		}
		source["state"], source["reasonCode"], source["observedAt"] = state, domain.Code, ObservedAt()
		if Bool(source, "required") {
			appendLimitations(Limitation{Code: domain.Code, Message: "A planned independent evidence source is unavailable", Source: Str(source, "kind"), RunID: Str(source, "runId")})
		}
		return nil, nil
	}
	page := func(source Object, read *ReadResult) {
		if read == nil {
			return
		}
		notes := plannerLimitations(read.Value["limitations"])
		source["returned"], source["providerReturned"], source["total"] = len(Objects(read.Value["items"])), Int(read.Value, "providerReturned"), nil
		more, defined := read.Value["hasMore"].(bool)
		if defined && !more && len(notes) == 0 {
			source["state"] = "complete"
		} else {
			source["state"], source["reasonCode"] = "partial", "BOUNDED_PAGE"
			if len(notes) != 0 {
				source["reasonCode"] = notes[0].Code
			}
		}
		for _, note := range notes {
			if !Bool(source, "required") && note.Code == "SCAN_COVERAGE_UNKNOWN" {
				continue
			}
			note.Source, note.RunID = Str(source, "kind"), Str(source, "runId")
			appendLimitations(note)
		}
	}
	notFailed := Str(initial, "state") == "finished" && Str(initial, "result") == "success"
	rootProblemsSource, rootTestsSource := coverage(rootID, "problems", !notFailed, ""), coverage(rootID, "tests", !notFailed, "")
	rootLogSource, changesSource := coverage(rootID, "log", false, ""), coverage(rootID, "changes", false, "")
	var graph Object
	graphReadAttempts, maxGraphReads, omittedGraphTargets, omittedDiagnosedRuns := 0, 0, 0, 0
	diagnosedRunIDs := []string{}
	if notFailed {
		graph = Object{"rootRunId": rootID, "complete": false, "nodes": []Object{{"run": GraphRun(initial), "expansion": "not_requested", "dependencyCount": nil, "observedDependencies": nil}}, "edges": []Object{}, "cycles": []Object{}, "unexpanded": 1}
		coverage(rootID, "dependencies", false, "")
	} else {
		rootProblems, err := collect(rootProblemsSource, ReadRequest{Kind: "problems.page", RunID: rootID, Count: 20, ScanLimit: 5000})
		if err != nil {
			return nil, err
		}
		page(rootProblemsSource, rootProblems)
		graphCeiling := max(0, budget.MaxChildProcesses-2)
		maxGraphReads = min(10, max(0, graphCeiling-childProcesses()))
		graphBudget := budget
		graphBudget.MaxChildProcesses = graphCeiling
		traversal, err := BuildGraph(ctx, plannerCountingReader{read: countedRead}, ec, rootID, GraphOptions{Root: initial, RootProjectID: primaryRead.Provenance.ProjectID, RootProjectKnown: true, Budget: graphBudget, Depth: options.Depth, MaxNodes: options.MaxNodes, MaxGraphReads: maxGraphReads, CanRead: func() bool { return childProcesses() < graphCeiling }, AssertProject: options.AssertProject})
		if err != nil {
			return nil, err
		}
		graph, graphReadAttempts, omittedGraphTargets = traversal.Graph, traversal.GraphReadAttempts, traversal.OmittedTargets
		appendLimitations(traversal.Limitations...)
		for id, project := range traversal.Projects {
			projects[id] = project
		}
		selected, omitted := SelectDiagnosedRuns(graph, traversal.Depths, options.MaxDiagnosedRuns, nil)
		omittedDiagnosedRuns = omitted
		for _, node := range selected {
			diagnosedRunIDs = append(diagnosedRunIDs, Str(Obj(node["run"]), "id"))
		}
		if omitted != 0 {
			appendLimitations(Limitation{Code: "DIAGNOSIS_LIMIT", Message: "Relevant retained executions remain outside the diagnosis bound", Source: "dependencies"})
		}
		for _, node := range Objects(graph["nodes"]) {
			run := Obj(node["run"])
			id, expansion := Str(run, "id"), Str(node, "expansion")
			source := coverage(id, "dependencies", true, "")
			source["returned"], source["total"] = node["observedDependencies"], node["dependencyCount"]
			state := "partial"
			if expansion == "complete" {
				state = "complete"
			} else if expansion == "call_limit" {
				state = "budget_exhausted"
			} else if (expansion == "permission_denied" || expansion == "unavailable") && node["observedDependencies"] == nil && node["dependencyCount"] == nil {
				state = "unavailable"
			}
			source["state"], source["observedAt"] = state, ObservedAt()
			if state == "complete" {
				delete(source, "reasonCode")
			} else {
				source["reasonCode"] = "GRAPH_" + strings.ToUpper(expansion)
				for _, note := range traversal.Limitations {
					if note.RunID == id {
						source["reasonCode"] = note.Code
						break
					}
				}
			}
			if !stringContains(diagnosedRunIDs, id) {
				for _, kind := range []string{"problems", "tests", "log"} {
					skipped := coverage(id, kind, false, "")
					skipped["reasonCode"] = "NOT_A_FAILURE_CANDIDATE"
					if failureResult(Str(run, "result")) || Str(run, "result") == "unknown" {
						skipped["reasonCode"] = "DIAGNOSIS_LIMIT"
					}
				}
			}
		}
		diagnosed := []Object{}
		for _, node := range selected {
			run := Obj(node["run"])
			id := Str(run, "id")
			if id != rootID {
				source := coverage(id, "run", true, "")
				source["state"], source["returned"], source["total"], source["observedAt"] = "complete", 1, 1, traversal.Observations[id]
				delete(source, "reasonCode")
				if failureResult(Str(run, "result")) {
					finding(run, source, "dependency_failure", id, "Snapshot prerequisite execution reports "+Str(run, "result")+"; the edge does not establish causation", "Observed prerequisite execution "+id+": lifecycle="+Str(run, "state")+", result="+Str(run, "result"), append([]string{"teamcity-axi", "run", "view", id}, scopedArgs(run, projects[id])...))
				}
			}
			problemsSource, testsSource, logSource := rootProblemsSource, rootTestsSource, rootLogSource
			if id != rootID {
				problemsSource, testsSource, logSource = coverage(id, "problems", true, ""), coverage(id, "tests", true, ""), coverage(id, "log", false, "")
			}
			var problems, tests *ReadResult
			var problemsErr, testsErr error
			var wait sync.WaitGroup
			if id == rootID {
				problems = rootProblems
			} else {
				wait.Add(1)
				go func() {
					defer wait.Done()
					problems, problemsErr = collect(problemsSource, ReadRequest{Kind: "problems.page", RunID: id, Count: 20, ScanLimit: 5000})
				}()
			}
			wait.Add(1)
			go func() {
				defer wait.Done()
				tests, testsErr = collect(testsSource, ReadRequest{Kind: "tests.page", RunID: id, Count: 20, ScanLimit: 5000, Failed: true, FailedSet: true, Muted: false, MutedSet: true})
			}()
			wait.Wait()
			if problemsErr != nil {
				return nil, problemsErr
			}
			if testsErr != nil {
				return nil, testsErr
			}
			if id != rootID {
				page(problemsSource, problems)
			}
			page(testsSource, tests)
			scope := scopedArgs(run, projects[id])
			hasDirectEvidence := false
			if problems != nil {
				for _, problem := range Objects(problems.Value["items"]) {
					text := Str(problem, "description")
					if text == "" {
						text = Str(problem, "identity")
					}
					if text == "" {
						text = Str(problem, "type")
					}
					argv := append([]string{"teamcity-axi", "run", "problems", id}, scope...)
					argv = append(argv, "--problem", Str(problem, "id"), "--full")
					finding(run, problemsSource, "build_problem", Str(problem, "id"), "Server reports build problem "+Str(problem, "type"), text, argv)
					if strings.TrimSpace(Str(problem, "description")) != "" && Str(problem, "type") != "TC_TESTS_FAILED" && Str(problem, "type") != "TC_EXIT_CODE" {
						hasDirectEvidence = true
					}
				}
			}
			if tests != nil {
				for _, test := range Objects(tests.Value["items"]) {
					if Str(test, "result") == "failure" && explicitlyFalse(test, "muted") && explicitlyFalse(test, "ignored") {
						text := Str(test, "details")
						if text == "" {
							text = Str(test, "name")
						}
						argv := append([]string{"teamcity-axi", "run", "tests", id}, scope...)
						argv = append(argv, "--test", Str(test, "id"), "--full")
						finding(run, testsSource, "failed_test", Str(test, "id"), "Unmuted test occurrence failed: "+Str(test, "name"), text, argv)
						hasDirectEvidence = hasDirectEvidence || strings.TrimSpace(Str(test, "details")) != ""
					}
				}
			}
			if !hasDirectEvidence {
				logSource["required"] = true
				log, err := collect(logSource, ReadRequest{Kind: "log.tail", ID: id, RunID: id, Tail: 80})
				if err != nil {
					return nil, err
				}
				if log != nil {
					messages := Objects(log.Value["messages"])
					var firstID, lastID any
					if len(messages) != 0 {
						firstID, lastID = messages[0]["id"], messages[len(messages)-1]["id"]
					}
					logSource["returned"], logSource["providerReturned"], logSource["total"] = len(messages), Int(log.Value, "providerReturned"), nil
					logSource["window"] = Object{"requested": 80, "firstMessageId": firstID, "lastMessageId": lastID, "omittedProviderMessages": Int(log.Value, "providerReturned") - len(messages)}
					notes := plannerLimitations(log.Value["limitations"])
					if len(notes) != 0 {
						logSource["state"], logSource["reasonCode"] = "partial", notes[0].Code
					}
					for _, note := range notes {
						note.Source, note.RunID = "log", id
						appendLimitations(note)
					}
					for _, message := range messages {
						text := Str(message, "text")
						if failureLogSignal.MatchString(text) {
							literal, _ := boundedText(text, 80)
							argv := append([]string{"teamcity-axi", "run", "log", id}, scope...)
							argv = append(argv, "--tail", "80", "--contains="+literal, "--full")
							finding(run, logSource, "log_signal", Str(message, "id"), "A failure-related text signal appears in the inspected tail window", text, argv)
						}
					}
					truncated = truncated || Bool(log.Value, "truncated")
				}
			} else {
				logSource["reasonCode"] = "DIRECT_EVIDENCE_AVAILABLE"
			}
			diagnosed = append(diagnosed, run)
		}
		for _, run := range diagnosed {
			id := Str(run, "id")
			source := coverage(id, "tests", false, ":muted")
			read, err := collect(source, ReadRequest{Kind: "tests.page", RunID: id, Count: 20, ScanLimit: 5000, Failed: true, FailedSet: true, Muted: true, MutedSet: true})
			if err != nil {
				return nil, err
			}
			page(source, read)
			if read != nil {
				for _, test := range Objects(read.Value["items"]) {
					if Str(test, "result") == "failure" && Bool(test, "muted") && explicitlyFalse(test, "ignored") {
						text := Str(test, "details")
						if text == "" {
							text = Str(test, "name")
						}
						argv := append([]string{"teamcity-axi", "run", "tests", id}, scopedArgs(run, projects[id])...)
						argv = append(argv, "--test", Str(test, "id"), "--full")
						finding(run, source, "failed_test", Str(test, "id"), "Muted test occurrence failed: "+Str(test, "name"), text, argv)
					}
				}
			}
		}
		changes, err := collect(changesSource, ReadRequest{Kind: "changes.page", RunID: rootID, Count: 10, ScanLimit: 5000})
		if err != nil {
			return nil, err
		}
		page(changesSource, changes)
		if changes != nil {
			roots := map[string]bool{}
			_, rootsKnown := initial["revisions"]
			for _, revision := range Objects(initial["revisions"]) {
				roots[Str(revision, "vcsRootId")] = true
			}
			foreign := false
			for _, item := range Objects(changes.Value["items"]) {
				foreign = foreign || (rootsKnown && !roots[Str(item, "vcsRootId")])
			}
			if foreign {
				changesSource["state"], changesSource["returned"], changesSource["reasonCode"] = "unavailable", nil, "CONTEXT_MISMATCH"
			} else {
				optionalChanges = []Object{}
				for _, item := range Objects(changes.Value["items"]) {
					message := Str(item, "message")
					if !options.Full {
						truncated = truncated || firstLine(message) != message
						message = firstLine(message)
					}
					optionalChanges = append(optionalChanges, Object{"id": item["id"], "version": item["version"], "vcsRootId": item["vcsRootId"], "timestamp": item["timestamp"], "message": excerpt(message, "changes", rootID)})
				}
			}
		}
	}
	if reserve != 0 {
		source := coverage(rootID, "run", true, ":final")
		final := countedRead(ReadRequest{Kind: "run.detail", ID: rootID}, options.Budget)
		source["observedAt"] = final.Provenance.ObservedAt
		if final.State == "unavailable" {
			err := readFailure(final)
			if err.Code == "INTERRUPTED" {
				return nil, err
			}
			source["state"], source["reasonCode"], finalFailed = "unavailable", err.Code, true
			appendLimitations(Limitation{Code: err.Code, Message: "Reserved final root observation is unavailable", Source: "run", RunID: rootID})
		} else {
			if Str(final.Value, "id") != rootID || Str(final.Value, "jobId") != Str(initial, "jobId") || final.Provenance.ProjectID != primaryRead.Provenance.ProjectID {
				return nil, NewError("CONTEXT_MISMATCH", "Final observation changed the frozen execution scope", 1)
			}
			source["state"], source["returned"], source["total"] = "complete", 1, 1
			delete(source, "reasonCode")
			primary, changed = final.Value, !SameGraphObservation(initial, final.Value)
			appendLimitations(final.Provenance.Limitations...)
			if changed {
				appendLimitations(Limitation{Code: "ROOT_STATE_CHANGED", Message: "Root observation changed while evidence was acquired", Source: "run", RunID: rootID})
			}
		}
		if changed || finalFailed {
			unexpanded := 0
			for _, node := range Objects(graph["nodes"]) {
				if Str(Obj(node["run"]), "id") == rootID {
					node["run"], node["expansion"] = GraphRun(primary), "unavailable"
				}
				if Str(node, "expansion") != "complete" {
					unexpanded++
				}
			}
			graph["complete"], graph["unexpanded"] = false, unexpanded
			for _, graphSource := range sources {
				if Str(graphSource, "id") == "dependencies:"+rootID {
					graphSource["state"], graphSource["reasonCode"] = "partial", "FINAL_READ_UNAVAILABLE"
					if changed {
						graphSource["reasonCode"] = "ROOT_STATE_CHANGED"
					}
				}
			}
		}
	}
	if !notFailed && Str(primary, "state") != "finished" {
		appendLimitations(Limitation{Code: "PROVISIONAL_EVIDENCE", Message: "Execution remains non-terminal; observations are provisional", Source: "run", RunID: rootID})
	}
	assessment := "inconclusive"
	if notFailed {
		assessment = "not_failed"
	} else if Str(primary, "state") == "queued" || Str(primary, "state") == "running" {
		assessment = "in_progress"
	} else if Str(primary, "state") == "finished" && failureResult(Str(primary, "result")) {
		assessment = "failure_observed"
	}
	if changed && Str(primary, "state") == "finished" && Str(primary, "result") == "success" {
		assessment = "inconclusive"
	}
	complete := (notFailed || Bool(graph, "complete")) && omittedDiagnosedRuns == 0 && !finalFailed && !changed && !hardTextLimit && (notFailed || Str(primary, "state") == "finished")
	for _, source := range sources {
		complete = complete && (!Bool(source, "required") || Str(source, "state") == "complete")
	}
	for _, note := range limitations {
		if note.Code == "MISSING_REVISION_METADATA" || note.Code == "UNKNOWN_LIFECYCLE" || note.Code == "UNKNOWN_RESULT" {
			complete = false
		}
	}
	indices := map[string]int{}
	for i, id := range diagnosedRunIDs {
		indices[id] = i
	}
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if indices[Str(a, "runId")] != indices[Str(b, "runId")] {
			return indices[Str(a, "runId")] < indices[Str(b, "runId")]
		}
		if Str(a, "kind") != Str(b, "kind") {
			return Str(a, "kind") < Str(b, "kind")
		}
		return Str(a, "id") < Str(b, "id")
	})
	omittedFindings := max(0, len(findings)-100)
	if omittedFindings != 0 {
		appendLimitations(Limitation{Code: "FINDING_LIMIT", Message: "Additional observed findings exceed the hundred-finding ceiling", Source: "output"})
	}
	data := Object{"run": GraphRun(primary), "assessment": assessment, "findings": findings[:min(100, len(findings))], "sources": sources, "graph": graph, "selection": Object{"maxDiagnosedRuns": options.MaxDiagnosedRuns, "diagnosedRunIds": diagnosedRunIDs, "omittedDiagnosedRuns": omittedDiagnosedRuns, "graphReadAttempts": graphReadAttempts, "maxGraphReads": maxGraphReads, "omittedGraphTargets": omittedGraphTargets, "omittedFindings": omittedFindings, "omittedChanges": 0, "consistency": "best_effort", "dependencyReferences": "unavailable"}}
	if optionalChanges != nil {
		data["changes"] = optionalChanges
	}
	return Object{"data": data, "limitations": limitations, "complete": complete && omittedFindings == 0, "truncated": truncated || omittedFindings != 0, "projects": projects}, nil
}

type plannerCountingReader struct {
	read func(ReadRequest, Budget) ReadResult
}

func (reader plannerCountingReader) Read(_ context.Context, request ReadRequest, budget Budget) ReadResult {
	return reader.read(request, budget)
}

func explicitlyFalse(object Object, key string) bool {
	value, ok := object[key].(bool)
	return ok && !value
}

func stringContains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
