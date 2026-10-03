package axi

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestAdapterRecordedJobPageAndUnknownEmptyExhaustion(t *testing.T) {
	q := adapterPageQuery()
	r, e := JobRequest(q)
	adapterOK(t, e)
	u, _ := url.Parse(r.Path)
	adapterWant(t, u.Query().Get("locator"), "project:(id:($base64:QXhpQ29udHJhY3Q)),count:1,start:0,lookupLimit:5000")
	page, e := NormalizeJobPage(adapterBody(t, "bounded-jobs"), q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, Objects(page["items"]), []Object{{"id": "AxiContract_Fail", "name": "AxiContract_Fail", "projectId": "AxiContract", "paused": false}})
	adapterWant(t, page["position"], 1)
	adapterWant(t, page["hasMore"], true)
	adapterWant(t, len(page["limitations"].([]Limitation)), 0)
	q.Start = 4999
	page, e = NormalizeJobPage(adapterBody(t, "bounded-jobs-empty"), q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, page["items"], []Object{})
	adapterWant(t, page["position"], nil)
	adapterWant(t, page["hasMore"], nil)
	adapterNotes(t, page, "SCAN_COVERAGE_UNKNOWN")
}
func TestAdapterJobForeignDuplicateMalformedRowsAndBounds(t *testing.T) {
	q := adapterPageQuery()
	page := adapterBody(t, "bounded-jobs")
	job := Objects(page["buildType"])[0]
	for _, patch := range []Object{{"count": 2}, {"count": 0}, {"buildType": nil}, {"buildType": []any{adapterPatch(job, Object{"projectId": "Forbidden"})}}} {
		_, e := NormalizeJobPage(adapterPatch(page, patch), q, adapterTestServer, nil)
		adapterError(t, e, "")
	}
	q.Count = 2
	_, e := NormalizeJobPage(Object{"count": 2, "buildType": []any{job, job}}, q, adapterTestServer, nil)
	adapterError(t, e, "")
	queries := []ReadRequest{adapterPageQuery(), adapterPageQuery(), adapterPageQuery(), adapterPageQuery(), adapterPageQuery()}
	queries[0].Count = 101
	queries[1].Start = 5000
	queries[2].ScanLimit = 5001
	queries[3].ProjectID = ""
	queries[4].ProjectID = strings.Repeat("x", 257)
	for _, q := range queries {
		_, e := JobRequest(q)
		adapterError(t, e, "USAGE_ERROR")
	}
	for _, patch := range []Object{{"id": nil}, {"name": 42}, {"paused": "false"}, {"paused": nil}, {"projectId": ""}} {
		_, e := NormalizeJob(adapterPatch(job, patch), nil)
		adapterError(t, e, "")
	}
}
func TestAdapterJobUnsafePagingUnknownPausedAndSanitizedMetadata(t *testing.T) {
	q := adapterPageQuery()
	dto := adapterBody(t, "bounded-jobs")
	original := Str(dto, "nextHref")
	for _, href := range []string{"https://attacker.invalid/app/rest/buildTypes", strings.Replace(original, "5000", "10000", 1), strings.Replace(original, "QXhpQ29udHJhY3Q", "Rm9yYmlkZGVu", 1), strings.Replace(original, "start:1", "start:0", 1)} {
		page, e := NormalizeJobPage(adapterPatch(dto, Object{"nextHref": href}), q, adapterTestServer, nil)
		adapterOK(t, e)
		adapterWant(t, len(Objects(page["items"])), 1)
		adapterWant(t, page["position"], nil)
		adapterWant(t, page["hasMore"], nil)
		adapterNotes(t, page, "UNSAFE_CONTINUATION")
	}
	job := adapterWithout(Objects(dto["buildType"])[0], "paused")
	page, e := NormalizeJobPage(Object{"count": 1, "buildType": []any{job}}, q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, Objects(page["items"])[0]["paused"], nil)
	adapterNotes(t, page, "JOB_PAUSED_UNAVAILABLE")
	rows := []any{}
	for i := 0; i < 100; i++ {
		rows = append(rows, adapterPatch(job, Object{"id": "Job" + strconvInt(i)}))
	}
	q.Count = 100
	page, e = NormalizeJobPage(Object{"count": 100, "buildType": rows}, q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, len(Objects(page["items"])), 100)
	count := 0
	for _, n := range page["limitations"].([]Limitation) {
		if n.Code == "JOB_PAUSED_UNAVAILABLE" {
			count++
		}
	}
	adapterWant(t, count, 1)
	normalized, e := NormalizeJob(adapterPatch(job, Object{"name": "credential-canary\x1b[31m"}), []string{"credential-canary"})
	adapterOK(t, e)
	adapterWant(t, normalized["name"], "[REDACTED]")
	normalized, e = NormalizeJob(adapterPatch(job, Object{"parameters": Object{"secret": "canary"}}), nil)
	adapterOK(t, e)
	if _, ok := normalized["parameters"]; ok {
		t.Fatal("raw parameters leaked")
	}
}
func TestAdapterNativeJobMethodsRecordedInputsAndErrors(t *testing.T) {
	q := adapterPageQuery()
	q.Kind = "job.page"
	fake := &adapterFakeTransport{execute: func(Operation) (Captured, error) { return adapterCapture(t, "bounded-jobs", false), nil }}
	reader := NewNativeReader(fake, adapterTestServer, nil)
	result := reader.Read(context.Background(), q, adapterBudget())
	adapterWant(t, result.State, "available")
	adapterWant(t, result.Provenance.ProjectID, "AxiContract")
	request, e := JobRequest(q)
	adapterOK(t, e)
	adapterWant(t, fake.calls[0].Path, request.Path)
	fake.execute = func(Operation) (Captured, error) { return adapterCapture(t, "bounded-jobs-denied", false), nil }
	result = reader.Read(context.Background(), q, adapterBudget())
	adapterWant(t, result.State, "unavailable")
	adapterWant(t, result.Error.Code, "PERMISSION_DENIED")
	fake.execute = func(Operation) (Captured, error) { return adapterCapture(t, "encoded-job", false), nil }
	result = reader.Read(context.Background(), ReadRequest{Kind: "job.detail", ID: "AxiContract_Fail"}, adapterBudget())
	adapterWant(t, result.Value["paused"], false)
	result = reader.Read(context.Background(), ReadRequest{Kind: "job.detail", ID: "Other"}, adapterBudget())
	adapterWant(t, result.Error.Code, "CONTEXT_MISMATCH")
}
func TestAdapterRecordedQueuePageIdentityTimeAndContinuation(t *testing.T) {
	q := adapterPageQuery()
	page, e := NormalizeQueuePage(adapterBody(t, "queue-project-positive"), q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, Objects(page["items"]), []Object{{"id": "10", "jobId": "AxiContract_QueueA", "state": "queued", "branch": nil, "queuedAt": "2026-10-02T15:41:26.000Z", "waitReason": "There are no idle compatible agents which can run this build"}})
	adapterWant(t, page["hasMore"], true)
	adapterWant(t, page["position"], 1)
	adapterWant(t, len(page["limitations"].([]Limitation)), 0)
	q.Start = 1
	page, e = NormalizeQueuePage(adapterBody(t, "queue-project-next"), q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, Str(Objects(page["items"])[0], "id"), "11")
	adapterWant(t, page["position"], 2)
	q.Start = 0
	q.Count = 20
	q.JobID = "AxiContract_QueueA"
	page, e = NormalizeQueuePage(adapterBody(t, "queue-intersection"), q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, Str(Objects(page["items"])[0], "id"), "10")
	adapterWant(t, page["hasMore"], nil)
	adapterNotes(t, page, "SCAN_COVERAGE_UNKNOWN")
}
func TestAdapterQueueMissingUnknownChangedAndRedactedMetadata(t *testing.T) {
	q := adapterPageQuery()
	build := Objects(adapterBody(t, "queue-project-positive")["build"])[0]
	page, e := NormalizeQueuePage(Object{"count": 1, "build": []any{adapterWithout(build, "waitReason", "queuedDate")}}, q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, Objects(page["items"])[0]["waitReason"], nil)
	adapterWant(t, Objects(page["items"])[0]["queuedAt"], nil)
	for _, state := range []string{"future-state", "running", "finished"} {
		page, e := NormalizeQueuePage(Object{"count": 1, "build": []any{adapterPatch(build, Object{"state": state})}}, q, adapterTestServer, nil)
		adapterOK(t, e)
		code := "QUEUE_STATE_CHANGED"
		want := state
		if state == "future-state" {
			code = "UNKNOWN_QUEUE_STATE"
			want = "unknown"
		}
		adapterWant(t, Str(Objects(page["items"])[0], "state"), want)
		adapterNotes(t, page, code)
	}
	page, e = NormalizeQueuePage(Object{"count": 1, "build": []any{adapterPatch(build, Object{"queuedDate": "20260230T000000+0000"})}}, q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, Objects(page["items"])[0]["queuedAt"], nil)
	adapterNotes(t, page, "INVALID_TIMESTAMP")
	found := false
	for _, note := range page["limitations"].([]Limitation) {
		found = found || (note.Code == "INVALID_TIMESTAMP" && note.Source == "queue")
	}
	if !found {
		t.Fatal("invalid queue timestamp lost its source identity")
	}
	page, e = NormalizeQueuePage(Object{"count": 1, "build": []any{adapterPatch(build, Object{"state": "private-credential", "waitReason": "private-credential\x1b[31m", "branchName": "private-credential"})}}, q, adapterTestServer, []string{"private-credential"})
	adapterOK(t, e)
	for _, key := range []string{"rawState", "waitReason", "branch"} {
		adapterWant(t, Objects(page["items"])[0][key], "[REDACTED]")
	}
}
func TestAdapterQueueRejectsScopeIdentityTypesCollectionsAndBounds(t *testing.T) {
	q := adapterPageQuery()
	build := Objects(adapterBody(t, "queue-project-positive")["build"])[0]
	q.JobID = Str(build, "buildTypeId")
	for _, patch := range []Object{{"id": float64(9007199254740992)}, {"id": "0"}, {"buildTypeId": "Foreign"}, {"buildType": Object{"id": "Other", "projectId": "AxiContract"}}, {"buildType": Object{"id": q.JobID, "projectId": "Forbidden"}}, {"state": nil}, {"waitReason": 42}, {"branchName": 42}} {
		_, e := NormalizeQueuePage(Object{"count": 1, "build": []any{adapterPatch(build, patch)}}, q, adapterTestServer, nil)
		adapterError(t, e, "")
	}
	q.Count = 2
	for _, page := range []Object{{"count": 0, "build": []any{build}}, {"count": 1, "build": nil}, {"count": 2, "build": []any{build, build}}} {
		_, e := NormalizeQueuePage(page, q, adapterTestServer, nil)
		adapterError(t, e, "")
	}
	for _, bad := range []string{string([]byte{0xed, 0xa0, 0x80}) + "job", "bad\u0085job", "bad\u202ejob", "bad\u2066job"} {
		for _, row := range []Object{adapterPatch(build, Object{"buildTypeId": bad, "buildType": Object{"id": bad, "projectId": "AxiContract"}}), adapterPatch(build, Object{"buildType": Object{"id": q.JobID, "projectId": bad}})} {
			_, e := NormalizeQueuePage(Object{"count": 1, "build": []any{row}}, q, adapterTestServer, nil)
			adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
		}
		for _, field := range []string{"job", "project"} {
			badq := q
			if field == "job" {
				badq.JobID = bad
			} else {
				badq.ProjectID = bad
			}
			_, e := QueueRequest(badq)
			adapterError(t, e, "USAGE_ERROR")
			adapterWant(t, AsDomainError(e).ExitCode, 2)
		}
	}
	queries := []ReadRequest{adapterPageQuery(), adapterPageQuery(), adapterPageQuery(), adapterPageQuery(), adapterPageQuery(), adapterPageQuery()}
	queries[0].ProjectID = ""
	queries[1].ProjectID = strings.Repeat("x", 257)
	queries[2].JobID = "bad\njob"
	queries[3].Count = 101
	queries[4].Start = 5000
	queries[5].ScanLimit = 5001
	for _, q := range queries {
		_, e := QueueRequest(q)
		adapterError(t, e, "")
		adapterWant(t, AsDomainError(e).ExitCode, 2)
	}
}
func TestAdapterQueueUnsafePagingUnknownExhaustionAndGroupedNotes(t *testing.T) {
	q := adapterPageQuery()
	dto := adapterBody(t, "queue-project-positive")
	original := Str(dto, "nextHref")
	for _, href := range []string{"https://attacker.invalid/app/rest/buildQueue", strings.Replace(original, "5000", "10000", 1), strings.Replace(original, "QXhpQ29udHJhY3Q", "Rm9yYmlkZGVu", 1), strings.Replace(original, "start:1", "start:0", 1)} {
		page, e := NormalizeQueuePage(adapterPatch(dto, Object{"nextHref": href}), q, adapterTestServer, nil)
		adapterOK(t, e)
		adapterWant(t, len(Objects(page["items"])), 1)
		adapterWant(t, page["position"], nil)
		adapterWant(t, page["hasMore"], nil)
		adapterNotes(t, page, "UNSAFE_CONTINUATION")
	}
	q.Start = 4999
	page, e := NormalizeQueuePage(adapterBody(t, "queue-project-empty"), q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, page["items"], []Object{})
	adapterWant(t, page["hasMore"], nil)
	q.Start = 0
	q.Count = 100
	rows := []any{}
	build := Objects(dto["build"])[0]
	for i := 0; i < 100; i++ {
		rows = append(rows, adapterPatch(build, Object{"id": i + 1, "state": "future-state", "queuedDate": "invalid"}))
	}
	page, e = NormalizeQueuePage(Object{"count": 100, "build": rows}, q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, len(Objects(page["items"])), 100)
	for _, code := range []string{"UNKNOWN_QUEUE_STATE", "INVALID_TIMESTAMP"} {
		count := 0
		for _, n := range page["limitations"].([]Limitation) {
			if n.Code == code {
				count++
			}
		}
		adapterWant(t, count, 1)
	}
}
func TestAdapterNativeQueueRecordedGETAndPermission(t *testing.T) {
	q := adapterPageQuery()
	q.Kind = "queue.page"
	fake := &adapterFakeTransport{execute: func(Operation) (Captured, error) { return adapterCapture(t, "queue-project-positive", false), nil }}
	reader := NewNativeReader(fake, adapterTestServer, nil)
	result := reader.Read(context.Background(), q, adapterBudget())
	adapterWant(t, result.State, "available")
	adapterWant(t, result.Provenance.ProjectID, "AxiContract")
	r, e := QueueRequest(q)
	adapterOK(t, e)
	adapterWant(t, fake.calls[0].Path, r.Path)
	fake.execute = func(Operation) (Captured, error) { return adapterCapture(t, "queue-project-denied", false), nil }
	result = reader.Read(context.Background(), q, adapterBudget())
	adapterWant(t, result.State, "unavailable")
	adapterWant(t, result.Error.Code, "PERMISSION_DENIED")
}
func TestAdapterRecordedAgentStatesPoolAndUnreportedActivity(t *testing.T) {
	q := adapterPageQuery()
	page, e := NormalizeAgentPage(adapterBody(t, "agents-project"), q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, Objects(page["items"]), []Object{{"id": "1", "name": "axi-contract-agent", "connected": true, "enabled": true, "authorized": true, "pool": Object{"id": "1", "name": "axi-contract-pool-20261002"}, "activeRun": nil, "activeRunState": "not_reported"}})
	adapterWant(t, page["position"], 1)
	adapterWant(t, page["hasMore"], true)
	adapterNotes(t, page, "ACTIVE_RUN_UNREPORTED")
	q.Start = 1
	page, e = NormalizeAgentPage(adapterBody(t, "agents-project-next"), q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, len(Objects(page["items"])), 0)
	adapterWant(t, page["hasMore"], nil)
	adapterNotes(t, page, "SCAN_COVERAGE_UNKNOWN")
	q.JobID = "AxiContract_Fail"
	filters, e := AgentFilters(q)
	adapterOK(t, e)
	count := 0
	for _, filter := range filters {
		if strings.HasPrefix(filter, "compatible:") {
			count++
		}
	}
	adapterWant(t, count, 1)
	if strings.Contains(AgentFields, "properties") {
		t.Fatal("unsafe projection")
	}
}
func TestAdapterAgentUnknownIdleActiveAndScopedPointers(t *testing.T) {
	q := adapterPageQuery()
	dto := adapterBody(t, "agent-detail")
	agent, _, e := NormalizeAgent(adapterPatch(dto, Object{"connected": true, "enabled": false, "authorized": false, "build": nil}), q, nil)
	adapterOK(t, e)
	adapterWant(t, agent["connected"], true)
	adapterWant(t, agent["enabled"], false)
	adapterWant(t, agent["authorized"], false)
	agent, notes, e := NormalizeAgent(Object{"id": 1, "name": "Synthetic"}, q, nil)
	adapterOK(t, e)
	for _, key := range []string{"connected", "enabled", "authorized", "pool"} {
		adapterWant(t, agent[key], nil)
	}
	adapterWant(t, Str(agent, "activeRunState"), "not_reported")
	count := 0
	for _, n := range notes {
		if n.Code == "AGENT_STATUS_UNAVAILABLE" {
			count++
		}
	}
	adapterWant(t, count, 1)
	for _, scope := range []ReadRequest{{}, {PoolID: "0"}} {
		agent, _, e := NormalizeAgent(adapterPatch(dto, Object{"pool": Object{"id": 0, "name": "Default"}, "build": nil}), scope, nil)
		adapterOK(t, e)
		adapterWant(t, Str(agent, "activeRunState"), "idle")
		adapterWant(t, Str(Obj(agent["pool"]), "id"), "0")
	}
	build := Object{"id": 12, "buildTypeId": "AxiContract_Fail", "buildType": Object{"id": "AxiContract_Fail", "projectId": "AxiContract"}}
	agent, _, e = NormalizeAgent(adapterPatch(dto, Object{"build": build}), q, nil)
	adapterOK(t, e)
	adapterWant(t, Obj(agent["activeRun"]), Object{"id": "12", "jobId": "AxiContract_Fail", "projectId": "AxiContract"})
	agent, notes, e = NormalizeAgent(adapterPatch(dto, Object{"build": adapterPatch(build, Object{"buildType": Object{"id": "AxiContract_Fail", "projectId": "Forbidden"}})}), q, nil)
	adapterOK(t, e)
	adapterWant(t, agent["activeRun"], nil)
	adapterWant(t, Str(agent, "activeRunState"), "unavailable")
	if !hasLimitation(notes, "ACTIVE_RUN_OUTSIDE_SCOPE") {
		t.Fatal(notes)
	}
	raw, _ := json.Marshal(Object{"agent": agent, "limitations": notes})
	if strings.Contains(string(raw), "Forbidden") {
		t.Fatal("foreign pointer exposed")
	}
	agent, _, e = NormalizeAgent(adapterPatch(dto, Object{"name": "canary\x1b[31m", "pool": Object{"id": 1, "name": "canary"}}), q, []string{"canary"})
	adapterOK(t, e)
	adapterWant(t, agent["name"], "[REDACTED]")
	adapterWant(t, Obj(agent["pool"])["name"], "[REDACTED]")
}
func TestAdapterAgentUnsafeIdentityTypesScopePagesAndBounds(t *testing.T) {
	q := adapterPageQuery()
	dto := adapterBody(t, "agent-detail")
	for _, patch := range []Object{{"id": 0}, {"id": float64(9007199254740992)}, {"id": "01"}, {"name": nil}, {"enabled": "true"}, {"pool": Object{"id": -1}}, {"pool": Object{"id": "01"}}, {"pool": Object{"id": string([]byte{0xed, 0xa0, 0x80})}}, {"build": Object{"id": 12, "buildTypeId": "x", "buildType": Object{"id": "other", "projectId": "AxiContract"}}}, {"build": Object{"id": 12, "buildTypeId": "bad\u0085job", "buildType": Object{"id": "bad\u0085job", "projectId": "AxiContract"}}}} {
		_, _, e := NormalizeAgent(adapterPatch(dto, patch), q, nil)
		adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
	}
	_, _, e := NormalizeAgent(dto, ReadRequest{PoolID: "2"}, nil)
	adapterError(t, e, "CONTEXT_MISMATCH")
	q.Count = 2
	for _, page := range []Object{{"count": 0, "agent": []any{dto}}, {"count": 1, "agent": nil}, {"count": 2, "agent": []any{dto, dto}}} {
		_, e := NormalizeAgentPage(page, q, adapterTestServer, nil)
		adapterError(t, e, "")
	}
	queries := []ReadRequest{adapterPageQuery(), adapterPageQuery(), adapterPageQuery(), adapterPageQuery(), adapterPageQuery(), adapterPageQuery(), adapterPageQuery(), adapterPageQuery()}
	queries[0].ProjectID = ""
	queries[1].PoolID = "01"
	queries[2].PoolID = "-1"
	queries[3].PoolID = "9007199254740993"
	queries[4].ProjectID = "bad\u202eid"
	queries[5].Count = 101
	queries[6].Start = 5000
	queries[7].ScanLimit = 5001
	for _, q := range queries {
		_, e := AgentRequest(q)
		adapterError(t, e, "")
		adapterWant(t, AsDomainError(e).ExitCode, 2)
	}
}
func TestAdapterAgentUnsafeContinuationAndGroupedHundredRows(t *testing.T) {
	q := adapterPageQuery()
	dto := adapterBody(t, "agents-project")
	href := Str(dto, "nextHref")
	for _, next := range []string{"https://attacker.invalid/app/rest/agents", strings.Replace(href, "defaultFilter:false", "defaultFilter:true", 1), strings.Replace(href, "5000", "10000", 1), strings.Replace(href, "start:1", "start:0", 1)} {
		page, e := NormalizeAgentPage(adapterPatch(dto, Object{"nextHref": next}), q, adapterTestServer, nil)
		adapterOK(t, e)
		adapterWant(t, len(Objects(page["items"])), 1)
		adapterWant(t, page["hasMore"], nil)
		adapterNotes(t, page, "UNSAFE_CONTINUATION")
	}
	q.Count = 100
	rows := []any{}
	for i := 0; i < 100; i++ {
		rows = append(rows, Object{"id": i + 1, "name": "Synthetic"})
	}
	page, e := NormalizeAgentPage(Object{"count": 100, "agent": rows}, q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, len(Objects(page["items"])), 100)
	adapterWant(t, len(page["limitations"].([]Limitation)), 4)
}
func TestAdapterNativeAgentExactIDInputsAndUnavailableScopes(t *testing.T) {
	fake := &adapterFakeTransport{execute: func(Operation) (Captured, error) { return adapterCapture(t, "agent-detail-job", false), nil }}
	reader := NewNativeReader(fake, adapterTestServer, nil)
	result := reader.Read(context.Background(), ReadRequest{Kind: "agent.detail", ID: "1", ProjectID: "AxiContract", JobID: "AxiContract_Fail"}, adapterBudget())
	adapterWant(t, result.State, "available")
	adapterWant(t, Str(result.Value, "id"), "1")
	adapterWant(t, fake.calls[0].Path, adapterFixture(t, false).Records["agent-detail-job"].Args[1])
	result = reader.Read(context.Background(), ReadRequest{Kind: "agent.detail", ID: "2"}, adapterBudget())
	adapterWant(t, result.State, "unavailable")
	adapterWant(t, result.Error.Code, "CONTEXT_MISMATCH")
	for _, entry := range []struct{ name, code, pool string }{{"agents-project-denied", "PERMISSION_DENIED", ""}, {"agents-pool", "NOT_FOUND", "1"}} {
		fake.execute = func(Operation) (Captured, error) { return adapterCapture(t, entry.name, false), nil }
		q := adapterPageQuery()
		q.Kind = "agents.page"
		q.PoolID = entry.pool
		result = reader.Read(context.Background(), q, adapterBudget())
		adapterWant(t, result.State, "unavailable")
		adapterWant(t, result.Error.Code, entry.code)
	}
}
