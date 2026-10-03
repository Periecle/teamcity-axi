package axi

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

type commandFixtureReader struct {
	mu        sync.Mutex
	requests  []ReadRequest
	responses map[string]ReadResult
	sequence  map[string][]ReadResult
}

func (f *commandFixtureReader) Read(_ context.Context, q ReadRequest, _ Budget) ReadResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, q)
	if list := f.sequence[q.Kind]; len(list) > 0 {
		r := list[0]
		f.sequence[q.Kind] = list[1:]
		return r
	}
	if r, ok := f.responses[q.Kind+":"+q.ID]; ok {
		return r
	}
	if r, ok := f.responses[q.Kind]; ok {
		return r
	}
	return ReadResult{State: "unavailable", Error: NewError("NOT_FOUND", "Fixture read unavailable", 1)}
}

func commandAvailable(value Object) ReadResult {
	return ReadResult{State: "available", Value: value, Provenance: Provenance{ObservedAt: "2026-10-03T10:00:00.000Z", ProjectID: "Payments"}}
}

func commandFixture(t *testing.T) (*commandFixtureReader, ExecutionContext) {
	t.Helper()
	clean := false
	run := Object{"id": "482193", "jobId": "Payments_Build", "state": "finished", "result": "failure", "branch": "feature/refund", "personal": false, "revisions": []Object{{"revision": "abc", "vcsRootId": "Payments_Git"}}}
	reader := &commandFixtureReader{responses: map[string]ReadResult{"run.detail": commandAvailable(run), "job.detail": commandAvailable(Object{"id": "Payments_Build", "projectId": "Payments", "name": "Build", "paused": false}), "project.detail": commandAvailable(Object{"id": "Payments", "name": "Payments", "parentProjectId": nil}), "identity.current": commandAvailable(Object{"fingerprint": "sha256:identity"}), "server.detail": commandAvailable(Object{"version": "mock", "buildNumber": "42"}), "dependencies.count": commandAvailable(Object{"count": 0})}, sequence: map[string][]ReadResult{}}
	for _, kind := range []string{"problems.page", "tests.page", "changes.page", "dependencies.page", "job.page", "queue.page", "agents.page"} {
		reader.responses[kind] = commandAvailable(Object{"items": []Object{}, "providerReturned": 0, "position": nil, "hasMore": nil, "limitations": []Limitation{}})
	}
	reader.responses["run.page"] = commandAvailable(Object{"runs": []Object{run}, "providerReturned": 1, "position": nil, "hasMore": false, "limitations": []Limitation{}})
	reader.responses["log.tail"] = commandAvailable(Object{"runId": "482193", "messages": []Object{}, "providerReturned": 0, "truncated": false})
	reader.responses["agent.detail"] = commandAvailable(Object{"id": "7", "name": "agent", "connected": true, "enabled": true, "authorized": true, "activeRun": nil, "activeRunState": "none"})
	original := openCommandSession
	openCommandSession = func(context.Context, ExecutionContext, string) (*ReadSession, error) {
		return &ReadSession{Reader: reader, NativeVersion: "1.5.0", MaxChildProcesses: 24, Binary: "teamcity"}, nil
	}
	t.Cleanup(func() { openCommandSession = original })
	ec := ExecutionContext{Deadline: time.Now().UnixMilli() + 30000, Server: "work", ServerURL: "https://ci.example", Project: "Payments", Job: "Payments_Build", Jobs: []string{"Payments_Build"}, Head: "abc", Revision: "abc", VCSRootID: "Payments_Git", Dirty: &clean, Config: &UserConfig{Servers: map[string]ServerConfig{"work": {AllowedProjects: []string{"Payments"}}}}}
	return reader, ec
}

func commandParsed(name string, flags Object) Parsed {
	if flags == nil {
		flags = Object{}
	}
	return Parsed{Descriptor: Descriptor{Name: name}, Positional: "482193", Flags: flags}
}

func TestCommandRunViewProjectionAndUnicodePreview(t *testing.T) {
	f, ec := commandFixture(t)
	r := f.responses["run.detail"]
	r.Value["statusText"] = strings.Repeat("😀", 1300)
	f.responses["run.detail"] = r
	output, err := ViewRun(context.Background(), commandParsed("run.view", nil), ec)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(Str(Obj(output.Data["run"]), "statusText"))) != 1200 || !Bool(output.Meta, "truncated") || len(output.Next) != 1 {
		t.Fatalf("preview: %#v", output)
	}
	parsed := commandParsed("run.view", Object{"fields": "number", "no-hints": true})
	output, err = ViewRun(context.Background(), parsed, ec)
	if err != nil {
		t.Fatal(err)
	}
	run := Obj(output.Data["run"])
	if len(run) != 4 || len(output.Next) != 0 {
		t.Fatalf("mandatory projection: %#v", output)
	}
	r = f.responses["run.detail"]
	r.Value["result"] = "unknown"
	r.Value["rawStatus"] = "mystery"
	f.responses["run.detail"] = r
	output, err = ViewRun(context.Background(), parsed, ec)
	if err != nil || Str(Obj(output.Data["run"]), "rawStatus") != "mystery" {
		t.Fatalf("unknown status lost: %#v %v", output, err)
	}
}

func TestCommandExactRunScopeRejectsMismatch(t *testing.T) {
	_, ec := commandFixture(t)
	for _, name := range []string{"run.view", "run.problems", "run.tests", "run.log", "run.changes", "run.tree", "run.failure"} {
		_, _, err := Dispatch(context.Background(), commandParsed(name, Object{"job": "Other"}), ec)
		if err == nil || AsDomainError(err).Code != "CONTEXT_MISMATCH" {
			t.Fatalf("%s mismatch: %v", name, err)
		}
	}
}

func TestCommandRunListExactTotalsAndUnknownCoverage(t *testing.T) {
	f, ec := commandFixture(t)
	parsed := commandParsed("run.list", Object{"all-branches": true})
	output, err := ListRuns(context.Background(), parsed, ec)
	if err != nil {
		t.Fatal(err)
	}
	page := Obj(output.Data["page"])
	if page["totalKind"] != "exact" || Int(page, "total") != 1 {
		t.Fatalf("exact page: %#v", page)
	}
	r := f.responses["run.page"]
	r.Value["hasMore"] = nil
	f.responses["run.page"] = r
	output, err = ListRuns(context.Background(), parsed, ec)
	if err != nil {
		t.Fatal(err)
	}
	if Obj(output.Data["page"])["total"] != nil || Str(Obj(output.Data["selection"]), "exhaustionBasis") != "unverified" {
		t.Fatal("unknown exhaustion promoted")
	}
	r.Value["limitations"] = []Limitation{{Code: "RUN_METADATA_UNAVAILABLE", Message: "partial", Source: "run"}}
	f.responses["run.page"] = r
	output, code, err := Dispatch(context.Background(), commandParsed("run.list", Object{"require-complete": true}), ec)
	if err != nil || output.Status != "partial" || code != 1 {
		t.Fatalf("strict partial: %#v %d %v", output, code, err)
	}
}

func TestCommandRunListBindsContinuationAndLiteralBranch(t *testing.T) {
	f, ec := commandFixture(t)
	ec.Branch = "feature/refund"
	ec.BranchDefined = true
	r := f.responses["run.page"]
	r.Value["position"] = 1
	r.Value["hasMore"] = true
	f.responses["run.page"] = r
	output, err := ListRuns(context.Background(), commandParsed("run.list", Object{"limit": 1}), ec)
	if err != nil {
		t.Fatal(err)
	}
	token := Str(Obj(output.Data["page"]), "cursor")
	if token == "" || len(output.Next) != 1 {
		t.Fatal("continuation missing")
	}
	seen := false
	for _, q := range f.requests {
		if q.Kind == "run.page" {
			seen = q.BranchSet && q.Branch == "feature/refund"
		}
	}
	if !seen {
		t.Fatal("literal branch lost")
	}
	_, err = ListRuns(context.Background(), commandParsed("run.list", Object{"limit": 1, "cursor": token, "result": "success"}), ec)
	if err == nil || AsDomainError(err).Code != "USAGE_ERROR" {
		t.Fatalf("cursor rebound: %v", err)
	}
}

func TestCommandCollectionsKeepUnknownTotalsAndSafeHints(t *testing.T) {
	f, ec := commandFixture(t)
	for _, name := range []string{"job.list", "queue.list", "agent.list"} {
		output, _, err := Dispatch(context.Background(), commandParsed(name, Object{"pool": "1"}), ec)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		page := Obj(output.Data["page"])
		if page["total"] != nil || page["totalKind"] != "unknown" || page["hasMore"] != nil {
			t.Fatalf("%s totals: %#v", name, page)
		}
	}
	f.responses["queue.page"] = commandAvailable(Object{"items": []Object{{"id": "9", "jobId": "Payments_Build"}}, "providerReturned": 1, "position": nil, "hasMore": nil, "limitations": []Limitation{}})
	output, err := ListQueue(context.Background(), commandParsed("queue.list", nil), ec)
	if err != nil || len(output.Next) != 1 {
		t.Fatalf("queue identity hint: %#v %v", output, err)
	}
	parsed := commandParsed("job.view", nil)
	parsed.Positional = "Payments_Build"
	output, err = ViewJob(context.Background(), parsed, ec)
	if err != nil || Str(Obj(output.Data["job"]), "id") != "Payments_Build" || len(output.Next) != 1 {
		t.Fatalf("job metadata: %#v %v", output, err)
	}
}

func TestCommandAgentActiveRunAdmission(t *testing.T) {
	f, ec := commandFixture(t)
	f.responses["agent.detail"] = commandAvailable(Object{"id": "7", "activeRun": Object{"id": "9", "jobId": "Private_Build", "projectId": "Private"}, "activeRunState": "reported"})
	f.responses["project.detail:Private"] = commandAvailable(Object{"id": "Private", "parentProjectId": nil})
	parsed := commandParsed("agent.view", nil)
	parsed.Positional = "7"
	output, err := ReadAgents(context.Background(), parsed, ec)
	if err != nil {
		t.Fatal(err)
	}
	agent := Obj(output.Data["agent"])
	if agent["activeRun"] != nil || Str(agent, "activeRunState") != "unavailable" || output.Status != "partial" || len(output.Next) > 0 {
		t.Fatalf("unadmitted pointer: %#v", output)
	}
}

func TestCommandEvidenceTextPreviewAndHardBound(t *testing.T) {
	f, ec := commandFixture(t)
	f.responses["tests.page"] = commandAvailable(Object{"items": []Object{{"id": "build:(id:482193),id:1", "runId": "482193", "name": "fails", "result": "failure", "muted": false, "ignored": false, "details": strings.Repeat("界", 33000)}}, "providerReturned": 1, "position": nil, "hasMore": nil, "limitations": []Limitation{}})
	output, err := ReadEvidence(context.Background(), commandParsed("run.tests", Object{"failed": true}), ec)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(Str(Objects(output.Data["tests"])[0], "details"))) != 2000 || !Bool(output.Meta, "truncated") || output.Status != "ok" || len(output.Next) != 1 {
		t.Fatalf("preview: %#v", output)
	}
	output, err = ReadEvidence(context.Background(), commandParsed("run.tests", Object{"full": true}), ec)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(Str(Objects(output.Data["tests"])[0], "details"))) != 32768 || output.Status != "partial" {
		t.Fatal("full text ceiling did not mark partial")
	}
	for _, q := range f.requests {
		if q.Kind == "tests.page" && q.Failed {
			if !q.MutedSet || q.Muted {
				t.Fatal("unmuted failure filtering lost")
			}
		}
	}
}

func TestCommandSelectedOccurrenceIdentityAndProjection(t *testing.T) {
	f, ec := commandFixture(t)
	f.responses["test.detail"] = commandAvailable(Object{"id": "build:(id:482193),id:1", "runId": "482193", "name": "test", "result": "unknown", "rawStatus": "NEW", "muted": nil, "ignored": nil, "details": "omitted"})
	parsed := commandParsed("run.tests", Object{"test": "build:(id:482193),id:1", "fields": "durationMs"})
	output, err := ReadEvidence(context.Background(), parsed, ec)
	if err != nil {
		t.Fatal(err)
	}
	item := Objects(output.Data["tests"])[0]
	if Str(item, "rawStatus") != "NEW" || item["details"] != nil || Obj(output.Data["page"])["total"] != 1 || Obj(output.Data["page"])["hasMore"] != false {
		t.Fatalf("exact selection: %#v", output)
	}
	_, err = ReadEvidence(context.Background(), commandParsed("run.tests", Object{"test": "build:(id:1),id:1"}), ec)
	if err == nil || AsDomainError(err).Code != "CONTEXT_MISMATCH" {
		t.Fatalf("unbound occurrence accepted: %v", err)
	}
}

func TestCommandFailedLogRetainsIndependentSources(t *testing.T) {
	f, ec := commandFixture(t)
	f.responses["problems.page"] = ReadResult{State: "unavailable", Error: NewError("PERMISSION_DENIED", "denied", 1)}
	f.responses["log.tail"] = commandAvailable(Object{"runId": "482193", "messages": []Object{{"id": "1", "text": "error alpha"}, {"id": "2", "text": "beta"}}, "providerReturned": 3, "truncated": true})
	output, err := ReadEvidence(context.Background(), commandParsed("run.log", Object{"failed": true, "contains": "alpha"}), ec)
	if err != nil {
		t.Fatal(err)
	}
	sources := Obj(output.Data["sources"])
	if Str(Obj(sources["problems"]), "availability") != "unavailable" || Str(Obj(sources["tests"]), "availability") != "available" || len(Objects(output.Data["messages"])) != 1 || output.Status != "partial" || !Bool(output.Meta, "truncated") {
		t.Fatalf("independent evidence: %#v", output)
	}
	if Int(Obj(output.Data["window"]), "omittedProviderMessages") != 1 {
		t.Fatal("tail omissions lost")
	}
}

func TestCommandLogCapabilityErrorsRemainFailures(t *testing.T) {
	f, ec := commandFixture(t)
	for _, code := range []string{"PERMISSION_DENIED", "DEADLINE_EXCEEDED", "CONTEXT_MISMATCH", "UPSTREAM_SCHEMA_MISMATCH"} {
		f.responses["log.tail"] = ReadResult{State: "unavailable", Error: NewError(code, "unavailable", 1)}
		_, err := ReadEvidence(context.Background(), commandParsed("run.log", nil), ec)
		want := code
		if code == "PERMISSION_DENIED" {
			want = "CAPABILITY_UNAVAILABLE"
		}
		if err == nil || AsDomainError(err).Code != want {
			t.Fatalf("%s: %v", code, err)
		}
	}
}

func TestCommandChangesRootBindingAndPreview(t *testing.T) {
	f, ec := commandFixture(t)
	change := Object{"id": "1", "version": "abc", "vcsRootId": "Payments_Git", "message": "subject\nbody", "files": []Object{}, "fileCoverage": Object{"omitted": 2}}
	f.responses["changes.page"] = commandAvailable(Object{"items": []Object{change}, "providerReturned": 1, "position": nil, "hasMore": nil, "limitations": []Limitation{}})
	output, err := ReadChanges(context.Background(), commandParsed("run.changes", Object{"fields": "files"}), ec)
	if err != nil {
		t.Fatal(err)
	}
	if Str(Objects(output.Data["changes"])[0], "message") != "subject" || !Bool(output.Meta, "truncated") || len(output.Next) != 1 || Objects(output.Data["changes"])[0]["fileCoverage"] == nil {
		t.Fatalf("changes preview: %#v", output)
	}
	change["vcsRootId"] = "Other"
	_, err = ReadChanges(context.Background(), commandParsed("run.changes", nil), ec)
	if err == nil || AsDomainError(err).Code != "CONTEXT_MISMATCH" {
		t.Fatal("wrong root accepted")
	}
}

func TestCommandWatchRetainsLastObservationAndCheckOutcome(t *testing.T) {
	f, ec := commandFixture(t)
	running := commandAvailable(Object{"id": "482193", "jobId": "Payments_Build", "state": "running", "result": "unknown"})
	f.sequence["run.detail"] = []ReadResult{running, {State: "unavailable", Error: NewError("NOT_FOUND", "gone", 1)}}
	output, code, err := Dispatch(context.Background(), commandParsed("run.watch", Object{"interval": 1, "check": true}), ec)
	if err != nil {
		t.Fatal(err)
	}
	if output.Data["outcome"] != "vanished" || Str(Obj(output.Data["run"]), "state") != "running" || Int(Obj(output.Data["polling"]), "count") != 2 || code != 1 {
		t.Fatalf("retained observation: %#v %d", output, code)
	}
	f.responses["run.detail"] = commandAvailable(Object{"id": "482193", "jobId": "Payments_Build", "state": "finished", "result": "success"})
	output, code, err = Dispatch(context.Background(), commandParsed("run.watch", Object{"check": true}), ec)
	if err != nil || code != 0 || !Bool(Obj(output.Data["check"]), "passed") {
		t.Fatalf("finished check: %#v %d %v", output, code, err)
	}
}

func TestCommandStatusRequiresCleanExactCheckout(t *testing.T) {
	f, ec := commandFixture(t)
	run := f.responses["run.detail"].Value
	run["result"] = "success"
	f.responses["status.snapshot"] = commandAvailable(Object{"snapshots": []Object{{"job": f.responses["job.detail"].Value, "page": Object{"runs": []Object{run}, "providerReturned": 1, "limitations": []Limitation{}}}}})
	output, code, err := Dispatch(context.Background(), commandParsed("status", Object{"check": true}), ec)
	if err != nil || output.Data["assessment"] != "passed" || code != 0 {
		t.Fatalf("exact checkout: %#v %d %v", output, code, err)
	}
	dirty := true
	ec.Dirty = &dirty
	output, code, err = Dispatch(context.Background(), commandParsed("status", Object{"check": true}), ec)
	if err != nil || output.Data["assessment"] != "unverified" || code != 1 || output.Status != "partial" {
		t.Fatalf("dirty checkout certified: %#v %d %v", output, code, err)
	}
}

func TestCommandDoctorDoesNotCertifyUnprobedCapabilities(t *testing.T) {
	_, ec := commandFixture(t)
	output, err := Diagnose(context.Background(), commandParsed("doctor", nil), ec)
	if err != nil {
		t.Fatal(err)
	}
	if output.Status != "partial" || Bool(output.Data, "liveCertified") {
		t.Fatal("doctor overclaimed capability")
	}
	capabilities := Objects(output.Data["capabilities"])
	if len(capabilities) != 9 {
		t.Fatal("capability inventory lost")
	}
	output, err = Diagnose(context.Background(), commandParsed("doctor", Object{"offline": true}), ExecutionContext{})
	if err != nil || Str(Obj(output.Data["authentication"]), "state") != "not_checked" || output.Status != "ok" {
		t.Fatalf("offline diagnosis: %#v %v", output, err)
	}
	output, err = Diagnose(context.Background(), commandParsed("context.show", Object{"verify": true}), ec)
	if err != nil || Str(Obj(output.Data["verification"]), "authentication") != "authenticated" {
		t.Fatalf("context verification: %#v %v", output, err)
	}
	local := LocalContext(ec)
	if Bool(Obj(local.Data["verification"]), "requested") {
		t.Fatal("local context claims remote verification")
	}
}

func TestCommandGraphFinishedRootAndFailureAssessment(t *testing.T) {
	_, ec := commandFixture(t)
	output, err := ReadTree(context.Background(), commandParsed("run.tree", nil), ec)
	if err != nil {
		t.Fatal(err)
	}
	graph := Obj(output.Data["graph"])
	if len(Objects(graph["nodes"])) != 1 || !Bool(graph, "complete") {
		t.Fatalf("root graph: %#v", output)
	}
	output, err = ReadFailure(context.Background(), commandParsed("run.failure", nil), ec)
	if err != nil {
		t.Fatal(err)
	}
	if Str(Obj(output.Data["run"]), "id") != "482193" || output.Data["assessment"] != "failure_observed" {
		t.Fatalf("investigation: %#v", output)
	}
}

func TestCommandFailureHintsOnlyExpandTruncatedRetainedEvidence(t *testing.T) {
	f, ec := commandFixture(t)
	problemID, testID := "build:(id:482193),problem:(id:1)", "build:(id:482193),id:1"
	problem := Object{"id": problemID, "runId": "482193", "type": "SYNTHETIC", "description": "Explicit failure"}
	test := Object{"id": testID, "runId": "482193", "name": "check", "result": "failure", "muted": false, "ignored": false, "details": "Retained detail"}
	f.responses["problems.page"] = commandAvailable(Object{"items": []Object{problem}, "providerReturned": 1, "position": nil, "hasMore": nil, "limitations": []Limitation{}})
	f.responses["tests.page"] = commandAvailable(Object{"items": []Object{test}, "providerReturned": 1, "position": nil, "hasMore": nil, "limitations": []Limitation{}})
	parsed := commandParsed("run.failure", Object{"depth": 0, "max-diagnosed-runs": 1})
	output, err := ReadFailure(context.Background(), parsed, ec)
	if err != nil || len(output.Next) != 0 || len(Objects(output.Data["findings"])) != 2 {
		t.Fatalf("retained evidence prompted a redundant read: %#v, %v", output, err)
	}
	for _, finding := range Objects(output.Data["findings"]) {
		if len(Strings(Obj(Objects(finding["evidence"])[0]["retrieve"])["argv"])) == 0 {
			t.Fatal("optional exact retrieval was removed from retained evidence")
		}
	}
	test["name"] = strings.Repeat("n", 1500)
	output, err = ReadFailure(context.Background(), parsed, ec)
	if err != nil || !Bool(output.Meta, "truncated") || len(output.Next) != 0 {
		t.Fatalf("bounded summary prompted expansion of an intact excerpt: %#v, %v", output, err)
	}
	test["details"] = strings.Repeat("x", 3000)
	output, err = ReadFailure(context.Background(), parsed, ec)
	if err != nil || !Bool(output.Meta, "truncated") || len(output.Next) != 1 {
		t.Fatalf("truncated detail lost its recovery read: %#v, %v", output, err)
	}
	argv := Strings(output.Next[0]["argv"])
	if _, err := Parse(argv[1:]); err != nil || !containsString(argv, testID) || containsString(argv, problemID) {
		t.Fatalf("recovery read selected the first finding instead of truncated evidence: %v, %v", argv, err)
	}
	parsed.Flags["no-hints"] = true
	output, err = ReadFailure(context.Background(), parsed, ec)
	if err != nil || len(output.Next) != 0 || len(Objects(output.Data["findings"])) != 2 {
		t.Fatalf("no-hints changed retained evidence: %#v, %v", output, err)
	}
}

func TestCommandTreeReservesFinalObservationAndPreservesChangedState(t *testing.T) {
	f, ec := commandFixture(t)
	running := commandAvailable(Object{"id": "482193", "jobId": "Payments_Build", "state": "running", "result": "unknown"})
	finished := commandAvailable(Object{"id": "482193", "jobId": "Payments_Build", "state": "finished", "result": "failure"})
	f.sequence["run.detail"] = []ReadResult{running, finished}
	output, err := ReadTree(context.Background(), commandParsed("run.tree", nil), ec)
	if err != nil {
		t.Fatal(err)
	}
	if Str(Obj(output.Data["run"]), "state") != "finished" || output.Status != "partial" || Bool(Obj(output.Data["graph"]), "complete") {
		t.Fatalf("changed root: %#v", output)
	}
}
