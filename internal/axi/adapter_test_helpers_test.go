package axi

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

const adapterTestServer = "http://127.0.0.1:32768"

type adapterRecorded struct {
	Stdout string   `json:"stdout"`
	Stderr string   `json:"stderr"`
	Code   int      `json:"code"`
	Signal *string  `json:"signal"`
	Args   []string `json:"args"`
}
type adapterContract struct {
	Records map[string]adapterRecorded `json:"records"`
	Fixture Object                     `json:"fixture"`
}

func adapterFixture(t *testing.T, old bool) adapterContract {
	t.Helper()
	name := "teamcity-2026.2-native-1.5.0"
	if old {
		name = "native-v1.5.0"
	}
	raw, e := os.ReadFile("../../tests/fixtures/" + name + "/contract.json")
	if e != nil {
		t.Fatal(e)
	}
	var c adapterContract
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	return c
}
func adapterCapture(t *testing.T, name string, old bool) Captured {
	t.Helper()
	r, present := adapterFixture(t, old).Records[name]
	if !present {
		t.Fatalf("missing required capture %s", name)
	}
	signal := ""
	if r.Signal != nil {
		signal = *r.Signal
	}
	return Captured{Stdout: []byte(r.Stdout), Stderr: []byte(r.Stderr), ExitCode: r.Code, Signal: signal}
}
func adapterBody(t *testing.T, name string) Object {
	t.Helper()
	r, e := ParseRaw(adapterCapture(t, name, false))
	if e != nil {
		t.Fatal(e)
	}
	return Obj(r.Body)
}
func adapterBaseRun() Object {
	return Object{"id": 482193, "buildTypeId": "Payments_Build", "number": "42", "state": "finished", "status": "FAILURE", "failedToStart": false, "branchName": "feature/refund", "statusText": "Tests failed", "personal": false, "composite": false, "buildType": Object{"id": "Payments_Build", "name": "Build", "projectId": "Payments"}, "revisions": Object{"count": 1, "revision": []any{Object{"version": strings.Repeat("a", 40), "vcs-root-instance": Object{"id": "17", "vcs-root-id": "Payments_Git"}}}}, "startDate": "20261001T140000+0000", "finishDate": "20261001T140100+0000"}
}
func adapterPatch(base, patch Object) Object {
	result := Object{}
	for k, v := range base {
		result[k] = v
	}
	for k, v := range patch {
		result[k] = v
	}
	return result
}
func adapterWithout(base Object, keys ...string) Object {
	result := adapterPatch(base, Object{})
	for _, k := range keys {
		delete(result, k)
	}
	return result
}
func adapterWant(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}
func adapterError(t *testing.T, e error, code string) {
	t.Helper()
	if e == nil {
		t.Fatal("expected failure")
	}
	if code != "" && AsDomainError(e).Code != code {
		t.Fatalf("got %v (%s); want %s", e, AsDomainError(e).Code, code)
	}
}
func adapterOK(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func adapterNotes(t *testing.T, page Object, code string) {
	t.Helper()
	if !hasLimitation(page["limitations"].([]Limitation), code) {
		t.Fatalf("missing %s: %+v", code, page["limitations"])
	}
}
func adapterPageQuery() ReadRequest {
	return ReadRequest{ProjectID: "AxiContract", Count: 1, Start: 0, ScanLimit: 5000}
}
func adapterBudget() Budget {
	return Budget{Deadline: time.Now().Add(time.Minute).UnixMilli(), MaxChildProcesses: -1}
}

type adapterFakeTransport struct {
	execute func(Operation) (Captured, error)
	calls   []Operation
}

func (f *adapterFakeTransport) Execute(_ context.Context, op Operation, _ int) (Captured, error) {
	f.calls = append(f.calls, op)
	return f.execute(op)
}
func adapterRawJSON(value any) Captured {
	raw, _ := json.Marshal(value)
	return Captured{Stdout: append([]byte("HTTP/1.1 200 OK\nContent-Type: application/json\n\n"), raw...)}
}
