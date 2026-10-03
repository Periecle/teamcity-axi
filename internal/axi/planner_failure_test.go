package axi

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func plannerProblem(id int) Object {
	return Object{"runId": fmt.Sprint(id), "id": fmt.Sprintf("build:(id:%d),problem:(id:1)", id), "type": "SYNTHETIC", "description": "Explicit synthetic build problem"}
}

func plannerOccurrence(id, number int, overrides Object) Object {
	result := Object{"runId": fmt.Sprint(id), "id": fmt.Sprintf("build:(id:%d),id:%d", id, number), "testId": "517450581327024597", "name": "same name", "result": "failure", "muted": false, "ignored": false, "durationMs": 25, "details": "Connection refused"}
	for key, value := range overrides {
		result[key] = value
	}
	return result
}

type failureFixtureOptions struct {
	primary      Object
	final        Object
	adjacency    map[string][]int
	runs         map[int]Object
	errors       map[string]string
	problems     map[string][]Object
	tests        map[string][]Object
	muted        map[string][]Object
	logs         map[string][]Object
	changes      []Object
	foreign      map[int]bool
	childProject string
	projects     map[string]string
	maxChildren  int
	depth        *int
	maxDiagnosed int
	full         bool
	partialPages bool
	secrets      []string
}

type failureFixtureResult struct {
	report   Object
	calls    []string
	launches int
	envelope Response
}

func plannerFailureFixture(options failureFixtureOptions) (failureFixtureResult, error) {
	launches, calls := 2, []string{}
	var mu sync.Mutex
	primary := plannerAvailable(plannerRun(1, options.primary))
	reader := plannerTestReader(func(_ context.Context, request ReadRequest, budget Budget) ReadResult {
		id := request.RunID
		if id == "" {
			id = request.ID
		}
		name := request.Kind
		switch request.Kind {
		case "dependencies.count":
			name = "count"
		case "dependencies.page":
			name = "dependencies"
		case "problems.page":
			name = "problems"
		case "tests.page":
			name = "tests"
			if request.Muted {
				name = "muted"
			}
		case "log.tail":
			name = "log"
		case "changes.page":
			name = "changes"
		case "run.detail":
			name = "final"
		case "project.detail":
			name = "project"
		}
		mu.Lock()
		if budget.MaxChildProcesses >= 0 && launches >= budget.MaxChildProcesses {
			mu.Unlock()
			return plannerUnavailable("CALL_LIMIT_EXCEEDED")
		}
		launches++
		calls = append(calls, name+":"+id)
		mu.Unlock()
		if code := options.errors[name+":"+id]; code != "" {
			read := plannerUnavailable(code)
			read.Provenance = primary.Provenance
			return read
		}
		page := func(items []Object) ReadResult {
			overrides := Object{}
			if options.partialPages {
				overrides = Object{"hasMore": nil, "limitations": []Limitation{{Code: "SCAN_COVERAGE_UNKNOWN", Message: "Unknown total"}}}
			}
			return plannerAvailable(plannerPage(items, overrides))
		}
		number := 1
		_, _ = fmt.Sscan(id, &number)
		switch request.Kind {
		case "dependencies.count":
			return plannerAvailable(Object{"count": len(options.adjacency[id])})
		case "dependencies.page":
			items := []Object{}
			for _, child := range options.adjacency[id] {
				project := "Allowed"
				if options.childProject != "" {
					project = options.childProject
				}
				if options.foreign[child] {
					project = "Foreign"
				}
				items = append(items, Object{"run": plannerRun(child, options.runs[child]), "projectId": project})
			}
			return page(items)
		case "problems.page":
			items, exists := options.problems[id]
			if !exists {
				items = []Object{plannerProblem(number)}
			}
			return page(items)
		case "tests.page":
			if !request.FailedSet || !request.Failed || !request.MutedSet {
				return plannerUnavailable("USAGE_ERROR")
			}
			if request.Muted {
				return page(options.muted[id])
			}
			items, exists := options.tests[id]
			if !exists {
				items = []Object{plannerOccurrence(number, 1, nil)}
			}
			return page(items)
		case "log.tail":
			items, exists := options.logs[id]
			if !exists {
				items = []Object{{"id": "12", "text": "Connection refused", "level": 0, "status": 4}}
			}
			return plannerAvailable(Object{"runId": id, "messages": items, "providerReturned": len(items), "truncated": false, "limitations": []Limitation{}})
		case "changes.page":
			return page(options.changes)
		case "run.detail":
			final := options.final
			if final == nil {
				final = options.primary
			}
			return plannerAvailable(plannerRun(number, final))
		case "project.detail":
			parent := options.projects[id]
			if parent == "" {
				parent = "Allowed"
			}
			return plannerAvailable(Object{"id": id, "name": id, "archived": false, "parentProjectId": parent})
		}
		return plannerUnavailable("INTERNAL_ERROR")
	})
	budget := Budget{Deadline: time.Now().Add(5 * time.Second).UnixMilli(), MaxChildProcesses: 24}
	assert := func(_ context.Context, id string, _ Budget) error {
		if id == "Foreign" {
			return NewError("POLICY_DENIED", "Foreign", 1)
		}
		return nil
	}
	if options.childProject != "" {
		assert = NewProjectPolicy(reader, []string{"Allowed"}, budget).AssertBudget
	}
	maxChildren, maxDiagnosed, depth := options.maxChildren, options.maxDiagnosed, 4
	if maxChildren == 0 {
		maxChildren = 24
	}
	if maxDiagnosed == 0 {
		maxDiagnosed = 3
	}
	if options.depth != nil {
		depth = *options.depth
	}
	budget.MaxChildProcesses = maxChildren
	result, err := InvestigateFailure(context.Background(), reader, ExecutionContext{Server: "work"}, "1", FailureOptions{Primary: &primary, Budget: budget, MaxChildProcesses: maxChildren, ChildProcesses: func() int { mu.Lock(); defer mu.Unlock(); return launches }, Depth: depth, MaxNodes: 30, MaxDiagnosedRuns: maxDiagnosed, Full: options.full, Secrets: options.secrets, AssertProject: assert})
	if err != nil {
		return failureFixtureResult{}, err
	}
	envelope := NewResponse("run.failure", Obj(result["data"]))
	envelope.Context = Object{"server": "work", "job": Str(primary.Value, "jobId"), "branch": primary.Value["branch"]}
	envelope.Meta["complete"], envelope.Meta["limitations"], envelope.Meta["truncated"] = Bool(result, "complete"), result["limitations"], Bool(result, "truncated")
	if !Bool(result, "complete") {
		envelope.Status = "partial"
	}
	if err := ValidateResponse(envelope); err != nil {
		return failureFixtureResult{}, fmt.Errorf("failure envelope schema: %w", err)
	}
	for _, finding := range Objects(envelope.Data["findings"]) {
		for _, evidence := range Objects(finding["evidence"]) {
			argv := Strings(Obj(evidence["retrieve"])["argv"])
			if _, err := Parse(argv[1:]); err != nil {
				return failureFixtureResult{}, fmt.Errorf("evidence recovery argv: %w", err)
			}
			source := plannerSource(envelope.Data, Str(evidence, "sourceRef"))
			if source == nil || (Str(source, "state") != "complete" && Str(source, "state") != "partial") {
				return failureFixtureResult{}, fmt.Errorf("evidence references unavailable source %s", Str(evidence, "sourceRef"))
			}
		}
	}
	return failureFixtureResult{report: result, calls: calls, launches: launches, envelope: envelope}, nil
}

func plannerInvestigate(t *testing.T, options failureFixtureOptions) failureFixtureResult {
	t.Helper()
	result, err := plannerFailureFixture(options)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func plannerSource(data Object, id string) Object {
	for _, source := range Objects(data["sources"]) {
		if Str(source, "id") == id {
			return source
		}
	}
	return nil
}

func plannerFindings(data Object, kind string) []Object {
	result := []Object{}
	for _, finding := range Objects(data["findings"]) {
		if kind == "" || Str(finding, "kind") == kind {
			result = append(result, finding)
		}
	}
	return result
}

func TestPlannerFailureSuccessShortCircuit(t *testing.T) {
	fixture := plannerInvestigate(t, failureFixtureOptions{primary: Object{"result": "success"}})
	data, graph := fixture.envelope.Data, Obj(fixture.envelope.Data["graph"])
	if Str(data, "assessment") != "not_failed" || !Bool(fixture.report, "complete") || len(fixture.calls) != 0 || fixture.launches != 2 || Bool(graph, "complete") || Str(plannerNode(graph, "1"), "expansion") != "not_requested" {
		t.Fatalf("success did not short circuit: %#v", fixture)
	}
	for _, source := range Objects(data["sources"]) {
		if Str(source, "kind") != "run" && Str(source, "state") != "not_requested" {
			t.Fatalf("success requested evidence: %#v", source)
		}
	}
}

func TestPlannerFailureDirectEvidenceStableIDsAndMutedCounts(t *testing.T) {
	options := failureFixtureOptions{tests: map[string][]Object{"1": {plannerOccurrence(1, 1, nil), plannerOccurrence(1, 2, nil)}}, muted: map[string][]Object{"1": {plannerOccurrence(1, 3, Object{"muted": true})}}}
	first, second := plannerInvestigate(t, options), plannerInvestigate(t, options)
	data := first.envelope.Data
	if !Bool(first.report, "complete") || Str(data, "assessment") != "failure_observed" || !reflect.DeepEqual(data["findings"], second.envelope.Data["findings"]) || len(plannerFindings(data, "failed_test")) != 3 || Int(plannerSource(data, "tests:1:muted"), "returned") != 1 || Int(plannerSource(data, "tests:1"), "returned") != 2 || plannerSource(data, "tests:1")["total"] != nil || stringContains(first.calls, "log:1") {
		t.Fatalf("direct evidence: %#v", first)
	}
	muted := false
	for _, finding := range plannerFindings(data, "") {
		if Str(finding, "claim") != "observation" {
			t.Fatal("causal claim inferred")
		}
		muted = muted || strings.HasPrefix(Str(finding, "summary"), "Muted")
	}
	if !muted {
		t.Fatal("muted occurrence lost")
	}
}

func TestPlannerFailureIndependentDenialAndBoundedCoverage(t *testing.T) {
	fixture := plannerInvestigate(t, failureFixtureOptions{errors: map[string]string{"problems:1": "PERMISSION_DENIED", "tests:1": "NOT_FOUND"}})
	source := plannerSource(fixture.envelope.Data, "problems:1")
	if Bool(fixture.report, "complete") || Str(source, "state") != "unavailable" || source["returned"] != nil || source["total"] != nil || !stringContains(fixture.calls, "log:1") || len(plannerFindings(fixture.envelope.Data, "log_signal")) == 0 {
		t.Fatalf("independent denial: %#v", fixture)
	}
	encoded, _ := json.Marshal(fixture.report)
	if strings.Contains(string(encoded), "rootCause") {
		t.Fatal("manufactured cause")
	}
	partial := plannerInvestigate(t, failureFixtureOptions{partialPages: true})
	if Bool(partial.report, "complete") || Str(plannerSource(partial.envelope.Data, "tests:1"), "state") != "partial" {
		t.Fatalf("bounded page certified complete: %#v", partial)
	}
}

func TestPlannerFailureBoundedDeterministicDAGSelection(t *testing.T) {
	fixture := plannerInvestigate(t, failureFixtureOptions{adjacency: map[string][]int{"1": {2, 3}, "2": {4}, "3": {4}}})
	data, graph, selection := fixture.envelope.Data, Obj(fixture.envelope.Data["graph"]), Obj(fixture.envelope.Data["selection"])
	if len(Objects(graph["nodes"])) != 4 || len(Objects(graph["edges"])) != 4 || !reflect.DeepEqual(selection["diagnosedRunIds"], []string{"1", "2", "3"}) || Int(selection, "omittedDiagnosedRuns") != 1 || Bool(fixture.report, "complete") || Str(plannerSource(data, "tests:4"), "state") != "not_requested" {
		t.Fatalf("DAG diagnosis bound: %#v", fixture)
	}
	selected, _ := SelectDiagnosedRuns(graph, map[string]int{"1": 0, "2": 1, "3": 1, "4": 2}, 2, map[string]bool{"4": true})
	if len(selected) != 2 || Str(Obj(selected[0]["run"]), "id") != "1" || Str(Obj(selected[1]["run"]), "id") != "4" {
		t.Fatalf("explicit reference priority: %#v", selected)
	}
	for _, finding := range plannerFindings(data, "") {
		if strings.Contains(Str(finding, "summary"), "root cause") {
			t.Fatal("ancestry inferred as cause")
		}
	}
}

func TestPlannerFailureCyclesForeignObservationsAndDepthCaps(t *testing.T) {
	cycle := plannerInvestigate(t, failureFixtureOptions{adjacency: map[string][]int{"1": {2}, "2": {1}}})
	if len(Objects(Obj(cycle.envelope.Data["graph"])["cycles"])) != 1 || len(Objects(Obj(cycle.envelope.Data["graph"])["nodes"])) != 2 {
		t.Fatalf("cycle lost: %#v", cycle)
	}
	denied := plannerInvestigate(t, failureFixtureOptions{adjacency: map[string][]int{"1": {2}}, foreign: map[int]bool{2: true}})
	encoded, _ := json.Marshal(denied.envelope.Data)
	if len(Objects(Obj(denied.envelope.Data["graph"])["nodes"])) != 1 || Bool(denied.report, "complete") || strings.Contains(string(encoded), "Job_2") {
		t.Fatalf("denied graph data retained: %#v", denied)
	}
	zero := 0
	capped := plannerInvestigate(t, failureFixtureOptions{depth: &zero})
	if Str(plannerNode(Obj(capped.envelope.Data["graph"]), "1"), "expansion") != "depth_limit" || Bool(capped.report, "complete") {
		t.Fatalf("depth boundary became leaf: %#v", capped)
	}
}

func TestPlannerFailurePrimaryReservationAndFinalRecheck(t *testing.T) {
	fixture := plannerInvestigate(t, failureFixtureOptions{primary: Object{"state": "running"}, final: Object{"state": "finished", "result": "success"}, maxChildren: 5, adjacency: map[string][]int{"1": {2}}})
	if fixture.launches != 5 || !stringContains(fixture.calls, "tests:1") || !stringContains(fixture.calls, "final:1") || Str(fixture.envelope.Data, "assessment") != "inconclusive" || Str(Obj(fixture.envelope.Data["run"]), "result") != "success" || Bool(fixture.report, "complete") || !plannerHasLimitation(plannerLimitations(fixture.report["limitations"]), "ROOT_STATE_CHANGED") {
		t.Fatalf("reserved evidence/final read lost: %#v", fixture)
	}
	denied := plannerInvestigate(t, failureFixtureOptions{primary: Object{"state": "running"}, errors: map[string]string{"final:1": "PERMISSION_DENIED"}})
	graph := Obj(denied.envelope.Data["graph"])
	if Bool(graph, "complete") || Str(plannerNode(graph, "1"), "expansion") != "unavailable" || Str(plannerSource(denied.envelope.Data, "run:1:final"), "state") != "unavailable" {
		t.Fatalf("final denial lost: %#v", denied)
	}
}

func TestPlannerFailureUnknownAndCanceledOutcomesAndInterrupt(t *testing.T) {
	for _, outcome := range []string{"canceled", "failed_to_start", "error"} {
		fixture := plannerInvestigate(t, failureFixtureOptions{primary: Object{"result": outcome, "composite": true}})
		if Str(fixture.envelope.Data, "assessment") != "failure_observed" || Str(Obj(fixture.envelope.Data["run"]), "result") != outcome {
			t.Fatalf("special outcome lost: %#v", fixture)
		}
	}
	unknown := plannerInvestigate(t, failureFixtureOptions{primary: Object{"result": "unknown", "rawStatus": "FUTURE"}})
	if Str(unknown.envelope.Data, "assessment") != "inconclusive" || Str(Obj(unknown.envelope.Data["run"]), "rawStatus") != "FUTURE" {
		t.Fatalf("unknown outcome lost: %#v", unknown)
	}
	if _, err := plannerFailureFixture(failureFixtureOptions{errors: map[string]string{"tests:1": "INTERRUPTED"}}); err == nil || AsDomainError(err).Code != "INTERRUPTED" {
		t.Fatal("interruption swallowed")
	}
}

func TestPlannerFailureRedactionBeforeFingerprintsAndFullBounds(t *testing.T) {
	secret := "fixture-only-secret"
	options := failureFixtureOptions{secrets: []string{secret}, tests: map[string][]Object{"1": {plannerOccurrence(1, 1, Object{"details": secret + strings.Repeat("x", 10000)})}}}
	preview := plannerInvestigate(t, options)
	options.full = true
	full := plannerInvestigate(t, options)
	a, b := plannerFindings(preview.envelope.Data, "failed_test")[0], plannerFindings(full.envelope.Data, "failed_test")[0]
	encoded, _ := json.Marshal(full.report)
	if Str(a, "id") != Str(b, "id") || len([]rune(Str(Objects(a["evidence"])[0], "excerpt"))) != 2000 || len([]rune(Str(Objects(b["evidence"])[0], "excerpt"))) != 8192 || Bool(full.report, "complete") || !Bool(full.report, "truncated") || strings.Contains(string(encoded), secret) {
		t.Fatalf("redaction or full ceiling: %#v", full)
	}
}

func TestPlannerFailureAncestryCannotSpendReservedPrimaryEvidence(t *testing.T) {
	fixture := plannerInvestigate(t, failureFixtureOptions{maxChildren: 8, adjacency: map[string][]int{"1": {2}}, childProject: "NestedA", projects: map[string]string{"NestedA": "NestedB", "NestedB": "Allowed"}, problems: map[string][]Object{"1": {}}, tests: map[string][]Object{"1": {}}})
	if fixture.launches != 8 || !stringContains(fixture.calls, "tests:1") || !stringContains(fixture.calls, "log:1") || stringContains(fixture.calls, "project:NestedB") || Str(plannerSource(fixture.envelope.Data, "tests:1"), "state") != "complete" || Str(plannerSource(fixture.envelope.Data, "log:1"), "state") != "complete" {
		t.Fatalf("ancestry spent reservation: %#v", fixture)
	}
	denied := plannerInvestigate(t, failureFixtureOptions{adjacency: map[string][]int{"1": {2}}, errors: map[string]string{"count:2": "PERMISSION_DENIED", "dependencies:2": "PERMISSION_DENIED"}})
	source := plannerSource(denied.envelope.Data, "dependencies:2")
	primaryFinding := false
	for _, finding := range plannerFindings(denied.envelope.Data, "") {
		primaryFinding = primaryFinding || Str(finding, "runId") == "1"
	}
	if Str(source, "state") != "unavailable" || Str(source, "reasonCode") != "PERMISSION_DENIED" || source["returned"] != nil || source["total"] != nil || !primaryFinding {
		t.Fatalf("wholly denied dependencies became zero: %#v", denied)
	}
}

func TestPlannerFailureSensitiveIdentityFingerprintRedaction(t *testing.T) {
	secret := plannerInvestigate(t, failureFixtureOptions{primary: Object{"jobId": "secret-job"}, secrets: []string{"secret-job"}})
	redacted := plannerInvestigate(t, failureFixtureOptions{primary: Object{"jobId": "[REDACTED]"}})
	a, b := plannerFindings(secret.envelope.Data, ""), plannerFindings(redacted.envelope.Data, "")
	if len(a) != len(b) {
		t.Fatal("redacted fingerprints changed finding count")
	}
	for i := range a {
		if Str(a[i], "id") != Str(b[i], "id") {
			t.Fatal("fingerprint generated before identity redaction")
		}
	}
}

func TestPlannerFailureOptionalChangesPreserveScopeAndTimestamp(t *testing.T) {
	change := Object{"id": "101", "version": strings.Repeat("a", 40), "vcsRootId": "Root", "message": "Context only", "timestamp": "2026-10-02T00:00:00Z"}
	good := plannerInvestigate(t, failureFixtureOptions{primary: Object{"revisions": []Object{{"vcsRootId": "Root", "revision": strings.Repeat("a", 40)}}}, changes: []Object{change}})
	if !reflect.DeepEqual(good.envelope.Data["changes"], []Object{change}) || len(plannerFindings(good.envelope.Data, "revision_difference")) != 0 {
		t.Fatalf("optional change identity lost: %#v", good)
	}
	foreign := plannerInvestigate(t, failureFixtureOptions{changes: []Object{change}})
	source := plannerSource(foreign.envelope.Data, "changes:1")
	if foreign.envelope.Data["changes"] != nil || Str(source, "state") != "unavailable" || Str(source, "reasonCode") != "CONTEXT_MISMATCH" || !Bool(foreign.report, "complete") {
		t.Fatalf("foreign optional changes affected required coverage: %#v", foreign)
	}
}

func TestPlannerFailurePreviewedSummariesAndMultilineChanges(t *testing.T) {
	name := plannerInvestigate(t, failureFixtureOptions{tests: map[string][]Object{"1": {plannerOccurrence(1, 1, Object{"name": strings.Repeat("x", 2000)})}}})
	if !Bool(name.report, "truncated") || len([]rune(Str(plannerFindings(name.envelope.Data, "failed_test")[0], "summary"))) != 1200 {
		t.Fatal("summary truncation unaccounted")
	}
	change := Object{"id": "101", "version": strings.Repeat("a", 40), "vcsRootId": "Root", "message": "Short line\nAdditional context", "timestamp": nil}
	fixture := plannerInvestigate(t, failureFixtureOptions{primary: Object{"revisions": []Object{{"vcsRootId": "Root", "revision": strings.Repeat("a", 40)}}}, changes: []Object{change}})
	if !Bool(fixture.report, "truncated") || Str(Objects(fixture.envelope.Data["changes"])[0], "message") != "Short line" {
		t.Fatal("multiline preview not declared")
	}
}

func TestPlannerFailureOptionalOutputReductionsPreserveRequiredEvidence(t *testing.T) {
	changes := []Object{}
	for i := 0; i < 10; i++ {
		changes = append(changes, Object{"id": fmt.Sprint(i + 101), "version": strings.Repeat("a", 40), "vcsRootId": "Root", "message": strings.Repeat("🦊", 4000), "timestamp": nil})
	}
	fixture := plannerInvestigate(t, failureFixtureOptions{full: true, primary: Object{"revisions": []Object{{"vcsRootId": "Root", "revision": strings.Repeat("a", 40)}}}, changes: changes})
	for _, format := range []string{"json", "toon"} {
		t.Run(format, func(t *testing.T) {
			rendered, err := Render(fixture.envelope, format, 10000, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			a, _ := json.Marshal(fixture.envelope.Data["findings"])
			b, _ := json.Marshal(rendered.Response.Data["findings"])
			ga, _ := json.Marshal(fixture.envelope.Data["graph"])
			gb, _ := json.Marshal(rendered.Response.Data["graph"])
			source := plannerSource(rendered.Response.Data, "changes:1")
			retained := len(Objects(rendered.Response.Data["changes"]))
			if len([]byte(rendered.Document)) > 10000 || rendered.Response.Status != "ok" || string(a) != string(b) || string(ga) != string(gb) || !Bool(rendered.Response.Meta, "truncated") || Int(Obj(rendered.Response.Data["selection"]), "omittedChanges") != 10-retained || Int(source, "returned") != retained || Int(source, "providerReturned") != 10 || Str(source, "state") != "partial" {
				t.Fatalf("optional output reduction: %#v", rendered.Response)
			}
			if err := ValidateResponse(rendered.Response); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPlannerFailureLogWindowAndHostileLiteralRetrieval(t *testing.T) {
	text := "--error=Connection refused " + strings.Repeat("x", 52) + "🦊"
	fixture := plannerInvestigate(t, failureFixtureOptions{problems: map[string][]Object{"1": {}}, tests: map[string][]Object{"1": {}}, logs: map[string][]Object{"1": {{"id": "12", "text": text, "level": 0, "status": 4}}}})
	source := plannerSource(fixture.envelope.Data, "log:1")
	expected := Object{"requested": 80, "firstMessageId": "12", "lastMessageId": "12", "omittedProviderMessages": 0}
	if !reflect.DeepEqual(source["window"], expected) || Int(source, "providerReturned") != 1 {
		t.Fatalf("log window unaccounted: %#v", source)
	}
	finding := plannerFindings(fixture.envelope.Data, "log_signal")[0]
	argv := Strings(Obj(Objects(finding["evidence"])[0]["retrieve"])["argv"])
	parsed, err := Parse(argv[1:])
	if err != nil || parsed.String("contains") != text {
		t.Fatalf("hostile literal was interpreted as flags: %#v, %v", parsed, err)
	}
}

func TestPlannerFailureDependencyEvidenceRedactsBeforeEscaping(t *testing.T) {
	secret := "fixture-quote\"and\\backslash"
	fixture := plannerInvestigate(t, failureFixtureOptions{adjacency: map[string][]int{"1": {2}}, runs: map[int]Object{2: {"branch": secret}}, secrets: []string{secret}})
	finding := plannerFindings(fixture.envelope.Data, "dependency_failure")[0]
	encoded, _ := json.Marshal(finding)
	quote, _ := json.Marshal(secret)
	if strings.Contains(string(encoded), string(quote[1:len(quote)-1])) {
		t.Fatal("sensitive metadata JSON-escaped before redaction")
	}
	for _, format := range []string{"json", "toon"} {
		rendered, err := Render(fixture.envelope, format, 24576, []string{secret}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(rendered.Document, secret) || strings.Contains(rendered.Document, string(quote[1:len(quote)-1])) || !strings.Contains(rendered.Document, "[REDACTED]") {
			t.Fatal("sensitive graph metadata leaked to rendered output")
		}
	}
}

func TestPlannerFailureValidationAndFinalScopeFence(t *testing.T) {
	reader := plannerTestReader(func(context.Context, ReadRequest, Budget) ReadResult { return plannerUnavailable("NOT_FOUND") })
	for _, cap := range []int{0, 11} {
		if _, err := InvestigateFailure(context.Background(), reader, ExecutionContext{}, "1", FailureOptions{MaxDiagnosedRuns: cap}); err == nil || AsDomainError(err).Code != "USAGE_ERROR" {
			t.Fatal("invalid diagnosis bound accepted")
		}
	}
	if _, err := InvestigateFailure(context.Background(), reader, ExecutionContext{}, "1", FailureOptions{MaxDiagnosedRuns: 3}); err == nil || AsDomainError(err).Code != "NOT_FOUND" {
		t.Fatal("primary read failure converted into a report")
	}
	if _, err := plannerFailureFixture(failureFixtureOptions{primary: Object{"state": "running"}, final: Object{"jobId": "Foreign"}}); err == nil || AsDomainError(err).Code != "CONTEXT_MISMATCH" {
		t.Fatal("final identity change accepted")
	}
}
