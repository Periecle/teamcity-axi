// Package livefixture implements bounded, read-only test tooling for the
// checksum-pinned official CLI. It does not create server fixtures or users.
package livefixture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Periecle/teamcity-axi/internal/axi"
)

type Result struct {
	Args   []string `json:"args,omitempty"`
	Code   int      `json:"code"`
	Signal *string  `json:"signal"`
	Stdout string   `json:"stdout"`
	Stderr string   `json:"stderr"`
}

func (r Result) Captured() axi.Captured {
	signal := ""
	if r.Signal != nil {
		signal = *r.Signal
	}
	return axi.Captured{Stdout: []byte(r.Stdout), Stderr: []byte(r.Stderr), ExitCode: r.Code, Signal: signal}
}

type Contract struct {
	SchemaVersion string            `json:"schemaVersion"`
	ObservedAt    string            `json:"observedAt"`
	Native        axi.Object        `json:"native"`
	Server        axi.Object        `json:"server"`
	Fixture       axi.Object        `json:"fixture"`
	Records       map[string]Result `json:"records"`
	Raw           axi.Object        `json:"-"`
}
type Fixture struct {
	Contract                                    Contract
	ServerURL, Dir, NativeBinary, WrapperBinary string
	env                                         []string
	secrets                                     []string
}

func RepositoryRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}
func ReadContract() (Contract, error) {
	raw, e := os.ReadFile(filepath.Join(RepositoryRoot(), "tests/fixtures/teamcity-2026.2-native-1.5.0/contract.json"))
	if e != nil {
		return Contract{}, e
	}
	var c Contract
	if e = json.Unmarshal(raw, &c); e != nil {
		return c, e
	}
	if e = json.Unmarshal(raw, &c.Raw); e != nil {
		return c, e
	}
	return c, nil
}
func NewFromEnvironment() (*Fixture, error) {
	return New(os.Getenv("TEAMCITY_AXI_LIVE_CREDENTIALS"), os.Getenv("TEAMCITY_AXI_TEST_BINARY"), os.Getenv("TEAMCITY_AXI_WRAPPER_BINARY"))
}
func New(credentialPath, native, wrapper string) (*Fixture, error) {
	if credentialPath == "" || native == "" {
		return nil, errors.New("Live tests require credential configuration path and checksum-verified native binary; they never skip")
	}
	fd, e := syscall.Open(credentialPath, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, errors.New("Cannot open live credential configuration")
	}
	file := os.NewFile(uintptr(fd), credentialPath)
	defer file.Close()
	st, e := file.Stat()
	if e != nil {
		return nil, errors.New("Cannot inspect live credential configuration")
	}
	owner, ok := st.Sys().(*syscall.Stat_t)
	if !st.Mode().IsRegular() || st.Size() > 65536 || st.Mode().Perm()&0077 != 0 || !ok || owner.Uid != uint32(os.Getuid()) {
		return nil, errors.New("Live credential configuration must be an owned private regular file")
	}
	raw, e := io.ReadAll(io.LimitReader(file, 65537))
	if e != nil {
		return nil, errors.New("Cannot read live credential configuration")
	}
	if len(raw) > 65536 {
		return nil, errors.New("Live credential configuration exceeds input limit")
	}
	var credentials struct{ Token, ServerURL, Password string }
	if json.Unmarshal(raw, &credentials) != nil {
		return nil, errors.New("Invalid live credential configuration JSON")
	}
	if credentials.Token == "" || len(credentials.Token) > 8192 || credentials.ServerURL == "" {
		return nil, errors.New("Invalid live credential configuration")
	}
	server, e := axi.CanonicalURL(credentials.ServerURL, true)
	if e != nil {
		return nil, errors.New("Invalid live server URL")
	}
	contract, e := ReadContract()
	if e != nil {
		return nil, e
	}
	native, e = filepath.Abs(native)
	if e != nil {
		return nil, errors.New("Invalid native executable path")
	}
	binary, e := os.ReadFile(native)
	if e != nil {
		return nil, errors.New("Cannot read native executable")
	}
	sha := sha256.Sum256(binary)
	if hex.EncodeToString(sha[:]) != axi.Str(contract.Native, "binarySha256") {
		return nil, errors.New("Live contract requires its pinned native executable")
	}
	dir, e := os.MkdirTemp("", "axi-live-")
	if e != nil {
		return nil, e
	}
	f := &Fixture{Contract: contract, ServerURL: server, Dir: dir, NativeBinary: native, WrapperBinary: wrapper, secrets: []string{credentials.Token}}
	if credentials.Password != "" {
		f.secrets = append(f.secrets, credentials.Password)
	}
	f.env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "XDG_CONFIG_HOME=" + dir, "TEAMCITY_URL=" + server, "TEAMCITY_TOKEN=" + credentials.Token, "TEAMCITY_RO=1", "TEAMCITY_NO_UPDATE=1", "DO_NOT_TRACK=1", "NO_COLOR=1", "TERM=dumb"}
	config := axi.UserConfig{SchemaVersion: "1.0", ReadOnly: true, DefaultServer: "sandbox", BinaryPath: native, Servers: map[string]axi.ServerConfig{"sandbox": {URL: server, AllowHTTPLoopback: true, AllowedProjects: []string{axi.Str(contract.Fixture, "projectId")}}}}
	if e = os.Mkdir(filepath.Join(dir, "teamcity-axi"), 0700); e == nil {
		b, _ := json.Marshal(config)
		e = os.WriteFile(filepath.Join(dir, "teamcity-axi/config.json"), b, 0600)
	}
	if e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func (f *Fixture) Close() error { return os.RemoveAll(f.Dir) }

type captureBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow func()
	exceeded bool
}

func (b *captureBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		b.exceeded = true
		b.overflow()
		return 0, io.ErrShortBuffer
	}
	return b.buffer.Write(p)
}

func (b *captureBuffer) Len() int       { return b.buffer.Len() }
func (b *captureBuffer) String() string { return b.buffer.String() }
func (f *Fixture) invoke(executable string, args []string, expired bool) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = f.Dir
	cmd.Env = append([]string{}, f.env...)
	if expired {
		for i, v := range cmd.Env {
			if strings.HasPrefix(v, "TEAMCITY_TOKEN=") {
				cmd.Env[i] = "TEAMCITY_TOKEN=invalid-synthetic-token"
			}
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 400 * time.Millisecond
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	out := &captureBuffer{limit: 2097152, overflow: cancel}
	stderr := &captureBuffer{limit: 65536, overflow: cancel}
	cmd.Stdout = out
	cmd.Stderr = stderr
	if e := cmd.Start(); e != nil {
		return Result{}, errors.New("Cannot start live test executable")
	}
	e := cmd.Wait()
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if ctx.Err() != nil {
		if out.exceeded {
			return Result{}, errors.New("Live stdout exceeded capture limit")
		}
		if stderr.exceeded {
			return Result{}, errors.New("Live stderr exceeded capture limit")
		}
		return Result{}, errors.New("Live operation exceeded its deadline or capture limit")
	}
	if e != nil {
		var exited *exec.ExitError
		if !errors.As(e, &exited) {
			return Result{}, errors.New("Live operation output could not be captured")
		}
	}
	result := Result{Code: cmd.ProcessState.ExitCode(), Args: append([]string{}, args...)}
	if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		s := status.Signal().String()
		result.Signal = &s
	}
	result.Stdout = axi.SanitizeText(out.String(), f.secrets)
	if executable == f.NativeBinary && (len(args) > 0 && (args[0] == "api" || args[0] == "run")) {
		result.Stdout, e = SanitizeStdout(out.String(), f.secrets)
		if e != nil {
			return Result{}, e
		}
	}
	if executable == f.NativeBinary {
		if stderr.Len() > 0 {
			result.Stderr = "[Native stderr omitted from public live captures]"
		}
	} else {
		result.Stderr = axi.SanitizeText(stderr.String(), f.secrets)
	}
	return result, nil
}
func (f *Fixture) Native(args []string, expired bool) (Result, error) {
	return f.invoke(f.NativeBinary, args, expired)
}
func (f *Fixture) Wrapper(args []string) (Result, error) {
	if f.WrapperBinary == "" {
		return Result{}, errors.New("Live wrapper executable is required")
	}
	return f.invoke(f.WrapperBinary, args, false)
}

var sessionID = regexp.MustCompile(`(?i);TCSESSIONID=[^/?#\s"\\]+`)
var sessionCookie = regexp.MustCompile(`(?im)^Set-Cookie:[^\r\n]*`)

func SanitizeStdout(stdout string, secrets []string) (string, error) {
	offset, separator := -1, "\n\n"
	if strings.HasPrefix(stdout, "HTTP/1.1 ") {
		offset = strings.Index(stdout, "\n\n")
		crlf := strings.Index(stdout, "\r\n\r\n")
		if offset < 0 || crlf >= 0 && crlf < offset {
			offset = crlf
			separator = "\r\n\r\n"
		}
		if offset < 0 {
			return "", errors.New("Native HTTP capture lacks a complete envelope")
		}
	}
	headers, payload := "", stdout
	if offset >= 0 {
		headers = sessionCookie.ReplaceAllString(axi.SanitizeText(stdout[:offset], secrets), "Set-Cookie: [REDACTED]") + separator
		payload = stdout[offset+len(separator):]
	}
	if payload == "" {
		return headers, nil
	}
	decoded, e := axi.DecodeJSON([]byte(payload))
	if e != nil {
		return headers + "[Unparseable payload omitted from public live capture]", nil
	}
	sanitized := axi.Sanitize(decoded, secrets, nil)
	var scrub func(any) any
	scrub = func(value any) any {
		switch v := value.(type) {
		case string:
			return sessionID.ReplaceAllString(v, ";TCSESSIONID=[REDACTED]")
		case []any:
			for i, x := range v {
				v[i] = scrub(x)
			}
		case map[string]any:
			for key, x := range v {
				v[key] = scrub(x)
			}
		}
		return value
	}
	safe := scrub(sanitized)
	raw, _ := json.Marshal(decoded)
	clean, _ := json.Marshal(safe)
	if bytes.Equal(raw, clean) {
		return headers + payload, nil
	}
	return headers + string(clean), nil
}
func body(records map[string]Result, name string) (axi.Object, error) {
	r, present := records[name]
	if !present {
		return nil, fmt.Errorf("Missing live capture %s", name)
	}
	raw, e := axi.ParseRaw(r.Captured())
	if e != nil {
		return nil, e
	}
	o, ok := raw.Body.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Invalid live capture %s", name)
	}
	return o, nil
}
func integer(value any) int {
	switch v := value.(type) {
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}
func identity(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}
func sameStrings(a, b []string) bool {
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}
func VerifyCapture(records map[string]Result, c Contract) error {
	server, e := body(records, "server")
	if e != nil || identity(server["buildNumber"]) != identity(c.Server["buildNumber"]) {
		return errors.New("Captured server build differs from the test fixture")
	}
	if e = VerifyPermissions(records, c, "permissions"); e != nil {
		return e
	}
	if _, present := records["outcome-permissions"]; present {
		if e = VerifyPermissions(records, c, "outcome-permissions"); e != nil {
			return e
		}
	}
	for _, row := range []struct{ name, id, job, status string }{{"run-detail", axi.Str(c.Fixture, "failedRunId"), axi.Str(c.Fixture, "jobId"), "FAILURE"}, {"green", axi.Str(c.Fixture, "greenRunId"), axi.Str(c.Fixture, "greenJobId"), "SUCCESS"}} {
		run, e := body(records, row.name)
		bt := axi.Obj(run["buildType"])
		if e != nil || identity(run["id"]) != row.id || axi.Str(run, "buildTypeId") != row.job || axi.Str(bt, "id") != row.job || axi.Str(bt, "projectId") != axi.Str(c.Fixture, "projectId") || axi.Str(run, "state") != "finished" || axi.Str(run, "status") != row.status {
			return errors.New("Captured execution identity or outcome differs from the fixture")
		}
	}
	for _, row := range []struct{ name, code string }{{"denied", "PERMISSION_DENIED"}, {"missing", "NOT_FOUND"}, {"invalid-auth", "AUTH_REQUIRED"}, {"bounded-jobs-denied", "PERMISSION_DENIED"}, {"queue-project-denied", "PERMISSION_DENIED"}, {"agents-project-denied", "PERMISSION_DENIED"}, {"agents-pool", "NOT_FOUND"}, {"agent-detail-missing", "NOT_FOUND"}, {"agent-detail-incompatible", "NOT_FOUND"}} {
		_, e := body(records, row.name)
		if e == nil || axi.AsDomainError(e).Code != row.code {
			return errors.New("Captured error contract differs from the restricted test fixture")
		}
	}
	for _, row := range []struct{ name, id string }{{"bounded-jobs", axi.Str(c.Fixture, "jobId")}, {"bounded-jobs-next", axi.Str(c.Fixture, "greenJobId")}} {
		page, e := body(records, row.name)
		jobs := axi.Objects(page["buildType"])
		if e != nil || integer(page["count"]) != 1 || len(jobs) != 1 || axi.Str(jobs[0], "id") != row.id || axi.Str(jobs[0], "projectId") != axi.Str(c.Fixture, "projectId") || jobs[0]["paused"] != false || axi.Str(page, "nextHref") == "" {
			return errors.New("Captured job page identity or scope differs from the fixture")
		}
	}
	empty, e := body(records, "bounded-jobs-empty")
	_, present := empty["nextHref"]
	if e != nil || integer(empty["count"]) != 0 || empty["buildType"] == nil || len(axi.Objects(empty["buildType"])) != 0 || present {
		return errors.New("Captured empty job page differs from the fixture")
	}
	ids, jobs := axi.Strings(c.Fixture["queuedRunIds"]), axi.Strings(c.Fixture["queueJobIds"])
	for _, row := range []struct {
		name  string
		index int
	}{{"queue-project-positive", 0}, {"queue-project-next", 1}, {"queue-intersection", 0}} {
		page, e := body(records, row.name)
		runs := axi.Objects(page["build"])
		if e != nil || integer(page["count"]) != 1 || len(runs) != 1 || len(ids) < 2 || len(jobs) < 2 {
			return errors.New("Captured queue identity, scope or lifecycle differs from the fixture")
		}
		run := runs[0]
		bt := axi.Obj(run["buildType"])
		if identity(run["id"]) != ids[row.index] || axi.Str(run, "buildTypeId") != jobs[row.index] || axi.Str(bt, "id") != jobs[row.index] || axi.Str(bt, "projectId") != axi.Str(c.Fixture, "projectId") || axi.Str(run, "state") != "queued" || axi.Str(run, "waitReason") == "" || axi.Str(run, "queuedDate") == "" {
			return errors.New("Captured queue identity, scope or lifecycle differs from the fixture")
		}
	}
	queued, e := body(records, "queued-run-detail")
	if e != nil || identity(queued["id"]) != ids[0] || axi.Str(queued, "state") != "queued" || axi.Str(queued, "buildTypeId") != jobs[0] || axi.Str(axi.Obj(queued["buildType"]), "projectId") != axi.Str(c.Fixture, "projectId") {
		return errors.New("Captured exact queued run differs from the fixture")
	}
	for _, name := range []string{"agent-detail", "agent-detail-job", "agent-detail-project"} {
		agent, e := body(records, name)
		pool := axi.Obj(agent["pool"])
		if e != nil || identity(agent["id"]) != axi.Str(c.Fixture, "agentId") || axi.Str(agent, "name") != "axi-contract-agent" || identity(pool["id"]) != axi.Str(c.Fixture, "agentPoolId") || axi.Str(pool, "name") != axi.Str(c.Fixture, "agentPoolName") || agent["connected"] != true || agent["enabled"] != true || agent["authorized"] != true {
			return errors.New("Captured exact agent identity or availability differs from the fixture")
		}
	}
	for _, name := range []string{"agents-project", "agents-job"} {
		page, e := body(records, name)
		agents := axi.Objects(page["agent"])
		if e != nil || integer(page["count"]) != 1 || len(agents) != 1 || identity(agents[0]["id"]) != axi.Str(c.Fixture, "agentId") || identity(axi.Obj(agents[0]["pool"])["id"]) != axi.Str(c.Fixture, "agentPoolId") {
			return errors.New("Captured scoped agent identity differs from the fixture")
		}
	}
	for _, name := range []string{"agents-project-next", "agents-job-next", "agents-impossible"} {
		page, e := body(records, name)
		_, present := page["nextHref"]
		if e != nil || integer(page["count"]) != 0 || page["agent"] == nil || len(axi.Objects(page["agent"])) != 0 || present {
			return errors.New("Captured empty bounded agent page differs from the fixture")
		}
	}
	return nil
}
func VerifyPermissions(records map[string]Result, c Contract, name string) error {
	page, e := body(records, name)
	if e != nil {
		return errors.New("Live identity does not match the restricted fixture permission inventory")
	}
	rows := axi.Objects(page["permissionAssignment"])
	if page["permissionAssignment"] == nil {
		return errors.New("Live identity does not match the restricted fixture permission inventory")
	}
	expected := []string{axi.Str(c.Fixture, "projectId"), "_Root"}
	lifecycle := axi.Str(axi.Obj(c.Fixture["lifecycle"]), "projectId")
	for _, row := range rows {
		if axi.Str(axi.Obj(row["permission"]), "id") == "view_project" && axi.Str(axi.Obj(row["project"]), "id") == lifecycle && lifecycle != "" {
			expected = append(expected, lifecycle)
			break
		}
	}
	permissions, projects := []string{}, []string{}
	for _, row := range rows {
		id := axi.Str(axi.Obj(row["permission"]), "id")
		permissions = append(permissions, id)
		if id == "view_project" {
			if row["isGlobalScope"] != false {
				return errors.New("Live identity does not match the restricted fixture permission inventory")
			}
			projects = append(projects, axi.Str(axi.Obj(row["project"]), "id"))
		}
	}
	required := []string{"change_own_profile"}
	for range expected {
		required = append(required, "view_project")
	}
	if !sameStrings(permissions, required) || !sameStrings(projects, expected) {
		return errors.New("Live identity does not match the restricted fixture permission inventory")
	}
	return nil
}
func (f *Fixture) Record(output string) error {
	if output == "" {
		return errors.New("Set TEAMCITY_AXI_LIVE_OUTPUT to an explicit destination for sanitized test captures")
	}
	records := map[string]Result{}
	names := []string{}
	for name := range f.Contract.Records {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		record := f.Contract.Records[name]
		value, e := f.Native(record.Args, name == "invalid-auth")
		if e != nil {
			return e
		}
		records[name] = value
	}
	if e := VerifyCapture(records, f.Contract); e != nil {
		return e
	}
	result := f.Contract.Raw
	result["observedAt"] = axi.ObservedAt()
	result["records"] = records
	raw, e := json.MarshalIndent(result, "", "  ")
	if e != nil {
		return e
	}
	file, e := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer file.Close()
	_, e = file.Write(append(raw, '\n'))
	return e
}
