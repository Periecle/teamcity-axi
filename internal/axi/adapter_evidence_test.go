package axi

import (
	"regexp"
	"strings"
	"testing"

	toon "github.com/toon-format/toon-go"
)

func TestAdapterRecordedProjectServerAndPrivateIdentity(t *testing.T) {
	project, e := NormalizeProject(adapterBody(t, "project"), "AxiContract", nil)
	adapterOK(t, e)
	adapterWant(t, project["parentProjectId"], "_Root")
	root, e := NormalizeProject(adapterBody(t, "root-project"), "_Root", nil)
	adapterOK(t, e)
	adapterWant(t, root["parentProjectId"], nil)
	server, e := NormalizeServer(adapterBody(t, "server"), nil)
	adapterOK(t, e)
	adapterWant(t, server["buildNumber"], "238924")
	current, e := NormalizeIdentity(adapterPatch(adapterBody(t, "identity"), Object{"token": "canary", "name": "not-public"}), adapterTestServer)
	adapterOK(t, e)
	adapterWant(t, len(current), 1)
	fingerprint := Str(current, "fingerprint")
	if !regexp.MustCompile(`^sha256:[a-f0-9]{32}$`).MatchString(fingerprint) {
		t.Fatal(current)
	}
	other, e := NormalizeIdentity(adapterBody(t, "identity"), "https://other.invalid")
	adapterOK(t, e)
	if fingerprint == other["fingerprint"] {
		t.Fatal("identity not server-bound")
	}
	_, e = NormalizeProject(adapterBody(t, "project"), "Other", nil)
	adapterError(t, e, "CONTEXT_MISMATCH")
	_, e = NormalizeProject(adapterWithout(adapterBody(t, "project"), "parentProjectId"), "AxiContract", nil)
	adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
	_, e = NormalizeIdentity(Object{"id": 0, "username": "guest"}, "https://a.invalid")
	adapterError(t, e, "")
}
func adapterLogDTO(t *testing.T) Object {
	t.Helper()
	value, e := DecodeJSON(adapterCapture(t, "log-probe", false).Stdout)
	adapterOK(t, e)
	return Obj(value)
}
func TestAdapterNativeLogOverdeliveryIdentityOrderAndRedaction(t *testing.T) {
	dto := adapterLogDTO(t)
	rows := Objects(dto["messages"])
	tail, e := NormalizeLogTail(dto, "1", 1, nil)
	adapterOK(t, e)
	adapterWant(t, Int(tail, "providerReturned"), 2)
	adapterWant(t, len(Objects(tail["messages"])), 1)
	adapterWant(t, tail["truncated"], true)
	id, _ := adapterNumber(rows[len(rows)-1]["id"])
	adapterWant(t, Str(Objects(tail["messages"])[0], "id"), strconvInt(int(id)))
	secret := "secret-" + strings.Repeat("z", 2500)
	tail, e = NormalizeLogTail(adapterPatch(dto, Object{"messages": []any{adapterPatch(rows[0], Object{"text": secret})}}), "1", 1, []string{secret})
	adapterOK(t, e)
	adapterWant(t, Objects(tail["messages"])[0]["text"], "[REDACTED]")
	_, e = NormalizeLogTail(adapterPatch(dto, Object{"run_id": "2"}), "1", 1, nil)
	adapterError(t, e, "CONTEXT_MISMATCH")
	_, e = NormalizeLogTail(adapterPatch(dto, Object{"messages": []any{rows[0], rows[0]}}), "1", 1, nil)
	adapterError(t, e, "")
	_, e = NormalizeLogTail(adapterPatch(dto, Object{"messages": []any{rows[1], rows[0]}}), "1", 1, nil)
	adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
	tail, e = NormalizeLogTail(adapterPatch(dto, Object{"messages": []any{adapterPatch(rows[0], Object{"id": 0}), adapterPatch(rows[1], Object{"id": 10})}}), "1", 1, nil)
	adapterOK(t, e)
	adapterWant(t, Str(Objects(tail["messages"])[0], "id"), "10")
}
func TestAdapterOccurrencePagesContinuationAndUnknownFilteredExhaustion(t *testing.T) {
	q := ReadRequest{RunID: "1", Count: 1, ScanLimit: 5000}
	for _, entry := range []struct{ kind, name string }{{"problems", "problems-page"}, {"tests", "tests-page"}} {
		dto := adapterBody(t, entry.name)
		page, e := NormalizeEvidencePage(entry.kind, dto, q, adapterTestServer, nil)
		adapterOK(t, e)
		adapterWant(t, len(Objects(page["items"])), 1)
		adapterWant(t, Str(Objects(page["items"])[0], "runId"), "1")
		adapterWant(t, page["position"], 1)
		adapterWant(t, page["hasMore"], true)
		for _, href := range []any{"https://attacker.invalid/app/rest/testOccurrences", 42, nil, "", Object{}} {
			page, e := NormalizeEvidencePage(entry.kind, adapterPatch(dto, Object{"nextHref": href}), q, adapterTestServer, nil)
			adapterOK(t, e)
			adapterWant(t, len(Objects(page["items"])), 1)
			adapterWant(t, page["position"], nil)
			adapterNotes(t, page, "UNSAFE_CONTINUATION")
		}
	}
	q.Count = 20
	q.Failed = true
	q.Muted = true
	page, e := NormalizeEvidencePage("tests", adapterBody(t, "tests-muted"), q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, page["items"], []Object{})
	adapterWant(t, page["hasMore"], nil)
	adapterNotes(t, page, "SCAN_COVERAGE_UNKNOWN")
}
func TestAdapterExactOccurrencesRunBoundAndDefinitionsRejected(t *testing.T) {
	locator, e := OccurrenceLocator("tests", "build:(id:1),id:2000000000", "1")
	adapterOK(t, e)
	adapterWant(t, locator, "build:(id:1),id:2000000000")
	_, e = OccurrenceLocator("tests", "517450581327024597", "1")
	adapterError(t, e, "USAGE_ERROR")
	_, e = OccurrenceLocator("problems", "build:(id:2),problem:(id:1)", "1")
	adapterError(t, e, "CONTEXT_MISMATCH")
	_, e = OccurrenceLocator("tests", "build:(id:1),id:1),item:(build:(id:9))", "1")
	adapterError(t, e, "USAGE_ERROR")
	_, e = NormalizeProblem(adapterPatch(adapterBody(t, "problem-selected"), Object{"build": Object{"id": 2}}), "1", nil)
	adapterError(t, e, "CONTEXT_MISMATCH")
	notes := []Limitation{}
	_, e = NormalizeTest(adapterPatch(adapterBody(t, "test-selected"), Object{"build": Object{"id": 2}}), "1", nil, &notes)
	adapterError(t, e, "CONTEXT_MISMATCH")
	test, e := NormalizeTest(adapterBody(t, "test-selected"), "1", nil, &notes)
	adapterOK(t, e)
	adapterWant(t, test["testId"], "517450581327024597")
}
func TestAdapterOccurrenceSecretsUnknownFlagsAndFilters(t *testing.T) {
	secret := "secret-" + strings.Repeat("x", 2200)
	notes := []Limitation{}
	dto := adapterWithout(adapterPatch(adapterBody(t, "test-selected"), Object{"status": secret, "details": secret}), "muted", "ignored")
	test, e := NormalizeTest(dto, "1", []string{secret}, &notes)
	adapterOK(t, e)
	adapterWant(t, test["rawStatus"], "[REDACTED]")
	adapterWant(t, test["details"], "[REDACTED]")
	adapterWant(t, test["result"], "unknown")
	adapterWant(t, test["muted"], nil)
	if !hasLimitation(notes, "TEST_FLAGS_UNAVAILABLE") {
		t.Fatal(notes)
	}
	page, e := NormalizeEvidencePage("tests", Object{"count": 1, "testOccurrence": []any{adapterWithout(adapterBody(t, "test-selected"), "muted", "ignored")}}, ReadRequest{RunID: "1", Count: 1, ScanLimit: 5000, Failed: true, MutedSet: true, Muted: false}, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, Int(page, "providerReturned"), 1)
	adapterWant(t, len(Objects(page["items"])), 0)
	if len(page["limitations"].([]Limitation)) == 0 {
		t.Fatal("unknown flags not reported")
	}
}
func TestAdapterLogTimestampOffsetAndInvalidDates(t *testing.T) {
	dto := adapterLogDTO(t)
	message := Objects(dto["messages"])[0]
	tail, e := NormalizeLogTail(adapterPatch(dto, Object{"messages": []any{adapterPatch(message, Object{"timestamp": "2026-10-02T12:13:30.123+0200"})}}), "1", 1, nil)
	adapterOK(t, e)
	adapterWant(t, Objects(tail["messages"])[0]["timestamp"], "2026-10-02T10:13:30.123Z")
	tail, e = NormalizeLogTail(adapterPatch(dto, Object{"messages": []any{adapterPatch(message, Object{"timestamp": "2026-02-30T12:13:30+0000"})}}), "1", 1, nil)
	adapterOK(t, e)
	adapterWant(t, Objects(tail["messages"])[0]["timestamp"], nil)
	if len(tail["limitations"].([]Limitation)) == 0 {
		t.Fatal("invalid timestamp not reported")
	}
	_, e = NormalizeLogTail(dto, "1", 0, nil)
	adapterError(t, e, "USAGE_ERROR")
}
func TestAdapterMalformedUnicodeVisibleAndJSONTOONEquivalent(t *testing.T) {
	malformed, e := DecodeJSON([]byte(`"\ud800broken\udfff valid \ud83d\ude00"`))
	adapterOK(t, e)
	expected := `\ud800broken\udfff valid 😀`
	problem, e := NormalizeProblem(adapterPatch(adapterBody(t, "problem-selected"), Object{"details": malformed}), "1", nil)
	adapterOK(t, e)
	notes := []Limitation{}
	test, e := NormalizeTest(adapterPatch(adapterBody(t, "test-selected"), Object{"name": malformed, "details": malformed}), "1", nil, &notes)
	adapterOK(t, e)
	dto := adapterLogDTO(t)
	tail, e := NormalizeLogTail(adapterPatch(dto, Object{"messages": []any{adapterPatch(Objects(dto["messages"])[0], Object{"text": malformed})}}), "1", 1, nil)
	adapterOK(t, e)
	adapterWant(t, problem["description"], expected)
	adapterWant(t, test["name"], expected)
	adapterWant(t, test["details"], expected)
	adapterWant(t, Objects(tail["messages"])[0]["text"], expected)
	for _, value := range []Object{problem, test, tail} {
		normalized, e := jsonValue(value)
		adapterOK(t, e)
		encoded, e := toon.MarshalString(normalized)
		adapterOK(t, e)
		var decoded any
		adapterOK(t, toon.UnmarshalString(encoded, &decoded))
		adapterWant(t, decoded, normalized)
	}
	_, e = Identity(malformed, false)
	adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
}
func TestAdapterActualChangesCommitRootAndFiles(t *testing.T) {
	q := ReadRequest{RunID: "9", Count: 1, ScanLimit: 5000}
	page, e := NormalizeChangePage(adapterBody(t, "changes-positive"), q, adapterTestServer, nil)
	adapterOK(t, e)
	item := Objects(page["items"])[0]
	adapterWant(t, item["id"], "3")
	adapterWant(t, item["vcsRootId"], "AxiContract_Git")
	adapterWant(t, page["position"], 1)
	adapterWant(t, page["hasMore"], true)
	if !strings.Contains(Str(item, "message"), "\n") || !strings.HasSuffix(Str(item, "timestamp"), "Z") {
		t.Fatal(item)
	}
	if _, ok := item["files"]; ok {
		t.Fatal("unexpected files")
	}
	q.Count = 10
	q.Files = true
	page, e = NormalizeChangePage(adapterBody(t, "changes-files-positive"), q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, len(Objects(page["items"])), 3)
	item = Objects(page["items"])[0]
	adapterWant(t, item["files"], []string{"fixture.txt"})
	adapterWant(t, item["fileCoverage"], Object{"returned": 1, "providerReturned": 1, "omitted": 0})
	adapterWant(t, page["hasMore"], nil)
	adapterNotes(t, page, "SCAN_COVERAGE_UNKNOWN")
}
func TestAdapterChangesUnsafePagingMissingRootsAndBoundedRedactedFiles(t *testing.T) {
	q := ReadRequest{RunID: "9", Count: 1, ScanLimit: 5000}
	dto := adapterBody(t, "changes-positive")
	for _, href := range []any{42, nil, "https://attacker.invalid/app/rest/changes", "/app/rest/changes?locator=build:(id:1),count:1,start:1"} {
		page, e := NormalizeChangePage(adapterPatch(dto, Object{"nextHref": href}), q, adapterTestServer, nil)
		adapterOK(t, e)
		adapterWant(t, len(Objects(page["items"])), 1)
		adapterWant(t, page["position"], nil)
		adapterNotes(t, page, "UNSAFE_CONTINUATION")
	}
	change := Objects(dto["change"])[0]
	page, e := NormalizeChangePage(adapterPatch(dto, Object{"change": []any{adapterPatch(change, Object{"vcsRootInstance": nil})}}), q, adapterTestServer, nil)
	adapterOK(t, e)
	adapterWant(t, len(Objects(page["items"])), 0)
	adapterWant(t, Int(page, "providerReturned"), 1)
	adapterNotes(t, page, "CHANGE_ROOT_UNAVAILABLE")
	secret := "synthetic-secret-" + strings.Repeat("x", 2200)
	files := []any{}
	for i := 0; i < 101; i++ {
		files = append(files, Object{"file": secret})
	}
	q.Files = true
	page, e = NormalizeChangePage(adapterPatch(dto, Object{"change": []any{adapterPatch(change, Object{"comment": secret, "files": Object{"count": 101, "file": files}})}}), q, adapterTestServer, []string{secret})
	adapterOK(t, e)
	item := Objects(page["items"])[0]
	adapterWant(t, item["message"], "[REDACTED]")
	names := Strings(item["files"])
	adapterWant(t, len(names), 100)
	adapterWant(t, names[0], "[REDACTED]")
	adapterWant(t, Int(Obj(item["fileCoverage"]), "omitted"), 1)
	adapterNotes(t, page, "CHANGE_FILE_LIMIT")
	_, e = NormalizeChangePage(adapterPatch(dto, Object{"count": 2}), q, "http://a.invalid", nil)
	adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
}
func TestAdapterSnapshotDirectionScopedCountsAndUnknownExhaustion(t *testing.T) {
	q := ReadRequest{RunID: "9", Count: 20, ScanLimit: 5000}
	dto := adapterBody(t, "dependencies-positive")
	page, e := NormalizeDependencyPage(dto, q, adapterTestServer, RunDetailFields, nil)
	adapterOK(t, e)
	item := Objects(page["items"])[0]
	adapterWant(t, Str(Obj(item["run"]), "id"), "8")
	adapterWant(t, item["projectId"], "AxiContract")
	adapterWant(t, page["hasMore"], nil)
	count, e := NormalizeDependencyCount(adapterBody(t, "dependency-count-positive"), "9")
	adapterOK(t, e)
	adapterWant(t, count, 1)
	count, e = NormalizeDependencyCount(adapterBody(t, "dependency-count-leaf"), "8")
	adapterOK(t, e)
	adapterWant(t, count, 0)
	_, e = NormalizeDependencyCount(adapterBody(t, "dependency-count-leaf"), "9")
	adapterError(t, e, "CONTEXT_MISMATCH")
	_, e = NormalizeDependencyCount(Object{"id": 9, "snapshot-dependencies": Object{}}, "9")
	adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
	builds := adapterArray(dto["build"])
	_, e = NormalizeDependencyPage(adapterPatch(dto, Object{"count": 2, "build": append(append([]any{}, builds...), builds...)}), q, "http://a.invalid", RunDetailFields, nil)
	adapterError(t, e, "UPSTREAM_SCHEMA_MISMATCH")
	r, e := RelatedRequest("dependencies", q, RunDetailFields)
	adapterOK(t, e)
	if !contains(r.Filters, "snapshotDependency:(to:(id:9),recursive:false)") {
		t.Fatal(r.Filters)
	}
}
