package axi

import (
	"net/url"
	"strings"
	"testing"
)

func TestAdapterRecordedPreciseNativeDatePredicates(t *testing.T) {
	contract := adapterFixture(t, false)
	for _, name := range []string{"date-millisecond-exact-before", "date-millisecond-exact-after", "date-build-exact-before", "date-build-exact-after"} {
		adapterWant(t, len(Objects(adapterBody(t, name)["build"])), 0)
	}
	precision := Obj(contract.Fixture["listPrecision"])
	for _, name := range []string{"date-millisecond-next-before", "date-millisecond-previous-after"} {
		rows := Objects(adapterBody(t, name)["build"])
		if len(rows) == 0 {
			t.Fatalf("%s lacks the recorded precision execution", name)
		}
		id, err := Identity(rows[0]["id"], true)
		adapterOK(t, err)
		adapterWant(t, id, Str(precision, "runId"))
	}
	query := ReadRequest{JobID: Str(precision, "jobId"), ProjectID: Str(contract.Fixture, "projectId"), State: "finished", Count: 20, ScanLimit: 5000, Window: &TimeWindow{Since: Str(precision, "lowerExclusive"), Until: Str(precision, "upperExclusive")}}
	fields := "count,nextHref,build(" + RunDetailFields + ")"
	filters, err := RunFilters(query)
	adapterOK(t, err)
	record, present := contract.Records["fractional-millisecond-page-positive"]
	if !present || len(record.Args) < 2 {
		t.Fatal("missing required native fractional page capture")
	}
	path, err := url.Parse(record.Args[1])
	adapterOK(t, err)
	adapterWant(t, path.Query().Get("fields"), fields)
	adapterWant(t, path.Query().Get("locator"), strings.Join(append(append([]string{}, filters...), "count:20", "start:0", "lookupLimit:5000"), ","))
	page, err := NormalizeRunPage(adapterBody(t, "fractional-millisecond-page-positive"), query, ContinuationRequest{ServerURL: adapterTestServer, Resource: "builds", Filters: filters, Fields: fields, Count: 20, ScanLimit: 5000}, nil)
	adapterOK(t, err)
	runs := Objects(page["runs"])
	if len(runs) == 0 {
		t.Fatal("positive recorded fractional window lost its execution")
	}
	adapterWant(t, runs[0]["id"], precision["runId"])
	adapterWant(t, runs[0]["finishedAt"], precision["finish"])
	if runs[0]["finishedAt"] == precision["exactFinish"] {
		t.Fatal("normalization fabricated milliseconds absent from the reported DTO")
	}
	for _, name := range []string{"fractional-millisecond-page-lower-excluded", "fractional-millisecond-page-upper-excluded"} {
		adapterWant(t, len(Objects(adapterBody(t, name)["build"])), 0)
	}
}
