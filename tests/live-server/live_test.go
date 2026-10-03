//go:build live

package liveserver

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Periecle/teamcity-axi/internal/axi"
	"github.com/Periecle/teamcity-axi/internal/livefixture"
	toon "github.com/toon-format/toon-go"
)

var liveBuildOnce sync.Once
var liveBinary, liveBuildDir string
var liveBuildError error

func TestMain(m *testing.M) {
	code := m.Run()
	if liveBuildDir != "" {
		_ = os.RemoveAll(liveBuildDir)
	}
	os.Exit(code)
}
func fixture(t *testing.T) *livefixture.Fixture {
	t.Helper()
	liveBuildOnce.Do(func() {
		liveBuildDir, liveBuildError = os.MkdirTemp("", "axi-live-go-build-")
		if liveBuildError != nil {
			return
		}
		liveBinary = filepath.Join(liveBuildDir, "teamcity-axi")
		cmd := exec.Command("go", "build", "-o", liveBinary, "./cmd/teamcity-axi")
		cmd.Dir = livefixture.RepositoryRoot()
		if output, e := cmd.CombinedOutput(); e != nil {
			liveBuildError = fmt.Errorf("build Go wrapper: %v: %s", e, output)
		}
	})
	if liveBuildError != nil {
		t.Fatal(liveBuildError)
	}
	f, e := livefixture.New(os.Getenv("TEAMCITY_AXI_LIVE_CREDENTIALS"), os.Getenv("TEAMCITY_AXI_TEST_BINARY"), liveBinary)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := f.Close(); e != nil {
			t.Error(e)
		}
	})
	return f
}
func equal(t *testing.T, got, want any) {
	t.Helper()
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		t.Fatalf("got %s; want %s", a, b)
	}
}
func require(t *testing.T, ok bool, message string) {
	t.Helper()
	if !ok {
		t.Fatal(message)
	}
}
func integer(value any) int {
	switch n := value.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}
func at(o axi.Object, keys ...string) any {
	var value any = o
	for _, key := range keys {
		value = axi.Obj(value)[key]
	}
	return value
}
func objects(value any) []axi.Object { return axi.Objects(value) }
func decode(t *testing.T, result livefixture.Result, format string) axi.Response {
	t.Helper()
	var raw any
	if format == "json" {
		if e := json.Unmarshal([]byte(result.Stdout), &raw); e != nil {
			t.Fatalf("JSON document: %v: %s", e, result.Stdout)
		}
	} else {
		if e := toon.UnmarshalString(result.Stdout, &raw); e != nil {
			t.Fatalf("TOON document: %v: %s", e, result.Stdout)
		}
	}
	b, e := json.Marshal(raw)
	if e != nil {
		t.Fatal(e)
	}
	var value axi.Response
	if e = json.Unmarshal(b, &value); e != nil {
		t.Fatal(e)
	}
	if e = axi.ValidateResponse(value); e != nil {
		t.Fatalf("invalid public response: %v: %s", e, result.Stdout)
	}
	return value
}
func wire(t *testing.T, f *livefixture.Fixture, args []string, code int) livefixture.Result {
	t.Helper()
	r, e := f.Wrapper(args)
	if e != nil {
		t.Fatal(e)
	}
	equal(t, r.Code, code)
	equal(t, r.Stderr, "")
	return r
}
func jsonCall(t *testing.T, f *livefixture.Fixture, args []string, code int) axi.Response {
	t.Helper()
	return decode(t, wire(t, f, append(append([]string{}, args...), "--json"), code), "json")
}
func native(t *testing.T, f *livefixture.Fixture, name string) livefixture.Result {
	t.Helper()
	record, ok := f.Contract.Records[name]
	if !ok {
		t.Fatalf("missing native record %s", name)
	}
	r, e := f.Native(record.Args, false)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func api(t *testing.T, r livefixture.Result) axi.Object {
	t.Helper()
	response, e := axi.ParseRaw(r.Captured())
	if e != nil {
		t.Fatal(e)
	}
	return axi.Obj(response.Body)
}
func ids(rows []axi.Object, key string) []string {
	result := []string{}
	for _, row := range rows {
		result = append(result, axi.Str(row, key))
	}
	return result
}
func parseActions(t *testing.T, actions []axi.Object) {
	t.Helper()
	for _, a := range actions {
		argv := axi.Strings(a["argv"])
		require(t, len(argv) > 1, "missing action argv")
		if _, e := axi.Parse(argv[1:]); e != nil {
			t.Fatal(e)
		}
	}
}
func action(t *testing.T, actions []axi.Object, flag string) []string {
	t.Helper()
	for _, a := range actions {
		argv := axi.Strings(a["argv"])
		if flag == "" || contains(argv, flag) {
			return argv[1:]
		}
	}
	t.Fatalf("missing %s action", flag)
	return nil
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func assertPermissions(t *testing.T, f *livefixture.Fixture, name string) {
	t.Helper()
	rows := objects(api(t, native(t, f, name))["permissionAssignment"])
	permissions, projects := []string{}, []string{}
	for _, row := range rows {
		id := axi.Str(axi.Obj(row["permission"]), "id")
		permissions = append(permissions, id)
		if id == "view_project" {
			equal(t, row["isGlobalScope"], false)
			projects = append(projects, axi.Str(axi.Obj(row["project"]), "id"))
		}
	}
	sort.Strings(permissions)
	sort.Strings(projects)
	expected := []string{axi.Str(f.Contract.Fixture, "projectId"), axi.Str(axi.Obj(f.Contract.Fixture["lifecycle"]), "projectId"), "_Root"}
	sort.Strings(expected)
	equal(t, permissions, []string{"change_own_profile", "view_project", "view_project", "view_project"})
	equal(t, projects, expected)
}
func TestLivePinnedCLIRestrictedPermissionAndBoundedReads(t *testing.T) {
	f := fixture(t)
	version, e := f.Native([]string{"--version"}, false)
	if e != nil {
		t.Fatal(e)
	}
	equal(t, version.Stdout, "teamcity version 1.5.0\n")
	equal(t, version.Code, 0)
	equal(t, api(t, native(t, f, "server"))["buildNumber"], f.Contract.Server["buildNumber"])
	assertPermissions(t, f, "permissions")
	for _, name := range []string{"problems", "tests", "dependencies", "changes", "queue", "jobs", "agents", "pages", "encoded-page", "empty-page"} {
		api(t, native(t, f, name))
	}
	unsupported := native(t, f, "dependencies-unsupported")
	equal(t, unsupported.Code, 1)
	_, e = axi.ParseRaw(unsupported.Captured())
	require(t, e != nil && axi.AsDomainError(e).Code == "UPSTREAM_FAILURE", "unsupported dependency lookup was accepted")
	log := native(t, f, "log")
	equal(t, log.Code, 0)
	value, e := axi.DecodeJSON([]byte(log.Stdout))
	if e != nil {
		t.Fatal(e)
	}
	equal(t, axi.Obj(value)["run_id"], f.Contract.Fixture["failedRunId"])
}
func TestLiveBoundedRunListContinuationTotalsAndCursorScope(t *testing.T) {
	f := fixture(t)
	p := f.Contract.Fixture
	args := []string{"run", "list", "--job", axi.Str(p, "jobId"), "--all-branches", "--limit", "1", "--since", "2026-10-01T00:00:00Z", "--until", "2026-10-03T00:00:00Z"}
	first := jsonCall(t, f, args, 0)
	equal(t, first.Status, "ok")
	runs := objects(first.Data["runs"])
	equal(t, runs[0]["id"], "1")
	equal(t, runs[0]["result"], "failure")
	equal(t, at(first.Data, "page", "returned"), 1)
	equal(t, at(first.Data, "page", "hasMore"), true)
	equal(t, at(first.Data, "page", "total"), nil)
	equal(t, at(first.Data, "page", "totalKind"), "unknown")
	equal(t, at(first.Meta, "counts", "childProcesses"), 3)
	parseActions(t, first.Next)
	second := jsonCall(t, f, action(t, first.Next, ""), 0)
	equal(t, objects(second.Data["runs"]), []axi.Object{})
	equal(t, second.Status, "ok")
	equal(t, at(second.Data, "page", "hasMore"), false)
	equal(t, at(second.Data, "page", "total"), nil)
	jsonCall(t, f, append(append([]string{}, args...), "--cursor", axi.Str(axi.Obj(first.Data["page"]), "cursor"), "--result", "success"), 2)
	project := jsonCall(t, f, []string{"run", "list", "--project", axi.Str(p, "projectId"), "--all-branches", "--fields", "number"}, 0)
	equal(t, ids(objects(project.Data["runs"]), "id"), p["projectRunIds"])
	equal(t, at(project.Data, "aggregates", "scope"), "returnedPage")
	equal(t, at(project.Data, "aggregates", "failure"), len(axi.Strings(p["projectFailedRunIds"])))
	for _, run := range objects(project.Data["runs"]) {
		_, present := run["branch"]
		require(t, present, "branch identity omitted")
	}
	precise := jsonCall(t, f, []string{"run", "list", "--job", axi.Str(p, "jobId"), "--all-branches", "--since", "2026-10-01T00:00:00.0001Z"}, 0)
	equal(t, at(precise.Data, "selection", "window", "since"), "2026-10-01T00:00:00.0001Z")
	equal(t, ids(objects(precise.Data["runs"]), "id"), []string{axi.Str(p, "failedRunId")})
}
func TestLiveExactFailedObservationDeniedMissingMismatchAndExpiredAuth(t *testing.T) {
	f := fixture(t)
	p := f.Contract.Fixture
	for _, format := range []string{"json", "toon"} {
		value := decode(t, wire(t, f, []string{"run", "view", axi.Str(p, "failedRunId"), "--format", format}, 0), format)
		equal(t, value.Status, "ok")
		equal(t, at(value.Data, "run", "id"), p["failedRunId"])
		equal(t, at(value.Data, "run", "result"), "failure")
		equal(t, value.Context["server"], "sandbox")
	}
	for _, entry := range []struct {
		id    string
		flags []string
		code  string
	}{{axi.Str(p, "deniedRunId"), nil, "PERMISSION_DENIED"}, {axi.Str(p, "missingRunId"), nil, "NOT_FOUND"}, {axi.Str(p, "failedRunId"), []string{"--job", "AnotherJob"}, "CONTEXT_MISMATCH"}} {
		value := jsonCall(t, f, append([]string{"run", "view", entry.id}, entry.flags...), 1)
		equal(t, value.Error.Code, entry.code)
		require(t, value.Data == nil, "error leaked observation data")
	}
	expired, e := f.Native(f.Contract.Records["invalid-auth"].Args, true)
	if e != nil {
		t.Fatal(e)
	}
	_, e = axi.ParseRaw(expired.Captured())
	require(t, e != nil && axi.AsDomainError(e).Code == "AUTH_REQUIRED", "expired token error changed")
}
func TestLiveContextDoctorRestrictedScopeAndOptionalLogLimits(t *testing.T) {
	f := fixture(t)
	p := f.Contract.Fixture
	local := jsonCall(t, f, []string{"context", "show", "--job", axi.Str(p, "jobId")}, 0)
	equal(t, at(local.Data, "verification", "requested"), false)
	verified := jsonCall(t, f, []string{"context", "show", "--verify", "--job", axi.Str(p, "jobId")}, 0)
	equal(t, at(verified.Data, "verification", "authentication"), "authenticated")
	equal(t, at(verified.Data, "verification", "project", "id"), p["projectId"])
	equal(t, at(verified.Data, "verification", "policy"), "verified")
	fingerprint := axi.Str(axi.Obj(verified.Data["verification"]), "identityFingerprint")
	require(t, strings.HasPrefix(fingerprint, "sha256:") && len(fingerprint) == 39, "identity fingerprint malformed")
	offline := jsonCall(t, f, []string{"doctor", "--offline"}, 0)
	equal(t, at(offline.Meta, "counts", "childProcesses"), 1)
	equal(t, at(offline.Data, "authentication", "state"), "not_checked")
	doctor := jsonCall(t, f, []string{"doctor", "--job", axi.Str(p, "jobId")}, 0)
	equal(t, doctor.Status, "partial")
	equal(t, at(doctor.Data, "server", "buildNumber"), "238924")
	equal(t, doctor.Data["liveCertified"], false)
	for _, name := range []string{"structuredRunDetail", "boundedRunPages", "structuredLogTail"} {
		found := false
		for _, capability := range objects(doctor.Data["capabilities"]) {
			if axi.Str(capability, "name") == name {
				equal(t, capability["state"], "available")
				found = true
			}
		}
		require(t, found, "doctor capability missing")
	}
	path := filepath.Join(f.Dir, "teamcity-axi/config.json")
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var config axi.UserConfig
	if e = json.Unmarshal(raw, &config); e != nil {
		t.Fatal(e)
	}
	server := config.Servers["sandbox"]
	server.AllowedProjects = []string{"_Root"}
	config.Servers["sandbox"] = server
	raw, _ = json.Marshal(config)
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	subtree := jsonCall(t, f, []string{"run", "view", axi.Str(p, "failedRunId")}, 0)
	equal(t, at(subtree.Data, "run", "id"), p["failedRunId"])
	policy := jsonCall(t, f, []string{"context", "show", "--verify", "--project", axi.Str(p, "projectId")}, 0)
	equal(t, at(policy.Data, "verification", "policy"), "verified")
}
func TestLiveIndependentOccurrenceIdentitySelectedFiltersAndPaging(t *testing.T) {
	f := fixture(t)
	id := axi.Str(f.Contract.Fixture, "failedRunId")
	for _, format := range []string{"json", "toon"} {
		value := decode(t, wire(t, f, []string{"run", "tests", id, "--format", format}, 0), format)
		test := objects(value.Data["tests"])[0]
		equal(t, test["runId"], id)
		equal(t, test["testId"], "517450581327024597")
		equal(t, test["result"], "failure")
		equal(t, at(value.Data, "page", "total"), nil)
	}
	detail := jsonCall(t, f, []string{"run", "tests", id, "--test", "build:(id:1),id:2000000000", "--full"}, 0)
	equal(t, detail.Status, "ok")
	equal(t, objects(detail.Data["tests"])[0]["durationMs"], 25)
	equal(t, at(detail.Data, "page", "total"), 1)
	first := jsonCall(t, f, []string{"run", "problems", id, "--limit", "1"}, 0)
	equal(t, first.Status, "ok")
	equal(t, at(first.Data, "page", "hasMore"), true)
	parseActions(t, first.Next)
	second := jsonCall(t, f, action(t, first.Next, ""), 0)
	require(t, objects(second.Data["problems"])[0]["id"] != objects(first.Data["problems"])[0]["id"], "occurrence pagination repeated row")
	problem := jsonCall(t, f, []string{"run", "problems", id, "--problem", axi.Str(objects(first.Data["problems"])[0], "id")}, 0)
	equal(t, objects(problem.Data["problems"])[0]["id"], objects(first.Data["problems"])[0]["id"])
	equal(t, at(problem.Data, "page", "totalKind"), "exact")
	failed := jsonCall(t, f, []string{"run", "tests", id, "--failed"}, 0)
	equal(t, len(objects(failed.Data["tests"])), 1)
	equal(t, objects(failed.Data["tests"])[0]["muted"], false)
	muted := jsonCall(t, f, []string{"run", "tests", id, "--muted"}, 0)
	equal(t, objects(muted.Data["tests"]), []axi.Object{})
	equal(t, muted.Status, "partial")
	equal(t, at(muted.Data, "page", "total"), nil)
	denied := jsonCall(t, f, []string{"run", "tests", axi.Str(f.Contract.Fixture, "deniedRunId")}, 1)
	equal(t, denied.Error.Code, "PERMISSION_DENIED")
}
func TestLiveLogsCapNativeOverdeliveryAndFailureIndependentSources(t *testing.T) {
	f := fixture(t)
	id := axi.Str(f.Contract.Fixture, "failedRunId")
	value := jsonCall(t, f, []string{"run", "log", id, "--tail", "1"}, 0)
	equal(t, len(objects(value.Data["messages"])), 1)
	equal(t, at(value.Data, "window", "providerReturned"), 2)
	equal(t, at(value.Data, "window", "omittedProviderMessages"), 1)
	message := objects(value.Data["messages"])[0]
	equal(t, message["runId"], id)
	require(t, strings.HasSuffix(axi.Str(message, "timestamp"), "Z"), "timestamp not UTC")
	none := jsonCall(t, f, []string{"run", "log", id, "--tail", "1", "--contains", "synthetic-no-match-canary"}, 0)
	equal(t, len(objects(none.Data["messages"])), 0)
	equal(t, at(none.Data, "window", "retained"), 1)
	failure := jsonCall(t, f, []string{"run", "log", id, "--failed"}, 0)
	equal(t, failure.Status, "partial")
	equal(t, len(objects(failure.Data["problems"])), 3)
	equal(t, len(objects(failure.Data["tests"])), 1)
	equal(t, at(failure.Data, "sources", "tests", "coverage"), "bounded_page")
	equal(t, at(failure.Data, "sources", "log", "coverage"), "tail_window")
	equal(t, at(failure.Meta, "counts", "childProcesses"), 5)
}
func TestLiveChangesRootMessageFilesAndCursorScope(t *testing.T) {
	f := fixture(t)
	id := axi.Str(f.Contract.Fixture, "vcsRunId")
	first := jsonCall(t, f, []string{"run", "changes", id, "--limit", "1"}, 0)
	equal(t, first.Status, "ok")
	change := objects(first.Data["changes"])[0]
	equal(t, change["vcsRootId"], f.Contract.Fixture["vcsRootId"])
	equal(t, change["message"], "Synthetic change 3")
	_, present := change["files"]
	require(t, !present, "unrequested files propagated")
	equal(t, at(first.Data, "page", "hasMore"), true)
	parseActions(t, first.Next)
	second := jsonCall(t, f, action(t, first.Next, "--cursor"), 0)
	require(t, objects(second.Data["changes"])[0]["id"] != change["id"], "change pagination repeated row")
	full := jsonCall(t, f, action(t, first.Next, "--full"), 0)
	require(t, strings.Contains(axi.Str(objects(full.Data["changes"])[0], "message"), "Contextual fixture evidence only"), "full change detail absent")
	files := decode(t, wire(t, f, []string{"run", "changes", id, "--files"}, 0), "toon")
	equal(t, len(objects(files.Data["changes"])), 3)
	equal(t, objects(files.Data["changes"])[0]["files"], []string{"fixture.txt"})
	equal(t, files.Status, "partial")
	jsonCall(t, f, []string{"run", "changes", id, "--limit", "1", "--files", "--cursor", axi.Str(axi.Obj(first.Data["page"]), "cursor")}, 2)
	denied := jsonCall(t, f, []string{"run", "changes", axi.Str(f.Contract.Fixture, "deniedRunId")}, 1)
	equal(t, denied.Error.Code, "PERMISSION_DENIED")
}
func TestLiveRestrictedRunTreeDirectionLeavesAndBoundedExpansion(t *testing.T) {
	f := fixture(t)
	root, child := axi.Str(f.Contract.Fixture, "vcsRunId"), axi.Str(f.Contract.Fixture, "dependencyRunId")
	args := []string{"run", "tree", root}
	result := jsonCall(t, f, args, 0)
	equal(t, result.Status, "ok")
	equal(t, result.Meta["complete"], true)
	equal(t, at(result.Meta, "counts", "childProcesses"), 5)
	graph := axi.Obj(result.Data["graph"])
	equal(t, graph["complete"], true)
	nodes := objects(graph["nodes"])
	equal(t, []any{at(nodes[0], "run", "id"), at(nodes[1], "run", "id")}, []string{root, child})
	equal(t, graph["edges"], []axi.Object{{"fromRunId": root, "toRunId": child, "kind": "snapshot"}})
	equal(t, graph["cycles"], []any{})
	for i, node := range nodes {
		equal(t, node["dependencyCount"], 1-i)
		equal(t, node["observedDependencies"], 1-i)
		equal(t, node["expansion"], "complete")
	}
	toonValue := decode(t, wire(t, f, args, 0), "toon")
	equal(t, toonValue.Data, result.Data)
	zero := jsonCall(t, f, append(append([]string{}, args...), "--depth", "0"), 0)
	equal(t, zero.Status, "partial")
	equal(t, objects(at(zero.Data, "graph", "nodes"))[0]["expansion"], "depth_limit")
	equal(t, objects(at(zero.Data, "graph", "nodes"))[0]["dependencyCount"], nil)
	equal(t, at(zero.Meta, "counts", "childProcesses"), 2)
	cap := jsonCall(t, f, append(append([]string{}, args...), "--max-nodes", "1", "--require-complete", "--no-hints"), 1)
	equal(t, cap.Status, "partial")
	equal(t, len(objects(at(cap.Data, "graph", "nodes"))), 1)
	equal(t, len(objects(at(cap.Data, "graph", "edges"))), 0)
	equal(t, objects(at(cap.Data, "graph", "nodes"))[0]["expansion"], "node_limit")
	equal(t, at(cap.Data, "selection", "omittedTargets"), 1)
	require(t, cap.Next == nil, "no-hints emitted next")
	denied := jsonCall(t, f, []string{"run", "tree", axi.Str(f.Contract.Fixture, "deniedRunId")}, 1)
	equal(t, denied.Error.Code, "PERMISSION_DENIED")
}
func TestLiveFailureReportsRunBoundSourcesAndSuccessShortCircuit(t *testing.T) {
	f := fixture(t)
	p := f.Contract.Fixture
	result := jsonCall(t, f, []string{"run", "failure", axi.Str(p, "failedRunId")}, 0)
	equal(t, result.Status, "partial")
	equal(t, result.Meta["complete"], false)
	equal(t, result.Data["assessment"], "failure_observed")
	equal(t, at(result.Data, "run", "id"), p["failedRunId"])
	equal(t, at(result.Data, "graph", "complete"), true)
	equal(t, at(result.Data, "selection", "diagnosedRunIds"), []string{axi.Str(p, "failedRunId")})
	require(t, integer(at(result.Meta, "counts", "childProcesses")) <= 24, "failure process budget exceeded")
	findings, sources := objects(result.Data["findings"]), objects(result.Data["sources"])
	found := false
	for _, finding := range findings {
		if axi.Str(finding, "kind") == "failed_test" {
			found = true
			equal(t, finding["claim"], "observation")
			evidence := objects(finding["evidence"])[0]
			equal(t, evidence["itemId"], "build:(id:1),id:2000000000")
			equal(t, evidence["sourceRef"], "tests:1")
		}
		for _, evidence := range objects(finding["evidence"]) {
			retrieve := axi.Obj(evidence["retrieve"])
			parseActions(t, []axi.Object{retrieve})
			argv := axi.Strings(retrieve["argv"])
			require(t, contains(argv, "--project"), "evidence action omitted project")
			matched := false
			for _, source := range sources {
				if source["id"] == evidence["sourceRef"] && source["runId"] == evidence["runId"] {
					matched = true
				}
			}
			require(t, matched, "evidence points outside source")
		}
	}
	require(t, found, "failed test finding absent")
	normal, muted := false, false
	for _, source := range sources {
		if source["id"] == "tests:1" {
			normal = true
			equal(t, source["total"], nil)
		}
		if source["id"] == "tests:1:muted" {
			muted = true
			equal(t, source["returned"], 0)
		}
	}
	require(t, normal && muted, "independent test sources missing")
	parseActions(t, result.Next)
	toonValue := decode(t, wire(t, f, []string{"run", "failure", axi.Str(p, "failedRunId")}, 0), "toon")
	equal(t, ids(objects(toonValue.Data["findings"]), "id"), ids(findings, "id"))
	success := jsonCall(t, f, []string{"run", "failure", axi.Str(p, "greenRunId")}, 0)
	equal(t, success.Status, "ok")
	equal(t, at(success.Meta, "counts", "childProcesses"), 2)
	equal(t, success.Data["assessment"], "not_failed")
	equal(t, objects(success.Data["findings"]), []axi.Object{})
	equal(t, objects(at(success.Data, "graph", "nodes"))[0]["expansion"], "not_requested")
	equal(t, at(success.Data, "graph", "complete"), false)
	strict := jsonCall(t, f, []string{"run", "failure", axi.Str(p, "failedRunId"), "--require-complete", "--no-hints"}, 1)
	equal(t, strict.Status, "partial")
	require(t, strict.Next == nil, "strict no-hints emitted next")
	denied := jsonCall(t, f, []string{"run", "failure", axi.Str(p, "deniedRunId")}, 1)
	equal(t, denied.Error.Code, "PERMISSION_DENIED")
}
