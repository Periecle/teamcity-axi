package axi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	toon "github.com/toon-format/toon-go"
)

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil || AsDomainError(err).Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}
func TestParserContracts(t *testing.T) {
	valid := [][]string{{"run", "view", "482193"}, {"run", "tests", "12", "--failed", "--include-muted", "--limit", "100"}, {"run", "list", "--job", "Build", "--literal-branch", "@this", "--fields", "id,number"}, {"run", "watch", "12", "--interval", "5s", "--timeout", "2m"}, {"schema", "run.view", "--json"}, {"--help"}, {"run", "view", "--help"}, {"run", "list", "--since", "2026-10-01T00:00:00.000001Z", "--until", "2026-10-01T00:00:00.000002Z"}}
	for _, a := range valid {
		t.Run(strings.Join(a, " "), func(t *testing.T) {
			if _, err := Parse(a); err != nil {
				t.Fatal(err)
			}
		})
	}
	invalid := [][]string{{"api", "/app/rest/builds"}, {"run", "log", "1", "--limit", "1"}, {"run", "list", "--limit", "1.5"}, {"run", "tests", "1", "--test", "o1", "--failed"}, {"run", "log", "1", "--failed", "--tail", "80"}, {"run", "list", "--since", "bad"}, {"run", "log", "1", "--contains", "--full"}, {"run", "start"}, {"run", "view", ""}, {"run", "view", "0"}, {"run", "view", "9007199254740993"}, {"run", "list", "--stat", "failure"}, {"run", "view", "1", "--job", "A", "--job", "B"}, {"run", "view", "1", "--help=true"}, {"run", "list", "--limit", "0"}, {"run", "list", "--limit", "101"}, {"run", "view", "1", "--timeout", "100"}, {"run", "view", "1", "--timeout", "999999999999999999999999999m"}, {"run", "view", "1", "--json", "--format", "toon"}, {"run", "list", "--branch", "x", "--all-branches"}, {"run", "tests", "1", "--failed", "--muted"}, {"run", "tests", "1", "--include-muted"}, {"run", "tests", "1", "--test", "a", "--limit", "1"}, {"run", "problems", "1", "--problem", "a", "--cursor", "x"}, {"run", "log", "1", "--failed", "--contains", "a"}, {"run", "list", "--fields", "number,number"}, {"run", "list", "--fields", "password"}, {"run", "list", "--since", "2026-10-02T00:00:00Z", "--until", "2026-10-01T00:00:00Z"}, {"run", "list", "--since", "2026-02-30T00:00:00Z"}, {"run", "list", "--since", "2026-10-01T00:00:00Z", "--state", "running"}, {"run", "watch", "1", "--interval", "4999ms"}, {"job", "view", "\x1bfoo"}, {"--version"}, {"--" + strings.Repeat("x", 4096)}, {"run", "view", "1", "extra"}}
	for _, a := range invalid {
		t.Run("reject "+strings.Join(a, " "), func(t *testing.T) { _, err := Parse(a); requireCode(t, err, "USAGE_ERROR") })
	}
	first, firstErr := Parse([]string{"--server", "work", "run", "view", "482193", "--json"})
	last, lastErr := Parse([]string{"run", "view", "482193", "--server", "work", "--json"})
	if firstErr != nil || lastErr != nil || !reflect.DeepEqual(first, last) || first.Positional != "482193" {
		t.Fatal("Global flag ordering changed identity", firstErr, lastErr)
	}
	for _, check := range []struct {
		args []string
		key  string
		want any
	}{
		{[]string{"run", "tree", "1", "--depth", "0"}, "depth", 0},
		{[]string{"run", "watch", "1", "--interval", "5s"}, "interval", 5000},
		{[]string{"run", "view", "--help"}, "help", true},
		{[]string{"run", "log", "1", "--contains=--error=Connection refused"}, "contains", "--error=Connection refused"},
	} {
		parsed, err := Parse(check.args)
		if err != nil || !reflect.DeepEqual(parsed.Flags[check.key], check.want) {
			t.Fatal("Literal or typed flag changed", check.args, parsed, err)
		}
	}
	jsonParsed, jsonErr := Parse([]string{"--json", "--format", "json"})
	if jsonErr != nil || jsonParsed.Format != "json" {
		t.Fatal("Equivalent JSON flags conflict", jsonErr)
	}
	_, err := Parse(make([]string, 101))
	requireCode(t, err, "USAGE_ERROR")
}
func TestTimePrecision(t *testing.T) {
	for input, want := range map[string]string{"2026-10-02T02:00:00.000000001+02:00": "2026-10-02T00:00:00.000000001Z", "2026-10-02T00:00:00.123400Z": "2026-10-02T00:00:00.1234Z"} {
		got, err := CanonicalTimestamp(input)
		if err != nil || got != want {
			t.Fatal("Canonicalization changed fractional precision", got, err)
		}
	}
	shifted, shiftErr := ShiftTimestamp("2026-10-02T00:00:00.000000001Z", -7*86400)
	if shiftErr != nil || shifted != "2026-09-25T00:00:00.000000001Z" {
		t.Fatal("Lookback lost fractional precision", shifted, shiftErr)
	}
	v, err := CanonicalTimestamp("2026-10-01T02:00:00.123400000000000001+02:00")
	if err != nil || v != "2026-10-01T00:00:00.123400000000000001Z" {
		t.Fatalf("precision lost: %s %v", v, err)
	}
	for _, pair := range [][2]string{{"2026-10-01T00:00:00.0000000001Z", "2026-10-01T00:00:00.0000000002Z"}, {"2026-10-01T00:00:00Z", "2026-10-01T00:00:01Z"}} {
		n, err := CompareTimestamps(pair[0], pair[1])
		if err != nil || n != -1 {
			t.Fatal(n, err)
		}
	}
	n, err := CompareTimestamps("2026-10-01T02:00:00+02:00", "2026-10-01T00:00:00.000Z")
	if n != 0 || err != nil {
		t.Fatal(n, err)
	}
	for _, v := range []string{"2025-02-29T00:00:00Z", "2026-10-01T24:00:00Z", "2026-10-01T00:00:60Z", "2026-13-01T00:00:00Z", "2026-01-01T00:00:00+24:00", "nope"} {
		_, e := CanonicalTimestamp(v)
		if e == nil {
			t.Fatalf("accepted %s", v)
		}
	}
	for _, tc := range []struct{ v, condition, want string }{{"2026-10-01T00:00:00.000001Z", "after", "20261001T000000+0000"}, {"2026-10-01T00:00:00.000001Z", "before", "20261001T000000.001+0000"}, {"2026-10-01T00:00:00.9999Z", "before", "20261001T000001+0000"}, {"9999-12-31T23:59:59.999999Z", "before", ""}, {"2026-10-02T00:00:00.123999999Z", "after", "20261002T000000.123+0000"}, {"2026-10-02T00:00:00.123999999Z", "before", "20261002T000000.124+0000"}, {"2026-10-02T00:00:00.000Z", "before", "20261002T000000+0000"}} {
		got, e := ProviderDate(tc.v, tc.condition)
		if got != tc.want || e != nil {
			t.Fatal(got, e)
		}
	}
}

func TestEffectiveReadLimitsRespectProfilesAndTrustedCeilings(t *testing.T) {
	deadline := time.Now().UnixMilli() + 10000
	for _, profile := range []struct {
		command                        string
		children, concurrency, capture int
	}{{"run.view", 8, 3, 2097152}, {"run.list", 8, 3, 2097152}, {"status", 6, 2, 1048576}, {"run.watch", 32, 1, 1048576}, {"run.tree", 24, 3, 2097152}, {"run.failure", 24, 3, 2097152}} {
		ec := ExecutionContext{Deadline: deadline}
		want := ProcessLimits{Deadline: deadline, MaxChildren: profile.children, Concurrency: profile.concurrency, StdoutBytes: profile.capture, StderrBytes: 65536}
		if got := ReadLimits(ec, ReadProfile(profile.command)); got != want {
			t.Fatalf("Wrong profile for %s: %+v", profile.command, got)
		}
		ec.Config = &UserConfig{Limits: UserLimits{Concurrency: 8, MaxChildProcesses: 256}}
		if got := ReadLimits(ec, ReadProfile(profile.command)); got != want {
			t.Fatal("Configuration expanded read authority", got)
		}
		ec.Config.Limits = UserLimits{Concurrency: 1, MaxChildProcesses: 2}
		tighter := ReadLimits(ec, ReadProfile(profile.command))
		want.Concurrency, want.MaxChildren = 1, 2
		if tighter != want || !reflect.DeepEqual(PublicLimits(tighter, 4096), Object{"maxBytes": 4096, "maxChildProcesses": 2, "concurrency": 1, "deadline": deadline, "stdoutCaptureBytes": profile.capture, "stderrCaptureBytes": 65536}) {
			t.Fatal("Effective diagnostic limits differ from enforced limits")
		}
	}
}
func TestOutputExamplesAndTOON(t *testing.T) {
	files, err := filepath.Glob("../../examples/*.json")
	if err != nil || len(files) == 0 {
		t.Fatal("missing examples")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			b, e := os.ReadFile(file)
			if e != nil {
				t.Fatal(e)
			}
			var value Object
			if e = json.Unmarshal(b, &value); e != nil {
				t.Fatal(e)
			}
			if strings.Contains(file, "config.json") {
				name := strings.TrimSuffix(filepath.Base(file), ".json")
				if e = ValidateConfig(name, value); e != nil {
					t.Fatal(e)
				}
				return
			}
			var response Response
			if e = json.Unmarshal(b, &response); e != nil {
				t.Fatal(e)
			}
			jsonResult, e := Render(response, "json", 262144, nil, nil, nil)
			if e != nil {
				t.Fatal(e)
			}
			toonResult, e := Render(response, "toon", 262144, nil, nil, nil)
			if e != nil {
				t.Fatal(e)
			}
			decoded, e := toon.Decode([]byte(toonResult.Document))
			if e != nil {
				t.Fatal(e)
			}
			var jsonResultValue any
			json.Unmarshal([]byte(jsonResult.Document), &jsonResultValue)
			if !reflect.DeepEqual(decoded, jsonResultValue) {
				t.Fatalf("TOON/JSON differ: %s", file)
			}
		})
	}
	r := NewResponse("schema", Object{"strings": []string{"123", "001", "true", "null"}, "unicode": "ą日本語🦊", "nested": []Object{{"x": nil, "n": 1}, {"x": "1", "n": 0}}})
	result, e := Render(r, "toon", 16384, nil, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := toon.Decode([]byte(result.Document))
	if e != nil {
		t.Fatal(e)
	}
	want, _ := jsonValue(r)
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("logical types differ: %#v != %#v", decoded, want)
	}
}

func TestConcurrentPackagedSchemaValidationFailsClosed(t *testing.T) {
	files, err := filepath.Glob("../../examples/*.json")
	if err != nil || len(files) == 0 {
		t.Fatal("missing schema validation examples")
	}
	var wait sync.WaitGroup
	start := make(chan struct{})
	for _, file := range files {
		bytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for worker := 0; worker < 8; worker++ {
			wait.Add(1)
			go func(file string, bytes []byte) {
				defer wait.Done()
				<-start
				var value Object
				if err := json.Unmarshal(bytes, &value); err != nil {
					t.Error(err)
					return
				}
				name := "response"
				if strings.Contains(file, "config.json") {
					name = strings.TrimSuffix(filepath.Base(file), ".json")
					if err := ValidateConfig(name, value); err != nil {
						t.Error(err)
					}
				} else {
					var response Response
					if err := json.Unmarshal(bytes, &response); err != nil {
						t.Error(err)
						return
					}
					if err := ValidateResponse(response); err != nil {
						t.Error(err)
					}
					response.Status = "unsupported"
					if ValidateResponse(response) == nil {
						t.Error("concurrent cached validation admitted an invalid envelope")
					}
				}
				if validateSchema(name, nil) == nil {
					t.Error("concurrent cached validation admitted a null contract")
				}
				schema, err := PackagedSchema(name)
				if err != nil {
					t.Error(err)
					return
				}
				delete(schema, "type") // A caller's schema copy cannot change compiled contracts.
				if validateSchema(name, nil) == nil {
					t.Error("schema inspection mutated the packaged validation contract")
				}
			}(file, bytes)
		}
	}
	close(start)
	wait.Wait()
}
func TestSanitizerContracts(t *testing.T) {
	secrets := KnownSecrets(map[string]string{"APP_TOKEN": "short-canary", "TEAMCITY_HEADER_X_CUSTOM": "prefix-short-canary-suffix"}, nil, []string{"TEAMCITY_HEADER_X_CUSTOM"})
	if got := SanitizeText("prefix-short-canary-suffix", secrets); got != "[REDACTED]" {
		t.Fatal(got)
	}
	headerOutput, headerErr := Render(NewResponse("schema", Object{"branch": "prefix-short-canary-suffix"}), "json", 2048, secrets, nil, nil)
	if headerErr != nil || Str(headerOutput.Response.Data, "branch") != "[REDACTED]" {
		t.Fatal("Forwarded header value reached public data", headerErr)
	}
	patternSecrets := KnownSecrets(map[string]string{"COMPANY_CREDENTIAL": "pattern-canary"}, []string{"^COMPANY_CREDENTIAL$"}, nil)
	if !reflect.DeepEqual(patternSecrets, []string{"pattern-canary"}) {
		t.Fatal("Trusted credential pattern did not select environment secret", patternSecrets)
	}
	patternOutput, patternErr := Render(NewResponse("schema", Object{"message": "arbitrary-private-text"}), "json", 2048, nil, []string{"^message$"}, nil)
	if patternErr != nil || Str(patternOutput.Response.Data, "message") != "[REDACTED]" {
		t.Fatal("Trusted field pattern did not sanitize arbitrary data", patternErr)
	}
	for _, tc := range []struct{ value, want string }{{"\x1b]0;injected title\x07safe", "safe"}, {"\u202e", "\\u202e"}, {"Authorization: Basic YWRtaW46cGFzc3dvcmQ=", "Authorization: Basic [REDACTED]"}, {"-----BEGIN RSA PRIVATE KEY-----\nprivate body\n-----END RSA PRIVATE KEY-----", "[REDACTED PRIVATE KEY]"}, {"https://user:pass@example.test", "https://[REDACTED]@example.test"}, {"can\x1b[31mary-secret", "[REDACTED]"}} {
		if got := SanitizeText(tc.value, []string{"canary-secret"}); got != tc.want {
			t.Fatalf("unexpected sanitized content: %q", got)
		}
	}
	for _, p := range []string{"(a+)+$", ".*.*.*", "foo|bar", "x{3}"} {
		_, err := SecretMatchers([]string{p})
		if err == nil {
			t.Fatal("accepted unsafe pattern")
		}
	}
	for _, format := range []string{"json", "toon"} {
		r := NewResponse("schema", Object{"text": "\x1b[31mcanary-secret\x1b[0m\u202eevil", "password": "unknown", "message": "Bearer abc123", "COMPANY_CREDENTIAL": "unknown-private-value"})
		out, e := Render(r, format, 2048, []string{"canary-secret"}, []string{"^COMPANY_CREDENTIAL$"}, nil)
		if e != nil {
			t.Fatal(e)
		}
		for _, s := range []string{"canary-secret", "abc123", "unknown-private-value", "\x1b", "\u202e"} {
			if strings.Contains(out.Document, s) {
				t.Fatal("sanitization failed")
			}
		}
	}
	for _, secret := range []string{"1.0", "meta"} {
		r := NewResponse("schema", Object{"foo": "x"})
		out, e := Render(r, "json", 2048, []string{secret}, nil, nil)
		if e != nil || out.Response.SchemaVersion != "1.0" || out.Response.Meta == nil {
			t.Fatal("structural constants corrupted", e)
		}
	}
}
func TestOutputBudgetsPreserveEvidence(t *testing.T) {
	for _, format := range []string{"json", "toon"} {
		r := NewResponse("run.view", Object{"run": Object{"id": "1", "jobId": "Build", "state": "finished", "result": "failure", "statusText": strings.Repeat("🦊", 100000)}})
		limits := Object{"maxBytes": 4096, "maxChildProcesses": 4, "concurrency": 1, "deadline": time.Now().UnixMilli() + 10000, "stdoutCaptureBytes": 2097152, "stderrCaptureBytes": 65536}
		out, e := Render(r, format, 2048, nil, nil, limits)
		if e != nil {
			t.Fatal(e)
		}
		if len(out.Document) > 2048 || out.Response.Status != "error" || Bool(out.Response.Meta, "complete") || !Bool(out.Response.Meta, "truncated") || out.Response.Error.Code != "INPUT_LIMIT_EXCEEDED" {
			t.Fatal("unsafe truncation")
		}
		if Str(out.Response.Error.Details, "limit") != "maxBytes" || Int(out.Response.Error.Details, "ceiling") != 2048 || Int(out.Response.Error.Details, "observed") <= 2048 {
			t.Fatal("limit evidence incorrect")
		}
		wantLimits := Object{}
		for key, value := range limits {
			wantLimits[key] = value
		}
		wantLimits["maxBytes"] = 2048
		if !reflect.DeepEqual(out.Response.Meta["limits"], wantLimits) {
			t.Fatal("Oversized error lost effective limit diagnostics", out.Response.Meta["limits"])
		}
		r = NewResponse("run.view", Object{"run": Object{"id": "1", "jobId": "Build", "state": "finished", "result": "failure"}})
		r.Context = Object{"server": "work", "job": "Payments_Build", "branch": strings.Repeat("b", 5000)}
		bounded, boundedErr := Render(r, format, 2048, nil, nil, nil)
		if boundedErr != nil || Str(bounded.Response.Context, "server") != "work" || Str(bounded.Response.Context, "job") != "Payments_Build" {
			t.Fatal("Scoped error lost identities", boundedErr)
		}
		r.Context = Object{"server": strings.Repeat("a", 64), "job": strings.Repeat("日", 256), "project": strings.Repeat("日", 256), "branch": strings.Repeat("b", 5000)}
		out, e = Render(r, format, 2048, nil, nil, nil)
		if e != nil || out.Response.Context["job"] != r.Context["job"] || out.Response.Context["project"] != r.Context["project"] {
			t.Fatal("Bounded UTF8 identities changed", e)
		}
		r.Context["vcsRootId"] = strings.Repeat("日", 256)
		out, e = Render(r, format, 2048, nil, nil, nil)
		if e != nil || out.Response.Context["server"] != r.Context["server"] || len(out.Document) > 2048 || out.Response.Status != "error" {
			t.Fatal("trusted destination lost", e)
		}
		r = NewResponse("run.view", Object{"run": Object{"id": "1", "jobId": "Build", "state": "finished", "result": "failure"}})
		out, e = Render(r, format, 2048, nil, nil, limits)
		if e != nil {
			t.Fatal(e)
		}
		if _, ok := out.Response.Meta["limits"]; ok {
			t.Fatal("unused limits emitted")
		}
		if !reflect.DeepEqual(out.Response, r) {
			t.Fatal("Renderer changed a response without byte reduction")
		}
		r.Next = []Object{{"reason": "Optional read", "argv": []string{"teamcity-axi", "run", "list", "--job", "Build", "--literal-branch", strings.Repeat("x", 3000)}}}
		out, e = Render(r, format, 2048, nil, nil, limits)
		if e != nil || out.Response.Status != "ok" || out.Response.Next != nil || Int(Obj(out.Response.Meta["limits"]), "maxBytes") != 2048 {
			t.Fatal("hint reduction incorrect", e)
		}
		if !reflect.DeepEqual(out.Response.Meta["limits"], wantLimits) {
			t.Fatal("Hint reduction lost effective limit diagnostics")
		}
	}
}
func TestDiagnosticCompactionPreservesUnknownRuns(t *testing.T) {
	for _, count := range []int{100, 200} {
		r := NewResponse("schema", Object{"runs": []Object{}})
		r.Status = "partial"
		r.Meta["complete"] = false
		notes := []Limitation{}
		for i := 1; i <= count; i++ {
			r.Data["runs"] = append(r.Data["runs"].([]Object), Object{"id": fmt.Sprint(i), "result": "unknown"})
			notes = append(notes, Limitation{Code: "OUTCOME_METADATA_UNAVAILABLE", Source: "run", RunID: fmt.Sprint(i), Message: "Explicit execution outcome metadata is unavailable"})
		}
		notes = append(notes, Limitation{Code: "SCAN_COVERAGE_UNKNOWN", Source: "run", Message: "Exhaustion is unverified"})
		r.Meta["limitations"] = notes
		for _, format := range []string{"json", "toon"} {
			out, e := Render(r, format, 65536, nil, nil, nil)
			if e != nil {
				t.Fatal(e)
			}
			retained := Objects(out.Response.Data["runs"])
			grouped := Limitations(out.Response.Meta["limitations"])
			if len(retained) != count || len(grouped) != 2 || out.Response.Status != "partial" || Bool(out.Response.Meta, "complete") {
				t.Fatal("compaction lost outcomes")
			}
			for _, run := range retained {
				if Str(run, "result") != "unknown" {
					t.Fatal("Compaction invented a known outcome")
				}
			}
			if grouped[0].RunID != "" || !strings.Contains(grouped[0].Message, fmt.Sprintf("%d distinct executions affected", count)) || grouped[1].Code != "SCAN_COVERAGE_UNKNOWN" {
				t.Fatal("Grouped diagnostics lost uncertainty or execution coverage", grouped)
			}
		}
	}
}
func TestLocalFastPathsAndErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TEAMCITY_TOKEN", "executable-canary")
	t.Setenv("TEAMCITY_URL", "https://attacker.invalid")
	for _, args := range [][]string{{"--version"}, {"-v"}, {"-V"}, {"--help"}, {"run", "view", "--help"}, {"run", "tests", "--help", "--json"}, {"schema", "run.view", "--json"}} {
		var out, stderr bytes.Buffer
		code := Run(context.Background(), args, &out, &stderr)
		if code != 0 || stderr.Len() != 0 || strings.Contains(out.String(), "executable-canary") {
			t.Fatalf("fast path failed: %v code=%d %s", args, code, out.String())
		}
		if len(args) == 1 && contains([]string{"--version", "-v", "-V"}, args[0]) && strings.TrimSpace(out.String()) != Version {
			t.Fatal("Version fast path changed its contract")
		}
		if args[0] == "schema" {
			var response Response
			if json.Unmarshal(out.Bytes(), &response) != nil || Str(Obj(response.Data["descriptor"]), "name") != "run.view" {
				t.Fatal("Offline schema lost exact command identity")
			}
		}
	}
	for _, args := range [][]string{{"--" + strings.Repeat("a", 3000), "--json"}, {"run", "list", "--stat", "failure", "--json"}, {"run", "view", "9007199254740993", "--json"}, {"run", "start", "--json"}} {
		var out, stderr bytes.Buffer
		code := Run(context.Background(), args, &out, &stderr)
		var r Response
		if json.Unmarshal(out.Bytes(), &r) != nil || code != 2 || r.Error == nil || r.Error.Code != "USAGE_ERROR" || stderr.Len() != 0 {
			t.Fatal("unsafe usage path", code, out.String())
		}
	}
}

func TestShortSecretPreservesExecutableErrorStructure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("TEAMCITY_TOKEN", "meta")
	for _, key := range []string{"TEAMCITY_URL", "TEAMCITY_AXI_SERVER"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"--json"}, &stdout, &stderr)
	var response Response
	if code != 1 || stderr.Len() != 0 || json.Unmarshal(stdout.Bytes(), &response) != nil || response.Error == nil || response.Error.Code != "AUTH_CONTEXT_MISMATCH" || response.Meta == nil {
		t.Fatal("Short secret corrupted executable error protocol", code, stdout.String())
	}
	if err := ValidateResponse(response); err != nil {
		t.Fatal(err)
	}
}
func TestUnconfiguredHomeAndRemoteContextGate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PATH", "/nonexistent")
	for _, key := range []string{"TEAMCITY_URL", "TEAMCITY_TOKEN", "TEAMCITY_AXI_SERVER"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
	var out, stderr bytes.Buffer
	code := Run(context.Background(), nil, &out, &stderr)
	if code != 0 || stderr.Len() != 0 || out.Len() >= 6144 {
		t.Fatal("unconfigured home failed", code, out.String())
	}
	v, err := toon.Decode(out.Bytes())
	if err != nil || Str(Obj(Obj(v)["data"]), "mode") != "unconfigured" {
		t.Fatal("home did not roundtrip", err)
	}
	out.Reset()
	code = Run(context.Background(), []string{"run", "view", "1", "--json"}, &out, &stderr)
	var response Response
	if json.Unmarshal(out.Bytes(), &response) != nil || code != 2 || response.Error == nil || response.Error.Code != "CONTEXT_REQUIRED" {
		t.Fatal("remote trusted gate failed", out.String())
	}
}
func TestMain(m *testing.M) {
	if os.Getenv("TEAMCITY_HEADER_X_AXI_FIXTURE") == "1" {
		fakeNative()
		return
	}
	os.Exit(m.Run())
}
func fakeNative() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("teamcity version 1.5.0")
		return
	}
	mode := "inspect"
	if len(os.Args) > 2 {
		if _, v, ok := strings.Cut(os.Args[2], "fields="); ok {
			mode = v
		}
	}
	if os.Getenv("TEAMCITY_HEADER_X_AXI_DESCENDANT") == "1" {
		signalIgnoreTERM()
		for {
			time.Sleep(time.Second)
		}
	}
	switch mode {
	case "hang":
		for {
			time.Sleep(time.Second)
		}
	case "wait":
		time.Sleep(100 * time.Millisecond)
	case "huge":
		os.Stdout.Write(bytes.Repeat([]byte{'a'}, 2097153))
		return
	case "huge-stderr":
		os.Stderr.Write(bytes.Repeat([]byte{'a'}, 65537))
		return
	case "nonzero":
		fmt.Print(`{"valid":true}`)
		os.Exit(17)
	case "grandchild":
		exe, _ := os.Executable()
		child := exec.Command(exe)
		child.Env = append(os.Environ(), "TEAMCITY_HEADER_X_AXI_DESCENDANT=1")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if child.Start() != nil {
			os.Exit(3)
		}
		os.WriteFile(os.Getenv("TEAMCITY_HEADER_X_FIXTURE_PID_PATH"), []byte(fmt.Sprint(child.Process.Pid)), 0600)
		for {
			time.Sleep(time.Second)
		}
	}
	env := Environment()
	present := env["TEAMCITY_TOKEN"] != ""
	delete(env, "TEAMCITY_TOKEN")
	cwd, _ := os.Getwd()
	b, _ := json.Marshal(Object{"args": os.Args[1:], "cwd": cwd, "tokenPresent": present, "env": env})
	fmt.Print(string(b))
}
func signalIgnoreTERM() { // The descendant must survive SIGTERM so cleanup proves group escalation.
	signal.Ignore(syscall.SIGTERM)
}
func transportFixture(t *testing.T, limits ProcessLimits) *ProcessTransport {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	if limits.Deadline == 0 {
		limits = ProcessLimits{Deadline: time.Now().UnixMilli() + 5000, Concurrency: 3, MaxChildren: 8, StdoutBytes: 2097152, StderrBytes: 65536}
	}
	env := map[string]string{"HOME": t.TempDir(), "PATH": os.Getenv("PATH"), "TEAMCITY_URL": "https://teamcity.example.test/teamcity", "TEAMCITY_TOKEN": "transport-canary", "TEAMCITY_GUEST": "1", "TEAMCITY_JOB": "Attacker", "BUILD_URL": "https://attacker.invalid", "NODE_OPTIONS": "--throw-deprecation", "UNRELATED": "drop-me", "TEAMCITY_HEADER_X_AXI_FIXTURE": "1", "TEAMCITY_HEADER_X_FIXTURE_PID_PATH": filepath.Join(t.TempDir(), "pid")}
	transport, e := NewProcessTransport(TransportOptions{Binary: exe, ServerURL: env["TEAMCITY_URL"], Env: env, HeaderNames: []string{"TEAMCITY_HEADER_X_AXI_FIXTURE", "TEAMCITY_HEADER_X_FIXTURE_PID_PATH"}, Limits: limits})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { transport.Close() })
	return transport
}
func TestTransportFrozenEnvironmentAndCWD(t *testing.T) {
	tr := transportFixture(t, ProcessLimits{})
	captured, err := tr.Execute(context.Background(), Operation{Kind: "api", Path: "/app/rest/builds?fields=inspect"}, -1)
	if err != nil {
		t.Fatal(err)
	}
	var v Object
	json.Unmarshal(captured.Stdout, &v)
	if Str(v, "cwd") == "" || Str(v, "cwd") == tr.options.Env["HOME"] || !Bool(v, "tokenPresent") {
		t.Fatal("child context not isolated")
	}
	env := Obj(v["env"])
	for _, k := range []string{"TEAMCITY_RO", "TEAMCITY_NO_UPDATE", "DO_NOT_TRACK", "NO_COLOR"} {
		if Str(env, k) != "1" {
			t.Fatal(k)
		}
	}
	for _, k := range []string{"TEAMCITY_GUEST", "TEAMCITY_JOB", "TEAMCITY_HEADER_AUTHORIZATION", "BUILD_URL", "NODE_OPTIONS", "UNRELATED"} {
		if _, ok := env[k]; ok {
			t.Fatal("untrusted env forwarded")
		}
	}
	cwd, _ := os.Getwd()
	if Str(v, "cwd") == cwd {
		t.Fatal("Child inherited repository CWD")
	}
	argv := []string{"api", "/app/rest/builds?fields=inspect", "-X", "GET", "--include", "--raw", "-H", "Accept: application/json", "--no-input"}
	if !reflect.DeepEqual(Strings(v["args"]), argv) {
		t.Fatal("argv changed")
	}
	tr.Close()
	if _, e := os.Stat(Str(v, "cwd")); !os.IsNotExist(e) {
		t.Fatal("neutral directory leaked")
	}
}
func TestTransportPrelaunchSecurity(t *testing.T) {
	for _, env := range []map[string]string{{"TEAMCITY_TOKEN": "canary"}, {"TEAMCITY_TOKEN": "canary", "TEAMCITY_URL": "https://other.test"}, {"TEAMCITY_TOKEN": "canary", "TEAMCITY_URL": "https://server.test/different"}} {
		_, e := ChildEnvironment(env, "https://server.test/teamcity", nil)
		requireCode(t, e, "AUTH_CONTEXT_MISMATCH")
	}
	for _, n := range []string{"TEAMCITY_HEADER_HOST", "TEAMCITY_HEADER_AUTHORIZATION", "TEAMCITY_HEADER_COOKIE"} {
		_, e := ChildEnvironment(nil, "https://server.test", []string{n})
		requireCode(t, e, "POLICY_DENIED")
	}
	tr := transportFixture(t, ProcessLimits{})
	for _, path := range []string{"https://evil.test/app/rest/builds", "//evil.test/app/rest/builds", "/app/rest/builds/../agents", "/app/rest/builds/%2e%2e/agents", "/app/rest/builds/%2e%2e/agents?fields=id", "/app/rest/builds/id:123/..?fields=id", "/app/rest/builds/%ZZ", "/app/rest/users", "/app/rest/users/current/tokens?fields=id,username", "/app/rest/users/current?fields=password", "/app/rest/users/current?fields=id,username&fields=password", "/app/rest/users/id:1?fields=id,username", "/app/rest/builds?unexpected=1", "/app/rest/builds#fragment"} {
		_, e := tr.Execute(context.Background(), Operation{Kind: "api", Path: path}, -1)
		requireCode(t, e, "POLICY_DENIED")
	}
	if tr.ChildProcesses() != 0 {
		t.Fatal("forged request launched child")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "teamcity")
	os.WriteFile(exe, []byte("fixture"), 0700)
	_, e := ResolveBinary(exe, dir, false, nil)
	requireCode(t, e, "POLICY_DENIED")
	hidden := filepath.Join(dir, "..hidden", "teamcity")
	if err := os.Mkdir(filepath.Dir(hidden), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hidden, []byte("untrusted"), 0700); err != nil {
		t.Fatal(err)
	}
	_, e = ResolveBinary(hidden, dir, false, nil)
	requireCode(t, e, "POLICY_DENIED")
	resolved, e := ResolveBinary(exe, dir, true, nil)
	if e != nil || resolved != exe {
		t.Fatal(e)
	}
}
func TestTransportConcurrentAndReservedLimits(t *testing.T) {
	l := ProcessLimits{Deadline: time.Now().UnixMilli() + 5000, Concurrency: 1, MaxChildren: 3, StdoutBytes: 2097152, StderrBytes: 65536}
	for _, reserved := range []int{-1, 2} {
		l.Deadline = time.Now().UnixMilli() + 10000
		tr := transportFixture(t, l)
		for _, invalid := range []int{-2, 257} {
			_, err := tr.Execute(context.Background(), Operation{Kind: "version"}, invalid)
			requireCode(t, err, "INTERNAL_ERROR")
		}
		if tr.ChildProcesses() != 0 {
			t.Fatal("Invalid budget launched child")
		}
		var wg sync.WaitGroup
		results := make(chan error, 4)
		start := time.Now()
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, e := tr.Execute(context.Background(), Operation{Kind: "api", Path: "/app/rest/builds?fields=wait"}, reserved)
				results <- e
			}()
		}
		wg.Wait()
		close(results)
		success := 0
		for e := range results {
			if e == nil {
				success++
			} else {
				want := "INPUT_LIMIT_EXCEEDED"
				if reserved >= 0 {
					want = "CALL_LIMIT_EXCEEDED"
				}
				requireCode(t, e, want)
				if reserved < 0 && !reflect.DeepEqual(AsDomainError(e).Details, Object{"limit": "maxChildProcesses", "ceiling": 3, "observed": 3}) {
					t.Fatal("Launch limit diagnostic differs from enforced ceiling", e)
				}
			}
		}
		want := 3
		if reserved >= 0 {
			want = 2
		}
		if success != want || tr.ChildProcesses() != want || time.Since(start) < time.Duration(want)*100*time.Millisecond {
			t.Fatal("concurrency or launch bound violated")
		}
		if reserved >= 0 {
			_, e := tr.Execute(context.Background(), Operation{Kind: "version"}, -1)
			if e != nil || tr.ChildProcesses() != 3 {
				t.Fatal("reserved launch lost", e)
			}
		}
		_, err := tr.Execute(context.Background(), Operation{Kind: "version"}, 256)
		requireCode(t, err, "INPUT_LIMIT_EXCEEDED")
		if tr.ChildProcesses() != 3 {
			t.Fatal("Per-read budget raised configured launch ceiling")
		}
	}
}
func TestTransportCaptureAndExit(t *testing.T) {
	tr := transportFixture(t, ProcessLimits{})
	for _, mode := range []string{"huge", "huge-stderr"} {
		_, e := tr.Execute(context.Background(), Operation{Kind: "api", Path: "/app/rest/builds?fields=" + mode}, -1)
		requireCode(t, e, "INPUT_LIMIT_EXCEEDED")
		d := AsDomainError(e).Details
		limit, ceiling := "stdoutCaptureBytes", 2097152
		if mode == "huge-stderr" {
			limit, ceiling = "stderrCaptureBytes", 65536
		}
		if Str(d, "limit") != limit || Int(d, "ceiling") != ceiling || Int(d, "observed") <= Int(d, "ceiling") {
			t.Fatal("missing capture evidence")
		}
	}
	r, e := tr.Execute(context.Background(), Operation{Kind: "api", Path: "/app/rest/builds?fields=nonzero"}, -1)
	if e != nil || r.ExitCode != 17 || string(r.Stdout) != `{"valid":true}` {
		t.Fatal("exit conflated with JSON", e)
	}
}
func TestTransportAbortActiveAndQueued(t *testing.T) {
	tr := transportFixture(t, ProcessLimits{Deadline: time.Now().UnixMilli() + 5000, Concurrency: 1, MaxChildren: 5, StdoutBytes: 2097152, StderrBytes: 65536})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, e := tr.Execute(ctx, Operation{Kind: "api", Path: "/app/rest/builds?fields=hang"}, -1)
			results <- e
		}()
	}
	time.Sleep(100 * time.Millisecond)
	cancel()
	for i := 0; i < 2; i++ {
		requireCode(t, <-results, "INTERRUPTED")
	}
	if tr.ChildProcesses() != 1 {
		t.Fatal("waiting child launched after abort")
	}
}
func TestTransportDeadlineKillsDescendants(t *testing.T) {
	tr := transportFixture(t, ProcessLimits{Deadline: time.Now().UnixMilli() + 500, Concurrency: 1, MaxChildren: 1, StdoutBytes: 2097152, StderrBytes: 65536})
	_, e := tr.Execute(context.Background(), Operation{Kind: "api", Path: "/app/rest/builds?fields=grandchild"}, -1)
	requireCode(t, e, "DEADLINE_EXCEEDED")
	details := AsDomainError(e).Details
	if Str(details, "limit") != "deadline" || int64(Int(details, "ceiling")) != tr.options.Limits.Deadline || Int(details, "observed") < Int(details, "ceiling") {
		t.Fatal("Missing exact deadline diagnostic", details)
	}
	data, e := os.ReadFile(tr.options.Env["TEAMCITY_HEADER_X_FIXTURE_PID_PATH"])
	if e != nil {
		t.Fatal(e)
	}
	pid := strings.TrimSpace(string(data))
	for i := 0; i < 30; i++ {
		stat, e := os.ReadFile("/proc/" + pid + "/stat")
		if os.IsNotExist(e) {
			return
		}
		parts := strings.Fields(string(stat))
		if len(parts) > 2 && parts[2] == "Z" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("descendant still alive")
}
