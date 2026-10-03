//go:build realcli

package axi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Periecle/teamcity-axi/internal/testfixture"
	toon "github.com/toon-format/toon-go"
)

const nativeCanary = "fixture-secret-"

var realWrapper struct {
	sync.Once
	path, native string
	err          error
}

type nativeMock = testfixture.Server
type nativeMockRequest = testfixture.Request

func newNativeMock(t *testing.T, tree bool) *nativeMock {
	t.Helper()
	m := testfixture.NewServer(testfixture.Options{Tree: tree})
	t.Cleanup(m.Close)
	return m
}
func longNativeCanary() string { return nativeCanary + strings.Repeat("q", 1600) }

func realCLIExecutable(t *testing.T) (string, string) {
	t.Helper()
	realWrapper.Do(func() {
		binary := os.Getenv("TEAMCITY_AXI_TEST_BINARY")
		if binary == "" {
			realWrapper.err = fmt.Errorf("TEAMCITY_AXI_TEST_BINARY is required; the official-binary suite never skips")
			return
		}
		binary, e := filepath.Abs(binary)
		if e != nil {
			realWrapper.err = e
			return
		}
		data, e := os.ReadFile(binary)
		if e != nil {
			realWrapper.err = e
			return
		}
		hash := sha256.Sum256(data)
		manifest, e := os.ReadFile("../../docs/compatibility.json")
		if e != nil {
			realWrapper.err = e
			return
		}
		var metadata Object
		if e = json.Unmarshal(manifest, &metadata); e != nil {
			realWrapper.err = e
			return
		}
		verified := false
		for _, a := range Objects(metadata["artifacts"]) {
			if Str(a, "binarySha256") == hex.EncodeToString(hash[:]) {
				verified = true
			}
		}
		if !verified {
			realWrapper.err = fmt.Errorf("native executable checksum is absent from compatibility manifest")
			return
		}
		dir, e := os.MkdirTemp("", "axi-realcli-wrapper-")
		if e != nil {
			realWrapper.err = e
			return
		}
		realWrapper.path = filepath.Join(dir, "teamcity-axi")
		cmd := exec.Command("go", "build", "-o", realWrapper.path, "../../cmd/teamcity-axi")
		if output, e := cmd.CombinedOutput(); e != nil {
			realWrapper.err = fmt.Errorf("build Go wrapper: %w: %s", e, output)
			return
		}
		realWrapper.native = binary
	})
	if realWrapper.err != nil {
		t.Fatal(realWrapper.err)
	}
	return realWrapper.path, realWrapper.native
}

type nativeCall struct {
	code           int
	stdout, stderr string
	value          Object
}
type nativeFixture struct {
	mock                 *nativeMock
	dir, wrapper, native string
	config               UserConfig
	env                  map[string]string
}

func newNativeFixture(t *testing.T, tree bool) *nativeFixture {
	t.Helper()
	wrapper, native := realCLIExecutable(t)
	mock := newNativeMock(t, tree)
	dir := t.TempDir()
	f := &nativeFixture{mock: mock, dir: dir, wrapper: wrapper, native: native, config: UserConfig{SchemaVersion: "1.0", ReadOnly: true, DefaultServer: "work", BinaryPath: native, Servers: map[string]ServerConfig{"work": {URL: mock.BaseURL, AllowHTTPLoopback: true, AllowedProjects: []string{"Payments"}}}}, env: map[string]string{"HOME": dir, "XDG_CONFIG_HOME": dir, "PATH": os.Getenv("PATH"), "TEAMCITY_URL": mock.BaseURL, "TEAMCITY_TOKEN": "fixture-only-token", "APP_TOKEN": longNativeCanary()}}
	f.writeConfig(t)
	return f
}
func (f *nativeFixture) writeConfig(t *testing.T) {
	t.Helper()
	os.MkdirAll(filepath.Join(f.dir, "teamcity-axi"), 0700)
	data, _ := json.Marshal(f.config)
	if err := os.WriteFile(filepath.Join(f.dir, "teamcity-axi", "config.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func (f *nativeFixture) call(t *testing.T, args ...string) nativeCall {
	return f.callSignal(t, 0, 0, args...)
}
func (f *nativeFixture) callSignal(t *testing.T, signal syscall.Signal, delay time.Duration, args ...string) nativeCall {
	return f.callWithSignalTrigger(t, signal, delay, nil, args...)
}
func (f *nativeFixture) callSignalAfterFirstRequest(t *testing.T, signal syscall.Signal, delay time.Duration, args ...string) nativeCall {
	t.Helper()
	return f.callWithSignalTrigger(t, signal, delay, func(requests []nativeMockRequest) bool {
		if len(requests) > 1 {
			t.Fatal("signal test must reach the first polling sleep before a second HTTP request")
		}
		return len(requests) == 1
	}, args...)
}
func (f *nativeFixture) callSignalAfterPolicyRequest(t *testing.T, signal syscall.Signal, args ...string) nativeCall {
	t.Helper()
	return f.callWithSignalTrigger(t, signal, 0, func(requests []nativeMockRequest) bool {
		for _, request := range requests {
			if strings.Contains(request.Path, "UGF5bWVudHNfQ2hpbGQ") {
				return true
			}
		}
		return false
	}, args...)
}
func (f *nativeFixture) callWithSignalTrigger(t *testing.T, signal syscall.Signal, delay time.Duration, trigger func([]nativeMockRequest) bool, args ...string) nativeCall {
	t.Helper()
	if i := indexString(args, "--"); i >= 0 && len(args) > i+1 && args[len(args)-1] == "--json" {
		args = append(append([]string{"--json"}, args[:len(args)-1]...), []string{}...)
	}
	cmd := exec.Command(f.wrapper, args...)
	cmd.Dir = f.dir
	for k, v := range f.env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var timer *time.Timer
	if signal != 0 {
		if trigger != nil {
			deadline := time.Now().Add(5 * time.Second)
			for !trigger(f.mock.Requests()) {
				if time.Now().After(deadline) {
					cmd.Process.Kill()
					cmd.Wait()
					t.Fatal("signal test did not reach the required HTTP request")
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
		timer = time.AfterFunc(delay, func() { cmd.Process.Signal(signal) })
	}
	err := cmd.Wait()
	if timer != nil {
		timer.Stop()
	}
	code := 0
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			code = e.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	r := nativeCall{code: code, stdout: stdout.String(), stderr: stderr.String()}
	jsonFormat := containsString(args, "--json") || containsString(args, "--format=json")
	var decodeErr error
	if jsonFormat {
		decodeErr = json.Unmarshal(stdout.Bytes(), &r.value)
	} else {
		var value any
		value, decodeErr = toon.Decode(stdout.Bytes())
		r.value = Obj(value)
	}
	if decodeErr != nil {
		t.Fatalf("decode result code %d stdout %q stderr %q: %v", code, r.stdout, r.stderr, decodeErr)
	}
	data, _ := json.Marshal(r.value)
	var response Response
	if err = json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	if err = ValidateResponse(response); err != nil {
		t.Fatalf("response schema: %v\n%s", err, r.stdout)
	}
	if strings.Contains(r.stdout, "fixture-only-token") || strings.Contains(r.stdout, longNativeCanary()[:80]) || strings.Contains(r.stdout, "server-private-parameter") {
		t.Fatal("credential/private DTO leaked")
	}
	if r.stderr != "" && !containsString(args, "--debug") {
		t.Fatalf("unexpected stderr: %s", r.stderr)
	}
	for _, hint := range Objects(r.value["next"]) {
		argv := Strings(hint["argv"])
		if len(argv) == 0 {
			t.Fatal("empty hint")
		}
		if _, e := Parse(argv[1:]); e != nil {
			t.Fatalf("hint invalid %v: %v", argv, e)
		}
	}
	if Str(r.value, "status") == "partial" && nativeMeta(r)["complete"] != false {
		t.Fatal("partial response marked complete")
	}
	if Str(r.value, "status") == "error" {
		requireNativeAbsent(t, r.value, "data")
	}
	if r.value["next"] == nil {
		requireNativeAbsent(t, r.value, "next")
	}
	requireNativeReadOnly(t, f.mock)
	for _, request := range f.mock.Requests() {
		fields := request.Query.Get("fields")
		for _, private := range []string{"parameters", "properties", "environment", "canceledInfo(user"} {
			if strings.Contains(fields, private) {
				t.Fatalf("private field requested: %s", fields)
			}
		}
	}
	return r
}
func requireNativeCode(t *testing.T, r nativeCall, code int) {
	t.Helper()
	if r.code != code {
		t.Fatalf("exit %d want %d: %s %s", r.code, code, r.stdout, r.stderr)
	}
}
func requireNativeError(t *testing.T, r nativeCall, code string) {
	t.Helper()
	requireNativeCode(t, r, 1)
	requireNativeValue(t, r.value["status"], "error")
	requireNativeAbsent(t, r.value, "data")
	if strings.Contains(r.stdout, "Forbidden") {
		t.Fatal("foreign/private identity leaked in error")
	}
	if Str(Obj(r.value["error"]), "code") != code || r.value["data"] != nil {
		t.Fatalf("expected %s error: %s", code, r.stdout)
	}
}
func requireNativeReadOnly(t *testing.T, m *nativeMock) {
	t.Helper()
	for _, r := range m.Requests() {
		if r.Method != "GET" || !r.Authenticated {
			t.Fatalf("unsafe request: %#v", r)
		}
	}
}
func nativeData(r nativeCall) Object { return Obj(r.value["data"]) }
func nativeMeta(r nativeCall) Object { return Obj(r.value["meta"]) }
func nativeNotes(r nativeCall, code string) bool {
	for _, n := range Objects(nativeMeta(r)["limitations"]) {
		if Str(n, "code") == code {
			return true
		}
	}
	return false
}

func indexString(a []string, value string) int {
	for i, v := range a {
		if v == value {
			return i
		}
	}
	return -1
}
