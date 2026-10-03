package axi

import (
	"regexp"
	"strconv"
)

const ProblemFields = "id,type,identity,details,build(id)"
const TestFields = "id,name,status,duration,muted,ignored,details,build(id),test(id)"

var problemOccurrence = regexp.MustCompile(`^build:\(id:([1-9][0-9]*)\),problem:\(id:([1-9][0-9]*)\)$`)
var testOccurrence = regexp.MustCompile(`^build:\(id:([1-9][0-9]*)\),id:([0-9]+)$`)

func OccurrenceLocator(kind, id, runID string) (result string, err error) {
	defer adapterRecover(&err)
	pattern := testOccurrence
	if kind == "problems" {
		pattern = problemOccurrence
	}
	m := pattern.FindStringSubmatch(id)
	if m == nil {
		adapterFail("USAGE_ERROR", "Use an exact supported occurrence ID, not a test definition or raw locator", 2)
	}
	for _, s := range m[1:] {
		n, e := strconv.ParseInt(s, 10, 64)
		if e != nil || n > maxSafeInteger {
			adapterFail("USAGE_ERROR", "Use an exact supported occurrence ID, not a test definition or raw locator", 2)
		}
	}
	if m[1] != runID {
		adapterFail("CONTEXT_MISMATCH", "Occurrence ID belongs to a different execution", 1)
	}
	if kind == "problems" {
		return "build:(id:" + runID + "),problem:(id:" + m[2] + ")", nil
	}
	return "build:(id:" + runID + "),id:" + m[2], nil
}
func occurrence(value any, kind, runID string) Object {
	dto := adapterObject(value)
	id := adapterIdentity(dto["id"], false)
	if adapterIdentity(adapterObject(dto["build"])["id"], true) != runID {
		adapterFail("CONTEXT_MISMATCH", "Evidence belongs to a different execution", 1)
	}
	if _, e := OccurrenceLocator(kind, id, runID); e != nil {
		if AsDomainError(e).Code == "CONTEXT_MISMATCH" {
			panic(AsDomainError(e))
		}
		adapterInvalid("Unsupported upstream occurrence identity")
	}
	return dto
}
func normalizeProblem(value any, runID string, secrets []string) Object {
	dto := occurrence(value, "problems", runID)
	item := Object{"id": adapterIdentity(dto["id"], false), "runId": runID, "type": SanitizeText(adapterIdentity(dto["type"], false), secrets), "description": SanitizeText(adapterString(dto["details"]), secrets)}
	if v, present := dto["identity"]; present {
		item["identity"] = SanitizeText(adapterString(v), secrets)
	}
	return item
}
func NormalizeProblem(value any, runID string, secrets []string) (result Object, err error) {
	defer adapterRecover(&err)
	return normalizeProblem(value, runID, secrets), nil
}
func normalizeTest(value any, runID string, secrets []string, notes *[]Limitation) Object {
	dto := occurrence(value, "tests", runID)
	status := adapterString(dto["status"])
	muted, ignored := adapterFlag(dto["muted"]), adapterFlag(dto["ignored"])
	var duration any
	if d, present := dto["duration"]; present {
		n, ok := adapterNumber(d)
		if !ok || n < 0 {
			adapterInvalid("Invalid independent evidence response")
		}
		duration = n
	}
	result := "unknown"
	if ignored == true || status == "IGNORED" {
		result = "ignored"
	} else if status == "SUCCESS" {
		result = "success"
	} else if status == "FAILURE" {
		result = "failure"
	}
	if result == "unknown" {
		adapterNote(notes, "UNKNOWN_TEST_RESULT", "Test occurrence has an unknown outcome", "tests", runID)
	}
	if muted == nil || ignored == nil {
		adapterNote(notes, "TEST_FLAGS_UNAVAILABLE", "Muted or ignored state was not supplied", "tests", runID)
	}
	item := Object{"id": adapterIdentity(dto["id"], false), "runId": runID, "name": SanitizeText(adapterString(dto["name"]), secrets), "result": result, "muted": muted, "ignored": ignored, "durationMs": duration}
	if v, present := dto["test"]; present {
		item["testId"] = adapterIdentity(adapterObject(v)["id"], false)
	}
	if v, present := dto["details"]; present {
		item["details"] = SanitizeText(adapterString(v), secrets)
	}
	if result == "unknown" {
		item["rawStatus"] = adapterPreview(SanitizeText(status, secrets), 80)
	}
	return item
}
func NormalizeTest(value any, runID string, secrets []string, notes *[]Limitation) (result Object, err error) {
	defer adapterRecover(&err)
	return normalizeTest(value, runID, secrets, notes), nil
}
func EvidenceRequest(kind string, q ReadRequest) (result AdapterRequest, err error) {
	defer adapterRecover(&err)
	runID := adapterIdentity(q.RunID, true)
	boundedQuery(q, "Invalid bounded evidence query")
	if kind == "problems" && (q.Failed || q.FailedSet || q.Muted || q.MutedSet) {
		adapterFail("USAGE_ERROR", "Problem pages do not support test filters", 2)
	}
	filters := []string{"build:(id:" + runID + ")"}
	if q.Failed {
		filters = append(filters, "status:FAILURE")
	}
	if q.MutedSet || q.Muted {
		filters = append(filters, "muted:"+strconv.FormatBool(q.Muted))
	}
	resource, field, item := "testOccurrences", TestFields, "testOccurrence"
	if kind == "problems" {
		resource, field, item = "problemOccurrences", ProblemFields, "problemOccurrence"
	}
	return pageRequest(resource, filters, "count,nextHref,"+item+"("+field+")", q), nil
}
func NormalizeEvidencePage(kind string, value any, q ReadRequest, server string, secrets []string) (result Object, err error) {
	defer adapterRecover(&err)
	dto := adapterObject(value)
	key := "testOccurrence"
	if kind == "problems" {
		key = "problemOccurrence"
	}
	rows := adapterRows(dto, key, q.Count)
	r, e := EvidenceRequest(kind, q)
	if e != nil {
		panic(AsDomainError(e))
	}
	notes := []Limitation{}
	items := []Object{}
	ids := map[string]bool{}
	for _, row := range rows {
		var item Object
		if kind == "problems" {
			item = normalizeProblem(row, q.RunID, secrets)
		} else {
			item = normalizeTest(row, q.RunID, secrets, &notes)
		}
		id := Str(item, "id")
		if ids[id] {
			adapterInvalid("Duplicate occurrence ID in one page")
		}
		ids[id] = true
		if kind == "tests" {
			if q.Failed && Str(item, "result") != "failure" || (q.MutedSet || q.Muted) && item["muted"] != nil && item["muted"] != q.Muted {
				adapterFail("CONTEXT_MISMATCH", "Test page does not match declared filters", 1)
			}
			if (q.MutedSet || q.Muted) && item["muted"] == nil {
				continue
			}
		}
		items = append(items, item)
	}
	page := adapterPage(items, len(rows), notes)
	adapterContinuation(dto, page, r, q, server, kind, "Occurrence rows retained; unsafe continuation was not followed", "No continuation returned; bounded lookup does not prove complete retained evidence")
	for i, n := range page["limitations"].([]Limitation) {
		if n.Code == "UNSAFE_CONTINUATION" || n.Code == "SCAN_COVERAGE_UNKNOWN" {
			n.RunID = q.RunID
			page["limitations"].([]Limitation)[i] = n
		}
	}
	return page, nil
}
