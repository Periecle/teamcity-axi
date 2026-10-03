package axi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Operation struct {
	Kind, Path, RunID string
	Tail              int
}
type Captured struct {
	Stdout, Stderr []byte
	ExitCode       int
	Signal         string
}
type TransportOptions struct {
	Binary, ServerURL string
	Env               map[string]string
	Limits            ProcessLimits
	HeaderNames       []string
}
type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	total    int
	exceeded func(int)
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.total += len(p)
	if b.total > b.limit {
		if b.exceeded != nil {
			b.exceeded(b.total)
		}
		return 0, io.ErrShortBuffer
	}
	return b.buffer.Write(p)
}
func (b *limitedBuffer) String() string { return b.buffer.String() }
func (b *limitedBuffer) Bytes() []byte  { return b.buffer.Bytes() }
func ResolveBinary(path, repository string, allowWorkspace bool, env map[string]string) (string, error) {
	paths := []string{}
	if path != "" {
		if !filepath.IsAbs(path) {
			return "", Usage("Trusted binaryPath must be absolute")
		}
		paths = append(paths, path)
	} else {
		for _, dir := range filepath.SplitList(env["PATH"]) {
			if filepath.IsAbs(dir) {
				paths = append(paths, filepath.Join(dir, "teamcity"))
			}
		}
	}
	for _, candidate := range paths {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		st, err := os.Stat(resolved)
		if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0111 == 0 {
			continue
		}
		if repository != "" && inside(resolved, repository) && !(allowWorkspace && path != "") {
			return "", NewError("POLICY_DENIED", "Native executable resolves inside the untrusted workspace", 1)
		}
		return resolved, nil
	}
	return "", NewError("DEPENDENCY_MISSING", "A trusted official teamcity executable is required", 1)
}

var safeHeaderName = regexp.MustCompile(`^TEAMCITY_HEADER_[A-Z0-9_]+$`)
var forbiddenHeaderName = regexp.MustCompile(`AUTHORIZATION|HOST|COOKIE|PROXY_AUTHORIZATION|CONNECTION|TRANSFER_ENCODING`)

func ChildEnvironment(env map[string]string, serverURL string, headers []string) (map[string]string, error) {
	child := map[string]string{}
	for _, n := range []string{"HOME", "PATH", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_RUNTIME_DIR", "USER", "LOGNAME", "LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR", "HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY", "NO_PROXY", "https_proxy", "http_proxy", "all_proxy", "no_proxy"} {
		if v, ok := env[n]; ok {
			child[n] = v
		}
	}
	if env["TEAMCITY_TOKEN"] != "" {
		u, err := CanonicalURL(env["TEAMCITY_URL"], true)
		target, e := CanonicalURL(serverURL, true)
		if err != nil || e != nil || u != target {
			return nil, NewError("AUTH_CONTEXT_MISMATCH", "Inherited token is not bound to the frozen trusted server", 1)
		}
		child["TEAMCITY_TOKEN"] = env["TEAMCITY_TOKEN"]
	}
	for _, n := range headers {
		if !safeHeaderName.MatchString(n) || forbiddenHeaderName.MatchString(n) {
			return nil, NewError("POLICY_DENIED", "Unsafe custom header environment name", 1)
		}
		if v, ok := env[n]; ok {
			if strings.ContainsAny(v, "\r\n\x00") {
				return nil, NewError("POLICY_DENIED", "Invalid custom header value", 1)
			}
			child[n] = v
		}
	}
	for k, v := range map[string]string{"TEAMCITY_URL": serverURL, "TEAMCITY_RO": "1", "TEAMCITY_NO_UPDATE": "1", "DO_NOT_TRACK": "1", "NO_COLOR": "1", "TERM": "dumb"} {
		child[k] = v
	}
	return child, nil
}
func OperationArgv(op Operation) ([]string, error) {
	switch op.Kind {
	case "version":
		return []string{"--version"}, nil
	case "log":
		n, err := strconv.ParseUint(op.RunID, 10, 64)
		if !positiveID.MatchString(op.RunID) || err != nil || n > 9007199254740991 || op.Tail < 1 || op.Tail > 1000 {
			return nil, Usage("Invalid bounded log request")
		}
		return []string{"run", "log", op.RunID, "--tail", strconv.Itoa(op.Tail), "--json", "--no-input"}, nil
	case "api":
	default:
		return nil, NewError("POLICY_DENIED", "Unsupported adapter operation", 1)
	}
	path := op.Path
	rawPath := strings.Split(path, "?")[0]
	if len(path) > 16384 || !strings.HasPrefix(path, "/app/rest/") || strings.ContainsAny(path, "\r\n\x00#\\") || unsafeURLEncoding.MatchString(rawPath) {
		return nil, NewError("POLICY_DENIED", "Unsafe adapter path", 1)
	}
	for _, p := range strings.Split(rawPath, "/") {
		if p == "." || p == ".." {
			return nil, NewError("POLICY_DENIED", "Unsafe adapter path", 1)
		}
	}
	u, err := url.Parse(path)
	if err != nil || u.Host != "" || u.Scheme != "" {
		return nil, NewError("POLICY_DENIED", "Invalid adapter path", 1)
	}
	parts := strings.Split(u.Path, "/")
	if len(parts) < 4 {
		return nil, NewError("POLICY_DENIED", "Unsafe adapter path", 1)
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, NewError("POLICY_DENIED", "Invalid adapter query", 1)
	}
	families := []string{"server", "builds", "buildTypes", "projects", "buildQueue", "agents", "testOccurrences", "problemOccurrences", "changes"}
	if !contains(families, parts[3]) && !(u.Path == "/app/rest/users/current" && q.Get("fields") == "id,username" && len(q) == 1 && len(q["fields"]) == 1) {
		return nil, NewError("POLICY_DENIED", "Resource or query is outside the read-only adapter surface", 1)
	}
	for k, v := range q {
		if !contains([]string{"locator", "fields"}, k) || len(v) != 1 {
			return nil, NewError("POLICY_DENIED", "Resource or query is outside the read-only adapter surface", 1)
		}
	}
	for _, p := range strings.Split(u.Path, "/") {
		if p == "." || p == ".." {
			return nil, NewError("POLICY_DENIED", "Unsafe path encoding", 1)
		}
	}
	return []string{"api", path, "-X", "GET", "--include", "--raw", "-H", "Accept: application/json", "--no-input"}, nil
}

type ProcessTransport struct {
	options  TransportOptions
	cwd      string
	env      []string
	sem      chan struct{}
	mu       sync.Mutex
	launches int
	disposed bool
	cancel   context.CancelFunc
	life     context.Context
	wg       sync.WaitGroup
}

func NewProcessTransport(options TransportOptions) (*ProcessTransport, error) {
	l := options.Limits
	if !filepath.IsAbs(options.Binary) || l.Concurrency < 1 || l.Concurrency > 8 || l.MaxChildren < 1 || l.MaxChildren > 256 || l.Deadline <= 0 || l.StdoutBytes < 1 || l.StdoutBytes > 2097152 || l.StderrBytes < 1 || l.StderrBytes > 65536 {
		return nil, NewError("INTERNAL_ERROR", "Invalid process limits", 1)
	}
	env, err := ChildEnvironment(options.Env, options.ServerURL, options.HeaderNames)
	if err != nil {
		return nil, err
	}
	cwd, err := os.MkdirTemp("", "teamcity-axi-child-")
	if err != nil {
		return nil, NewError("INTERNAL_ERROR", "Cannot create neutral child directory", 1)
	}
	life, cancel := context.WithCancel(context.Background())
	return &ProcessTransport{options: options, cwd: cwd, env: envList(env), sem: make(chan struct{}, l.Concurrency), life: life, cancel: cancel}, nil
}
func (t *ProcessTransport) ChildProcesses() int { t.mu.Lock(); defer t.mu.Unlock(); return t.launches }
func limitError(code, message, limit string, ceiling, observed int) *DomainError {
	return &DomainError{Code: code, Message: message, ExitCode: 1, Details: Object{"limit": limit, "ceiling": ceiling, "observed": observed}}
}
func (t *ProcessTransport) Execute(ctx context.Context, op Operation, maxChildren int) (Captured, error) {
	zero := Captured{}
	if maxChildren < -1 || maxChildren > 256 {
		return zero, NewError("INTERNAL_ERROR", "Invalid reserved launch ceiling", 1)
	}
	args, err := OperationArgv(op)
	if err != nil {
		return zero, err
	}
	call, cancel := context.WithDeadline(ctx, time.UnixMilli(t.options.Limits.Deadline))
	defer cancel()
	select {
	case <-t.life.Done():
		return zero, NewError("INTERRUPTED", "Invocation interrupted", 1)
	case <-call.Done():
		if ctx.Err() != nil {
			return zero, NewError("INTERRUPTED", "Invocation interrupted", 1)
		}
		return zero, deadlineError(t.options.Limits.Deadline)
	case t.sem <- struct{}{}:
	}
	defer func() { <-t.sem }()
	t.mu.Lock()
	if t.disposed || ctx.Err() != nil {
		t.mu.Unlock()
		return zero, NewError("INTERRUPTED", "Invocation interrupted", 1)
	}
	if time.Now().UnixMilli() >= t.options.Limits.Deadline {
		t.mu.Unlock()
		return zero, deadlineError(t.options.Limits.Deadline)
	}
	if maxChildren >= 0 && t.launches >= maxChildren {
		observed := t.launches
		t.mu.Unlock()
		return zero, limitError("CALL_LIMIT_EXCEEDED", "Reserved child launch capacity cannot be consumed", "maxChildProcesses", maxChildren, observed)
	}
	if t.launches >= t.options.Limits.MaxChildren {
		observed := t.launches
		t.mu.Unlock()
		return zero, limitError("INPUT_LIMIT_EXCEEDED", "Child launch budget exhausted", "maxChildProcesses", t.options.Limits.MaxChildren, observed)
	}
	t.launches++
	t.wg.Add(1)
	t.mu.Unlock()
	defer t.wg.Done()
	child, stop := context.WithCancelCause(call)
	defer stop(nil)
	cmd := exec.Command(t.options.Binary, args...)
	cmd.Dir = t.cwd
	cmd.Env = t.env
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 400 * time.Millisecond
	out := &limitedBuffer{limit: t.options.Limits.StdoutBytes, exceeded: func(n int) {
		stop(limitError("INPUT_LIMIT_EXCEEDED", "Child stdout capture limit exceeded", "stdoutCaptureBytes", t.options.Limits.StdoutBytes, n))
	}}
	stderr := &limitedBuffer{limit: t.options.Limits.StderrBytes, exceeded: func(n int) {
		stop(limitError("INPUT_LIMIT_EXCEEDED", "Child stderr capture limit exceeded", "stderrCaptureBytes", t.options.Limits.StderrBytes, n))
	}}
	cmd.Stdout = out
	cmd.Stderr = stderr
	if cmd.Start() != nil {
		return zero, NewError("DEPENDENCY_MISSING", "Cannot start the selected native executable", 1)
	}
	pid := cmd.Process.Pid
	done := make(chan struct{})
	cleanup := make(chan struct{})
	go func() {
		defer close(cleanup)
		select {
		case <-done:
			return
		case <-child.Done():
		case <-t.life.Done():
			stop(NewError("INTERRUPTED", "Invocation interrupted", 1))
		}
		_ = syscall.Kill(-pid, syscall.SIGTERM)
		timer := time.NewTimer(200 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		case <-done:
		}
	}()
	waitErr := cmd.Wait()
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	close(done)
	<-cleanup
	if cause := context.Cause(child); cause != nil {
		if e, ok := cause.(*DomainError); ok {
			return zero, e
		}
		if errors.Is(cause, context.DeadlineExceeded) {
			return zero, deadlineError(t.options.Limits.Deadline)
		}
		return zero, NewError("INTERRUPTED", "Invocation interrupted", 1)
	}
	captured := Captured{Stdout: out.Bytes(), Stderr: stderr.Bytes(), ExitCode: cmd.ProcessState.ExitCode()}
	if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		captured.Signal = status.Signal().String()
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(waitErr, &exitErr) {
			return zero, NewError("UPSTREAM_FAILURE", "Native process output could not be captured", 1)
		}
	}
	return captured, nil
}
func (t *ProcessTransport) Close() error {
	t.mu.Lock()
	t.disposed = true
	t.cancel()
	t.mu.Unlock()
	t.wg.Wait()
	return os.RemoveAll(t.cwd)
}
func ReadProfile(command string) string {
	switch command {
	case "run.watch":
		return "watch"
	case "status":
		return "status"
	case "run.tree", "run.failure":
		return "graph"
	}
	return "simple"
}
func ReadLimits(ec ExecutionContext, profile string) ProcessLimits {
	concurrency, children := 3, 8
	switch profile {
	case "watch":
		concurrency, children = 1, 32
	case "status":
		concurrency, children = 2, 6
	case "graph":
		children = 24
	}
	if ec.Config != nil {
		if ec.Config.Limits.Concurrency > 0 {
			concurrency = min(concurrency, ec.Config.Limits.Concurrency)
		}
		if ec.Config.Limits.MaxChildProcesses > 0 {
			children = min(children, ec.Config.Limits.MaxChildProcesses)
		}
	}
	stdout := 2097152
	if profile == "watch" || profile == "status" {
		stdout = 1048576
	}
	return ProcessLimits{Deadline: ec.Deadline, Concurrency: concurrency, MaxChildren: children, StdoutBytes: stdout, StderrBytes: 65536}
}
func PublicLimits(l ProcessLimits, maxBytes int) Object {
	return Object{"maxBytes": maxBytes, "maxChildProcesses": l.MaxChildren, "concurrency": l.Concurrency, "deadline": l.Deadline, "stdoutCaptureBytes": l.StdoutBytes, "stderrCaptureBytes": l.StderrBytes}
}
