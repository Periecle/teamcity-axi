package axi

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestAdapterLocatorNamesCannotInjectDimensions(t *testing.T) {
	for _, value := range []string{"feature/refund", "a,b", "branch:(default:any)", "a:b", "$base64", "()", "space branch", "日🦊", "@this", "-"} {
		encoded, e := Literal(value)
		adapterOK(t, e)
		if !strings.HasPrefix(encoded, "($base64:") {
			t.Fatal(encoded)
		}
		decoded, e := base64.RawURLEncoding.DecodeString(encoded[9 : len(encoded)-1])
		adapterOK(t, e)
		adapterWant(t, string(decoded), value)
		id, e := IDCondition(value)
		adapterOK(t, e)
		adapterWant(t, id, "(id:"+encoded+")")
		branch, e := BranchCondition(value)
		adapterOK(t, e)
		adapterWant(t, branch, "(name:(value:"+encoded+"))")
		path := APIPath("builds", []string{"buildType:" + id, "branch:" + branch, "count:20"}, "count,nextHref,build(id)")
		u, e := url.Parse(path)
		adapterOK(t, e)
		adapterWant(t, len(u.Query()), 2)
		adapterWant(t, u.Query().Get("locator"), "buildType:(id:"+encoded+"),branch:(name:(value:"+encoded+")),count:20")
	}
	for _, bad := range []string{"", "\n", strings.Repeat("a", 4097), string([]byte{0xed, 0xa0, 0x80}), string([]byte{0xed, 0xbf, 0xbf})} {
		_, e := Literal(bad)
		adapterError(t, e, "USAGE_ERROR")
	}
}
func adapterContinuationFixture() ContinuationRequest {
	return ContinuationRequest{ServerURL: "https://tc.example/teamcity", Resource: "builds", Filters: []string{"buildType:(id:Build)", "branch:(name:(value:($base64:YSwp)))"}, Fields: "count,nextHref,build(id)", Count: 20, Start: 0, ScanLimit: 5000}
}
func adapterNextHref(r ContinuationRequest, position int) string {
	return "/teamcity" + APIPath(r.Resource, append(append([]string{}, r.Filters...), "count:20", "start:"+strconvInt(position), "lookupLimit:5000"), r.Fields)
}
func strconvInt(value int) string { raw, _ := json.Marshal(value); return string(raw) }
func TestAdapterContinuationReducedToBoundedPosition(t *testing.T) {
	r := adapterContinuationFixture()
	next := adapterNextHref(r, 20)
	for _, href := range []string{next, strings.Replace(next, "/teamcity/", "/", 1), "https://tc.example" + next} {
		n, e := NextPosition(href, r)
		adapterOK(t, e)
		adapterWant(t, n, 20)
	}
	patch := func(key, value string) string {
		u, _ := url.Parse(next)
		q := u.Query()
		q.Set(key, value)
		u.RawQuery = q.Encode()
		return u.String()
	}
	for _, href := range []string{"https://tc.example" + strings.Replace(next, "/teamcity/", "/", 1), "https://evil.example" + next, "//tc.example" + next, strings.Replace(next, "/builds?", "/agents?", 1), strings.Replace(next, "/teamcity/", "/teamcity/../", 1), strings.Replace(next, "/teamcity/", "/teamcity/%2e%2e/", 1), strings.Replace(next, "/teamcity/", "/teamcity/%252e%252e/", 1), patch("fields", "build(parameters)"), patch("extra", "x"), patch("locator", "buildType:(id:Other),count:20,start:20"), patch("locator", strings.Join(append(append([]string{}, r.Filters...), "count:100", "start:20"), ",")), patch("locator", strings.Join(append(append([]string{}, r.Filters...), "count:20", "start:20", "lookupLimit:10000"), ",")), adapterNextHref(r, 0), adapterNextHref(r, 5000)} {
		_, e := NextPosition(href, r)
		adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
	}
}
func TestAdapterCursorBindingAndHostileInputs(t *testing.T) {
	now := int64(1790938800000)
	binding := CursorBinding{Command: "run.list", Server: "work", FilterHash: strings.Repeat("a", 64), Count: 20, Window: &TimeWindow{Since: "2026-09-25T08:00:00.000Z", Until: "2026-10-02T08:00:00.000Z"}}
	cursor := Cursor{CursorBinding: binding, Version: 1, Position: 20, ExpiresAt: now + 1800000}
	token, e := EncodeCursor(cursor, now)
	adapterOK(t, e)
	decoded, e := DecodeCursor(token, now)
	adapterOK(t, e)
	adapterWant(t, decoded, cursor)
	adapterOK(t, AssertCursor(cursor, binding))
	bindings := []CursorBinding{binding, binding, binding, binding}
	bindings[0].Command = "job.list"
	bindings[1].Server = "other"
	bindings[2].FilterHash = strings.Repeat("b", 64)
	bindings[3].Count = 100
	for _, b := range bindings {
		adapterError(t, AssertCursor(cursor, b), "USAGE_ERROR")
	}
	c := cursor
	c.Window = &TimeWindow{Since: "1970-01-01T00:00:00.000Z", Until: "2099-01-01T00:00:00.000Z"}
	adapterError(t, AssertCursor(c, binding), "USAGE_ERROR")
	for _, bad := range []string{"https://evil.example/api", token + "=", token + "\n", strings.Repeat("x", 4097), base64.RawURLEncoding.EncodeToString([]byte{255})} {
		_, e := DecodeCursor(bad, now)
		adapterError(t, e, "USAGE_ERROR")
	}
	raw, _ := json.Marshal(cursor)
	var object Object
	adapterOK(t, json.Unmarshal(raw, &object))
	for _, patch := range []Object{{"position": 0}, {"position": 5000}, {"position": 1.5}, {"expiresAt": now}, {"expiresAt": now + 1800001}, {"token": "credential"}, {"providerUrl": "https://evil.example"}, {"window": Object{"since": "2026-02-30T00:00:00.000Z", "until": cursor.Window.Until}}, {"window": Object{"since": cursor.Window.Since, "until": cursor.Window.Until, "extra": true}}} {
		raw, _ := json.Marshal(adapterPatch(object, patch))
		_, e := DecodeCursor(base64.RawURLEncoding.EncodeToString(raw), now)
		adapterError(t, e, "USAGE_ERROR")
	}
}
func TestAdapterCursorExactFractionalWindow(t *testing.T) {
	now := int64(1790938800000)
	w := &TimeWindow{Since: "2026-10-01T00:00:00.000000001Z", Until: "2026-10-02T00:00:00.1234Z"}
	c := Cursor{CursorBinding: CursorBinding{Command: "run.list", Server: "work", FilterHash: strings.Repeat("a", 64), Count: 20, Window: w}, Version: 1, Position: 20, ExpiresAt: now + 1800000}
	token, e := EncodeCursor(c, now)
	adapterOK(t, e)
	got, e := DecodeCursor(token, now)
	adapterOK(t, e)
	adapterWant(t, got.Window, w)
	for _, bad := range []*TimeWindow{{Since: "2026-10-02T00:00:00.000000002Z", Until: "2026-10-02T00:00:00.000000001Z"}, {Since: "2026-10-01T02:00:00.000000001+02:00", Until: w.Until}, {Since: w.Since, Until: "2026-10-02T00:00:00.123400Z"}} {
		c.Window = bad
		_, e := EncodeCursor(c, now)
		adapterError(t, e, "USAGE_ERROR")
	}
}
func adapterRunQuery() ReadRequest {
	return ReadRequest{JobID: "Payments_Build", Branch: "feature/refund", State: "finished", Count: 20, ScanLimit: 5000, AllowedProjects: []string{"Payments"}}
}
func adapterRunRequest(t *testing.T, q ReadRequest) ContinuationRequest {
	t.Helper()
	filters, e := RunFilters(q)
	adapterOK(t, e)
	return ContinuationRequest{ServerURL: "https://fixture.test/teamcity", Resource: "builds", Filters: filters, Fields: "count,nextHref,build(id)", Count: 20, Start: 0, ScanLimit: 5000}
}
func TestAdapterRunPagesBoundedContinuationAndUnknownExhaustion(t *testing.T) {
	q := adapterRunQuery()
	r := adapterRunRequest(t, q)
	for _, rows := range [][]any{{adapterBaseRun()}, {}} {
		position := 20
		if len(rows) == 0 {
			position = 40
		}
		page, e := NormalizeRunPage(Object{"build": rows, "count": len(rows), "nextHref": adapterNextHref(r, position)}, q, r, nil)
		adapterOK(t, e)
		adapterWant(t, len(Objects(page["runs"])), len(rows))
		if len(rows) != 0 {
			adapterWant(t, Str(Objects(page["runs"])[0], "id"), "482193")
		} else {
			adapterWant(t, page["runs"], []Object{})
		}
		adapterWant(t, page["position"], position)
		adapterWant(t, page["hasMore"], true)
		adapterWant(t, Int(page, "providerReturned"), len(rows))
	}
	page, e := NormalizeRunPage(Object{"build": []any{}, "count": 0}, q, r, nil)
	adapterOK(t, e)
	adapterWant(t, page["hasMore"], nil)
	adapterNotes(t, page, "SCAN_COVERAGE_UNKNOWN")
}
func TestAdapterRunPagesRejectScopeCountsDuplicates(t *testing.T) {
	q := adapterRunQuery()
	r := adapterRunRequest(t, q)
	for _, dto := range []Object{{"build": nil, "count": 0}, {"build": []any{}, "count": 1}, {"build": []any{adapterBaseRun(), adapterBaseRun()}, "count": 2}, {"build": []any{adapterPatch(adapterBaseRun(), Object{"buildTypeId": "Other", "buildType": Object{"id": "Other", "projectId": "Payments"}})}, "count": 1}, {"build": []any{adapterPatch(adapterBaseRun(), Object{"buildType": Object{"id": "Payments_Build", "projectId": "Forbidden"}})}, "count": 1}, {"build": []any{adapterPatch(adapterBaseRun(), Object{"branchName": "Other"})}, "count": 1}, {"build": []any{adapterPatch(adapterBaseRun(), Object{"state": "running"})}, "count": 1}} {
		_, e := NormalizeRunPage(dto, q, r, nil)
		adapterError(t, e, "")
	}
}
func TestAdapterRunPagesUnsafeContinuationKeepsRows(t *testing.T) {
	q := adapterRunQuery()
	r := adapterRunRequest(t, q)
	for _, href := range []string{"https://attacker.test/app/rest/builds?locator=count:20,start:20", adapterNextHref(r, 0), strings.Replace(adapterNextHref(r, 20), "lookupLimit%3A5000", "lookupLimit%3A10000", 1)} {
		page, e := NormalizeRunPage(Object{"build": []any{adapterBaseRun()}, "count": 1, "nextHref": href}, q, r, nil)
		adapterOK(t, e)
		adapterWant(t, len(Objects(page["runs"])), 1)
		adapterWant(t, page["hasMore"], nil)
		adapterWant(t, page["position"], nil)
		adapterNotes(t, page, "UNSAFE_CONTINUATION")
	}
}
func TestAdapterRunPagesExactRevisionRoot(t *testing.T) {
	q := adapterRunQuery()
	q.Revision = strings.Repeat("a", 40)
	q.VCSRootID = "Payments_Git"
	r := adapterRunRequest(t, q)
	page, e := NormalizeRunPage(Object{"build": []any{adapterBaseRun()}, "count": 1, "nextHref": adapterNextHref(r, 20)}, q, r, nil)
	adapterOK(t, e)
	adapterWant(t, len(Objects(page["runs"])), 1)
	for _, run := range []Object{adapterWithout(adapterBaseRun(), "revisions"), adapterPatch(adapterBaseRun(), Object{"revisions": Object{"revision": []any{Object{"version": q.Revision, "vcs-root-instance": Object{"vcs-root-id": "OtherRoot"}}}}})} {
		page, e := NormalizeRunPage(Object{"build": []any{run}, "count": 1, "nextHref": adapterNextHref(r, 20)}, q, r, nil)
		adapterOK(t, e)
		adapterWant(t, page["runs"], []Object{})
		adapterNotes(t, page, "REVISION_UNVERIFIED")
	}
}
func TestAdapterRunPagesUnknownOutcomeRetainsProviderCoverage(t *testing.T) {
	q := adapterRunQuery()
	r := adapterRunRequest(t, q)
	q.Result = "unknown"
	filters, e := RunFilters(q)
	adapterOK(t, e)
	adapterWant(t, filters, r.Filters)
	rows := []any{adapterBaseRun(), adapterPatch(adapterBaseRun(), Object{"id": 482194, "status": "FUTURE_RESULT"}), adapterWithout(adapterPatch(adapterBaseRun(), Object{"id": 482195, "status": "SUCCESS"}), "failedToStart"), adapterPatch(adapterBaseRun(), Object{"id": 482196, "status": "UNKNOWN", "canceledInfo": Object{"timestamp": "20261001T110000+0000"}})}
	page, e := NormalizeRunPage(Object{"count": 4, "build": rows, "nextHref": adapterNextHref(r, 20)}, q, r, nil)
	adapterOK(t, e)
	adapterWant(t, Int(page, "providerReturned"), 4)
	runs := Objects(page["runs"])
	adapterWant(t, len(runs), 2)
	adapterWant(t, Str(runs[0], "id"), "482194")
	adapterWant(t, Str(runs[1], "id"), "482195")
	for _, run := range runs {
		adapterWant(t, Str(run, "result"), "unknown")
	}
	adapterWant(t, page["hasMore"], true)
	adapterWant(t, page["position"], 20)
	adapterNotes(t, page, "OUTCOME_METADATA_UNAVAILABLE")
	found := false
	for _, note := range page["limitations"].([]Limitation) {
		found = found || (note.Code == "OUTCOME_METADATA_UNAVAILABLE" && note.RunID == "482195")
	}
	if !found {
		t.Fatal("outcome uncertainty lost its execution identity")
	}
	page, e = NormalizeRunPage(Object{"count": 1, "build": []any{adapterBaseRun()}, "nextHref": adapterNextHref(r, 20)}, q, r, nil)
	adapterOK(t, e)
	adapterWant(t, len(Objects(page["runs"])), 0)
	adapterWant(t, Int(page, "providerReturned"), 1)
	adapterWant(t, page["hasMore"], true)
	_, e = NormalizeRunPage(Object{"count": 1, "build": []any{adapterPatch(adapterBaseRun(), Object{"branchName": "Foreign"})}}, q, r, nil)
	adapterError(t, e, "CONTEXT_MISMATCH")
}
func TestAdapterRunPagesFractionalFinishMembership(t *testing.T) {
	q := adapterRunQuery()
	q.Window = &TimeWindow{Since: "2026-10-01T11:00:00.000000001Z", Until: "2026-10-01T11:00:01.000000001Z"}
	r := adapterRunRequest(t, q)
	if !contains(r.Filters, "finishDate:(date:20261001T110000+0000,condition:after)") || !contains(r.Filters, "finishDate:(date:20261001T110001.001+0000,condition:before)") {
		t.Fatal(r.Filters)
	}
	rows := []any{adapterPatch(adapterBaseRun(), Object{"finishDate": "20261001T110000+0000"}), adapterPatch(adapterBaseRun(), Object{"id": 482194, "finishDate": "20261001T110001+0000"}), adapterPatch(adapterBaseRun(), Object{"id": 482195, "finishDate": "20261001T110002+0000"})}
	page, e := NormalizeRunPage(Object{"count": 3, "build": rows}, q, r, nil)
	adapterOK(t, e)
	adapterWant(t, Int(page, "providerReturned"), 3)
	runs := Objects(page["runs"])
	adapterWant(t, len(runs), 2)
	adapterWant(t, Str(runs[0], "id"), "482193")
	adapterWant(t, Str(runs[1], "id"), "482194")
}
func TestAdapterReportedSecondDoesNotSubstituteForPreciseFinish(t *testing.T) {
	q := adapterRunQuery()
	q.Window = &TimeWindow{Since: "2026-10-01T11:00:00.568999999Z", Until: "2026-10-01T11:00:00.569000001Z"}
	r := adapterRunRequest(t, q)
	if !contains(r.Filters, "finishDate:(date:20261001T110000.568+0000,condition:after)") || !contains(r.Filters, "finishDate:(date:20261001T110000.570+0000,condition:before)") {
		t.Fatal(r.Filters)
	}
	page, e := NormalizeRunPage(Object{"count": 1, "build": []any{adapterPatch(adapterBaseRun(), Object{"finishDate": "20261001T110000+0000"})}}, q, r, nil)
	adapterOK(t, e)
	adapterWant(t, len(Objects(page["runs"])), 1)
	adapterWant(t, Objects(page["runs"])[0]["finishedAt"], "2026-10-01T11:00:00.000Z")
	if hasLimitation(page["limitations"].([]Limitation), "FINISH_TIME_OUTSIDE_WINDOW") {
		t.Fatal(page)
	}
}
func TestAdapterOrdinaryAndExceptionalOutcomeFilters(t *testing.T) {
	base := ReadRequest{JobID: "Payments_Build", State: "finished", Count: 20, ScanLimit: 5000}
	for _, result := range []string{"success", "failure", "error"} {
		q := base
		q.Result = result
		filters, e := RunFilters(q)
		adapterOK(t, e)
		if !contains(filters, "canceled:false") || !contains(filters, "failedToStart:false") {
			t.Fatal(filters)
		}
	}
	for _, entry := range []struct{ result, filter string }{{"canceled", "canceled:true"}, {"failed_to_start", "failedToStart:true"}} {
		q := base
		q.Result = entry.result
		filters, e := RunFilters(q)
		adapterOK(t, e)
		if !contains(filters, entry.filter) {
			t.Fatal(filters)
		}
	}
	base.Result = "failure"
	r := adapterRunRequest(t, base)
	_, e := NormalizeRunPage(Object{"count": 1, "build": []any{adapterPatch(adapterBaseRun(), Object{"failedToStart": true})}}, base, r, nil)
	adapterError(t, e, "CONTEXT_MISMATCH")
}
