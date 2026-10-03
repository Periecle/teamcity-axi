//go:build realcli

package axi

import (
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func nativeJobList(extra ...string) []string {
	return append([]string{"job", "list", "--project", "Payments"}, extra...)
}
func nativeJobView(extra ...string) []string {
	return append([]string{"job", "view", "Payments_Build"}, extra...)
}
func nativeQueue(extra ...string) []string {
	return append([]string{"queue", "list", "--job", "Payments_Build"}, extra...)
}
func nativeAgentList(extra ...string) []string {
	return append([]string{"agent", "list", "--project", "Payments"}, extra...)
}
func nativeAgentView(extra ...string) []string {
	return append([]string{"agent", "view", "7"}, extra...)
}
func nativeCursor(t *testing.T, r nativeCall) Cursor {
	t.Helper()
	c, err := DecodeCursor(Str(Obj(nativeData(r)["page"]), "cursor"), time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func nativeToken(t *testing.T, c Cursor) string {
	t.Helper()
	s, err := EncodeCursor(c, time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Original: jobs.test.mjs — released CLI job views and scoped pages expose only safe metadata with typed continuation.
func TestRealCLIJobMetadataAndPaging(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("jobs-page")
	var prev any
	for _, flags := range nativeFormats() {
		r := f.call(t, nativeJobList(flags...)...)
		requireNativeCode(t, r, 0)
		jobs := Objects(nativeData(r)["jobs"])
		requireNativeValue(t, nativeData(r)["jobs"], []Object{{"id": "Payments_Build", "name": "Build", "projectId": "Payments", "paused": false}})
		requireNativeValue(t, Obj(nativeData(r)["selection"])["membership"], "direct")
		requireNativeValue(t, Obj(nativeData(r)["page"])["total"], nil)
		if len(jobs) != 1 || Str(jobs[0], "id") != "Payments_Build" || Obj(nativeData(r)["page"])["hasMore"] != true || Int(Obj(nativeMeta(r)["counts"]), "childProcesses") != 3 {
			t.Fatal("job page incorrect")
		}
		if prev != nil && !reflect.DeepEqual(prev, nativeData(r)["jobs"]) {
			t.Fatal("formats disagree")
		}
		prev = nativeData(r)["jobs"]
		hint := Strings(Objects(r.value["next"])[0]["argv"])
		next := f.call(t, append(hint[1:], "--json")...)
		if Int(Obj(nativeData(next)["selection"]), "position") != 20 {
			t.Fatal("cursor position lost")
		}
	}
	first := f.call(t, nativeJobList("--json")...)
	r := f.call(t, nativeJobView("--project", "Payments", "--json")...)
	if !reflect.DeepEqual(Obj(nativeData(r)["job"]), Objects(nativeData(first)["jobs"])[0]) {
		t.Fatal("detail disagrees with page")
	}
	f.mock.SetMode("jobs-empty-next")
	r = f.call(t, nativeJobList("--cursor", Str(Obj(nativeData(first)["page"]), "cursor"), "--json")...)
	if Obj(nativeData(r)["page"])["hasMore"] != true || Int(Obj(nativeData(r)["page"]), "returned") != 0 {
		t.Fatal("empty continuation lost")
	}
	f.mock.SetMode("jobs-empty")
	requireNativeCode(t, f.call(t, nativeJobList("--require-complete", "--json")...), 1)
	f.mock.SetMode("jobs-leading-id")
	r = f.call(t, nativeJobList("--json")...)
	hint := Strings(Objects(r.value["next"])[0]["argv"])
	expanded := f.call(t, append(hint[1:], "--json")...)
	if Str(Obj(nativeData(expanded)["job"]), "id") != "--job" {
		t.Fatalf("dash identity reinterpreted: %s", expanded.stdout)
	}
	requireNativeReadOnly(t, f.mock)

	f.mock.SetMode("jobs-page")
	exactPage := f.call(t, nativeJobList("--json")...)
	requireNativeValue(t, nativeData(exactPage)["jobs"], []Object{{"id": "Payments_Build", "name": "Build", "projectId": "Payments", "paused": false}})
	requireNativeValue(t, Obj(nativeData(exactPage)["selection"])["membership"], "direct")
	requireNativeValue(t, Obj(nativeData(exactPage)["page"])["total"], nil)
	exactView := f.call(t, nativeJobView("--project", "Payments", "--json")...)
	requireNativeCode(t, exactView, 0)
	requireNativeValue(t, Obj(nativeMeta(exactView)["counts"])["childProcesses"], 2)
	f.mock.SetMode("jobs-empty-next")
	empty := f.call(t, nativeJobList("--cursor", Str(Obj(nativeData(exactPage)["page"]), "cursor"), "--json")...)
	if Str(Obj(nativeData(empty)["page"]), "cursor") == "" {
		t.Fatal("empty job page dropped continuation")
	}
	f.mock.SetMode("jobs-empty")
	unknown := f.call(t, nativeJobList("--require-complete", "--json")...)
	requireNativePartial(t, unknown)
	requireNativeValue(t, Obj(nativeData(unknown)["page"])["hasMore"], nil)
	requireNativeValue(t, Obj(nativeData(unknown)["page"])["total"], nil)
	f.mock.SetMode("jobs-leading-id")
	leading := f.call(t, nativeJobList("--json")...)
	requireNativeValue(t, Objects(nativeData(leading)["jobs"])[0]["id"], "--job")
}

// Original: jobs.test.mjs — job permission, identity, policy, capability and schema errors never become empty success.
func TestRealCLIJobFailures(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, c := range []struct {
		mode, code string
		view       bool
	}{{"jobs-denied", "PERMISSION_DENIED", false}, {"jobs-unsupported", "NOT_FOUND", false}, {"jobs-foreign", "CONTEXT_MISMATCH", false}, {"jobs-duplicate", "UPSTREAM_SCHEMA_MISMATCH", false}, {"jobs-malformed", "UPSTREAM_SCHEMA_MISMATCH", false}, {"jobs-detail-denied", "PERMISSION_DENIED", true}, {"jobs-detail-missing", "NOT_FOUND", true}, {"jobs-wrong-id", "CONTEXT_MISMATCH", true}, {"jobs-foreign", "POLICY_DENIED", true}} {
		f.mock.SetMode(c.mode)
		args := nativeJobList("--json")
		if c.view {
			args = nativeJobView("--json")
		}
		requireNativeError(t, f.call(t, args...), c.code)
	}
	f.mock.SetMode("jobs-page")
	requireNativeError(t, f.call(t, nativeJobView("--project", "Other", "--json")...), "CONTEXT_MISMATCH")
	for _, args := range [][]string{{"job", "list", "--json"}, {"job", "view", strings.Repeat("x", 257), "--json"}, nativeJobList("--limit", "101", "--json"), {"job", "list", "--project", strings.Repeat("x", 257), "--json"}, {"job", "list", "--project", "invalid\nproject", "--json"}} {
		f.mock.ClearRequests()
		requireNativeCode(t, f.call(t, args...), 2)
		if len(f.mock.Requests()) != 0 {
			t.Fatal("invalid selector launched HTTP")
		}
	}
}

// Original: jobs.test.mjs — job cursor reconstruction rejects scope changes and preserves rows after unsafe continuation.
func TestRealCLIJobCursorBinding(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, mode := range []string{"jobs-unsafe", "jobs-escalating", "jobs-scope-change"} {
		f.mock.SetMode(mode)
		r := f.call(t, nativeJobList("--require-complete", "--json")...)
		requireNativeCode(t, r, 1)
		if len(Objects(nativeData(r)["jobs"])) != 1 || !nativeNotes(r, "UNSAFE_CONTINUATION") || Obj(nativeData(r)["page"])["cursor"] != nil {
			t.Fatal("unsafe jobs discarded")
		}
	}
	f.mock.SetMode("jobs-page")
	first := f.call(t, nativeJobList("--json")...)
	cursor := nativeCursor(t, first)
	for _, mode := range []string{"hash", "command", "malformed", "size"} {
		c := cursor
		token := Str(Obj(nativeData(first)["page"]), "cursor")
		args := nativeJobList()
		switch mode {
		case "hash":
			c.FilterHash = strings.Repeat("0", 64)
			token = nativeToken(t, c)
		case "command":
			c.Command = "run.list"
			token = nativeToken(t, c)
		case "malformed":
			token = "invalid-cursor"
		case "size":
			args = append(args, "--limit", "1")
		}
		f.mock.ClearRequests()
		requireNativeCode(t, f.call(t, append(args, "--cursor", token, "--json")...), 2)
		if len(f.mock.Requests()) != 0 {
			t.Fatal("invalid cursor made request")
		}
	}
	r := f.call(t, nativeJobList("--no-hints", "--json")...)
	if r.value["next"] != nil {
		t.Fatal("no-hints ignored")
	}

	for _, mode := range []string{"jobs-unsafe", "jobs-escalating", "jobs-scope-change"} {
		f.mock.SetMode(mode)
		requireNativePartial(t, f.call(t, nativeJobList("--require-complete", "--json")...))
	}
	f.mock.SetMode("jobs-page")
	for _, mode := range []string{"hash", "command", "malformed"} {
		c := cursor
		token := "invalid-cursor"
		if mode == "hash" {
			c.FilterHash = strings.Repeat("0", 64)
			token = nativeToken(t, c)
		}
		if mode == "command" {
			c.Command = "run.list"
			token = nativeToken(t, c)
		}
		f.mock.ClearRequests()
		rejected := f.call(t, nativeJobList("--cursor", token, "--json")...)
		requireNativeValue(t, Obj(rejected.value["error"])["code"], "USAGE_ERROR")
		if len(f.mock.Requests()) != 0 {
			t.Fatal("forged job cursor launched HTTP")
		}
	}
}

// Original: jobs.test.mjs — unknown paused state, redacted names and oversized job output remain explicit in both serializers.
func TestRealCLIJobUnknownAndLimits(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, flags := range nativeFormats() {
		f.mock.SetMode("jobs-unknown-paused")
		r := f.call(t, nativeJobView(append(flags, "--require-complete")...)...)
		requireNativeCode(t, r, 1)
		requireNativePartial(t, r)
		if Obj(nativeData(r)["job"])["paused"] != nil || !nativeNotes(r, "JOB_PAUSED_UNAVAILABLE") {
			t.Fatal("paused omission hidden")
		}
		f.mock.SetMode("jobs-secret")
		for _, args := range [][]string{nativeJobList(flags...), nativeJobView(flags...)} {
			r = f.call(t, args...)
			requireNativeCode(t, r, 0)
			job := Obj(nativeData(r)["job"])
			if job == nil {
				job = Objects(nativeData(r)["jobs"])[0]
			}
			if Str(job, "name") != "[REDACTED]" {
				t.Fatal("job name unredacted")
			}
		}
		f.mock.SetMode("jobs-huge")
		requireNativeError(t, f.call(t, nativeJobList(flags...)...), "INPUT_LIMIT_EXCEEDED")
		f.mock.SetMode("jobs-oversized")
		r = f.call(t, nativeJobList(append([]string{"--limit", "100", "--max-bytes", "2048"}, flags...)...)...)
		requireNativeError(t, r, "INPUT_LIMIT_EXCEEDED")
		if len(r.stdout) > 2048 {
			t.Fatal("byte cap exceeded")
		}
	}
	f.mock.SetMode("jobs-all-unknown")
	r := f.call(t, nativeJobList("--limit", "100", "--max-bytes", "65536", "--json")...)
	requireNativeCode(t, r, 0)
	if len(Objects(nativeData(r)["jobs"])) != 100 || len(Objects(nativeMeta(r)["limitations"])) > 5 {
		t.Fatal("unknown paused grouping lost")
	}

	for _, flags := range nativeFormats() {
		f.mock.SetMode("jobs-unknown-paused")
		requireNativePartial(t, f.call(t, nativeJobView(append(flags, "--require-complete")...)...))
		f.mock.SetMode("jobs-oversized")
		large := f.call(t, nativeJobList(append([]string{"--limit", "100", "--max-bytes", "2048"}, flags...)...)...)
		found := false
		for _, note := range Objects(nativeMeta(large)["limitations"]) {
			if note["code"] == "OUTPUT_LIMIT_EXCEEDED" && note["source"] == "output" {
				found = true
			}
		}
		if !found {
			t.Fatal("job output ceiling lacks output-source limitation")
		}
	}
	f.mock.SetMode("jobs-all-unknown")
	unknown := f.call(t, nativeJobList("--limit", "100", "--require-complete", "--json")...)
	requireNativeCode(t, unknown, 1)
	requireNativePartial(t, unknown)
	requireNativeValue(t, len(Objects(nativeData(unknown)["jobs"])), 100)
	for _, job := range Objects(nativeData(unknown)["jobs"]) {
		requireNativeValue(t, job["paused"], nil)
	}
	requireNativeValue(t, nativeNoteCount(unknown, "JOB_PAUSED_UNAVAILABLE"), 1)
	for _, name := range []string{"job.list", "job.view"} {
		f.mock.ClearRequests()
		schema := f.call(t, "schema", name, "--json")
		requireNativeCode(t, schema, 0)
		requireNativeValue(t, Obj(nativeData(schema)["payload"])["$id"], "urn:teamcity-axi:"+strings.ReplaceAll(name, ".", "-")+":1.0")
		requireNativeValue(t, len(f.mock.Requests()), 0)
	}
}

// Original: queue.test.mjs — released CLI scoped queue pages preserve observed state/reason, nullable totals and query-bound hints.
func TestRealCLIQueuePaging(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("queue-page")
	for _, flags := range nativeFormats() {
		r := f.call(t, nativeQueue(flags...)...)
		requireNativeCode(t, r, 0)
		items := Objects(nativeData(r)["items"])
		if len(items) != 1 || Str(items[0], "id") != "482194" || Str(items[0], "state") != "queued" || Str(items[0], "queuedAt") != "2026-10-01T14:00:00.000Z" || Obj(nativeData(r)["page"])["total"] != nil {
			t.Fatal("queue state lost")
		}
		next := f.call(t, append(Strings(Objects(r.value["next"])[0]["argv"])[1:], "--json")...)
		if Int(Obj(nativeData(next)["selection"]), "position") != 20 {
			t.Fatal("queue position lost")
		}
	}
	first := f.call(t, nativeQueue("--json")...)
	f.mock.SetMode("queue-empty-next")
	r := f.call(t, nativeQueue("--cursor", Str(Obj(nativeData(first)["page"]), "cursor"), "--json")...)
	if len(Objects(nativeData(r)["items"])) != 0 || Obj(nativeData(r)["page"])["hasMore"] != true {
		t.Fatal("queue empty continuation lost")
	}
	f.mock.SetMode("queue-empty")
	requireNativeCode(t, f.call(t, nativeQueue("--require-complete", "--json")...), 1)
	f.mock.SetMode("queue-no-reason")
	r = f.call(t, nativeQueue("--json")...)
	if Objects(nativeData(r)["items"])[0]["waitReason"] != nil {
		t.Fatal("omitted wait reason fabricated")
	}
	expanded := f.call(t, append(Strings(Objects(r.value["next"])[0]["argv"])[1:], "--json")...)
	if nativeRun(expanded)["id"] != "482194" || nativeRun(expanded)["state"] != "queued" || nativeRun(expanded)["result"] != "unknown" {
		t.Fatal("queued view hint changed identity")
	}
	requireNativeReadOnly(t, f.mock)

	f.mock.SetMode("queue-page")
	var rendered any
	for _, flags := range nativeFormats() {
		page := f.call(t, nativeQueue(flags...)...)
		requireNativeValue(t, nativeData(page)["items"], []Object{{"id": "482194", "jobId": "Payments_Build", "state": "queued", "branch": "feature/refund", "queuedAt": "2026-10-01T14:00:00.000Z", "waitReason": "Waiting for compatible agent"}})
		requireNativeValue(t, Obj(page.value["context"])["project"], "Payments")
		requireNativeValue(t, Obj(nativeData(page)["page"])["hasMore"], true)
		requireNativeValue(t, Obj(nativeMeta(page)["counts"])["childProcesses"], 3)
		if rendered != nil {
			requireNativeValue(t, nativeData(page)["items"], rendered)
		}
		rendered = nativeData(page)["items"]
	}
	scoped := f.call(t, "queue", "list", "--project", "Payments", "--json")
	requireNativeValue(t, Obj(nativeData(scoped)["selection"])["jobId"], nil)
	f.mock.SetMode("queue-empty")
	bounded := f.call(t, nativeQueue("--require-complete", "--json")...)
	requireNativePartial(t, bounded)
	requireNativeValue(t, Obj(nativeData(bounded)["page"])["hasMore"], nil)
	requireNativeValue(t, Obj(nativeData(bounded)["page"])["total"], nil)
	f.mock.SetMode("queue-no-reason")
	observed := f.call(t, nativeQueue("--json")...)
	detail := f.call(t, append(Strings(Objects(observed.value["next"])[0]["argv"])[1:], "--json")...)
	requireNativeCode(t, detail, 0)
	requireNativeValue(t, nativeRun(detail)["rawStatus"], nil)
	if !nativeNotes(detail, "RESULT_UNAVAILABLE") {
		t.Fatal("queued run missing explicit unknown-result limitation")
	}
}

// Original: queue.test.mjs — queue permission/capability/scope/schema failures remain errors; unsafe continuation keeps useful rows.
func TestRealCLIQueueFailures(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, c := range []struct{ mode, code string }{{"queue-denied", "PERMISSION_DENIED"}, {"queue-unsupported", "NOT_FOUND"}, {"queue-foreign", "CONTEXT_MISMATCH"}, {"queue-wrong-job", "CONTEXT_MISMATCH"}, {"queue-conflict", "UPSTREAM_SCHEMA_MISMATCH"}, {"queue-surrogate-id", "UPSTREAM_SCHEMA_MISMATCH"}, {"queue-control-id", "UPSTREAM_SCHEMA_MISMATCH"}, {"queue-bidi-id", "UPSTREAM_SCHEMA_MISMATCH"}, {"queue-duplicate", "UPSTREAM_SCHEMA_MISMATCH"}, {"queue-malformed", "UPSTREAM_SCHEMA_MISMATCH"}} {
		f.mock.SetMode(c.mode)
		requireNativeError(t, f.call(t, nativeQueue("--json")...), c.code)
	}
	for _, mode := range []string{"queue-unsafe", "queue-unsafe-scope", "queue-escalating"} {
		f.mock.SetMode(mode)
		r := f.call(t, nativeQueue("--require-complete", "--json")...)
		requireNativeCode(t, r, 1)
		if len(Objects(nativeData(r)["items"])) != 1 || !nativeNotes(r, "UNSAFE_CONTINUATION") {
			t.Fatal("unsafe queue row lost")
		}
	}

	for _, mode := range []string{"queue-surrogate-id", "queue-control-id", "queue-bidi-id"} {
		f.mock.SetMode(mode)
		bad := f.call(t, "queue", "list", "--project", "Payments", "--json")
		requireNativeError(t, bad, "UPSTREAM_SCHEMA_MISMATCH")
		if strings.Contains(bad.stdout, "bad") {
			t.Fatal("malformed identity disclosed")
		}
	}
	for _, mode := range []string{"queue-unsafe", "queue-escalating", "queue-unsafe-scope"} {
		f.mock.SetMode(mode)
		unsafe := f.call(t, nativeQueue("--require-complete", "--json")...)
		requireNativePartial(t, unsafe)
		requireNativeValue(t, Obj(nativeData(unsafe)["page"])["cursor"], nil)
	}
	f.mock.SetMode("queue-page")
	f.mock.ClearRequests()
	requireNativeCode(t, f.call(t, "queue", "list", "--json"), 2)
	requireNativeValue(t, len(f.mock.Requests()), 0)
	for _, scope := range []string{strings.Repeat("x", 257), "bad\nproject", "bad\u0085project", "bad\u202eproject"} {
		f.mock.ClearRequests()
		requireNativeCode(t, f.call(t, "queue", "list", "--project", scope, "--json"), 2)
		requireNativeValue(t, len(f.mock.Requests()), 0)
	}
	requireNativeError(t, f.call(t, nativeQueue("--project", "Other", "--json")...), "CONTEXT_MISMATCH")
}

// Original: queue.test.mjs — queue cursors bind page size, command, project, resolved job scope and reject malformed tokens locally.
func TestRealCLIQueueCursorBinding(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("queue-page")
	first := f.call(t, nativeQueue("--json")...)
	cursor := nativeCursor(t, first)
	for _, mode := range []string{"hash", "command", "size", "project", "malformed"} {
		c := cursor
		token := Str(Obj(nativeData(first)["page"]), "cursor")
		args := nativeQueue()
		switch mode {
		case "hash":
			c.FilterHash = strings.Repeat("0", 64)
			token = nativeToken(t, c)
		case "command":
			c.Command = "job.list"
			token = nativeToken(t, c)
		case "size":
			args = append(args, "--limit", "1")
		case "project":
			args = append(args, "--project", "Other")
		case "malformed":
			token = "bad-token"
		}
		r := f.call(t, append(args, "--cursor", token, "--json")...)
		if r.code == 0 {
			t.Fatal("queue query rebound")
		}
	}
	f.mock.ClearRequests()
	requireNativeCode(t, f.call(t, "queue", "list", "--json"), 2)
	if len(f.mock.Requests()) != 0 {
		t.Fatal("unscoped queue made request")
	}

	f.mock.SetMode("queue-page")
	projectArgs := []string{"queue", "list", "--project", "Payments"}
	projectPage := f.call(t, append(projectArgs, "--json")...)
	projectCursor := nativeCursor(t, projectPage)
	for _, mode := range []string{"hash", "command", "malformed"} {
		c := projectCursor
		token := "invalid-token"
		if mode == "hash" {
			c.FilterHash = strings.Repeat("0", 64)
			token = nativeToken(t, c)
		}
		if mode == "command" {
			c.Command = "job.list"
			token = nativeToken(t, c)
		}
		f.mock.ClearRequests()
		rejected := f.call(t, append(projectArgs, "--cursor", token, "--json")...)
		requireNativeCode(t, rejected, 2)
		requireNativeValue(t, Obj(rejected.value["error"])["code"], "USAGE_ERROR")
		requireNativeValue(t, len(f.mock.Requests()), 0)
	}
	f.mock.ClearRequests()
	resized := f.call(t, append(projectArgs, "--limit", "1", "--cursor", Str(Obj(nativeData(projectPage)["page"]), "cursor"), "--json")...)
	requireNativeCode(t, resized, 2)
	requireNativeValue(t, len(f.mock.Requests()), 0)
	wrongJob := f.call(t, nativeQueue("--cursor", Str(Obj(nativeData(projectPage)["page"]), "cursor"), "--json")...)
	requireNativeCode(t, wrongJob, 2)
	requireNativeValue(t, Obj(wrongJob.value["error"])["code"], "USAGE_ERROR")
	noHints := f.call(t, nativeQueue("--no-hints", "--json")...)
	requireNativeValue(t, noHints.value["next"], nil)
	schema := f.call(t, "schema", "queue.list", "--json")
	requireNativeCode(t, schema, 0)
	requireNativeValue(t, Obj(nativeData(schema)["payload"])["$id"], "urn:teamcity-axi:queue-list:1.0")
}

// Original: queue.test.mjs — queue unknown/changed state, wait-text redaction, output/capture ceilings and grouped diagnostics stay explicit.
func TestRealCLIQueueUnknownAndLimits(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, mode := range []string{"queue-unknown", "queue-running", "queue-finished", "queue-bad-date"} {
		f.mock.SetMode(mode)
		r := f.call(t, nativeQueue("--json")...)
		requireNativeCode(t, r, 0)
		if Str(r.value, "status") != "partial" {
			t.Fatal("changed queue state promoted")
		}
	}
	f.mock.SetMode("queue-secret")
	for _, flags := range nativeFormats() {
		requireNativeCode(t, f.call(t, nativeQueue(flags...)...), 0)
	}
	f.mock.SetMode("queue-huge")
	requireNativeError(t, f.call(t, nativeQueue("--json")...), "INPUT_LIMIT_EXCEEDED")
	f.mock.SetMode("queue-oversized")
	for _, flags := range nativeFormats() {
		r := f.call(t, nativeQueue(append([]string{"--max-bytes", "2048"}, flags...)...)...)
		requireNativeError(t, r, "INPUT_LIMIT_EXCEEDED")
		if len(r.stdout) > 2048 {
			t.Fatal("queue output ceiling lost")
		}
	}
	f.mock.SetMode("queue-many-unknown")
	r := f.call(t, nativeQueue("--limit", "100", "--max-bytes", "65536", "--json")...)
	if len(Objects(nativeData(r)["items"])) != 100 || len(Objects(nativeMeta(r)["limitations"])) > 7 {
		t.Fatal("grouped queue diagnostics lost")
	}

	for _, flags := range nativeFormats() {
		for _, c := range []struct{ mode, state, code string }{{"queue-unknown", "unknown", "UNKNOWN_QUEUE_STATE"}, {"queue-running", "running", "QUEUE_STATE_CHANGED"}, {"queue-finished", "finished", "QUEUE_STATE_CHANGED"}} {
			f.mock.SetMode(c.mode)
			r := f.call(t, nativeQueue(append([]string{"--require-complete"}, flags...)...)...)
			requireNativeCode(t, r, 1)
			requireNativePartial(t, r)
			requireNativeValue(t, Objects(nativeData(r)["items"])[0]["state"], c.state)
			if !nativeNotes(r, c.code) {
				t.Fatal("queue state limitation missing")
			}
		}
		f.mock.SetMode("queue-secret")
		clean := f.call(t, nativeQueue(flags...)...)
		requireNativeValue(t, Objects(nativeData(clean)["items"])[0]["waitReason"], "[REDACTED]")
		requireNativeValue(t, Objects(nativeData(clean)["items"])[0]["branch"], "[REDACTED]")
		f.mock.SetMode("queue-oversized")
		large := f.call(t, nativeQueue(append([]string{"--max-bytes", "2048"}, flags...)...)...)
		if !nativeNotes(large, "OUTPUT_LIMIT_EXCEEDED") {
			t.Fatal("queue output limit not diagnosed")
		}
	}
	f.mock.SetMode("queue-many-unknown")
	many := f.call(t, nativeQueue("--limit", "100", "--max-bytes", "65536", "--json")...)
	requireNativeCode(t, many, 0)
	requireNativePartial(t, many)
	requireNativeValue(t, nativeNoteCount(many, "UNKNOWN_QUEUE_STATE"), 1)
	requireNativeValue(t, nativeNoteCount(many, "INVALID_TIMESTAMP"), 1)
}

// Original: agents.test.mjs — released agent reads preserve separate states, scoped pages, pool zero and exact retrieval in both serializers.
func TestRealCLIAgentStatesAndScopes(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("agent-page")
	for _, flags := range nativeFormats() {
		r := f.call(t, nativeAgentList(flags...)...)
		requireNativeCode(t, r, 0)
		a := Objects(nativeData(r)["agents"])[0]
		if Str(a, "id") != "7" || a["connected"] != true || a["enabled"] != true || a["authorized"] != true || a["activeRunState"] != "not_reported" || Obj(nativeData(r)["page"])["hasMore"] != true {
			t.Fatal("agent observations conflated")
		}
		next := f.call(t, append(Strings(Objects(r.value["next"])[0]["argv"])[1:], "--json")...)
		if Int(Obj(nativeData(next)["selection"]), "position") != 20 {
			t.Fatal("agent continuation lost")
		}
	}
	r := f.call(t, "agent", "list", "--job", "Payments_Build", "--json")
	if Str(Obj(nativeData(r)["selection"]), "meaning") != "compatible_agents" {
		t.Fatal("compatibility scope lost")
	}
	f.mock.SetMode("agent-idle")
	r = f.call(t, nativeAgentView("--json")...)
	if Str(Obj(nativeData(r)["agent"]), "activeRunState") != "idle" || Str(r.value, "status") != "ok" {
		t.Fatal("explicit idle unknown")
	}
	f.mock.SetMode("agent-state-mix")
	r = f.call(t, nativeAgentView("--json")...)
	a := Obj(nativeData(r)["agent"])
	if a["connected"] != true || a["enabled"] != false || a["authorized"] != false {
		t.Fatal("mixed state collapsed")
	}
	f.mock.SetMode("agent-zero-pool")
	r = f.call(t, "agent", "list", "--pool", "0", "--json")
	requireNativeCode(t, r, 0)
	if Str(Obj(Objects(nativeData(r)["agents"])[0]["pool"]), "id") != "0" {
		t.Fatal("pool zero lost")
	}
	requireNativeReadOnly(t, f.mock)

	f.mock.SetMode("agent-page")
	var rendered any
	for _, flags := range nativeFormats() {
		page := f.call(t, nativeAgentList(flags...)...)
		requireNativePartial(t, page)
		requireNativeValue(t, Obj(nativeData(page)["page"])["total"], nil)
		requireNativeValue(t, Obj(nativeMeta(page)["counts"])["childProcesses"], 3)
		if rendered != nil {
			requireNativeValue(t, nativeData(page)["agents"], rendered)
		}
		rendered = nativeData(page)["agents"]
	}
	scoped := f.call(t, "agent", "list", "--job", "Payments_Build", "--json")
	requireNativeValue(t, Obj(scoped.value["context"])["project"], "Payments")
	pooled := f.call(t, "agent", "list", "--pool", "1", "--json")
	requireNativeCode(t, pooled, 0)
	requireNativeValue(t, Obj(nativeData(pooled)["selection"])["projectId"], nil)
	requireNativeValue(t, Obj(nativeData(pooled)["selection"])["meaning"], "pool_agents")
	f.mock.SetMode("agent-idle")
	exact := f.call(t, nativeAgentView("--json")...)
	requireNativeCode(t, exact, 0)
	requireNativeValue(t, Obj(nativeMeta(exact)["counts"])["childProcesses"], 2)
	listing := f.call(t, "agent", "list", "--pool", "1", "--json")
	hinted := f.call(t, append(Strings(Objects(listing.value["next"])[0]["argv"])[1:], "--json")...)
	requireNativeValue(t, Obj(nativeData(hinted)["agent"])["id"], "7")
	requireNativeValue(t, Obj(nativeData(hinted)["selection"])["poolId"], "1")
	f.mock.SetMode("agent-state-mix")
	mixed := f.call(t, nativeAgentView("--json")...)
	requireNativeValue(t, mixed.value["status"], "ok")
}

// Original: agents.test.mjs — agent denial, missing capability, wrong exact ID, pool mismatch and malformed DTOs cannot become empty success.
func TestRealCLIAgentFailures(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, c := range []struct {
		mode, code string
		args       []string
	}{{"agent-denied", "PERMISSION_DENIED", nativeAgentList()}, {"agent-unsupported", "NOT_FOUND", nativeAgentList()}, {"agent-missing", "NOT_FOUND", nativeAgentView()}, {"agent-wrong-id", "CONTEXT_MISMATCH", nativeAgentView()}, {"agent-wrong-pool", "CONTEXT_MISMATCH", []string{"agent", "list", "--pool", "1"}}, {"agent-malformed-id", "UPSTREAM_SCHEMA_MISMATCH", nativeAgentList()}, {"agent-bad-state", "UPSTREAM_SCHEMA_MISMATCH", nativeAgentList()}, {"agent-malformed-page", "UPSTREAM_SCHEMA_MISMATCH", nativeAgentList()}, {"agent-duplicate", "UPSTREAM_SCHEMA_MISMATCH", nativeAgentList()}, {"agent-conflict-active", "UPSTREAM_SCHEMA_MISMATCH", nativeAgentView()}} {
		f.mock.SetMode(c.mode)
		requireNativeError(t, f.call(t, append(c.args, "--json")...), c.code)
	}
	f.mock.SetMode("agent-page")
	for _, args := range [][]string{{"agent", "list"}, {"agent", "list", "--pool", "-1"}, {"agent", "list", "--pool", "01"}, {"agent", "list", "--project", "bad\u202eid"}, {"agent", "view", "0"}, {"agent", "view", "9007199254740993"}} {
		f.mock.ClearRequests()
		requireNativeCode(t, f.call(t, append(args, "--json")...), 2)
		if len(f.mock.Requests()) != 0 {
			t.Fatal("invalid agent selector made request")
		}
	}

	f.mock.SetMode("agent-page")
	requireNativeError(t, f.call(t, "agent", "list", "--job", "Payments_Build", "--project", "Other", "--json"), "CONTEXT_MISMATCH")
}

// Original: agents.test.mjs — agent continuation binds policy/scope/filter/page size and retains useful rows after unsafe or expired continuation.
func TestRealCLIAgentCursorBinding(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("agent-page")
	first := f.call(t, nativeAgentList("--json")...)
	cursor := nativeCursor(t, first)
	for _, mode := range []string{"hash", "command", "size", "pool"} {
		c := cursor
		token := Str(Obj(nativeData(first)["page"]), "cursor")
		args := nativeAgentList()
		switch mode {
		case "hash":
			c.FilterHash = strings.Repeat("0", 64)
			token = nativeToken(t, c)
		case "command":
			c.Command = "queue.list"
			token = nativeToken(t, c)
		case "size":
			args = append(args, "--limit", "1")
		case "pool":
			args = append(args, "--pool", "1")
		}
		f.mock.ClearRequests()
		requireNativeCode(t, f.call(t, append(args, "--cursor", token, "--json")...), 2)
		if len(f.mock.Requests()) != 0 {
			t.Fatal("invalid agent cursor made request")
		}
	}
	for _, mode := range []string{"agent-unsafe", "agent-unsafe-scope", "agent-unsafe-filter"} {
		f.mock.SetMode(mode)
		r := f.call(t, nativeAgentList("--require-complete", "--json")...)
		requireNativeCode(t, r, 1)
		if len(Objects(nativeData(r)["agents"])) != 1 || !nativeNotes(r, "UNSAFE_CONTINUATION") {
			t.Fatal("unsafe agent row lost")
		}
	}
	f.mock.SetMode("agent-slow-page")
	cursor.ExpiresAt = time.Now().UnixMilli() + 700
	r := f.call(t, nativeAgentList("--cursor", nativeToken(t, cursor), "--json")...)
	requireNativeCode(t, r, 0)
	if Obj(nativeData(r)["page"])["cursor"] != nil || !nativeNotes(r, "CURSOR_EXPIRED") {
		t.Fatal("expired acquisition cursor retained")
	}

	requireNativeValue(t, len(Objects(nativeData(r)["agents"])), 1)
	for _, mode := range []string{"agent-unsafe", "agent-unsafe-scope", "agent-unsafe-filter"} {
		f.mock.SetMode(mode)
		unsafe := f.call(t, nativeAgentList("--require-complete", "--json")...)
		requireNativePartial(t, unsafe)
		requireNativeValue(t, Obj(nativeData(unsafe)["page"])["cursor"], nil)
	}
}

// Original: agents.test.mjs — agent active pointers obey current project policy, preserve exact retrieval and respect strict/no-hints and byte ceilings.
func TestRealCLIAgentPolicyAndLimits(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("agent-active")
	r := f.call(t, nativeAgentView("--json")...)
	requireNativeCode(t, r, 0)
	expanded := f.call(t, append(Strings(Objects(r.value["next"])[0]["argv"])[1:], "--json")...)
	if nativeRun(expanded)["id"] != "482193" {
		t.Fatal("active pointer changed run")
	}
	f.mock.SetMode("agent-foreign-active")
	for _, args := range [][]string{nativeAgentView("--json"), nativeAgentView("--project", "Payments", "--json"), nativeAgentList("--json")} {
		r = f.call(t, args...)
		a := Obj(nativeData(r)["agent"])
		if a == nil {
			a = Objects(nativeData(r)["agents"])[0]
		}
		if a["activeRun"] != nil || Str(a, "activeRunState") != "unavailable" || strings.Contains(r.stdout, "Forbidden") {
			t.Fatal("foreign pointer disclosed")
		}
	}
	f.mock.SetMode("agent-unknown")
	r = f.call(t, nativeAgentList("--require-complete", "--no-hints", "--json")...)
	requireNativeCode(t, r, 1)
	if r.value["next"] != nil || Objects(nativeData(r)["agents"])[0]["connected"] != nil {
		t.Fatal("unknown flags promoted")
	}
	f.mock.SetMode("agent-secret")
	for _, flags := range nativeFormats() {
		r = f.call(t, nativeAgentView(flags...)...)
		if Str(Obj(nativeData(r)["agent"]), "name") != "[REDACTED]" {
			t.Fatal("agent secret leaked")
		}
	}
	f.mock.SetMode("agent-huge")
	requireNativeError(t, f.call(t, nativeAgentView("--json")...), "INPUT_LIMIT_EXCEEDED")
	f.mock.SetMode("agent-oversized")
	for _, flags := range nativeFormats() {
		r = f.call(t, nativeAgentView(append([]string{"--max-bytes", "2048"}, flags...)...)...)
		requireNativeError(t, r, "INPUT_LIMIT_EXCEEDED")
	}
	f.mock.SetMode("agent-many-unknown")
	r = f.call(t, nativeAgentList("--limit", "100", "--max-bytes", "65536", "--json")...)
	if len(Objects(nativeData(r)["agents"])) != 100 || len(Objects(nativeMeta(r)["limitations"])) > 5 {
		t.Fatal("unknown agent grouping lost")
	}
	r = f.call(t, "schema", "agent.view", "--json")
	if Str(Obj(nativeData(r)["payload"]), "$id") != "urn:teamcity-axi:agent-view:1.0" {
		t.Fatal("schema missing")
	}

	f.mock.SetMode("agent-active")
	active := f.call(t, nativeAgentView("--json")...)
	requireNativeValue(t, Obj(Obj(nativeData(active)["agent"])["activeRun"])["id"], "482193")
	f.mock.SetMode("agent-foreign-active")
	for _, args := range [][]string{nativeAgentView("--json"), nativeAgentView("--project", "Payments", "--json"), nativeAgentList("--json")} {
		foreign := f.call(t, args...)
		requireNativeCode(t, foreign, 0)
		if args[1] == "view" {
			requireNativeValue(t, foreign.value["next"], nil)
		}
	}
	f.mock.SetMode("agent-secret")
	for _, flags := range nativeFormats() {
		requireNativeCode(t, f.call(t, nativeAgentView(flags...)...), 0)
	}
	f.mock.SetMode("agent-oversized")
	for _, flags := range nativeFormats() {
		large := f.call(t, nativeAgentView(append([]string{"--max-bytes", "2048"}, flags...)...)...)
		if len(large.stdout) > 2048 {
			t.Fatal("agent output cap exceeded")
		}
	}
	f.mock.SetMode("agent-many-unknown")
	many := f.call(t, nativeAgentList("--limit", "100", "--max-bytes", "65536", "--json")...)
	requireNativeCode(t, many, 0)
}

// Original: agents.test.mjs — interruption during an active-pointer policy read preserves exit 130 and emits one error document.
func TestRealCLIAgentPolicyInterrupt(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("agent-policy-hang")
	r := f.callSignalAfterPolicyRequest(t, syscall.SIGINT, nativeAgentView("--json")...)
	requireNativeCode(t, r, 130)
	if Str(Obj(r.value["error"]), "code") != "INTERRUPTED" {
		t.Fatal("policy cancellation downgraded")
	}
	observed := false
	for _, q := range f.mock.Requests() {
		if strings.Contains(q.Path, "UGF5bWVudHNfQ2hpbGQ") {
			observed = true
		}
	}
	if !observed {
		t.Fatal("signal test failed to reach policy read")
	}

	requireNativeValue(t, r.value["status"], "error")
	requireNativeValue(t, r.value["data"], nil)
}
