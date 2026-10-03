package axi

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type contextWorkspace struct {
	repo, dir, configHome string
	config                Object
	env                   map[string]string
}

func newContextWorkspace(t *testing.T) *contextWorkspace {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	configHome := filepath.Join(dir, "config")
	if e := os.MkdirAll(repo, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Join(configHome, "teamcity-axi"), 0700); e != nil {
		t.Fatal(e)
	}
	w := &contextWorkspace{repo: repo, dir: dir, configHome: configHome, config: Object{"schemaVersion": "1.0", "readOnly": true, "defaultServer": "work", "servers": Object{"work": Object{"url": "https://TEAMCITY.example.test:443/teamcity/"}, "other": Object{"url": "https://other.example.test"}}}, env: map[string]string{"HOME": dir, "PATH": os.Getenv("PATH"), "XDG_CONFIG_HOME": configHome}}
	w.save(t)
	w.git(t, repo, "init", "-b", "main")
	w.git(t, repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "Fixture")
	return w
}
func (w *contextWorkspace) save(t *testing.T) {
	t.Helper()
	b, e := json.Marshal(w.config)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(w.configHome, "teamcity-axi", "config.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
}
func (w *contextWorkspace) git(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	cmd.Env = envList(w.env)
	b, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("fixture git failed: %s %v", b, e)
	}
	return string(b)
}
func (w *contextWorkspace) resolve(args []string, cwd string, extra map[string]string) (ExecutionContext, error) {
	if cwd == "" {
		cwd = w.repo
	}
	p, e := Parse(append([]string{"context", "show", "--cwd", cwd}, args...))
	if e != nil {
		return ExecutionContext{}, e
	}
	env := map[string]string{}
	for k, v := range w.env {
		env[k] = v
	}
	for k, v := range extra {
		env[k] = v
	}
	return ResolveContext(context.Background(), p, env)
}
func TestCanonicalURLTrust(t *testing.T) {
	u, e := CanonicalURL("https://EXAMPLE.test:443/teamcity/", false)
	if e != nil || u != "https://example.test/teamcity" {
		t.Fatal(u, e)
	}
	for _, v := range []string{"https://host/teamcity/%2e%2e/other", "https://host/teamcity/../other", "https://user:secret@host", "https://host/?a=1", "https://host/#a", "http://host", "http://127.0.0.1", "https://host/teamcity\\other"} {
		if _, e := CanonicalURL(v, false); e == nil {
			t.Fatalf("accepted unsafe URL %s", v)
		}
	}
	for _, v := range []string{"http://127.0.0.1:8111/teamcity", "http://[::1]:8111/teamcity"} {
		u, e := CanonicalURL(v, true)
		if e != nil || u != v {
			t.Fatal(u, e)
		}
	}
}
func TestNativeBindingDeepestScopeAndFlags(t *testing.T) {
	w := newContextWorkspace(t)
	cwd := filepath.Join(w.repo, "packages", "payments", "src")
	os.MkdirAll(cwd, 0700)
	os.WriteFile(filepath.Join(w.repo, "teamcity.toml"), []byte("[[server]]\nurl=\"https://teamcity.example.test/teamcity\"\nproject=\"Base\"\njob=\"Base_Build\"\njobs=[\"Base_Build\",\"Deploy\"]\n[server.paths.\"packages\"]\nproject=\"Packages\"\n[server.paths.\"packages/payments\"]\njob=\"Payments_Build\"\n"), 0600)
	ec, e := w.resolve(nil, cwd, nil)
	if e != nil {
		t.Fatal(e)
	}
	if ec.Server != "work" || ec.Project != "Base" || ec.Job != "Payments_Build" || ec.Branch != "main" || Str(ec.Sources, "server") != "repository" || !reflect.DeepEqual(ec.Jobs, []string{"Base_Build", "Deploy"}) {
		t.Fatalf("scope lost: %+v", ec)
	}
	ec, e = w.resolve([]string{"--job", "Explicit"}, cwd, nil)
	if e != nil || ec.Job != "Explicit" || !reflect.DeepEqual(ec.Jobs, []string{"Explicit"}) {
		t.Fatal(ec, e)
	}
}
func TestWorktreeIsolation(t *testing.T) {
	w := newContextWorkspace(t)
	second := filepath.Join(w.dir, "second")
	w.git(t, w.repo, "worktree", "add", "-b", "feature/refund", second)
	var a, b ExecutionContext
	var ea, eb error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a, ea = w.resolve([]string{"--server", "work"}, w.repo, nil) }()
	go func() { defer wg.Done(); b, eb = w.resolve([]string{"--server", "other"}, second, nil) }()
	wg.Wait()
	if ea != nil || eb != nil || a.Branch != "main" || b.Branch != "feature/refund" || a.Server != "work" || b.Server != "other" || b.RepositoryRoot != second {
		t.Fatal(a, b, ea, eb)
	}
}
func TestRepositoryCannotRedirectCredentials(t *testing.T) {
	w := newContextWorkspace(t)
	os.WriteFile(filepath.Join(w.repo, "teamcity.toml"), []byte("[[server]]\nurl=\"https://attacker.invalid\"\njob=\"Build\"\n"), 0600)
	_, e := w.resolve(nil, "", nil)
	requireCode(t, e, "UNTRUSTED_SERVER")
	_, e = w.resolve([]string{"--server", "other"}, "", map[string]string{"TEAMCITY_URL": "https://teamcity.example.test/teamcity", "TEAMCITY_TOKEN": "context-canary"})
	requireCode(t, e, "AUTH_CONTEXT_MISMATCH")
	_, e = w.resolve([]string{"--server", "work"}, "", map[string]string{"TEAMCITY_TOKEN": "context-canary"})
	requireCode(t, e, "AUTH_CONTEXT_MISMATCH")
	os.Remove(filepath.Join(w.repo, "teamcity.toml"))
	overlay := filepath.Join(w.repo, ".teamcity-axi.json")
	os.WriteFile(overlay, []byte(`{"schemaVersion":"1.0","binaryPath":"/tmp/evil"}`), 0600)
	_, e = w.resolve(nil, "", nil)
	requireCode(t, e, "USAGE_ERROR")
	os.Remove(overlay)
	outside := filepath.Join(w.dir, "outside.json")
	os.WriteFile(outside, []byte(`{"schemaVersion":"1.0"}`), 0600)
	os.Symlink(outside, overlay)
	_, e = w.resolve(nil, "", nil)
	requireCode(t, e, "POLICY_DENIED")
}
func TestDetachedDirtyLiteralAndVCSMapping(t *testing.T) {
	w := newContextWorkspace(t)
	w.git(t, w.repo, "remote", "add", "origin", "https://git.example.test/payments.git")
	os.WriteFile(filepath.Join(w.repo, ".teamcity-axi.json"), []byte(`{"schemaVersion":"1.0","vcsRoots":[{"server":"work","remote":"origin","rootId":"Payments_Git"}]}`), 0600)
	ec, e := w.resolve(nil, "", nil)
	if e != nil || ec.VCSRootID != "Payments_Git" || ec.Dirty == nil || !*ec.Dirty {
		t.Fatal(ec, e)
	}
	ec, e = w.resolve([]string{"--literal-branch", "@this"}, "", nil)
	if e != nil || ec.Branch != "@this" {
		t.Fatal(ec, e)
	}
	w.git(t, w.repo, "checkout", "--detach")
	ec, e = w.resolve(nil, "", nil)
	if e != nil || ec.BranchDefined || ec.Head == "" {
		t.Fatal(ec, e)
	}
	_, e = w.resolve([]string{"--branch", "@this"}, "", nil)
	requireCode(t, e, "CONTEXT_REQUIRED")
	_, e = w.resolve([]string{"--vcs-root", strings.Repeat("x", 257)}, "", nil)
	requireCode(t, e, "USAGE_ERROR")
	head := ec.Head
	ec, e = w.resolve([]string{"--revision", "@head"}, "", nil)
	if e != nil || ec.Revision != head {
		t.Fatal(ec, e)
	}
}
func TestConfigurationBoundsAndAmbiguity(t *testing.T) {
	w := newContextWorkspace(t)
	native := filepath.Join(w.repo, "teamcity.toml")
	if e := syscall.Mkfifo(native, 0600); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	_, e := w.resolve([]string{"--timeout", "500ms"}, "", nil)
	requireCode(t, e, "USAGE_ERROR")
	if time.Since(start) >= 500*time.Millisecond {
		t.Fatal("FIFO blocked")
	}
	os.Remove(native)
	for _, b := range [][]byte{make([]byte, 65537), {255, 254}} {
		os.WriteFile(native, b, 0600)
		_, e = w.resolve(nil, "", nil)
		requireCode(t, e, "USAGE_ERROR")
	}
	os.Remove(native)
	Obj(w.config["servers"])["other"] = Object{"url": "https://teamcity.example.test/teamcity"}
	w.save(t)
	_, e = w.resolve(nil, "", nil)
	requireCode(t, e, "AMBIGUOUS_CONTEXT")
}
func TestTrustedConfigurationOwnershipAndUnknownKeys(t *testing.T) {
	w := newContextWorkspace(t)
	path := filepath.Join(w.configHome, "teamcity-axi", "config.json")
	os.Chmod(path, 0666)
	_, e := w.resolve(nil, "", nil)
	requireCode(t, e, "POLICY_DENIED")
	os.Chmod(path, 0600)
	w.config["unexpected"] = true
	w.save(t)
	_, e = w.resolve(nil, "", nil)
	requireCode(t, e, "USAGE_ERROR")
}
