package axi

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestAdapterRecordedFiveJobStatusQueuedLifecycle(t *testing.T) {
	dto := adapterBody(t, "status-five-outcomes")
	jobIDs := []string{}
	for _, job := range Objects(dto["buildType"]) {
		jobIDs = append(jobIDs, Str(job, "id"))
	}
	snapshots, e := NormalizeStatus(dto, ReadRequest{JobIDs: jobIDs}, adapterTestServer, nil)
	adapterOK(t, e)
	actual, queued := []string{}, []string{}
	for _, snapshot := range snapshots {
		job := Obj(snapshot["job"])
		actual = append(actual, Str(job, "id"))
		page := Obj(snapshot["page"])
		runs := Objects(page["runs"])
		if len(runs) > 0 && Str(runs[0], "state") == "queued" {
			queued = append(queued, Str(runs[0], "id"))
		}
		if Str(job, "id") == "AxiContract_Vcs" {
			adapterWant(t, page["hasMore"], nil)
			if len(Objects(runs[0]["revisions"])) != 1 {
				t.Fatal(runs)
			}
			selected := AssessStatusJob(Str(job, "id"), snapshot, Str(Objects(runs[0]["revisions"])[0], "revision"), "AxiContract_Git")
			adapterWant(t, selected["match"], "exact")
			adapterWant(t, selected["assessment"], "passed")
		}
	}
	adapterWant(t, actual, jobIDs)
	adapterWant(t, queued, []string{"10", "11"})
}
func TestAdapterStatusProductionPathsAndEmptyMissingJobs(t *testing.T) {
	for _, entry := range []struct{ name, branch string }{{"status-one-outcomes", ""}, {"status-branch-outcomes", "feature/status,project:AxiDenied"}} {
		q := ReadRequest{JobIDs: []string{"AxiContract_Vcs"}, Branch: entry.branch}
		path, e := StatusRequest(q)
		adapterOK(t, e)
		adapterWant(t, path, adapterFixture(t, false).Records[entry.name].Args[1])
		if entry.branch != "" {
			snapshots, e := NormalizeStatus(adapterBody(t, entry.name), q, adapterTestServer, nil)
			adapterOK(t, e)
			adapterWant(t, len(Objects(Obj(snapshots[0]["page"])["runs"])), 0)
		}
	}
	for _, name := range []string{"status-missing-job-outcomes", "status-denied-outcomes"} {
		snapshots, e := NormalizeStatus(adapterBody(t, name), ReadRequest{JobIDs: []string{"AxiContract_Missing"}}, adapterTestServer, nil)
		adapterOK(t, e)
		adapterWant(t, snapshots, []Object{})
	}
}
func adapterStatusDTO(rows []any) Object {
	return Object{"count": 1, "buildType": []any{Object{"id": "Payments_Build", "name": "Build", "projectId": "Payments", "paused": false, "builds": Object{"count": len(rows), "build": rows}}}}
}
func TestAdapterStatusForeignDuplicatesOrderRootAndBoundedCollections(t *testing.T) {
	q := ReadRequest{JobIDs: []string{"Payments_Build"}, Branch: "feature/refund"}
	many := []any{}
	for i := 0; i < 21; i++ {
		many = append(many, adapterPatch(adapterBaseRun(), Object{"id": 482193 - i}))
	}
	revision := adapterArray(Obj(adapterBaseRun()["revisions"])["revision"])
	missing := adapterStatusDTO([]any{})
	delete(Objects(missing["buildType"])[0], "builds")
	for _, dto := range []Object{adapterStatusDTO([]any{adapterPatch(adapterBaseRun(), Object{"buildTypeId": "Foreign"})}), adapterStatusDTO([]any{adapterPatch(adapterBaseRun(), Object{"buildType": Object{"id": "Payments_Build", "projectId": "Foreign"}})}), adapterStatusDTO([]any{adapterPatch(adapterBaseRun(), Object{"branchName": "Foreign"})}), adapterStatusDTO([]any{adapterBaseRun(), adapterBaseRun()}), adapterStatusDTO([]any{adapterPatch(adapterBaseRun(), Object{"id": 482192}), adapterBaseRun()}), adapterStatusDTO([]any{adapterPatch(adapterBaseRun(), Object{"revisions": Object{"revision": append(append([]any{}, revision...), revision...)}})}), adapterStatusDTO(many), Object{"count": 2, "buildType": []any{Objects(adapterStatusDTO(nil)["buildType"])[0], Objects(adapterStatusDTO(nil)["buildType"])[0]}}, missing} {
		_, e := NormalizeStatus(dto, q, adapterTestServer, nil)
		adapterError(t, e, "")
	}
}
func TestAdapterStatusSelectorsRejectInvalidAndEncodeInjection(t *testing.T) {
	_, e := StatusRequest(ReadRequest{JobIDs: []string{"Job"}, Branch: strings.Repeat("b", 300)})
	adapterOK(t, e)
	for _, q := range []ReadRequest{{}, {JobIDs: []string{"A", "A"}}, {JobIDs: []string{"A", "B", "C", "D", "E", "F"}}, {JobIDs: []string{"A\u202e"}}, {JobIDs: []string{"A"}, Branch: string([]byte{0xed, 0xa0, 0x80})}} {
		_, e := StatusRequest(q)
		adapterError(t, e, "USAGE_ERROR")
	}
	path, e := StatusRequest(ReadRequest{JobIDs: []string{"job,project:attacker"}, Branch: "x,state:finished"})
	adapterOK(t, e)
	u, _ := url.Parse(path)
	if strings.Contains(u.Query().Get("locator"), "attacker") || strings.Contains(u.Query().Get("fields"), "x,state:finished") {
		t.Fatal(path)
	}
}
func TestAdapterStatusReaderRecordedInputAndPermissionErrors(t *testing.T) {
	fake := &adapterFakeTransport{execute: func(Operation) (Captured, error) { return adapterCapture(t, "status-one-outcomes", false), nil }}
	reader := NewNativeReader(fake, adapterTestServer, nil)
	q := ReadRequest{Kind: "status.snapshot", JobIDs: []string{"AxiContract_Vcs"}}
	read := reader.Read(context.Background(), q, adapterBudget())
	adapterWant(t, read.State, "available")
	adapterWant(t, read.Provenance.Operation, "status.snapshot")
	adapterWant(t, fake.calls[0].Path, adapterFixture(t, false).Records["status-one-outcomes"].Args[1])
	fake.execute = func(Operation) (Captured, error) { return adapterCapture(t, "denied", false), nil }
	read = reader.Read(context.Background(), q, adapterBudget())
	adapterWant(t, read.Error.Code, "PERMISSION_DENIED")
}
func adapterExhaustionReader(t *testing.T, capture Captured, serverError *DomainError, other bool) (Reader, *adapterFakeTransport) {
	t.Helper()
	fake := &adapterFakeTransport{execute: func(op Operation) (Captured, error) {
		if strings.HasPrefix(op.Path, "/app/rest/server") {
			if serverError != nil {
				return Captured{}, serverError
			}
			if other {
				return adapterRawJSON(Object{"version": "2026.2 (build 238925)", "buildNumber": "238925"}), nil
			}
			return adapterCapture(t, "server", false), nil
		}
		return capture, nil
	}}
	return NewNativeReader(fake, adapterTestServer, nil), fake
}
func TestAdapterVerifiedServerOnlyCertifiesExhaustedEmptyRunPage(t *testing.T) {
	fixture := adapterFixture(t, false)
	p := Obj(fixture.Fixture["exhaustion"])
	q := ReadRequest{Kind: "run.page", ProjectID: Str(fixture.Fixture, "projectId"), JobID: Str(p, "emptyFinishedJobId"), State: "finished", Count: 20, ScanLimit: 5000, Window: &TimeWindow{Since: Str(p, "since"), Until: Str(p, "until")}}
	capture := adapterCapture(t, "exhaustion-empty-finished-job", false)
	reader, fake := adapterExhaustionReader(t, capture, nil, false)
	read := reader.Read(context.Background(), q, adapterBudget())
	adapterWant(t, fake.calls[0].Path, fixture.Records["exhaustion-empty-finished-job"].Args[1])
	adapterWant(t, len(fake.calls), 2)
	adapterWant(t, read.State, "available")
	adapterWant(t, read.Value["runs"], []Object{})
	adapterWant(t, read.Value["hasMore"], false)
	adapterWant(t, len(read.Value["limitations"].([]Limitation)), 0)
	for _, e := range []*DomainError{nil, NewError("PERMISSION_DENIED", "Probe denied", 1), NewError("PROCESS_BUDGET_EXCEEDED", "No reserved process", 1), NewError("CAPTURE_LIMIT_EXCEEDED", "Probe too large", 1), NewError("DEADLINE_EXCEEDED", "Probe deadline", 1)} {
		reader, _ := adapterExhaustionReader(t, capture, e, e == nil)
		read := reader.Read(context.Background(), q, adapterBudget())
		adapterWant(t, read.State, "available")
		adapterWant(t, read.Value["hasMore"], nil)
		adapterWant(t, read.Value["runs"], []Object{})
		adapterNotes(t, read.Value, "SCAN_COVERAGE_UNKNOWN")
	}
	reader, _ = adapterExhaustionReader(t, capture, NewError("INTERRUPTED", "Stopped", 1), false)
	read = reader.Read(context.Background(), q, adapterBudget())
	adapterWant(t, read.State, "unavailable")
	adapterWant(t, read.Error.Code, "INTERRUPTED")
}
func TestAdapterCappedRunPagesCannotCertifyExhaustionOrIncreaseBudget(t *testing.T) {
	fixture := adapterFixture(t, false)
	for _, entry := range []struct {
		name, result string
		count        int
	}{{"exhaustion-cap-empty", "failure", 0}, {"exhaustion-cap-positive", "", 1}} {
		q := ReadRequest{Kind: "run.page", ProjectID: Str(fixture.Fixture, "projectId"), State: "finished", Count: 20, ScanLimit: 1, Result: entry.result}
		reader, fake := adapterExhaustionReader(t, adapterCapture(t, entry.name, false), nil, false)
		read := reader.Read(context.Background(), q, adapterBudget())
		adapterWant(t, fake.calls[0].Path, fixture.Records[entry.name].Args[1])
		adapterWant(t, len(fake.calls), 1)
		adapterWant(t, read.State, "available")
		adapterWant(t, read.Value["hasMore"], nil)
		adapterWant(t, read.Value["position"], nil)
		adapterNotes(t, read.Value, "UNSAFE_CONTINUATION")
		adapterWant(t, len(Objects(read.Value["runs"])), entry.count)
	}
}
func TestAdapterMalformedOrMissingFullPageContinuationRetainsUncertainty(t *testing.T) {
	dto := adapterBody(t, "exhaustion-cap-positive")
	for _, entry := range []struct {
		body  Object
		count int
	}{{adapterPatch(dto, Object{"nextHref": nil}), 20}, {adapterWithout(dto, "nextHref"), 1}} {
		q := ReadRequest{Kind: "run.page", ProjectID: Str(adapterFixture(t, false).Fixture, "projectId"), State: "finished", Count: entry.count, ScanLimit: 5000}
		reader, fake := adapterExhaustionReader(t, adapterRawJSON(entry.body), nil, false)
		read := reader.Read(context.Background(), q, adapterBudget())
		adapterWant(t, read.State, "available")
		adapterWant(t, read.Value["hasMore"], nil)
		adapterWant(t, len(Objects(read.Value["runs"])), 1)
		adapterWant(t, len(fake.calls), 1)
	}
}
