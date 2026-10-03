//go:build realcli

package axi

import (
	"encoding/json"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func nativeListArgs(extra ...string) []string {
	return append([]string{"run", "list", "--job", "Payments_Build", "--branch", "feature/refund", "--since", "2026-10-01T00:00:00Z", "--until", "2026-10-02T00:00:00Z"}, extra...)
}
func nativeFormats() [][]string     { return [][]string{{"--json"}, {}} }
func nativeRun(r nativeCall) Object { return Obj(nativeData(r)["run"]) }

// Original: run-view.test.mjs — actual wrapper and released CLI observe an exact failed run successfully in both formats.
func TestRealCLIRunViewFormats(t *testing.T) {
	f := newNativeFixture(t, false)
	var previous Object
	for _, flags := range nativeFormats() {
		r := f.call(t, append([]string{"run", "view", "482193"}, flags...)...)
		requireNativeCode(t, r, 0)
		requireNativeValue(t, r.value["status"], "ok")
		requireNativeValue(t, nativeRun(r)["id"], "482193")
		requireNativeValue(t, nativeRun(r)["result"], "failure")
		requireNativeValue(t, Obj(r.value["context"])["server"], "work")
		requireNativeValue(t, Obj(r.value["context"])["job"], "Payments_Build")
		requireNativeValue(t, Obj(nativeMeta(r)["counts"])["childProcesses"], 2)
		if previous != nil {
			requireNativeValue(t, nativeRun(r), previous)
		}
		previous = nativeRun(r)
	}
	projected := f.call(t, "run", "view", "482193", "--fields", "number", "--json")
	requireNativeKeys(t, nativeRun(projected), "id", "jobId", "number", "result", "state")
	mismatch := f.call(t, "run", "view", "482193", "--job", "Other", "--json")
	requireNativeError(t, mismatch, "CONTEXT_MISMATCH")
	requireNativeValue(t, Obj(mismatch.value["context"])["server"], "work")
}

// Original: run-view.test.mjs — permission, authentication, wrong identity, malformed and oversized reads never become empty success.
func TestRealCLIRunViewFailures(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, c := range []struct{ mode, code string }{{"denied", "PERMISSION_DENIED"}, {"expired", "AUTH_REQUIRED"}, {"missing", "NOT_FOUND"}, {"malformed", "UPSTREAM_SCHEMA_MISMATCH"}, {"html", "AUTH_REQUIRED"}, {"wrong-id", "CONTEXT_MISMATCH"}, {"invalid-identity", "UPSTREAM_SCHEMA_MISMATCH"}, {"huge", "INPUT_LIMIT_EXCEEDED"}} {
		t.Run(c.mode, func(t *testing.T) {
			f.mock.SetMode(c.mode)
			r := f.call(t, "run", "view", "482193", "--json")
			requireNativeError(t, r, c.code)
			requireNativeValue(t, Obj(r.value["context"])["server"], "work")
		})
	}
	f.mock.SetMode("unknown-enum")
	unknown := f.call(t, "run", "view", "482193", "--json")
	requireNativeCode(t, unknown, 0)
	requireNativeValue(t, nativeRun(unknown)["state"], "unknown")
	requireNativeValue(t, nativeRun(unknown)["result"], "unknown")
}

// Original: run-view.test.mjs — preview expansion is bounded and next actions parse; SIGINT/deadline remain read-only.
func TestRealCLIRunViewBoundsAndInterrupt(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("huge-text")
	preview := f.call(t, "run", "view", "482193", "--json", "--job", "Payments_Build", "--project", "Payments")
	requireNativeCode(t, preview, 0)
	requireNativeValue(t, nativeMeta(preview)["truncated"], true)
	requireNativeValue(t, len([]rune(Str(nativeRun(preview), "statusText"))), 1200)
	argv := Strings(Objects(preview.value["next"])[0]["argv"])
	if !containsString(argv, "--job") || !containsString(argv, "--project") {
		t.Fatal("preview hint lost exact scope")
	}
	full := f.call(t, "run", "view", "482193", "--full", "--max-bytes", "2048", "--json")
	requireNativeError(t, full, "INPUT_LIMIT_EXCEEDED")
	if len(full.stdout) > 2048 {
		t.Fatal("output byte ceiling lost")
	}
	requireNativeValue(t, Obj(full.value["context"])["server"], "work")
	requireNativeValue(t, Obj(Obj(full.value["error"])["details"])["ceiling"], 2048)
	limits := Obj(nativeMeta(full)["limits"])
	requireNativeValue(t, limits["maxBytes"], 2048)
	requireNativeValue(t, limits["maxChildProcesses"], 8)
	requireNativeValue(t, limits["stdoutCaptureBytes"], 2097152)
	f.mock.SetMode("hang")
	requireNativeError(t, f.call(t, "run", "view", "482193", "--timeout", "200ms", "--json"), "DEADLINE_EXCEEDED")
	interrupted := f.callSignal(t, syscall.SIGINT, 500*time.Millisecond, "run", "view", "482193", "--json")
	requireNativeCode(t, interrupted, 130)
	requireNativeValue(t, Obj(interrupted.value["error"])["code"], "INTERRUPTED")
}

// Original: run-view.test.mjs — released CLI output redacts long credentials before previews and marks omitted revisions partial.
func TestRealCLIRunViewRedactionAndMissingRevisions(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, mode := range []string{"long-secret", "decorated-secret"} {
		f.mock.SetMode(mode)
		r := f.call(t, "run", "view", "482193", "--json")
		requireNativeCode(t, r, 0)
		requireNativeValue(t, nativeRun(r)["rawStatus"], "[REDACTED]")
		requireNativeValue(t, nativeRun(r)["statusText"], "[REDACTED]")
	}
	f.mock.SetMode("missing-revisions")
	missing := f.call(t, "run", "view", "482193", "--json")
	requireNativeCode(t, missing, 0)
	requireNativePartial(t, missing)
	requireNativeAbsent(t, nativeRun(missing), "revisions")
	strict := f.call(t, "run", "view", "482193", "--json", "--require-complete")
	requireNativeCode(t, strict, 1)
	requireNativePartial(t, strict)
}

// Original: run-view.test.mjs — released native outcome projections remain explicit across view, list, watch and investigation.
func TestRealCLIExceptionalOutcomes(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, c := range []struct{ mode, result string }{{"outcome-canceled", "canceled"}, {"outcome-failed-to-start", "failed_to_start"}, {"outcome-composite", "success"}} {
		f.mock.SetMode(c.mode)
		for _, flags := range nativeFormats() {
			view := f.call(t, append([]string{"run", "view", "482193"}, flags...)...)
			requireNativeCode(t, view, 0)
			requireNativeValue(t, nativeRun(view)["result"], c.result)
			requireNativeValue(t, nativeRun(view)["state"], "finished")
			requireNativeValue(t, nativeRun(view)["composite"], c.mode == "outcome-composite")
			if strings.Contains(view.stdout, "Private") {
				t.Fatal("private cancellation data leaked")
			}
		}
		list := f.call(t, "run", "list", "--job", "Payments_Build", "--all-branches", "--result", c.result, "--since", "2026-10-01T00:00:00Z", "--until", "2026-10-02T00:00:00Z", "--json")
		requireNativeCode(t, list, 0)
		requireNativeValue(t, Objects(nativeData(list)["runs"])[0]["result"], c.result)
		requireNativeValue(t, Obj(nativeData(list)["aggregates"])[c.result], 1)
		watch := f.call(t, "run", "watch", "482193", "--check", "--json")
		requireNativeValue(t, nativeRun(watch)["result"], c.result)
		requireNativeValue(t, Obj(nativeData(watch)["check"])["passed"], c.result == "success")
		expectedCode := 1
		if c.result == "success" {
			expectedCode = 0
		}
		requireNativeCode(t, watch, expectedCode)
		failure := f.call(t, "run", "failure", "482193", "--json")
		requireNativeValue(t, nativeRun(failure)["result"], c.result)
		assessment := "failure_observed"
		if c.result == "success" {
			assessment = "not_failed"
		}
		requireNativeValue(t, nativeData(failure)["assessment"], assessment)
	}
	f.mock.SetMode("outcome-missing")
	missing := f.call(t, "run", "watch", "482193", "--check", "--json")
	requireNativeCode(t, missing, 1)
	requireNativeValue(t, nativeRun(missing)["result"], "unknown")
	requireNativeValue(t, Obj(nativeData(missing)["check"])["passed"], false)
	projected := false
	for _, q := range f.mock.Requests() {
		if strings.Contains(q.Query.Get("fields"), "canceledInfo") {
			projected = true
		}
	}
	if !projected {
		t.Fatal("explicit cancellation metadata was not requested")
	}
}

// Original: run-view.test.mjs — a full unknown-outcome page retains all rows within the public diagnostic ceiling.
func TestRealCLIUnknownOutcomePage(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("outcome-many-missing")
	for _, flags := range nativeFormats() {
		args := append([]string{"run", "list", "--job", "Payments_Build", "--all-branches", "--limit", "100", "--max-bytes", "65536", "--since", "2026-10-01T00:00:00Z", "--until", "2026-10-02T00:00:00Z"}, flags...)
		r := f.call(t, args...)
		requireNativeCode(t, r, 0)
		requireNativePartial(t, r)
		requireNativeValue(t, len(Objects(nativeData(r)["runs"])), 100)
		requireNativeValue(t, Obj(nativeData(r)["aggregates"])["unknown"], 100)
		if !nativeNoteMessage(r, "100 distinct executions affected") {
			t.Fatal("unknown outcome diagnostic grouping lost exact affected count")
		}
		if len(Objects(nativeMeta(r)["limitations"])) > 8 {
			t.Fatal("diagnostics ungrouped")
		}
	}
}

// Original: run-view.test.mjs — debug and limit-hit responses expose actual tighter ceilings without credentials in both serializers.
func TestRealCLIEffectiveLimits(t *testing.T) {
	f := newNativeFixture(t, false)
	f.config.Limits = UserLimits{MaxBytes: 4096, MaxChildProcesses: 4, Concurrency: 1}
	f.writeConfig(t)
	for _, flags := range nativeFormats() {
		f.mock.SetMode("ok")
		r := f.call(t, append([]string{"run", "view", "482193", "--debug", "--max-bytes", "32768", "--timeout", "1000ms"}, flags...)...)
		requireNativeCode(t, r, 0)
		var debug Object
		if err := json.Unmarshal([]byte(r.stderr), &debug); err != nil {
			t.Fatal(err)
		}
		limits := Obj(nativeMeta(r)["limits"])
		requireNativeValue(t, debug["limits"], limits)
		requireNativeValue(t, debug["command"], "run.view")
		requireNativeValue(t, debug["childProcesses"], 2)
		for key, want := range (Object{"maxBytes": 4096, "maxChildProcesses": 4, "concurrency": 1, "stdoutCaptureBytes": 2097152, "stderrCaptureBytes": 65536}) {
			requireNativeValue(t, limits[key], want)
		}
		deadline := int64(Int(limits, "deadline"))
		now := time.Now().UnixMilli()
		if deadline < now || deadline > now+1000 {
			t.Fatal("effective deadline outside requested interval")
		}
		if strings.Contains(r.stderr, "fixture-only-token") || strings.Contains(r.stderr, longNativeCanary()) {
			t.Fatal("debug credentials leaked")
		}
		f.config.Limits.MaxChildProcesses = 1
		f.writeConfig(t)
		blocked := f.call(t, append([]string{"run", "view", "482193", "--debug"}, flags...)...)
		requireNativeError(t, blocked, "INPUT_LIMIT_EXCEEDED")
		requireNativeValue(t, Obj(Obj(blocked.value["error"])["details"]), Object{"limit": "maxChildProcesses", "ceiling": 1, "observed": 1})
		requireNativeValue(t, Obj(nativeMeta(blocked)["limits"])["maxChildProcesses"], 1)
		if err := json.Unmarshal([]byte(blocked.stderr), &debug); err != nil {
			t.Fatal(err)
		}
		requireNativeValue(t, debug["limits"], nativeMeta(blocked)["limits"])
		f.config.Limits.MaxChildProcesses = 4
		f.writeConfig(t)
		f.mock.SetMode("huge")
		capture := f.call(t, append([]string{"run", "view", "482193"}, flags...)...)
		requireNativeError(t, capture, "INPUT_LIMIT_EXCEEDED")
		details := Obj(Obj(capture.value["error"])["details"])
		requireNativeValue(t, details["limit"], "stdoutCaptureBytes")
		requireNativeValue(t, details["ceiling"], 2097152)
		if Int(details, "observed") <= 2097152 {
			t.Fatal("capture limit lacks observed byte count")
		}
		requireNativeValue(t, Obj(nativeMeta(capture)["limits"])["stdoutCaptureBytes"], 2097152)
		if len(capture.stdout) > 4096 {
			t.Fatal("capture error exceeds serialization ceiling")
		}
	}
}

// Original: run-list.test.mjs — released CLI list preserves one page, mandatory identities, typed continuation and empty continuation.
func TestRealCLIRunListPaging(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("list-page")
	for _, flags := range nativeFormats() {
		r := f.call(t, nativeListArgs(flags...)...)
		requireNativeCode(t, r, 0)
		if Obj(nativeData(r)["page"])["hasMore"] != true || Str(Objects(nativeData(r)["runs"])[0], "id") != "482193" {
			t.Fatal("page identity lost")
		}
		requireNativeValue(t, Obj(nativeData(r)["page"])["total"], nil)
		requireNativeValue(t, Objects(nativeData(r)["runs"])[0]["result"], "failure")
	}
	first := f.call(t, nativeListArgs("--fields", "number", "--json")...)
	if len(Objects(nativeData(first)["runs"])[0]) != 5 {
		t.Fatal("mandatory list fields lost")
	}
	f.mock.SetMode("list-empty")
	r := f.call(t, nativeListArgs("--fields", "number", "--cursor", Str(Obj(nativeData(first)["page"]), "cursor"), "--json")...)
	requireNativeCode(t, r, 0)
	if Int(Obj(nativeData(r)["page"]), "returned") != 0 || Obj(nativeData(r)["page"])["hasMore"] != true || Obj(nativeData(r)["page"])["total"] != nil {
		t.Fatal("empty continuation promoted")
	}
	requireNativeReadOnly(t, f.mock)

	requireNativeKeys(t, Objects(nativeData(first)["runs"])[0], "id", "jobId", "number", "result", "state")
	requireNativeValue(t, Obj(nativeData(first)["selection"])["consistency"], "best_effort_offset")
	requireNativeValue(t, Obj(nativeData(r)["selection"])["consistency"], "best_effort_offset")
	if Str(Obj(nativeData(r)["page"]), "cursor") == "" {
		t.Fatal("empty page continuation missing")
	}
}

// Original: run-list.test.mjs — verified exhausted empty lists return exact scoped zero; capped and unverified emptiness stay partial.
func TestRealCLIRunListExhaustion(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, flags := range nativeFormats() {
		f.mock.SetMode("list-verified-empty")
		r := f.call(t, nativeListArgs(append(flags, "--require-complete")...)...)
		requireNativeCode(t, r, 0)
		requireNativeValue(t, r.value["status"], "ok")
		requireNativeValue(t, nativeMeta(r)["complete"], true)
		requireNativeValue(t, nativeData(r)["runs"], []Object{})
		requireNativeValue(t, nativeData(r)["page"], Object{"returned": 0, "total": 0, "totalKind": "exact", "hasMore": false, "cursor": nil})
		requireNativeValue(t, Obj(nativeData(r)["selection"])["exhaustionBasis"], "verified_server_pagination")
		requireNativeValue(t, Obj(r.value["context"])["job"], "Payments_Build")
		requireNativeValue(t, Obj(r.value["context"])["project"], "Payments")
		requireNativeAbsent(t, r.value, "next")
		for _, mode := range []string{"list-unverified-empty", "list-exhaustion-probe-denied", "list-verified-cap-empty", "list-verified-malformed-next"} {
			f.mock.SetMode(mode)
			uncertain := f.call(t, nativeListArgs(append(flags, "--require-complete")...)...)
			requireNativeCode(t, uncertain, 1)
			requireNativePartial(t, uncertain)
			page := Obj(nativeData(uncertain)["page"])
			requireNativeValue(t, page["total"], nil)
			requireNativeValue(t, page["totalKind"], "unknown")
			requireNativeValue(t, page["hasMore"], nil)
			requireNativeValue(t, Obj(nativeData(uncertain)["selection"])["exhaustionBasis"], "unverified")
		}
	}
	f.mock.SetMode("list-page")
	first := f.call(t, nativeListArgs("--json")...)
	f.mock.SetMode("list-verified-empty")
	last := f.call(t, nativeListArgs("--cursor", Str(Obj(nativeData(first)["page"]), "cursor"), "--json")...)
	requireNativeValue(t, Obj(nativeData(last)["page"])["hasMore"], false)
	requireNativeValue(t, Obj(nativeData(last)["page"])["total"], nil)
	requireNativeValue(t, Obj(nativeData(last)["page"])["totalKind"], "unknown")
	if !strings.Contains(Str(nativeData(last), "emptyReason"), "this page") || strings.Contains(Str(nativeData(last), "emptyReason"), "exhausted query scope") {
		t.Fatal("later empty reason overclaims exhausted query")
	}
	f.mock.SetMode("list-verified-missing-revision")
	missing := f.call(t, nativeListArgs("--revision", strings.Repeat("a", 40), "--vcs-root", "Payments_Git", "--require-complete", "--json")...)
	requireNativeCode(t, missing, 1)
	requireNativeValue(t, Obj(nativeData(missing)["selection"])["providerReturned"], 1)
	requireNativeValue(t, nativeData(missing)["runs"], []Object{})
	requireNativeValue(t, Obj(nativeData(missing)["page"])["hasMore"], false)
	requireNativeValue(t, Obj(nativeData(missing)["page"])["total"], nil)
	if !nativeNotes(missing, "REVISION_UNVERIFIED") {
		t.Fatal("unverified root revision retained")
	}
	if !strings.Contains(Str(nativeData(missing), "emptyReason"), "this page") || strings.Contains(Str(nativeData(missing), "emptyReason"), "exhausted query scope") {
		t.Fatal("filtered empty reason overclaims exhausted query")
	}
}

// Original: run-list.test.mjs — unsafe continuations preserve rows while scope, malformed and denied responses fail closed.
func TestRealCLIRunListUnsafeAndFailures(t *testing.T) {
	f := newNativeFixture(t, false)
	for _, mode := range []string{"list-unsafe", "list-escalating"} {
		f.mock.SetMode(mode)
		r := f.call(t, nativeListArgs("--require-complete", "--json")...)
		requireNativeCode(t, r, 1)
		if len(Objects(nativeData(r)["runs"])) != 1 || Obj(nativeData(r)["page"])["cursor"] != nil || !nativeNotes(r, "UNSAFE_CONTINUATION") {
			t.Fatal("unsafe continuation rows lost")
		}
		requireNativePartial(t, r)
	}
	for _, c := range []struct{ mode, code string }{{"denied", "PERMISSION_DENIED"}, {"malformed", "UPSTREAM_SCHEMA_MISMATCH"}, {"list-wrong-branch", "CONTEXT_MISMATCH"}, {"list-duplicate", "UPSTREAM_SCHEMA_MISMATCH"}} {
		f.mock.SetMode(c.mode)
		requireNativeError(t, f.call(t, nativeListArgs("--json")...), c.code)
	}
	f.mock.SetMode("list-page")
	first := f.call(t, nativeListArgs("--json")...)
	cursor, err := DecodeCursor(Str(Obj(nativeData(first)["page"]), "cursor"), time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	cursor.FilterHash = strings.Repeat("0", 64)
	forged, err := EncodeCursor(cursor, time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	requireNativeCode(t, f.call(t, nativeListArgs("--cursor", forged, "--json")...), 2)
	requireNativeCode(t, f.call(t, nativeListArgs("--cursor", "not-a-cursor", "--json")...), 2)
	f.mock.SetMode("list-unknown-result")
	r := f.call(t, nativeListArgs("--fields", "number", "--json")...)
	if Str(Objects(nativeData(r)["runs"])[0], "rawStatus") != "FUTURE_RESULT" {
		t.Fatal("raw unknown status dropped")
	}

	requireNativeValue(t, Objects(nativeData(r)["runs"])[0]["result"], "unknown")
	for _, token := range []string{forged, "not-a-cursor"} {
		f.mock.SetMode("list-page")
		rejected := f.call(t, nativeListArgs("--cursor", token, "--json")...)
		requireNativeValue(t, Obj(rejected.value["error"])["code"], "USAGE_ERROR")
	}
}

// Original: run-list.test.mjs — unknown-result pages retain candidate coverage, empty continuation and independent cursor identity.
func TestRealCLIUnknownResultCandidates(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("list-unknown-candidates")
	for _, flags := range nativeFormats() {
		first := f.call(t, nativeListArgs(append([]string{"--result", "unknown"}, flags...)...)...)
		requireNativeCode(t, first, 0)
		selection := Obj(nativeData(first)["selection"])
		requireNativeValue(t, selection["result"], "unknown")
		requireNativeValue(t, selection["resultBasis"], "normalized_candidates")
		requireNativeValue(t, selection["providerReturned"], 2)
		runs := Objects(nativeData(first)["runs"])
		requireNativeValue(t, len(runs), 1)
		requireNativeValue(t, runs[0]["id"], "482194")
		requireNativeValue(t, runs[0]["rawStatus"], "FUTURE_RESULT")
		requireNativeValue(t, Obj(nativeData(first)["page"])["hasMore"], true)
		token := Str(Obj(nativeData(first)["page"]), "cursor")
		next := f.call(t, nativeListArgs(append([]string{"--result", "unknown", "--cursor", token}, flags...)...)...)
		requireNativeCode(t, next, 0)
		requireNativeValue(t, Obj(nativeData(next)["selection"])["providerReturned"], 2)
		requireNativeValue(t, nativeData(next)["runs"], []Object{})
		requireNativeValue(t, Obj(nativeData(next)["page"])["hasMore"], true)
		if Str(Obj(nativeData(next)["page"]), "cursor") == "" || !containsString(Strings(Objects(next.value["next"])[0]["argv"]), "unknown") {
			t.Fatal("empty unknown continuation loses selector")
		}
		crossed := f.call(t, nativeListArgs("--cursor", token, "--json")...)
		requireNativeCode(t, crossed, 2)
		requireNativeValue(t, Obj(crossed.value["error"])["code"], "USAGE_ERROR")
		ordinary := f.call(t, nativeListArgs("--json")...)
		reverse := f.call(t, nativeListArgs("--result", "unknown", "--cursor", Str(Obj(nativeData(ordinary)["page"]), "cursor"), "--json")...)
		requireNativeCode(t, reverse, 2)
	}
	locators := 0
	for _, q := range f.mock.Requests() {
		if strings.HasSuffix(q.Path, "/app/rest/builds") {
			locators++
			if strings.Contains(q.Query.Get("locator"), "status:UNKNOWN") {
				t.Fatal("provider unknown filter fabricated")
			}
		}
	}
	if locators == 0 {
		t.Fatal("no bounded candidate page request")
	}
}

// Original: run-list.test.mjs — fractional windows survive provider bounds, exact verification, default lookback and cursor continuation.
func TestRealCLIFractionalWindows(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("list-fractional")
	base := []string{"run", "list", "--job", "Payments_Build", "--branch", "feature/refund"}
	args := append(append([]string{}, base...), "--since", "2026-10-01T13:00:00.999999999+02:00", "--until", "2026-10-01T11:00:01.000000001Z", "--json")
	r := f.call(t, args...)
	requireNativeCode(t, r, 0)
	runs := Objects(nativeData(r)["runs"])
	if len(runs) != 1 || Str(runs[0], "id") != "482194" {
		t.Fatalf("fractional membership: %s", r.stdout)
	}
	window := Obj(Obj(nativeData(r)["selection"])["window"])
	token := Str(Obj(nativeData(r)["page"]), "cursor")
	next := f.call(t, append(append([]string{}, base...), "--cursor", token, "--json")...)
	requireNativeCode(t, next, 0)
	if !reflect.DeepEqual(window, Obj(Obj(nativeData(next)["selection"])["window"])) {
		t.Fatal("window changed")
	}
	requireNativeCode(t, f.call(t, append(append([]string{}, base...), "--cursor", token, "--until", "2026-10-01T11:00:01.000000002Z", "--json")...), 2)
	r = f.call(t, append(append([]string{}, base...), "--until", "2026-10-01T11:00:01.000000001Z", "--json")...)
	if Str(Obj(Obj(nativeData(r)["selection"])["window"]), "since") != "2026-09-24T11:00:01.000000001Z" {
		t.Fatal("fractional lookback rounded")
	}

	requireNativeValue(t, window["since"], "2026-10-01T11:00:00.999999999Z")
	requireNativeValue(t, window["until"], "2026-10-01T11:00:01.000000001Z")
	cursor := nativeCursor(t, next)
	requireNativeValue(t, cursor.Window.Until, "2026-10-01T11:00:01.000000001Z")
	locators := []string{}
	for _, q := range f.mock.Requests() {
		if strings.HasSuffix(q.Path, "/app/rest/builds") {
			locators = append(locators, q.Query.Get("locator"))
		}
	}
	if len(locators) == 0 || !strings.Contains(locators[0], "finishDate:(date:20261001T110001.001+0000,condition:before)") || !strings.Contains(locators[0], "finishDate:(date:20261001T110000.999+0000,condition:after)") {
		t.Fatal("provider precision bounds weakened")
	}
}

// Original: run-list.test.mjs — long exact bounds preserve useful rows when continuation exceeds its cursor budget.
func TestRealCLILongWindowCursorBudget(t *testing.T) {
	f := newNativeFixture(t, false)
	f.mock.SetMode("list-page")
	until := "2026-10-02T00:00:00." + strings.Repeat("1", 2000) + "Z"
	for _, flags := range [][]string{{"--json"}, {}, {"--require-complete", "--json"}} {
		args := append([]string{"run", "list", "--job", "Payments_Build", "--branch", "feature/refund", "--until", until, "--max-bytes", "65536"}, flags...)
		r := f.call(t, args...)
		want := 0
		if containsString(flags, "--require-complete") {
			want = 1
		}
		requireNativeCode(t, r, want)
		requireNativePartial(t, r)
		requireNativeValue(t, len(Objects(nativeData(r)["runs"])), 1)
		window := Obj(Obj(nativeData(r)["selection"])["window"])
		requireNativeValue(t, window["until"], until)
		requireNativeValue(t, window["since"], strings.Replace(until, "2026-10-02", "2026-09-25", 1))
		requireNativeValue(t, Obj(nativeData(r)["page"])["cursor"], nil)
		requireNativeValue(t, Obj(nativeData(r)["page"])["hasMore"], true)
		if !nativeNotes(r, "CURSOR_LIMIT_EXCEEDED") {
			t.Fatal("long bounds discarded useful page")
		}
		requireNativeAbsent(t, r.value, "next")
	}
}

// Original: diagnostics.test.mjs — context is local until verify; verified scope and identity agree in JSON and TOON.
func TestRealCLIContextVerification(t *testing.T) {
	f := newNativeFixture(t, false)
	local := f.call(t, "context", "show", "--job", "Payments_Build", "--json")
	requireNativeCode(t, local, 0)
	requireNativeValue(t, len(f.mock.Requests()), 0)
	requireNativeValue(t, Obj(nativeData(local)["verification"])["requested"], false)
	longBranch := f.call(t, "context", "show", "--literal-branch", strings.Repeat("x", 300), "--json")
	requireNativeCode(t, longBranch, 0)
	requireNativeValue(t, len(Str(Obj(nativeData(longBranch)["checkout"]), "branch")), 300)
	for _, flags := range nativeFormats() {
		r := f.call(t, append([]string{"context", "show", "--verify", "--job", "Payments_Build"}, flags...)...)
		requireNativeCode(t, r, 0)
		verification := Obj(nativeData(r)["verification"])
		requireNativeValue(t, verification["authentication"], "authenticated")
		requireNativeValue(t, verification["policy"], "verified")
		requireNativeValue(t, Objects(verification["jobs"])[0]["projectId"], "Payments")
		if !strings.HasPrefix(Str(verification, "identityFingerprint"), "sha256:") || strings.Contains(r.stdout, "fixture-reader") {
			t.Fatal("verified context identity failed")
		}
	}
	requireNativeError(t, f.call(t, "context", "show", "--verify", "--job", "Payments_Build", "--project", "Other", "--json"), "CONTEXT_MISMATCH")
	for _, q := range f.mock.Requests() {
		if q.Path == "/teamcity/app/rest/projects" {
			t.Fatal("context verification used unbounded project enumeration")
		}
	}
}

// Original: diagnostics.test.mjs — offline doctor has no HTTP; online probes are bounded and unsupported optional logs stay explicit.
func TestRealCLIDoctorProbes(t *testing.T) {
	f := newNativeFixture(t, false)
	offline := f.call(t, "doctor", "--offline", "--json")
	requireNativeCode(t, offline, 0)
	requireNativeValue(t, len(f.mock.Requests()), 0)
	requireNativeValue(t, Obj(nativeData(offline)["executable"])["version"], "1.5.0")
	requireNativeValue(t, Obj(nativeData(offline)["authentication"])["state"], "not_checked")
	requireNativeValue(t, Obj(nativeMeta(offline)["counts"])["childProcesses"], 1)
	requireNativeValue(t, nativeData(offline)["liveCertified"], false)
	originalURL, originalToken := f.env["TEAMCITY_URL"], f.env["TEAMCITY_TOKEN"]
	delete(f.env, "TEAMCITY_URL")
	f.env["TEAMCITY_TOKEN"] = "offline-canary"
	unbound := f.call(t, "doctor", "--offline", "--json")
	requireNativeCode(t, unbound, 0)
	requireNativeValue(t, Obj(nativeData(unbound)["executable"])["version"], "1.5.0")
	requireNativeValue(t, len(f.mock.Requests()), 0)
	if strings.Contains(unbound.stdout, "offline-canary") {
		t.Fatal("offline credential leaked")
	}
	f.env["TEAMCITY_URL"], f.env["TEAMCITY_TOKEN"] = originalURL, originalToken
	online := f.call(t, "doctor", "--job", "Payments_Build", "--json")
	requireNativeCode(t, online, 0)
	requireNativePartial(t, online)
	requireNativeValue(t, Obj(nativeMeta(online)["counts"])["childProcesses"], 8)
	requireNativeValue(t, Obj(nativeData(online)["limits"])["maxChildProcesses"], 8)
	for _, name := range []string{"structuredRunDetail", "boundedRunPages", "structuredLogTail"} {
		requireNativeValue(t, nativeCapability(online, name)["state"], "available")
	}
	requireNativeValue(t, nativeCapability(online, "safeAgentRead")["state"], "not_probed")
	f.mock.SetMode("logs-unsupported")
	optional := f.call(t, "doctor", "--job", "Payments_Build", "--json")
	requireNativeCode(t, optional, 0)
	requireNativeValue(t, nativeCapability(optional, "structuredLogTail")["state"], "unavailable")
	if !nativeNotes(optional, "OPTIONAL_LOG_UNAVAILABLE") {
		t.Fatal("optional capability lost")
	}
	strict := f.call(t, "doctor", "--job", "Payments_Build", "--require-complete", "--json")
	requireNativeCode(t, strict, 1)
	requireNativePartial(t, strict)
	f.mock.SetMode("logs-hang")
	timed := f.call(t, "doctor", "--job", "Payments_Build", "--timeout", "2s", "--json")
	requireNativeCode(t, timed, 0)
	requireNativePartial(t, timed)
	requireNativeValue(t, Obj(nativeData(timed)["server"])["buildNumber"], "not-a-TeamCity-server")
	requireNativeValue(t, Obj(nativeData(timed)["authentication"])["state"], "authenticated")
	requireNativeValue(t, nativeCapability(timed, "structuredRunDetail")["state"], "available")
	if !nativeNotes(timed, "DEADLINE_EXCEEDED") {
		t.Fatal("deadline discarded acquired metadata")
	}
	for _, c := range []struct{ mode, code string }{{"denied", "PERMISSION_DENIED"}, {"expired", "AUTH_REQUIRED"}, {"project-wrong-id", "CONTEXT_MISMATCH"}, {"project-cycle", "UPSTREAM_SCHEMA_MISMATCH"}} {
		f.mock.SetMode(c.mode)
		requireNativeError(t, f.call(t, "context", "show", "--verify", "--project", "Payments", "--json"), c.code)
	}
}
