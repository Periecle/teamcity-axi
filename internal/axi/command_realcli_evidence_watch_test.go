//go:build realcli

package axi

import (
	"context"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Original: evidence.test.mjs — independent pages preserve duplicate names, muted/ignored categories, safe cursors and exact expansion.
func TestRealCLIEvidenceIdentityAndPaging(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, flags := range nativeFormats() {
		r := f.call(t, append([]string{"run", "tests", "482193"}, flags...)...)
		requireNativeCode(t, r, 0)
		requireNativePartial(t, r)
		items := Objects(nativeData(r)["tests"])
		if len(items) != 3 || items[0]["name"] != items[1]["name"] || items[0]["id"] == items[1]["id"] || items[1]["muted"] != true || items[2]["ignored"] != true {
			t.Fatal("occurrence categories lost")
		}
		requireNativeValue(t, items[2]["result"], "ignored")
		requireNativeValue(t, items[0]["testId"], "517450581327024597")
		r = f.call(t, append([]string{"run", "tests", "482193", "--failed"}, flags...)...)
		requireNativeCode(t, r, 0)
		requireNativeValue(t, Obj(nativeData(r)["page"])["total"], nil)
		if len(Objects(nativeData(r)["tests"])) != 1 {
			t.Fatal("unmuted filter wrong")
		}
		r = f.call(t, append([]string{"run", "tests", "482193", "--muted"}, flags...)...)
		requireNativeCode(t, r, 0)
		requireNativeValue(t, Obj(nativeData(r)["page"])["total"], nil)
		if len(Objects(nativeData(r)["tests"])) != 1 || Objects(nativeData(r)["tests"])[0]["muted"] != true {
			t.Fatal("muted filter wrong")
		}
		r = f.call(t, append([]string{"run", "tests", "482193", "--failed", "--include-muted"}, flags...)...)
		requireNativeCode(t, r, 0)
		requireNativeValue(t, Obj(nativeData(r)["page"])["total"], nil)
		if len(Objects(nativeData(r)["tests"])) != 2 {
			t.Fatal("include-muted wrong")
		}
	}
	first := f.call(t, "run", "tests", "482193", "--limit", "1", "--json")
	next := f.call(t, append(Strings(Objects(first.value["next"])[0]["argv"])[1:], "--json")...)
	if Str(Objects(nativeData(next)["tests"])[0], "id") != "build:(id:482193),id:2000000001" {
		t.Fatal("occurrence cursor wrong")
	}
	r := f.call(t, "run", "tests", "482193", "--test", "build:(id:482193),id:2000000000", "--fields", "durationMs", "--full", "--json")
	requireNativeCode(t, r, 0)
	if len(Objects(nativeData(r)["tests"])) != 1 || Obj(nativeData(r)["page"])["totalKind"] != "exact" || Objects(nativeData(r)["tests"])[0]["details"] != nil {
		t.Fatal("selected projection wrong")
	}
	requireNativeAbsent(t, Objects(nativeData(r)["tests"])[0], "details")
	f.mock.SetMode("evidence-preview")
	r = f.call(t, "run", "problems", "482193", "--json")
	requireNativeCode(t, r, 0)
	if !Bool(nativeMeta(r), "truncated") {
		t.Fatal("preview missing")
	}
	expanded := f.call(t, append(Strings(Objects(r.value["next"])[0]["argv"])[1:], "--json")...)
	requireNativeCode(t, expanded, 0)
	if len([]rune(Str(Objects(nativeData(expanded)["problems"])[0], "description"))) != 2500 {
		t.Fatal("full occurrence did not expand")
	}
	f.mock.SetMode("evidence-secret")
	r = f.call(t, "run", "tests", "482193", "--json")
	requireNativeCode(t, r, 0)
	if Str(Objects(nativeData(r)["tests"])[0], "details") != "[REDACTED]" {
		t.Fatal("evidence secret failed")
	}
	f.mock.SetMode("evidence-wrong-id")
	requireNativeError(t, f.call(t, "run", "tests", "482193", "--test", "build:(id:482193),id:2000000000", "--json"), "CONTEXT_MISMATCH")

	f.mock.SetMode("ok")
	problemPage := f.call(t, "run", "problems", "482193", "--limit", "1", "--json")
	requireNativeCode(t, problemPage, 0)
	requireNativeValue(t, problemPage.value["status"], "ok")
	requireNativeValue(t, Obj(nativeData(problemPage)["page"])["hasMore"], true)
	problemNext := f.call(t, append(Strings(Objects(problemPage.value["next"])[0]["argv"])[1:], "--json")...)
	requireNativeCode(t, problemNext, 0)
	if Objects(nativeData(problemNext)["problems"])[0]["id"] == Objects(nativeData(problemPage)["problems"])[0]["id"] {
		t.Fatal("problem continuation repeated identity")
	}
	selected := f.call(t, "run", "tests", "482193", "--test", "build:(id:482193),id:2000000000", "--json")
	requireNativeCode(t, selected, 0)
	requireNativeValue(t, Obj(nativeData(selected)["page"])["total"], 1)
	requireNativeValue(t, Objects(nativeData(selected)["tests"])[0]["id"], "build:(id:482193),id:2000000000")
	requireNativeCode(t, f.call(t, "run", "tests", "482193", "--limit", "1", "--cursor", Str(Obj(nativeData(problemPage)["page"]), "cursor"), "--json"), 2)
	requireNativeCode(t, f.call(t, "run", "tests", "482193", "--test", "517450581327024597", "--json"), 2)
	f.mock.SetMode("evidence-preview")
	preview := f.call(t, "run", "problems", "482193", "--json")
	requireNativeCode(t, preview, 0)
	requireNativeValue(t, len([]rune(Str(Objects(nativeData(preview)["problems"])[1], "description"))), 1200)
	foundHint := false
	for _, action := range Objects(preview.value["next"]) {
		argv := Strings(action["argv"])
		if containsString(argv, "--problem") {
			requireNativeValue(t, argv[len(argv)-2], "build:(id:482193),problem:(id:2)")
			foundHint = true
		}
	}
	if !foundHint {
		t.Fatal("problem expansion hint missing")
	}
	oversized := f.call(t, "run", "problems", "482193", "--problem", "build:(id:482193),problem:(id:2)", "--full", "--max-bytes", "2048", "--json")
	requireNativeError(t, oversized, "INPUT_LIMIT_EXCEEDED")
	if len(oversized.stdout) > 2048 {
		t.Fatal("problem output exceeds cap")
	}
	f.mock.SetMode("evidence-secret")
	secret := f.call(t, "run", "problems", "482193", "--json")
	requireNativeCode(t, secret, 0)
	requireNativeValue(t, Objects(nativeData(secret)["problems"])[0]["description"], "[REDACTED]")

}

// Original: evidence.test.mjs — independent source errors, unsafe continuation, empty pages and malformed identities cannot become false success.
func TestRealCLIEvidenceFailuresAndUnknown(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, c := range []struct{ mode, command, code string }{{"problems-denied", "problems", "PERMISSION_DENIED"}, {"tests-denied", "tests", "PERMISSION_DENIED"}, {"evidence-wrong-run", "tests", "CONTEXT_MISMATCH"}, {"evidence-duplicate-id", "tests", "UPSTREAM_SCHEMA_MISMATCH"}, {"evidence-huge", "problems", "INPUT_LIMIT_EXCEEDED"}} {
		f.mock.SetMode(c.mode)
		requireNativeError(t, f.call(t, "run", c.command, "482193", "--json"), c.code)
	}
	f.mock.SetMode("evidence-unsafe")
	r := f.call(t, "run", "tests", "482193", "--json")
	requireNativeCode(t, r, 0)
	if len(Objects(nativeData(r)["tests"])) != 3 || Obj(nativeData(r)["page"])["cursor"] != nil || Str(r.value, "status") != "partial" {
		t.Fatal("unsafe evidence lost")
	}
	f.mock.SetMode("evidence-empty")
	r = f.call(t, "run", "tests", "482193", "--json")
	if len(Objects(nativeData(r)["tests"])) != 0 || Obj(nativeData(r)["page"])["hasMore"] != true {
		t.Fatal("empty continuation lost")
	}
	requireNativeValue(t, Obj(nativeData(r)["page"])["total"], nil)
	f.mock.SetMode("evidence-unknown-test")
	r = f.call(t, "run", "tests", "482193", "--fields", "durationMs", "--json")
	if Str(Objects(nativeData(r)["tests"])[0], "rawStatus") != "FUTURE_RESULT" {
		t.Fatal("unknown raw status projected out")
	}
	requireNativeValue(t, Objects(nativeData(r)["tests"])[0]["result"], "unknown")
	f.mock.SetMode("evidence-no-flags")
	r = f.call(t, "run", "tests", "482193", "--failed", "--json")
	if len(Objects(nativeData(r)["tests"])) != 0 || Str(r.value, "status") != "partial" {
		t.Fatal("unknown muted flags certify failure category")
	}
	strict := f.call(t, "run", "tests", "482193", "--failed", "--require-complete", "--json")
	requireNativeCode(t, strict, 1)
	requireNativePartial(t, strict)
}

// Original: evidence.test.mjs — log filtering searches declared full messages, tail bounds hold, and failure mode retains sibling evidence.
func TestRealCLILogWindowAndIndependentSources(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("log-window")
	r := f.call(t, "run", "log", "482193", "--tail", "80", "--contains", "literal[needle]", "--json")
	requireNativeCode(t, r, 0)
	requireNativeValue(t, Obj(nativeData(r)["window"])["matched"], 1)
	if len(Objects(nativeData(r)["messages"])) != 1 || len(Str(Objects(nativeData(r)["messages"])[0], "text")) != 2000 {
		t.Fatal("literal filtering searched preview only")
	}
	expanded := f.call(t, append(Strings(Objects(r.value["next"])[0]["argv"])[1:], "--json")...)
	requireNativeCode(t, expanded, 0)
	if !strings.Contains(Str(Objects(nativeData(expanded)["messages"])[0], "text"), "literal[needle]") {
		t.Fatal("log expansion did not recover literal")
	}
	r = f.call(t, "run", "log", "482193", "--contains", "absent", "--json")
	if len(Objects(nativeData(r)["messages"])) != 0 || Int(Obj(nativeData(r)["window"]), "retained") != 2 {
		t.Fatal("empty literal window wrong")
	}
	if !strings.Contains(Str(nativeData(r), "emptyReason"), "window") {
		t.Fatal("empty log reason loses observed window")
	}
	r = f.call(t, "run", "log", "482193", "--tail", "1", "--json")
	requireNativeCode(t, r, 0)
	if Int(Obj(nativeData(r)["window"]), "omittedProviderMessages") != 1 || !Bool(nativeMeta(r), "truncated") || r.value["next"] != nil {
		t.Fatal("tail cap generated false expansion")
	}
	f.mock.SetMode("log-overdelivery")
	r = f.call(t, "run", "log", "482193", "--failed", "--json")
	requireNativeCode(t, r, 0)
	requireNativeValue(t, nativeMeta(r)["truncated"], true)
	if Int(Obj(nativeData(r)["window"]), "omittedProviderMessages") != 1 || r.value["next"] != nil {
		t.Fatal("failure tail overdelivery uncapped")
	}
	f.mock.SetMode("logs-unsupported")
	requireNativeError(t, f.call(t, "run", "log", "482193", "--json"), "CAPABILITY_UNAVAILABLE")
	r = f.call(t, "run", "log", "482193", "--failed", "--json")
	requireNativeCode(t, r, 0)
	requireNativePartial(t, r)
	sources := Obj(nativeData(r)["sources"])
	if Str(Obj(sources["log"]), "availability") != "unavailable" || Str(Obj(sources["tests"]), "availability") != "available" || nativeData(r)["messages"] != nil {
		t.Fatal("log unavailable erased siblings")
	}
	requireNativeAbsent(t, nativeData(r), "messages")
	f.mock.SetMode("tests-denied")
	r = f.call(t, "run", "log", "482193", "--failed", "--json")
	requireNativeCode(t, r, 0)
	if Str(Obj(Obj(nativeData(r)["sources"])["tests"]), "errorCode") != "PERMISSION_DENIED" || nativeData(r)["tests"] != nil || len(Objects(nativeData(r)["problems"])) == 0 {
		t.Fatal("test failure erased problems")
	}
	requireNativeAbsent(t, nativeData(r), "tests")
	for _, q := range f.mock.Requests() {
		if strings.Contains(q.Path, "downloadBuildLog") {
			t.Fatal("unbounded log download attempted")
		}
	}

}

// Original: evidence.test.mjs — changes preserve mandatory identity, scoped paging, file caps, expansion and source errors.
func TestRealCLIChangesEvidence(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, flags := range nativeFormats() {
		r := f.call(t, append([]string{"run", "changes", "482193", "--limit", "1", "--files"}, flags...)...)
		requireNativeCode(t, r, 0)
		changes := Objects(nativeData(r)["changes"])
		if len(changes) != 1 || Str(changes[0], "vcsRootId") != "Payments_Git" || changes[0]["fileCoverage"] == nil || Obj(nativeData(r)["page"])["hasMore"] != true {
			t.Fatal("changes identity/files lost")
		}
		next := f.call(t, append(Strings(Objects(r.value["next"])[0]["argv"])[1:], "--json")...)
		if Str(Objects(nativeData(next)["changes"])[0], "id") != "102" {
			t.Fatal("changes continuation wrong")
		}
	}
	f.mock.SetMode("changes-preview")
	r := f.call(t, "run", "changes", "482193", "--json")
	if len([]rune(Str(Objects(nativeData(r)["changes"])[0], "message"))) != 200 {
		t.Fatal("change summary unbounded")
	}
	hints := Objects(r.value["next"])
	expanded := f.call(t, append(Strings(hints[len(hints)-1]["argv"])[1:], "--json")...)
	if len([]rune(Str(Objects(nativeData(expanded)["changes"])[0], "message"))) != 2511 {
		t.Fatalf("full change message wrong: %d", len([]rune(Str(Objects(nativeData(expanded)["changes"])[0], "message"))))
	}
	f.mock.SetMode("changes-file-limit")
	r = f.call(t, "run", "changes", "482193", "--files", "--json")
	if Int(Obj(Objects(nativeData(r)["changes"])[0]["fileCoverage"]), "omitted") != 1 || len(Strings(Objects(nativeData(r)["changes"])[0]["files"])) != 100 {
		t.Fatal("files cap lost")
	}
	for _, c := range []struct{ mode, code string }{{"changes-denied", "PERMISSION_DENIED"}, {"changes-unsupported", "NOT_FOUND"}, {"changes-wrong-root", "CONTEXT_MISMATCH"}, {"changes-huge", "INPUT_LIMIT_EXCEEDED"}} {
		f.mock.SetMode(c.mode)
		requireNativeError(t, f.call(t, "run", "changes", "482193", "--json"), c.code)
	}
	f.mock.SetMode("changes-no-root")
	r = f.call(t, "run", "changes", "482193", "--json")
	requireNativeValue(t, Obj(nativeData(r)["selection"])["providerReturned"], 3)
	if len(Objects(nativeData(r)["changes"])) != 2 || !nativeNotes(r, "CHANGE_ROOT_UNAVAILABLE") {
		t.Fatal("missing change root promoted")
	}
	f.mock.SetMode("changes-unsafe")
	r = f.call(t, "run", "changes", "482193", "--json")
	requireNativeCode(t, r, 0)
	requireNativePartial(t, r)
	requireNativeValue(t, Obj(nativeData(r)["page"])["cursor"], nil)
	if !nativeNotes(r, "UNSAFE_CONTINUATION") || len(Objects(nativeData(r)["changes"])) != 3 {
		t.Fatal("unsafe changes lost")
	}
	f.mock.SetMode("changes-secret")
	r = f.call(t, "run", "changes", "482193", "--json")
	if Str(Objects(nativeData(r)["changes"])[0], "message") != "[REDACTED]" {
		t.Fatal("change credential leaked")
	}

	f.mock.SetMode("ok")
	first := f.call(t, "run", "changes", "482193", "--limit", "1", "--json")
	requireNativeCode(t, first, 0)
	requireNativeValue(t, Objects(nativeData(first)["changes"])[0]["message"], "Synthetic change 0")
	requireNativeValue(t, Objects(nativeData(first)["changes"])[0]["vcsRootId"], "Payments_Git")
	requireNativeAbsent(t, Objects(nativeData(first)["changes"])[0], "files")
	noHints := f.call(t, "run", "changes", "482193", "--limit", "1", "--no-hints", "--json")
	requireNativeAbsent(t, noHints.value, "next")
	fullHintFound, cursorHintFound := false, false
	for _, action := range Objects(first.value["next"]) {
		argv := Strings(action["argv"])
		if containsString(argv, "--full") {
			full := f.call(t, append(argv[1:], "--json")...)
			if !strings.Contains(Str(Objects(nativeData(full)["changes"])[0], "message"), "\n") {
				t.Fatal("change full hint did not retrieve newline")
			}
			fullHintFound = true
		}
		if containsString(argv, "--cursor") {
			next := f.call(t, append(argv[1:], "--json")...)
			if Objects(nativeData(next)["changes"])[0]["id"] == Objects(nativeData(first)["changes"])[0]["id"] {
				t.Fatal("change continuation repeated identity")
			}
			cursorHintFound = true
		}
	}
	if !fullHintFound || !cursorHintFound {
		t.Fatal("change expansion or continuation hint missing")
	}
	requireNativeCode(t, f.call(t, "run", "changes", "482193", "--limit", "1", "--files", "--cursor", Str(Obj(nativeData(first)["page"]), "cursor"), "--json"), 2)
	f.mock.SetMode("changes-file-limit")
	files := f.call(t, "run", "changes", "482193", "--files", "--fields", "files", "--json")
	requireNativePartial(t, files)
	requireNativeValue(t, len(Strings(Objects(nativeData(files)["changes"])[0]["files"])), 100)
	requireNativeValue(t, Obj(Objects(nativeData(files)["changes"])[0]["fileCoverage"])["omitted"], 1)
	if Str(Objects(nativeData(files)["changes"])[0], "message") == "" {
		t.Fatal("change file projection lost mandatory message")
	}
	f.mock.SetMode("changes-preview")
	oversized := f.call(t, "run", "changes", "482193", "--full", "--max-bytes", "2048", "--json")
	requireNativeError(t, oversized, "INPUT_LIMIT_EXCEEDED")
	if len(oversized.stdout) > 2048 {
		t.Fatal("change output exceeds cap")
	}
	strict := f.call(t, "run", "changes", "482193", "--require-complete", "--json")
	requireNativeCode(t, strict, 1)
	requireNativePartial(t, strict)

}

// Original: evidence.test.mjs — immediate dependency adapters preserve direction, scoped counts and independent failures with the pinned CLI.
func TestRealCLIDependencyAdapters(t *testing.T) {
	f := newNativeFixture(t, false)
	transport, err := NewProcessTransport(TransportOptions{Binary: f.native, ServerURL: f.mock.BaseURL, Env: f.env, Limits: ProcessLimits{Deadline: time.Now().UnixMilli() + 15000, Concurrency: 3, MaxChildren: 8, StdoutBytes: 2097152, StderrBytes: 65536}})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	reader := NewNativeReader(transport, f.mock.BaseURL, []string{"fixture-only-token"})
	budget := Budget{Deadline: time.Now().UnixMilli() + 15000, MaxChildProcesses: -1}
	query := ReadRequest{Kind: "dependencies.page", RunID: "482193", Count: 20, ScanLimit: 5000}
	r := reader.Read(context.Background(), query, budget)
	if r.State != "available" || Str(Obj(Objects(r.Value["items"])[0]["run"]), "id") != "482188" || Str(Objects(r.Value["items"])[0], "projectId") != "Payments" || r.Value["hasMore"] != nil {
		t.Fatalf("dependency page: %#v", r)
	}
	r = reader.Read(context.Background(), ReadRequest{Kind: "dependencies.count", ID: "482193"}, budget)
	if r.State != "available" || Int(r.Value, "count") != 1 {
		t.Fatal("exact dependency count wrong")
	}
	for _, c := range []struct{ mode, code string }{{"dependencies-denied", "PERMISSION_DENIED"}, {"dependencies-unsupported", "NOT_FOUND"}, {"dependencies-huge", "INPUT_LIMIT_EXCEEDED"}, {"dependencies-malformed", "UPSTREAM_SCHEMA_MISMATCH"}} {
		f.mock.SetMode(c.mode)
		r = reader.Read(context.Background(), query, budget)
		if r.State != "unavailable" || r.Error.Code != c.code || r.Value != nil {
			t.Fatalf("%s: %#v", c.mode, r)
		}
	}
	f.mock.SetMode("dependency-count-wrong-id")
	r = reader.Read(context.Background(), ReadRequest{Kind: "dependencies.count", ID: "482193"}, budget)
	if r.State != "unavailable" || r.Error.Code != "CONTEXT_MISMATCH" || transport.ChildProcesses() != 7 {
		t.Fatal("dependency count binding failed")
	}
	requireNativeReadOnly(t, f.mock)
}

// Original: watch.test.mjs — watch emits one document for completed failure and success, with explicit assertion exits.
func TestRealCLIWatchFinished(t *testing.T) {
	f := newNativeFixture(t, false)
	var rendered Object
	for _, flags := range nativeFormats() {
		r := f.call(t, append([]string{"run", "watch", "482193"}, flags...)...)
		requireNativeCode(t, r, 0)
		if Str(nativeData(r), "outcome") != "finished" || Str(nativeRun(r), "result") != "failure" || Int(Obj(nativeMeta(r)["counts"]), "childProcesses") != 2 || Int(Obj(nativeMeta(r)["limits"]), "maxChildProcesses") != 32 || len(r.stdout) > 8192 {
			t.Fatal("watch terminal evidence incorrect")
		}
		requireNativeValue(t, nativeRun(r)["id"], "482193")
		requireNativeValue(t, Obj(nativeMeta(r)["limits"])["concurrency"], 1)
		if rendered != nil {
			requireNativeValue(t, nativeRun(r), rendered)
		}
		rendered = nativeRun(r)
	}
	requireNativeCode(t, f.call(t, "run", "watch", "482193", "--check", "--json"), 1)
	f.mock.SetMode("watch-success")
	r := f.call(t, "run", "watch", "482193", "--check", "--json")
	requireNativeCode(t, r, 0)
	if !Bool(Obj(nativeData(r)["check"]), "passed") {
		t.Fatal("success check failed")
	}
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	contextual := f.call(t, "run", "watch", "482193", "--cwd", repository, "--json")
	requireNativeValue(t, Obj(contextual.value["context"])["branch"], "feature/refund")
	requireNativeAbsent(t, Obj(contextual.value["context"]), "jobs")
	f.mock.SetMode("huge-text")
	concise := f.call(t, "run", "watch", "482193", "--json")
	requireNativeCode(t, concise, 0)
	requireNativeAbsent(t, nativeRun(concise), "statusText")
	if len(concise.stdout) > 8192 {
		t.Fatal("watch output unbounded")
	}
	for _, q := range f.mock.Requests() {
		if q.Path != "/teamcity/app/rest/builds/id:482193" {
			t.Fatalf("watch read a different execution: %s", q.Path)
		}
	}

}

// Original: watch.test.mjs — bounded polls distinguish terminal transition, vanished and inaccessible executions and retain prior identity.
func TestRealCLIWatchTransitions(t *testing.T) {
	for _, mode := range []string{"watch-transition", "watch-vanish", "watch-inaccessible"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			f := newNativeFixture(t, false)
			f.mock.SetMode(mode)
			r := f.call(t, "run", "watch", "482193", "--interval", "5s", "--timeout", "9s", "--json")
			requireNativeCode(t, r, 0)
			expected := "finished"
			state := "finished"
			if mode == "watch-vanish" {
				expected = "vanished"
				state = "running"
			}
			if mode == "watch-inaccessible" {
				expected = "inaccessible"
				state = "running"
			}
			if nativeData(r)["outcome"] != expected || nativeRun(r)["state"] != state || Int(Obj(nativeData(r)["polling"]), "count") != 2 || Int(Obj(nativeMeta(r)["counts"]), "childProcesses") != 3 {
				t.Fatal("retained watch transition incorrect")
			}
			requireNativeValue(t, nativeRun(r)["id"], "482193")
			wantStatus := "partial"
			if mode == "watch-transition" {
				wantStatus = "ok"
			}
			requireNativeValue(t, r.value["status"], wantStatus)
			for _, q := range f.mock.Requests() {
				if !strings.HasSuffix(q.Path, "/builds/id:482193") {
					t.Fatal("watch replaced frozen ID")
				}
			}
			requireNativeReadOnly(t, f.mock)
		})
	}
}

// Original: watch.test.mjs — watch deadline retains latest queued/running evidence, remains partial and fails check.
func TestRealCLIWatchDeadline(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, mode := range []string{"watch-running", "watch-queued"} {
		f.mock.SetMode(mode)
		for _, extra := range [][]string{{}, {"--check"}, {"--require-complete"}} {
			args := append([]string{"run", "watch", "482193", "--timeout", "700ms", "--json"}, extra...)
			r := f.call(t, args...)
			want := 0
			if len(extra) > 0 {
				want = 1
			}
			requireNativeCode(t, r, want)
			if Str(nativeData(r), "outcome") != "deadline" || Str(nativeRun(r), "state") != strings.TrimPrefix(mode, "watch-") || !nativeNotes(r, "DEADLINE_EXCEEDED") {
				t.Fatal("deadline discarded retained watch evidence")
			}
			requireNativePartial(t, r)
			requireNativeValue(t, Obj(nativeData(r)["check"])["passed"], false)
		}
	}
	f.mock.SetMode("watch-missing-revisions")
	r := f.call(t, "run", "watch", "482193", "--timeout", "700ms", "--json")
	requireNativeAbsent(t, nativeRun(r), "revisions")
	if !nativeNotes(r, "MISSING_REVISION_METADATA") {
		t.Fatal("latest watch provenance lost")
	}
}

// Original: watch.test.mjs — watch interruption during polling sleep preserves signal exit and never mutates the run.
func TestRealCLIWatchInterrupt(t *testing.T) {
	for _, s := range []struct {
		signal syscall.Signal
		code   int
	}{{syscall.SIGINT, 130}, {syscall.SIGTERM, 143}} {
		f := newNativeFixture(t, false)
		f.mock.SetMode("watch-running")
		r := f.callSignalAfterFirstRequest(t, s.signal, 50*time.Millisecond, "run", "watch", "482193", "--json")
		requireNativeCode(t, r, s.code)
		if Str(Obj(r.value["error"]), "code") != "INTERRUPTED" {
			t.Fatal("watch sleep interrupt lost")
		}
		if len(f.mock.Requests()) != 1 {
			t.Fatal("interrupt test did not reach polling sleep")
		}
		requireNativeReadOnly(t, f.mock)
	}
}

// Original: watch.test.mjs — watch cannot replace the frozen ID or selected scope and surfaces inaccessible initial reads.
func TestRealCLIWatchIdentityFailures(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("wrong-id")
	r := f.call(t, "run", "watch", "482193", "--check", "--json")
	requireNativeCode(t, r, 1)
	if nativeData(r)["run"] != nil || nativeData(r)["outcome"] != "unavailable" {
		t.Fatal("watch replaced frozen identity")
	}
	f.mock.SetMode("ok")
	requireNativeError(t, f.call(t, "run", "watch", "482193", "--job=Other", "--json"), "CONTEXT_MISMATCH")
	for _, mode := range []string{"missing", "denied", "expired"} {
		f.mock.SetMode(mode)
		r = f.call(t, "run", "watch", "482193", "--check", "--json")
		requireNativeCode(t, r, 1)
		want := "inaccessible"
		if mode == "missing" {
			want = "vanished"
		}
		if nativeData(r)["run"] != nil || nativeData(r)["outcome"] != want {
			t.Fatal("initial visibility failed")
		}
	}
}
