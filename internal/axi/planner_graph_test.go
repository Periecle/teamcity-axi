package axi

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

type plannerTestReader func(context.Context, ReadRequest, Budget) ReadResult

func (read plannerTestReader) Read(ctx context.Context, request ReadRequest, budget Budget) ReadResult {
	return read(ctx, request, budget)
}

func plannerRun(id int, overrides Object) Object {
	run := Object{"id": fmt.Sprint(id), "jobId": fmt.Sprintf("Job_%d", id), "state": "finished", "result": "failure", "branch": "main", "revisions": []Object{}}
	for key, value := range overrides {
		run[key] = value
	}
	return run
}

func plannerAvailable(value Object) ReadResult {
	return ReadResult{State: "available", Value: value, Provenance: Provenance{ObservedAt: "2026-10-02T00:00:00Z", Operation: "fixture", ProjectID: "Allowed", Limitations: []Limitation{}}}
}

func plannerUnavailable(code string) ReadResult {
	return ReadResult{State: "unavailable", Error: NewError(code, "Synthetic independent source failure", 1), Provenance: Provenance{Limitations: []Limitation{}}}
}

func plannerPage(items []Object, overrides Object) Object {
	page := Object{"items": items, "providerReturned": len(items), "position": nil, "hasMore": false, "limitations": []Limitation{}}
	for key, value := range overrides {
		page[key] = value
	}
	return page
}

type graphFixtureOptions struct {
	countErrors map[string]string
	pageErrors  map[string]string
	counts      map[string]int
	pages       map[string]map[int]Object
	foreign     map[string]bool
	alter       func(ReadRequest, Object)
}

func plannerGraphFixture(adjacency map[string][]int, options graphFixtureOptions) (Reader, *[]string) {
	calls := []string{}
	reader := plannerTestReader(func(_ context.Context, request ReadRequest, _ Budget) ReadResult {
		id := request.RunID
		if request.Kind == "dependencies.count" {
			calls = append(calls, "count:"+id)
			if code := options.countErrors[id]; code != "" {
				return plannerUnavailable(code)
			}
			count := len(adjacency[id])
			if value, exists := options.counts[id]; exists {
				count = value
			}
			return plannerAvailable(Object{"count": count})
		}
		calls = append(calls, fmt.Sprintf("page:%s:%d", id, request.Start))
		if code := options.pageErrors[id]; code != "" {
			return plannerUnavailable(code)
		}
		var page Object
		if pages, exists := options.pages[id]; exists {
			page = pages[request.Start]
		} else {
			items := []Object{}
			ids := adjacency[id]
			for _, childID := range ids[min(request.Start, len(ids)):min(request.Start+request.Count, len(ids))] {
				project := "Allowed"
				if options.foreign[fmt.Sprint(childID)] {
					project = "Foreign"
				}
				items = append(items, Object{"run": plannerRun(childID, nil), "projectId": project})
			}
			page = plannerPage(items, Object{"hasMore": nil, "limitations": []Limitation{{Code: "SCAN_COVERAGE_UNKNOWN", Message: "No continuation", Source: "dependencies"}}})
		}
		if options.alter != nil {
			options.alter(request, page)
		}
		return plannerAvailable(page)
	})
	return reader, &calls
}

func plannerGraphOptions() GraphOptions {
	return GraphOptions{Root: plannerRun(1, nil), RootProjectID: "Allowed", Budget: Budget{Deadline: time.Now().Add(5 * time.Second).UnixMilli(), MaxChildProcesses: 24}, Depth: 4, MaxNodes: 30, MaxGraphReads: 20, AssertProject: func(_ context.Context, project string, _ Budget) error {
		if project == "Foreign" {
			return NewError("POLICY_DENIED", "Synthetic foreign project", 1)
		}
		return nil
	}}
}

func plannerTraverse(t *testing.T, reader Reader, options GraphOptions) GraphReport {
	t.Helper()
	report, err := BuildGraph(context.Background(), reader, ExecutionContext{}, "1", options)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func plannerNode(graph Object, id string) Object {
	for _, node := range Objects(graph["nodes"]) {
		if Str(Obj(node["run"]), "id") == id {
			return node
		}
	}
	return nil
}

func plannerHasLimitation(notes []Limitation, code string) bool {
	for _, note := range notes {
		if note.Code == code {
			return true
		}
	}
	return false
}

func TestPlannerGraphSharedDAG(t *testing.T) {
	reader, calls := plannerGraphFixture(map[string][]int{"1": {2, 3}, "2": {4}, "3": {4}}, graphFixtureOptions{})
	report := plannerTraverse(t, reader, plannerGraphOptions())
	if !Bool(report.Graph, "complete") || len(Objects(report.Graph["nodes"])) != 4 || len(Objects(report.Graph["edges"])) != 4 || len(Objects(report.Graph["cycles"])) != 0 || Int(report.Graph, "unexpanded") != 0 {
		t.Fatalf("unexpected DAG: %#v", report.Graph)
	}
	count4 := 0
	for _, call := range *calls {
		if call == "count:4" {
			count4++
		}
	}
	leaf := plannerNode(report.Graph, "4")
	if count4 != 1 || leaf["dependencyCount"] != 0 || Str(leaf, "expansion") != "complete" || report.GraphReadAttempts != 7 || plannerHasLimitation(report.Limitations, "SCAN_COVERAGE_UNKNOWN") {
		t.Fatalf("shared expansion was not reconciled: %#v, %v", report, *calls)
	}
}

func TestPlannerGraphCycle(t *testing.T) {
	reader, _ := plannerGraphFixture(map[string][]int{"1": {2}, "2": {3}, "3": {1}}, graphFixtureOptions{})
	report := plannerTraverse(t, reader, plannerGraphOptions())
	expected := []Object{{"fromRunId": "3", "toRunId": "1", "kind": "snapshot"}}
	if len(Objects(report.Graph["nodes"])) != 3 || len(Objects(report.Graph["edges"])) != 3 || !reflect.DeepEqual(report.Graph["cycles"], expected) || report.GraphReadAttempts != 6 || !Bool(report.Graph, "complete") {
		t.Fatalf("cycle handling: %#v", report)
	}
}

func TestPlannerGraphDepthBoundaries(t *testing.T) {
	for _, depth := range []int{0, 1} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			reader, calls := plannerGraphFixture(map[string][]int{"1": {2, 3}}, graphFixtureOptions{})
			options := plannerGraphOptions()
			options.Depth = depth
			report := plannerTraverse(t, reader, options)
			if Bool(report.Graph, "complete") {
				t.Fatal("depth boundary certified as a leaf")
			}
			if depth == 0 {
				root := plannerNode(report.Graph, "1")
				if len(*calls) != 0 || Str(root, "expansion") != "depth_limit" || root["dependencyCount"] != nil || root["observedDependencies"] != nil || Int(report.Graph, "unexpanded") != 1 {
					t.Fatalf("zero-depth graph: %#v", report)
				}
			} else {
				if Str(plannerNode(report.Graph, "1"), "expansion") != "complete" || Str(plannerNode(report.Graph, "2"), "expansion") != "depth_limit" || Str(plannerNode(report.Graph, "3"), "expansion") != "depth_limit" {
					t.Fatalf("one-depth graph: %#v", report)
				}
			}
		})
	}
}

func TestPlannerGraphNodeCap(t *testing.T) {
	reader, _ := plannerGraphFixture(map[string][]int{"1": {2, 3}}, graphFixtureOptions{})
	options := plannerGraphOptions()
	options.MaxNodes = 2
	report := plannerTraverse(t, reader, options)
	if len(Objects(report.Graph["nodes"])) != 2 || !reflect.DeepEqual(report.Graph["edges"], []Object{{"fromRunId": "1", "toRunId": "2", "kind": "snapshot"}}) || Str(plannerNode(report.Graph, "1"), "expansion") != "node_limit" || Str(plannerNode(report.Graph, "2"), "expansion") != "complete" || report.OmittedTargets != 1 || Bool(report.Graph, "complete") {
		t.Fatalf("node cap: %#v", report)
	}
}

func TestPlannerGraphIndependentFailures(t *testing.T) {
	reader, _ := plannerGraphFixture(map[string][]int{"1": {2, 3}, "3": {4}}, graphFixtureOptions{countErrors: map[string]string{"2": "PERMISSION_DENIED"}, pageErrors: map[string]string{"2": "PERMISSION_DENIED"}})
	report := plannerTraverse(t, reader, plannerGraphOptions())
	if Str(plannerNode(report.Graph, "2"), "expansion") != "permission_denied" || Str(plannerNode(report.Graph, "4"), "expansion") != "complete" || Bool(report.Graph, "complete") {
		t.Fatalf("independent denial: %#v", report)
	}
	reader, _ = plannerGraphFixture(map[string][]int{"1": {2}}, graphFixtureOptions{counts: map[string]int{"1": 2}})
	report = plannerTraverse(t, reader, plannerGraphOptions())
	if len(Objects(report.Graph["nodes"])) != 2 || Str(plannerNode(report.Graph, "1"), "expansion") != "unavailable" || !plannerHasLimitation(report.Limitations, "DEPENDENCY_COUNT_MISMATCH") {
		t.Fatalf("count mismatch: %#v", report)
	}
	reader, _ = plannerGraphFixture(nil, graphFixtureOptions{counts: map[string]int{"1": 1}, pages: map[string]map[int]Object{"1": {0: plannerPage([]Object{{"run": plannerRun(2, nil), "projectId": "Allowed"}}, Object{"limitations": []Limitation{{Code: "UNSAFE_CONTINUATION", Message: "Unsafe cursor"}}})}}})
	report = plannerTraverse(t, reader, plannerGraphOptions())
	if len(Objects(report.Graph["nodes"])) != 2 || Str(plannerNode(report.Graph, "1"), "expansion") != "unavailable" {
		t.Fatalf("unsafe continuation: %#v", report)
	}
}

func TestPlannerGraphPolicyAndCallCaps(t *testing.T) {
	reader, _ := plannerGraphFixture(map[string][]int{"1": {2, 3}}, graphFixtureOptions{foreign: map[string]bool{"3": true}})
	report := plannerTraverse(t, reader, plannerGraphOptions())
	if len(Objects(report.Graph["nodes"])) != 2 || plannerNode(report.Graph, "3") != nil || Str(plannerNode(report.Graph, "1"), "expansion") != "permission_denied" {
		t.Fatalf("foreign node retained: %#v", report)
	}
	ids := []string{}
	for _, node := range Objects(report.Graph["nodes"]) {
		ids = append(ids, Str(Obj(node["run"]), "id"))
	}
	if !reflect.DeepEqual(ids, []string{"1", "2"}) {
		t.Fatalf("policy changed retained nodes: %v", ids)
	}
	for _, edge := range Objects(report.Graph["edges"]) {
		if Str(edge, "toRunId") == "3" {
			t.Fatal("foreign dangling edge retained")
		}
	}
	reader, calls := plannerGraphFixture(map[string][]int{"1": {2}}, graphFixtureOptions{})
	options := plannerGraphOptions()
	options.MaxGraphReads = 1
	report = plannerTraverse(t, reader, options)
	if !reflect.DeepEqual(*calls, []string{"count:1"}) || Str(plannerNode(report.Graph, "1"), "expansion") != "call_limit" || plannerNode(report.Graph, "1")["observedDependencies"] != nil {
		t.Fatalf("graph read cap: %#v, %v", report, *calls)
	}
}

func TestPlannerGraphOffsetContinuation(t *testing.T) {
	item := Object{"run": plannerRun(2, nil), "projectId": "Allowed"}
	reader, calls := plannerGraphFixture(nil, graphFixtureOptions{counts: map[string]int{"1": 1}, pages: map[string]map[int]Object{"1": {0: plannerPage([]Object{}, Object{"position": 100, "hasMore": true}), 100: plannerPage([]Object{item}, Object{"hasMore": nil, "limitations": []Limitation{{Code: "SCAN_COVERAGE_UNKNOWN", Message: "No continuation", Source: "dependencies"}}})}}})
	report := plannerTraverse(t, reader, plannerGraphOptions())
	if !Bool(report.Graph, "complete") || !stringContains(*calls, "page:1:100") {
		t.Fatalf("empty continued page: %#v, %v", report, *calls)
	}
	reader, _ = plannerGraphFixture(nil, graphFixtureOptions{counts: map[string]int{"1": 3}, pages: map[string]map[int]Object{"1": {0: plannerPage([]Object{item}, Object{"position": 100, "hasMore": true}), 100: plannerPage([]Object{item}, nil)}}})
	report = plannerTraverse(t, reader, plannerGraphOptions())
	if Str(plannerNode(report.Graph, "1"), "expansion") != "unavailable" || !plannerHasLimitation(report.Limitations, "DUPLICATE_DEPENDENCY") {
		t.Fatalf("duplicate dependency: %#v", report)
	}
}

func TestPlannerGraphRejectsMetadataFromDeniedOrConflictingIdentities(t *testing.T) {
	for _, denied := range []bool{true, false} {
		t.Run(fmt.Sprint(denied), func(t *testing.T) {
			root, project, code := plannerRun(1, Object{"jobId": "Substituted"}), "Allowed", "MISSING_REVISION_METADATA"
			if denied {
				root, project, code = plannerRun(1, Object{"result": "unknown"}), "Foreign", "UNKNOWN_RESULT"
			}
			reader, _ := plannerGraphFixture(nil, graphFixtureOptions{counts: map[string]int{"1": 1, "2": 1}, pages: map[string]map[int]Object{"1": {0: plannerPage([]Object{{"run": plannerRun(2, nil), "projectId": "Allowed"}}, nil)}, "2": {0: plannerPage([]Object{{"run": root, "projectId": project}}, Object{"limitations": []Limitation{{Code: code, Message: "Rejected metadata", Source: "run", RunID: "1"}}})}}})
			report := plannerTraverse(t, reader, plannerGraphOptions())
			if len(Objects(report.Graph["edges"])) != 1 || plannerHasLimitation(report.Limitations, code) {
				t.Fatalf("rejected identity leaked metadata: %#v", report)
			}
			if denied && Str(plannerNode(report.Graph, "2"), "expansion") != "permission_denied" {
				t.Fatal("denial not preserved")
			}
			if !denied && !plannerHasLimitation(report.Limitations, "CONTEXT_MISMATCH") {
				t.Fatal("identity conflict not preserved")
			}
		})
	}
}

func TestPlannerGraphChangedObservationAndDiagnosticDeduplication(t *testing.T) {
	reader, _ := plannerGraphFixture(map[string][]int{"1": {2, 3}, "2": {4}, "3": {4}}, graphFixtureOptions{alter: func(request ReadRequest, page Object) {
		if request.RunID == "3" {
			Obj(Objects(page["items"])[0]["run"])["branch"] = "different"
		}
	}})
	report := plannerTraverse(t, reader, plannerGraphOptions())
	if Bool(report.Graph, "complete") || !plannerHasLimitation(report.Limitations, "RUN_STATE_CHANGED") || Str(Obj(plannerNode(report.Graph, "4")["run"]), "branch") != "main" {
		t.Fatalf("changed shared observation: %#v", report)
	}
	wide := []int{}
	for id := 2; id <= 200; id++ {
		wide = append(wide, id)
	}
	reader, _ = plannerGraphFixture(map[string][]int{"1": wide}, graphFixtureOptions{})
	options := plannerGraphOptions()
	options.MaxNodes, options.MaxGraphReads = 200, 24
	report = plannerTraverse(t, reader, options)
	count := 0
	for _, note := range report.Limitations {
		if note.Code == "CALL_LIMIT_EXCEEDED" {
			count++
		}
	}
	if len(Objects(report.Graph["nodes"])) != 101 || Bool(report.Graph, "complete") || count != 1 {
		t.Fatalf("bounded budget diagnostics: %#v", report)
	}
	callLimited := false
	for _, node := range Objects(report.Graph["nodes"]) {
		callLimited = callLimited || Str(node, "expansion") == "call_limit"
	}
	if !callLimited {
		t.Fatal("wide graph omitted call-limited expansion states")
	}
}

func TestPlannerGraphValidationAndInterrupt(t *testing.T) {
	reader, _ := plannerGraphFixture(nil, graphFixtureOptions{})
	for _, alter := range []func(*GraphOptions){func(o *GraphOptions) { o.Depth = -1 }, func(o *GraphOptions) { o.Depth = 13 }, func(o *GraphOptions) { o.MaxNodes = 0 }, func(o *GraphOptions) { o.MaxNodes = 201 }, func(o *GraphOptions) { o.MaxGraphReads = -1 }, func(o *GraphOptions) { o.MaxGraphReads = 25 }} {
		options := plannerGraphOptions()
		alter(&options)
		if _, err := BuildGraph(context.Background(), reader, ExecutionContext{}, "1", options); err == nil || AsDomainError(err).Code != "USAGE_ERROR" {
			t.Fatal("invalid graph bound accepted")
		}
	}
	reader, _ = plannerGraphFixture(nil, graphFixtureOptions{countErrors: map[string]string{"1": "INTERRUPTED"}})
	if _, err := BuildGraph(context.Background(), reader, ExecutionContext{}, "1", plannerGraphOptions()); err == nil || AsDomainError(err).Code != "INTERRUPTED" {
		t.Fatal("interruption not propagated")
	}
}

func TestPlannerGraphRunProjectionAndRevisionComparison(t *testing.T) {
	run := plannerRun(1, Object{"result": "unknown", "rawStatus": "FUTURE", "secretUpstream": "must not escape", "revisions": []Object{{"vcsRootId": "B", "revision": "2"}, {"vcsRootId": "A", "revision": "1"}}})
	other := plannerRun(1, Object{"result": "unknown", "rawStatus": "FUTURE", "revisions": []Object{{"vcsRootId": "A", "revision": "1"}, {"vcsRootId": "B", "revision": "2"}}})
	if GraphRun(run)["secretUpstream"] != nil || Str(GraphRun(run), "rawStatus") != "FUTURE" || !SameGraphObservation(run, other) {
		t.Fatal("graph projection or revision order equivalence failed")
	}
	other["branch"] = "different"
	if SameGraphObservation(run, other) {
		t.Fatal("changed branch hidden")
	}
}
