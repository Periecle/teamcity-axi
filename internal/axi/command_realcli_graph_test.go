//go:build realcli

package axi

import (
	"reflect"
	"strings"
	"testing"
)

func nativeTree(extra ...string) []string { return append([]string{"run", "tree", "482193"}, extra...) }
func nativeFailure(extra ...string) []string {
	return append([]string{"run", "failure", "482193"}, extra...)
}
func nativeGraph(r nativeCall) Object { return Obj(nativeData(r)["graph"]) }
func nativeGraphIDs(r nativeCall) []string {
	ids := []string{}
	for _, node := range Objects(nativeGraph(r)["nodes"]) {
		ids = append(ids, Str(Obj(node["run"]), "id"))
	}
	return ids
}
func nativeExpansion(r nativeCall, id string) string {
	for _, node := range Objects(nativeGraph(r)["nodes"]) {
		if Str(Obj(node["run"]), "id") == id {
			return Str(node, "expansion")
		}
	}
	return ""
}

// Original: tree.test.mjs — released CLI tree retains a shared DAG and cycle, equivalent renderers and exact scope.
func TestRealCLITreeDAGAndCycle(t *testing.T) {
	f := newNativeFixture(t, true)
	r := f.call(t, nativeTree("--json")...)
	requireNativeCode(t, r, 0)
	requireNativeValue(t, r.value["status"], "ok")
	requireNativeValue(t, nativeGraph(r)["cycles"], []Object{})
	if !reflect.DeepEqual(nativeGraphIDs(r), []string{"482193", "482190", "482191", "482188"}) || len(Objects(nativeGraph(r)["edges"])) != 4 || !Bool(nativeGraph(r), "complete") || Int(Obj(nativeMeta(r)["counts"]), "childProcesses") != 9 || Int(Obj(nativeData(r)["selection"]), "graphReadAttempts") != 7 {
		t.Fatalf("DAG: %s", r.stdout)
	}
	nodes := Objects(nativeGraph(r)["nodes"])
	requireNativeValue(t, nodes[len(nodes)-1]["dependencyCount"], 0)
	toon := f.call(t, nativeTree()...)
	if !reflect.DeepEqual(nativeData(r), nativeData(toon)) || len(toon.stdout) > 24576 {
		t.Fatal("tree formats disagree")
	}
	requireNativeError(t, f.call(t, nativeTree("--job", "Other", "--json")...), "CONTEXT_MISMATCH")
	f.mock.SetMode("cycle")
	r = f.call(t, nativeTree("--json")...)
	requireNativeCode(t, r, 0)
	if len(Objects(nativeGraph(r)["nodes"])) != 3 || len(Objects(nativeGraph(r)["edges"])) != 3 || len(Objects(nativeGraph(r)["cycles"])) != 1 || !Bool(nativeGraph(r), "complete") {
		t.Fatal("cycle graph lost")
	}
	requireNativeValue(t, r.value["status"], "ok")
	for _, q := range f.mock.Requests() {
		if q.Method != "GET" {
			t.Fatal("graph mutation")
		}
		if strings.HasSuffix(q.Path, "/app/rest/builds") && !strings.Contains(q.Query.Get("locator"), "recursive:false") {
			t.Fatal("recursive unbounded query")
		}
	}
}

// Original: tree.test.mjs — tree boundaries and process ceilings remain explicit without dangling edges or false leaves.
func TestRealCLITreeBoundaries(t *testing.T) {
	f := newNativeFixture(t, true)
	r := f.call(t, nativeTree("--depth", "0", "--json")...)
	if Str(r.value, "status") != "partial" || len(Objects(nativeGraph(r)["nodes"])) != 1 || nativeExpansion(r, "482193") != "depth_limit" || Objects(nativeGraph(r)["nodes"])[0]["dependencyCount"] != nil || Int(Obj(nativeMeta(r)["counts"]), "childProcesses") != 2 {
		t.Fatal("depth zero false leaf")
	}
	r = f.call(t, nativeTree("--max-nodes", "2", "--json")...)
	requireNativeCode(t, r, 0)
	if len(Objects(nativeGraph(r)["nodes"])) != 2 || Int(Obj(nativeData(r)["selection"]), "omittedTargets") != 2 || nativeExpansion(r, "482193") != "node_limit" {
		t.Fatal("node limit false completeness")
	}
	if !containsString(Strings(Objects(r.value["next"])[0]["argv"]), "--project") {
		t.Fatal("graph retrieval hint lost project scope")
	}
	ids := nativeGraphIDs(r)
	for _, edge := range Objects(nativeGraph(r)["edges"]) {
		if !containsString(ids, Str(edge, "toRunId")) {
			t.Fatal("dangling graph edge")
		}
	}
	r = f.call(t, nativeTree("--depth", "1", "--require-complete", "--no-hints", "--json")...)
	requireNativeCode(t, r, 1)
	requireNativePartial(t, r)
	if r.value["next"] != nil {
		t.Fatal("no-hints ignored")
	}
	f.mock.SetMode("wide")
	r = f.call(t, nativeTree("--max-nodes", "200", "--json")...)
	if Bool(nativeGraph(r), "complete") || Int(Obj(nativeMeta(r)["counts"]), "childProcesses") != 24 {
		t.Fatal("wide graph ignored call ceiling")
	}
	callLimited := false
	for _, node := range Objects(nativeGraph(r)["nodes"]) {
		if Str(node, "expansion") == "call_limit" {
			callLimited = true
		}
	}
	if !callLimited {
		t.Fatal("wide graph did not retain explicit call limit")
	}
	r = f.call(t, nativeTree("--max-bytes", "2048", "--json")...)
	requireNativeError(t, r, "INPUT_LIMIT_EXCEEDED")
	if len(r.stdout) > 2048 {
		t.Fatal("graph stdout cap failed")
	}
}

// Original: tree.test.mjs — independent graph failures preserve siblings and exclude foreign identities and diagnostics.
func TestRealCLITreeIndependentFailures(t *testing.T) {
	f := newNativeFixture(t, true)
	for _, mode := range []string{"denied", "mismatch", "unsafe", "foreign", "missing-metadata"} {
		f.mock.SetMode(mode)
		r := f.call(t, nativeTree("--json")...)
		requireNativeCode(t, r, 0)
		if Str(r.value, "status") != "partial" || len(Objects(nativeGraph(r)["nodes"])) < 2 {
			t.Fatalf("%s graph discarded: %s", mode, r.stdout)
		}
		if mode == "denied" && (nativeExpansion(r, "482190") != "permission_denied" || nativeExpansion(r, "482188") != "complete") {
			t.Fatal("denial discarded sibling")
		}
		if mode == "foreign" {
			if containsString(nativeGraphIDs(r), "482191") || strings.Contains(r.stdout, "Foreign") {
				t.Fatal("foreign graph identity leaked")
			}
			for _, n := range Objects(nativeMeta(r)["limitations"]) {
				if Str(n, "runId") == "482191" {
					t.Fatal("foreign diagnostics leaked")
				}
			}
		}
		if (mode == "mismatch" || mode == "unsafe") && nativeExpansion(r, "482193") != "unavailable" {
			t.Fatal("unsafe root marked leaf")
		}
	}
	f.mock.SetMode("invalid-timestamp")
	r := f.call(t, nativeTree("--json")...)
	for _, n := range Objects(nativeMeta(r)["limitations"]) {
		if Str(n, "code") == "INVALID_TIMESTAMP" && (Str(n, "runId") == "" || Str(n, "runId") == "482193") {
			t.Fatal("child provenance attributed to root")
		}
	}
	f.mock.SetMode("empty")
	r = f.call(t, nativeTree("--json")...)
	if !Bool(nativeGraph(r), "complete") {
		t.Fatal("counted empty continuation remained incomplete")
	}
	requireNativeValue(t, r.value["status"], "ok")
	continued := false
	for _, request := range f.mock.Requests() {
		if strings.Contains(request.Query.Get("locator"), "start:100") {
			continued = true
		}
	}
	if !continued {
		t.Fatal("empty intermediate graph page stopped traversal")
	}
	f.mock.SetMode("root-denied")
	requireNativeError(t, f.call(t, nativeTree("--json")...), "PERMISSION_DENIED")
}

// Original: tree.test.mjs — tree reserves a final non-terminal root observation even when graph discovery has no capacity.
func TestRealCLITreeReservedFinal(t *testing.T) {
	f := newNativeFixture(t, true)
	f.config.Limits.MaxChildProcesses = 3
	f.writeConfig(t)
	for _, mode := range []string{"changed", "changed-metadata", "provisional", "final-unavailable"} {
		f.mock.SetMode(mode)
		f.mock.ClearRequests()
		r := f.call(t, nativeTree("--json")...)
		requireNativeCode(t, r, 0)
		if Str(r.value, "status") != "partial" || Int(Obj(nativeMeta(r)["counts"]), "childProcesses") != 3 || Int(Obj(nativeData(r)["selection"]), "maxGraphReads") != 0 || Bool(nativeGraph(r), "complete") {
			t.Fatal("reservation lost final capacity")
		}
		details := 0
		for _, q := range f.mock.Requests() {
			if q.Path == "/teamcity/app/rest/builds/id:482193" {
				details++
			}
		}
		if details != 2 {
			t.Fatal("reserved final was not read")
		}
		if mode == "changed" && (nativeRun(r)["state"] != "finished" || nativeRun(r)["result"] != "success" || !nativeNotes(r, "ROOT_STATE_CHANGED")) {
			t.Fatal("changed final state not retained")
		}
		if mode == "changed" {
			requireNativeValue(t, Objects(nativeGraph(r)["nodes"])[0]["run"], nativeRun(r))
		}
		if mode == "changed-metadata" && nativeRun(r)["branch"] != "changed-branch" {
			t.Fatal("changed metadata lost")
		}
		if mode == "changed-metadata" && !nativeNotes(r, "ROOT_STATE_CHANGED") {
			t.Fatal("metadata change limitation missing")
		}
	}
	f.config.Limits = UserLimits{}
	f.writeConfig(t)
	f.mock.SetMode("final-unavailable")
	r := f.call(t, nativeTree("--json")...)
	requireNativeValue(t, nativeGraph(r)["complete"], false)
	if nativeExpansion(r, "482193") != "unavailable" || Int(nativeGraph(r), "unexpanded") != 1 {
		t.Fatal("final unavailable leaf promoted")
	}
}

// Original: tree.test.mjs — released CLI failure report preserves independent source references, duplicate/muted identities and bounded diagnosis.
func TestRealCLIFailureSourcesAndDiagnosis(t *testing.T) {
	f := newNativeFixture(t, true)
	r := f.call(t, nativeFailure("--json")...)
	requireNativeCode(t, r, 0)
	requireNativePartial(t, r)
	data := nativeData(r)
	if Str(data, "assessment") != "failure_observed" || !reflect.DeepEqual(Strings(Obj(data["selection"])["diagnosedRunIds"]), []string{"482193", "482190", "482191"}) || Int(Obj(data["selection"]), "omittedDiagnosedRuns") != 1 || Int(Obj(nativeMeta(r)["counts"]), "childProcesses") > 24 {
		t.Fatalf("bounded diagnosis: %s", r.stdout)
	}
	failed := 0
	dependencyFailure := false
	if Int(Obj(data["selection"]), "graphReadAttempts") > 10 {
		t.Fatal("failure graph exceeded reserved discovery budget")
	}
	for _, finding := range Objects(data["findings"]) {
		if finding["claim"] != "observation" {
			t.Fatal("finding overclaimed causation")
		}
		if Str(finding, "kind") == "failed_test" {
			failed++
		}
		if Str(finding, "kind") == "dependency_failure" {
			dependencyFailure = true
		}
		for _, e := range Objects(finding["evidence"]) {
			hint := Obj(e["retrieve"])
			argv := Strings(hint["argv"])
			if !containsString(argv, "--project") {
				t.Fatal("failure evidence retrieval lost project scope")
			}
			if _, err := Parse(argv[1:]); err != nil {
				t.Fatal(err)
			}
			sourceExists := false
			for _, source := range Objects(data["sources"]) {
				if source["id"] == e["sourceRef"] && source["runId"] == e["runId"] && containsString([]string{"complete", "partial"}, Str(source, "state")) {
					sourceExists = true
				}
			}
			if !sourceExists {
				t.Fatal("evidence has dangling sourceRef")
			}
		}
	}
	if failed != 12 {
		t.Fatalf("duplicate/muted findings lost: %d", failed)
	}
	if !dependencyFailure {
		t.Fatal("independent dependency failure finding missing")
	}
	toon := f.call(t, nativeFailure()...)
	requireNativeCode(t, toon, 0)
	requireNativeValue(t, stableNativeFindings(nativeData(toon)["findings"]), stableNativeFindings(data["findings"]))
	if len(toon.stdout) > 24576 {
		t.Fatal("failure TOON output cap exceeded")
	}
	if !reflect.DeepEqual(nativeGraph(r), nativeGraph(toon)) {
		t.Fatal("failure graph formats disagree")
	}
	strict := f.call(t, nativeFailure("--require-complete", "--no-hints", "--json")...)
	requireNativeCode(t, strict, 1)
	requireNativePartial(t, strict)
	requireNativeAbsent(t, strict.value, "next")
	f.mock.SetMode("success")
	f.mock.ClearRequests()
	r = f.call(t, nativeFailure("--json")...)
	if Str(nativeData(r), "assessment") != "not_failed" || Str(r.value, "status") != "ok" || Int(Obj(nativeMeta(r)["counts"]), "childProcesses") != 2 || len(f.mock.Requests()) != 1 {
		t.Fatal("successful run unnecessarily investigated")
	}
	requireNativeValue(t, Objects(nativeGraph(r)["nodes"])[0]["expansion"], "not_requested")
}

// Original: tree.test.mjs — released CLI failure source errors, final read, redaction and byte ceilings remain explicit.
func TestRealCLIFailureErrorsAndBounds(t *testing.T) {
	f := newNativeFixture(t, true)
	for _, mode := range []string{"sources-denied", "logs-needed"} {
		f.mock.SetMode(mode)
		r := f.call(t, nativeFailure("--json")...)
		requireNativeCode(t, r, 0)
		unavailable := false
		for _, s := range Objects(nativeData(r)["sources"]) {
			if Str(s, "state") == "unavailable" && s["returned"] == nil {
				unavailable = true
			}
		}
		if !unavailable || Str(r.value, "status") != "partial" {
			t.Fatal("independent failure hidden")
		}
		if mode == "sources-denied" {
			failedTest := false
			for _, finding := range Objects(nativeData(r)["findings"]) {
				if Str(finding, "kind") == "failed_test" {
					failedTest = true
				}
			}
			if !failedTest {
				t.Fatal("source denial erased sibling failed tests")
			}
		}
		if mode == "logs-needed" {
			requireNativeValue(t, nativeSource(r, "log:482193")["state"], "unavailable")
		}
	}
	f.config.Limits.MaxChildProcesses = 4
	f.writeConfig(t)
	f.mock.SetMode("changed")
	r := f.call(t, nativeFailure("--json")...)
	requireNativeCode(t, r, 0)
	if Int(Obj(nativeMeta(r)["counts"]), "childProcesses") != 4 || nativeRun(r)["state"] != "finished" || nativeData(r)["assessment"] != "inconclusive" || Bool(nativeGraph(r), "complete") {
		t.Fatal("failure final observation lost")
	}
	exhausted := false
	for _, source := range Objects(nativeData(r)["sources"]) {
		if Str(source, "state") == "budget_exhausted" {
			exhausted = true
		}
	}
	if !exhausted {
		t.Fatal("reserved final read lost explicit exhausted source")
	}
	f.config.Limits = UserLimits{}
	f.writeConfig(t)
	f.mock.SetMode("secret")
	r = f.call(t, nativeFailure("--max-diagnosed-runs", "1", "--json")...)
	requireNativeCode(t, r, 0)
	if !strings.Contains(r.stdout, "[REDACTED]") {
		t.Fatal("failure evidence redaction absent")
	}
	limited := f.call(t, nativeFailure("--max-bytes", "2048", "--json")...)
	requireNativeError(t, limited, "INPUT_LIMIT_EXCEEDED")
	if len(limited.stdout) > 2048 {
		t.Fatal("failure output cap exceeded")
	}
	f.mock.SetMode("changes-positive")
	r = f.call(t, nativeFailure("--max-diagnosed-runs", "1", "--json")...)
	requireNativeCode(t, r, 0)
	changes := Objects(nativeData(r)["changes"])
	if len(changes) != 1 || Str(changes[0], "vcsRootId") != "Payments_Git" || Str(changes[0], "timestamp") != "2026-10-01T13:59:00.000Z" || Str(changes[0], "message") != "Synthetic contextual change" {
		t.Fatal("failure contextual change lost")
	}
	f.mock.SetMode("root-denied")
	requireNativeError(t, f.call(t, nativeFailure("--json")...), "PERMISSION_DENIED")
}

// Original: tree.test.mjs — released CLI failure preserves Unicode tail retrieval and reduces optional changes before core evidence.
func TestRealCLIFailureUnicodeAndReduction(t *testing.T) {
	f := newNativeFixture(t, true)
	f.mock.SetMode("log-unicode")
	r := f.call(t, nativeFailure("--max-diagnosed-runs", "1", "--json")...)
	requireNativeCode(t, r, 0)
	var finding Object
	for _, value := range Objects(nativeData(r)["findings"]) {
		if Str(value, "kind") == "log_signal" {
			finding = value
			break
		}
	}
	if finding == nil {
		t.Fatal("unicode log signal missing")
	}
	action := Strings(Obj(Objects(finding["evidence"])[0]["retrieve"])["argv"])
	expanded := f.call(t, append(action[1:], "--json")...)
	requireNativeCode(t, expanded, 0)
	if len(Objects(nativeData(expanded)["messages"])) != 1 || Str(Objects(nativeData(expanded)["messages"])[0], "text") != "--error=Connection refused "+strings.Repeat("x", 52)+"🦊" {
		t.Fatal("Unicode retrieval failed")
	}
	requireNativeValue(t, nativeSource(r, "log:482193")["window"], Object{"requested": 80, "firstMessageId": "12", "lastMessageId": "12", "omittedProviderMessages": 0})
	f.mock.SetMode("changes-wide")
	r = f.call(t, nativeFailure("--max-diagnosed-runs", "1", "--full", "--max-bytes", "16384", "--json")...)
	requireNativeCode(t, r, 0)
	data := nativeData(r)
	if len(Objects(data["findings"])) == 0 || Int(Obj(data["selection"]), "omittedChanges") != 10-len(Objects(data["changes"])) || len(r.stdout) > 16384 {
		t.Fatal("optional change reduction erased core evidence")
	}
	if Int(Obj(data["selection"]), "omittedChanges") <= 0 {
		t.Fatal("optional change reduction was not exercised")
	}
	requireNativeValue(t, nativeSource(r, "changes:482193")["returned"], len(Objects(data["changes"])))
}
