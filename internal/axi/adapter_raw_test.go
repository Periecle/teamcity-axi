package axi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAdapterRawReleasedCLIEnvelope(t *testing.T) {
	r, e := ParseRaw(adapterCapture(t, "run-view", true))
	adapterOK(t, e)
	adapterWant(t, Obj(r.Body)["id"], json.Number("482193"))
	for _, entry := range []struct{ mode, code string }{{"denied", "PERMISSION_DENIED"}, {"missing", "NOT_FOUND"}, {"expired", "AUTH_REQUIRED"}, {"malformed", "UPSTREAM_SCHEMA_MISMATCH"}, {"html", "AUTH_REQUIRED"}} {
		t.Run(entry.mode, func(t *testing.T) {
			_, e := ParseRaw(adapterCapture(t, "error-"+entry.mode, true))
			adapterError(t, e, entry.code)
		})
	}
	for _, text := range []string{`{"id":1}`, "HTTP/1.1 200 OK\nInvalid\n\n{}", "HTTP/1.1 200 OK\nContent-Type: application/json\nContent-Type: text/html\n\n{}"} {
		_, e := ParseRaw(Captured{Stdout: []byte(text)})
		adapterError(t, e, "")
	}
	nonzero := adapterCapture(t, "run-view", true)
	nonzero.ExitCode = 17
	_, e = ParseRaw(nonzero)
	adapterError(t, e, "UPSTREAM_FAILURE")
	nonzero.Stdout = []byte{255}
	_, e = ParseRaw(nonzero)
	adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
}
func TestAdapterRawInternalPacingMetadata(t *testing.T) {
	_, e := ParseRaw(Captured{Stdout: []byte("HTTP/1.1 429 Too Many Requests\nRetry-After: 15\n\n"), ExitCode: 1})
	adapterError(t, e, "UPSTREAM_FAILURE")
	de := AsDomainError(e)
	adapterWant(t, de.HTTPStatus, 429)
	adapterWant(t, de.RetryAfter, "15")
	adapterWant(t, de.Retryable, true)
	raw, _ := json.Marshal(de)
	if strings.Contains(string(raw), "15") || strings.Contains(string(raw), "httpStatus") {
		t.Fatal("internal headers serialized")
	}
}

func TestAdapterJSONIntegralDecimalAndExponentNumbers(t *testing.T) {
	for _, literal := range []string{"1.0", "1e0", "1E+0"} {
		value, e := DecodeJSON([]byte(`{"id":` + literal + `,"buildTypeId":"Job","state":"finished","status":"SUCCESS","failedToStart":false,"revisions":{"revision":[]}}`))
		adapterOK(t, e)
		run, _, _, e := NormalizeRun(value, adapterTestServer, nil)
		adapterOK(t, e)
		adapterWant(t, run["id"], "1")
		value, e = DecodeJSON([]byte(`{"count":` + literal + `,"buildType":[{"id":"Job","name":"Job","projectId":"AxiContract","paused":false}]}`))
		adapterOK(t, e)
		page, e := NormalizeJobPage(value, adapterPageQuery(), adapterTestServer, nil)
		adapterOK(t, e)
		adapterWant(t, Int(page, "providerReturned"), 1)
		value, e = DecodeJSON([]byte(`{"id":1,"snapshot-dependencies":{"count":` + literal + `}}`))
		adapterOK(t, e)
		count, e := NormalizeDependencyCount(value, "1")
		adapterOK(t, e)
		adapterWant(t, count, 1)
		value, e = DecodeJSON([]byte(`{"run_id":"1","messages":[{"id":` + literal + `,"level":1.0,"status":1e0,"text":"safe"}]}`))
		adapterOK(t, e)
		log, e := NormalizeLogTail(value, "1", 1, nil)
		adapterOK(t, e)
		adapterWant(t, Objects(log["messages"])[0]["id"], "1")
	}
	for _, value := range []json.Number{"1.1", "1e-1", "9007199254740992", "1e100000"} {
		_, ok := adapterNumber(value)
		adapterWant(t, ok, false)
	}
}
func TestAdapterRunSafeDTOContract(t *testing.T) {
	run, project, _, e := NormalizeRun(adapterPatch(adapterBaseRun(), Object{"unexpected": Object{"token": "not-propagated"}}), "https://teamcity.example.test/teamcity", nil)
	adapterOK(t, e)
	adapterWant(t, Str(run, "id"), "482193")
	adapterWant(t, Str(run, "result"), "failure")
	adapterWant(t, Str(run, "startedAt"), "2026-10-01T14:00:00.000Z")
	adapterWant(t, run["durationMs"], int64(60000))
	adapterWant(t, project, "Payments")
	if _, ok := run["unexpected"]; ok {
		t.Fatal("raw property propagated")
	}
	for _, patch := range []Object{{"id": float64(9007199254740992)}, {"id": "9007199254740993"}, {"id": nil}, {"state": nil}, {"status": nil}, {"buildTypeId": ""}, {"buildType": Object{"id": "Other"}}, {"revisions": Object{"revision": nil}}, {"revisions": Object{"revision": []any{Object{"version": "sha", "vcs-root-instance": Object{"id": "17"}}}}}} {
		_, _, _, e := NormalizeRun(adapterPatch(adapterBaseRun(), patch), adapterTestServer, nil)
		adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
	}
	for _, status := range []string{"NEW_RESULT", "toString", "__proto__"} {
		run, _, _, e := NormalizeRun(adapterPatch(adapterBaseRun(), Object{"status": status}), adapterTestServer, nil)
		adapterOK(t, e)
		adapterWant(t, Str(run, "result"), "unknown")
	}
	run, _, notes, e := NormalizeRun(adapterPatch(adapterBaseRun(), Object{"finishDate": "20260230T140000+0000"}), adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, run["finishedAt"], nil)
	if !hasLimitation(notes, "INVALID_TIMESTAMP") {
		t.Fatal("invalid date not reported")
	}
}
func TestAdapterRunRecordedQueuedUnknownResult(t *testing.T) {
	dto := adapterBody(t, "queued-run-detail")
	_, present := dto["status"]
	adapterWant(t, present, false)
	run, project, notes, e := NormalizeRun(dto, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, Str(run, "id"), "10")
	adapterWant(t, Str(run, "state"), "queued")
	adapterWant(t, Str(run, "result"), "unknown")
	adapterWant(t, run["rawStatus"], nil)
	adapterWant(t, project, "AxiContract")
	if !hasLimitation(notes, "RESULT_UNAVAILABLE") {
		t.Fatal(notes)
	}
	for _, patch := range []Object{{"state": "running"}, {"state": "finished"}, {"status": nil}} {
		_, _, _, e := NormalizeRun(adapterPatch(dto, patch), adapterTestServer, nil)
		adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
	}
}
func TestAdapterRunRequestedIDCannotBeReplacedByGreen(t *testing.T) {
	capture := adapterCapture(t, "run-view", true)
	capture.Stdout = []byte(strings.ReplaceAll(strings.ReplaceAll(string(capture.Stdout), `"id":482193`, `"id":482100`), `"status":"FAILURE"`, `"status":"SUCCESS"`))
	fake := &adapterFakeTransport{execute: func(Operation) (Captured, error) { return capture, nil }}
	reader := NewNativeReader(fake, "https://teamcity.example.test/teamcity", nil)
	result := reader.Read(context.Background(), ReadRequest{Kind: "run.detail", ID: "482193"}, adapterBudget())
	adapterWant(t, result.State, "unavailable")
	adapterWant(t, result.Error.Code, "CONTEXT_MISMATCH")
}
func TestAdapterRunRedactionBeforeTruncationAndMissingRevisions(t *testing.T) {
	secret := "canary-" + strings.Repeat("q", 1600)
	for _, text := range []string{secret, secret[:600] + "\x1b[31m" + secret[600:]} {
		run, _, _, e := NormalizeRun(adapterPatch(adapterBaseRun(), Object{"status": text, "statusText": text}), adapterTestServer, []string{secret})
		adapterOK(t, e)
		adapterWant(t, run["rawStatus"], "[REDACTED]")
		adapterWant(t, run["statusText"], "[REDACTED]")
	}
	run, _, notes, e := NormalizeRun(adapterWithout(adapterBaseRun(), "revisions"), adapterTestServer, nil)
	adapterOK(t, e)
	if _, ok := run["revisions"]; ok {
		t.Fatal("fabricated revisions")
	}
	if !hasLimitation(notes, "MISSING_REVISION_METADATA") {
		t.Fatal(notes)
	}
	run, _, notes, e = NormalizeRun(adapterPatch(adapterBaseRun(), Object{"startDate": "00991001T140000+0000"}), adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, run["startedAt"], nil)
	if !hasLimitation(notes, "INVALID_TIMESTAMP") {
		t.Fatal(notes)
	}
}
func TestAdapterRecordedExplicitOutcomes(t *testing.T) {
	for _, entry := range []struct{ name, id, result string }{{"outcome-normal-failed", "1", "failure"}, {"outcome-normal-green", "2", "success"}, {"outcome-failed-to-start", "5", "failed_to_start"}, {"outcome-canceled-queued", "13", "canceled"}, {"outcome-canceled-running", "14", "canceled"}, {"outcome-composite", "16", "success"}} {
		t.Run(entry.name, func(t *testing.T) {
			run, _, notes, e := NormalizeRun(adapterBody(t, entry.name), adapterTestServer, nil)
			adapterOK(t, e)
			adapterWant(t, Str(run, "id"), entry.id)
			adapterWant(t, Str(run, "state"), "finished")
			adapterWant(t, Str(run, "result"), entry.result)
			adapterWant(t, run["composite"], entry.name == "outcome-composite")
			if hasLimitation(notes, "UNKNOWN_RESULT") {
				t.Fatal(notes)
			}
			raw, _ := json.Marshal(run)
			if strings.Contains(string(raw), "canceledInfo") {
				t.Fatal("raw cancellation metadata")
			}
		})
	}
}
func TestAdapterMissingConflictingAndMalformedExplicitOutcomes(t *testing.T) {
	for _, patch := range []Object{{"failedToStart": nil}, {"failedToStart": true}, {"canceledInfo": Object{"timestamp": "20261002T180555+0000"}}, {"canceledInfo": Object{}}, {"canceledInfo": Object{"timestamp": "invalid"}}, {"failedToStart": true, "canceledInfo": Object{"timestamp": "20261002T180555+0000"}}} {
		run, _, notes, e := NormalizeRun(adapterPatch(adapterPatch(adapterBaseRun(), Object{"status": "SUCCESS"}), patch), adapterTestServer, nil)
		adapterOK(t, e)
		adapterWant(t, Str(run, "result"), "unknown")
		if !hasLimitation(notes, "OUTCOME_METADATA_UNAVAILABLE") && !hasLimitation(notes, "CONFLICTING_OUTCOME_METADATA") {
			t.Fatal(notes)
		}
	}
	for _, patch := range []Object{{"failedToStart": "false"}, {"canceledInfo": true}, {"canceledInfo": Object{"timestamp": 123}}} {
		_, _, _, e := NormalizeRun(adapterPatch(adapterBaseRun(), patch), adapterTestServer, nil)
		adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
	}
	run, _, _, e := NormalizeRun(adapterPatch(adapterBaseRun(), Object{"status": "UNKNOWN", "canceledInfo": Object{"timestamp": "20261002T180555+0000", "text": "Private cancellation comment", "user": Object{"username": "Private actor"}}}), adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, Str(run, "result"), "canceled")
	raw, _ := json.Marshal(run)
	if strings.Contains(string(raw), "Private") {
		t.Fatal("private actor exposed")
	}
}
func TestAdapterRecordedRunningOutcome(t *testing.T) {
	run, project, _, e := NormalizeRun(adapterBody(t, "outcome-running"), adapterTestServer, nil)
	adapterOK(t, e)
	fixture := Obj(adapterFixture(t, false).Fixture["lifecycle"])
	adapterWant(t, Str(run, "id"), Str(fixture, "runningObservedRunId"))
	adapterWant(t, Str(run, "jobId"), Str(Obj(fixture["jobs"]), "slow"))
	adapterWant(t, project, Str(fixture, "projectId"))
	adapterWant(t, Str(run, "state"), "running")
	adapterWant(t, Str(run, "result"), "success")
	adapterWant(t, run["composite"], false)
	terminal := Obj(fixture["runningWatchEvidence"])
	adapterWant(t, Str(terminal, "runId"), Str(run, "id"))
	adapterWant(t, Str(terminal, "initialState"), "running")
	adapterWant(t, Str(terminal, "terminalResult"), "canceled")
	if Int(terminal, "polls") < 2 {
		t.Fatal(terminal)
	}
	adapterWant(t, terminal["checkPassed"], false)
}
