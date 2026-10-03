//go:build realcli

package axi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Periecle/teamcity-axi/internal/testfixture"
)

type nativeStatusFixture struct {
	*nativeFixture
	repo, head string
	jobs       []string
}

func (f *nativeStatusFixture) git(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.repo
	for k, v := range f.env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
func (f *nativeStatusFixture) commit(t *testing.T, message string) {
	t.Helper()
	f.git(t, "add", ".")
	f.git(t, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-m", message)
}
func newNativeStatus(t *testing.T, jobCount int) *nativeStatusFixture {
	t.Helper()
	f := &nativeStatusFixture{nativeFixture: newNativeFixture(t, false)}
	f.repo = filepath.Join(f.dir, "repo")
	os.MkdirAll(f.repo, 0700)
	f.env["GIT_CONFIG_GLOBAL"] = "/dev/null"
	f.env["GIT_CONFIG_NOSYSTEM"] = "1"
	f.git(t, "init", "-b", "feature/refund")
	f.git(t, "remote", "add", "origin", "https://example.invalid/repository.git")
	for i := 0; i < jobCount; i++ {
		id := "Payments_Build"
		if i > 0 {
			id = fmt.Sprintf("Payments_Job%d", i)
		}
		f.jobs = append(f.jobs, id)
	}
	data, _ := json.Marshal(f.jobs)
	if err := os.WriteFile(filepath.Join(f.repo, "teamcity.toml"), []byte(fmt.Sprintf("[[server]]\nurl = %q\nproject = \"Payments\"\njobs = %s\n", f.mock.BaseURL, data)), 0600); err != nil {
		t.Fatal(err)
	}
	mapping := Object{"schemaVersion": "1.0", "vcsRoots": []Object{{"server": "work", "remote": "origin", "rootId": "Payments_Git"}}}
	data, _ = json.Marshal(mapping)
	if err := os.WriteFile(filepath.Join(f.repo, ".teamcity-axi.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	f.commit(t, "Owned status test fixture")
	f.head = f.git(t, "rev-parse", "HEAD")
	f.mock.SetStatusRevision(f.head)
	return f
}

func (f *nativeStatusFixture) callStatus(t *testing.T, args ...string) nativeCall {
	t.Helper()
	args = append(args, "--cwd", f.repo)
	return f.call(t, args...)
}

// Original: status.test.mjs — exact clean status checks five required jobs within home budgets and both serializers.
func TestRealCLIStatusFiveJobs(t *testing.T) {
	f := newNativeStatus(t, 5)
	var previous Object
	for _, flags := range nativeFormats() {
		r := f.callStatus(t, append([]string{"status", "--check"}, flags...)...)
		requireNativeCode(t, r, 0)
		requireNativeValue(t, r.value["status"], "ok")
		data := nativeData(r)
		if Str(data, "assessment") != "passed" || !Bool(Obj(data["check"]), "passed") || len(Objects(data["jobs"])) != 5 || Str(Obj(data["checkout"]), "head") != f.head || Int(Obj(nativeMeta(r)["counts"]), "childProcesses") != 2 || Int(Obj(nativeMeta(r)["limits"]), "maxChildProcesses") != 6 || Int(Obj(nativeMeta(r)["limits"]), "concurrency") != 2 || len(r.stdout) > 6144 {
			t.Fatalf("five job status: %s", r.stdout)
		}
		if previous != nil && !reflect.DeepEqual(previous, data) {
			t.Fatal("status formats disagree")
		}
		previous = data
	}
	if len(f.mock.Requests()) != 2 {
		t.Fatal("status used per-job network expansion")
	}
	for _, q := range f.mock.Requests() {
		if q.Method != "GET" || q.Path != "/teamcity/app/rest/buildTypes" || strings.Count(q.Query.Get("locator"), "item:") != 5 || !strings.Contains(q.Query.Get("fields"), "state:any") || !strings.Contains(q.Query.Get("fields"), "count:20,start:0,lookupLimit:5000") || strings.Contains(q.Query.Get("fields"), "parameters") {
			t.Fatal("status snapshot unbounded")
		}
	}
}

// Original: status.test.mjs — red is exit zero for observation and one for assertion; active exact executions never fall back to green.
func TestRealCLIStatusRedAndActivity(t *testing.T) {
	f := newNativeStatus(t, 1)
	for _, mode := range []string{"status-red", "status-running", "status-queued"} {
		f.mock.SetMode(mode)
		r := f.callStatus(t, "status", "--json")
		requireNativeCode(t, r, 0)
		r = f.callStatus(t, "status", "--check", "--json")
		requireNativeCode(t, r, 1)
		requireNativeValue(t, Obj(nativeData(r)["check"])["passed"], false)
		job := Objects(nativeData(r)["jobs"])[0]
		want := "in_progress"
		if mode == "status-red" {
			want = "failed"
		}
		if Str(job, "match") != "exact" || Str(job, "assessment") != want || Bool(Obj(nativeData(r)["check"]), "passed") {
			t.Fatal("active exact execution fell back to green")
		}
		if mode == "status-red" && !reflect.DeepEqual(Strings(nativeData(r)["failedRunIds"]), []string{"482193"}) {
			t.Fatal("failed identity omitted")
		}
	}
}

// Original: status.test.mjs — stale, missing, newer unknown, personal and multi-root checkouts never assert green.
func TestRealCLIStatusUnverifiedCandidates(t *testing.T) {
	f := newNativeStatus(t, 1)
	for _, mode := range []string{"status-stale", "status-unknown", "status-newer-unknown", "status-personal", "status-multi-root", "status-missing-job", "status-unsafe-continuation"} {
		f.mock.SetMode(mode)
		r := f.callStatus(t, "status", "--check", "--json")
		requireNativeCode(t, r, 1)
		requireNativeValue(t, Obj(nativeData(r)["check"])["passed"], false)
		if Str(r.value, "status") != "partial" || Str(nativeData(r), "assessment") != "unverified" || Bool(Obj(nativeData(r)["check"]), "passed") {
			t.Fatalf("%s falsely passed: %s", mode, r.stdout)
		}
		if mode == "status-multi-root" && len(Objects(Obj(Objects(nativeData(r)["jobs"])[0]["run"])["revisions"])) != 2 {
			t.Fatal("other VCS root omitted")
		}
		if mode == "status-missing-job" && Str(Objects(nativeData(r)["jobs"])[0], "availability") != "unavailable" {
			t.Fatal("missing required job assumed absent")
		}
	}
}

// Original: status.test.mjs — dirty, detached, explicit foreign revision and omitted sixth required job have distinct checks.
func TestRealCLIStatusCheckoutBoundaries(t *testing.T) {
	f := newNativeStatus(t, 6)
	r := f.callStatus(t, "status", "--check", "--json")
	requireNativeCode(t, r, 1)
	coverage := Obj(nativeData(r)["coverage"])
	if Int(coverage, "requiredJobs") != 6 || Int(coverage, "displayedJobs") != 5 || len(Strings(Obj(r.value["context"])["jobs"])) != 5 || !nativeNotes(r, "TRACKED_JOBS_TRUNCATED") {
		t.Fatal("sixth required job hidden")
	}
	args := []string{"status", "--job", "Payments_Build", "--check", "--json"}
	requireNativeCode(t, f.callStatus(t, args...), 0)
	path := filepath.Join(f.repo, "uncommitted.txt")
	os.WriteFile(path, []byte("Owned dirty worktree fixture"), 0600)
	r = f.callStatus(t, args...)
	requireNativeCode(t, r, 1)
	if Obj(nativeData(r)["checkout"])["dirty"] != true {
		t.Fatal("dirty checkout asserted")
	}
	os.Remove(path)
	r = f.callStatus(t, append(args, "--revision", strings.Repeat("b", 40))...)
	requireNativeCode(t, r, 1)
	if !nativeNotes(r, "CHECKOUT_IDENTITY_UNVERIFIED") {
		t.Fatal("foreign revision asserted")
	}
	f.git(t, "checkout", "--detach", f.head)
	r = f.callStatus(t, args...)
	requireNativeCode(t, r, 0)
	branch, present := Obj(nativeData(r)["checkout"])["branch"]
	if !present || branch != nil {
		t.Fatal("detached branch fabricated")
	}
}

// Original: status.test.mjs — status preserves local unconfigured home, scoped errors, permission denial and output limits.
func TestRealCLIStatusErrorsAndUnconfiguredHome(t *testing.T) {
	f := newNativeStatus(t, 1)
	r := f.call(t, "--json")
	requireNativeCode(t, r, 0)
	if Str(nativeData(r), "mode") != "unconfigured" || len(f.mock.Requests()) != 0 {
		t.Fatal("unconfigured home made network request")
	}
	r = f.call(t, "status", "--json")
	requireNativeCode(t, r, 2)
	requireNativeValue(t, Obj(r.value["error"])["code"], "CONTEXT_REQUIRED")
	if len(f.mock.Requests()) != 0 {
		t.Fatal("unconfigured status made network request")
	}
	for _, mode := range []string{"denied", "malformed", "expired", "status-foreign", "status-wrong-run"} {
		f.mock.SetMode(mode)
		r = f.callStatus(t, "status", "--check", "--json")
		requireNativeCode(t, r, 1)
		_, dataPresent := r.value["data"]
		if Str(r.value, "status") != "error" || dataPresent {
			t.Fatalf("%s error became status: %s", mode, r.stdout)
		}
	}
	f.mock.SetMode("status-huge")
	r = f.callStatus(t, "status", "--json")
	requireNativeError(t, r, "INPUT_LIMIT_EXCEEDED")
	if len(r.stdout) > 6144 {
		t.Fatal("status output ceiling exceeded")
	}
	f.config.Limits.MaxChildProcesses = 1
	f.writeConfig(t)
	f.mock.ClearRequests()
	r = f.callStatus(t, "status", "--check", "--json")
	requireNativeError(t, r, "INPUT_LIMIT_EXCEEDED")
	if len(f.mock.Requests()) != 0 {
		t.Fatal("process ceiling made network request")
	}
}

// Original: status.test.mjs — strict status and dash-leading identity hints preserve public flag contracts.
func TestRealCLIStatusStrictAndDashHints(t *testing.T) {
	f := newNativeStatus(t, 1)
	r := f.callStatus(t, "status", "--job=-leading-job", "--check", "--json")
	requireNativeCode(t, r, 0)
	for _, hint := range Objects(r.value["next"]) {
		if !containsString(Strings(hint["argv"]), "--job=-leading-job") || !containsString(Strings(hint["argv"]), "--project=Payments") {
			t.Fatal("dash identity hint lost")
		}
	}
	f.mock.SetMode("status-unknown")
	r = f.callStatus(t, "status", "--require-complete", "--json")
	requireNativeCode(t, r, 1)
	requireNativeValue(t, r.value["status"], "partial")
	r = f.call(t, "--check", "--json")
	requireNativeCode(t, r, 1)
	if Str(nativeData(r), "mode") != "unconfigured" {
		t.Fatal("unconfigured assertion became remote error")
	}
}

// Original: status.test.mjs — concurrent separate checkouts preserve their own revisions with invocation-local status state.
func TestRealCLIStatusConcurrentCheckouts(t *testing.T) {
	a, b := newNativeStatus(t, 1), newNativeStatus(t, 1)
	os.WriteFile(filepath.Join(b.repo, "another.txt"), []byte("Different committed worktree"), 0600)
	b.commit(t, "Different checkout")
	head := b.git(t, "rev-parse", "HEAD")
	b.mock.SetStatusRevision(head)
	results := make([]nativeCall, 2)
	var wg sync.WaitGroup
	for i, f := range []*nativeStatusFixture{a, b} {
		wg.Add(1)
		go func(i int, f *nativeStatusFixture) {
			defer wg.Done()
			results[i] = f.callStatus(t, "status", "--check", "--json")
		}(i, f)
	}
	wg.Wait()
	for i, r := range results {
		requireNativeCode(t, r, 0)
		expected := a.head
		if i == 1 {
			expected = head
		}
		if Str(Obj(nativeData(r)["checkout"]), "head") != expected || Str(Objects(Obj(Objects(nativeData(r)["jobs"])[0]["run"])["revisions"])[0], "revision") != expected {
			t.Fatal("concurrent checkout identity crossed")
		}
	}
	if head == a.head {
		t.Fatal("fixture checkouts not distinct")
	}
}

// Original: status.test.mjs — linked worktrees use independent servers and origin-bound tokens concurrently without config writes.
func TestRealCLIStatusLinkedWorktreeOrigins(t *testing.T) {
	a := newNativeStatus(t, 1)
	token := "second-server-fixture-token"
	server := testfixture.NewServer(testfixture.Options{Token: token, ProjectID: "Accounts", VCSRootID: "Accounts_Git"})
	t.Cleanup(server.Close)
	second := filepath.Join(a.dir, "second-worktree")
	a.git(t, "worktree", "add", "-b", "feature/other", second)
	b := &nativeStatusFixture{nativeFixture: &nativeFixture{mock: server, dir: a.dir, wrapper: a.wrapper, native: a.native, env: map[string]string{}}, repo: second}
	for k, v := range a.env {
		b.env[k] = v
	}
	b.env["TEAMCITY_URL"] = server.BaseURL
	b.env["TEAMCITY_TOKEN"] = token
	os.WriteFile(filepath.Join(second, "teamcity.toml"), []byte(fmt.Sprintf("[[server]]\nurl = %q\nproject = \"Accounts\"\njob = \"Accounts_Build\"\n", server.BaseURL)), 0600)
	mapping, _ := json.Marshal(Object{"schemaVersion": "1.0", "vcsRoots": []Object{{"server": "other", "remote": "origin", "rootId": "Accounts_Git"}}})
	os.WriteFile(filepath.Join(second, ".teamcity-axi.json"), mapping, 0600)
	b.commit(t, "Second linked worktree binding")
	head := b.git(t, "rev-parse", "HEAD")
	if head == a.head {
		t.Fatal("linked worktree checkouts not distinct")
	}
	server.SetStatusRevision(head)
	server.SetStatusBranch("feature/other")
	server.SetMode("status-red")
	a.config.Servers["other"] = ServerConfig{URL: server.BaseURL, AllowHTTPLoopback: true, AllowedProjects: []string{"Accounts"}}
	a.writeConfig(t)
	configPath := filepath.Join(a.dir, "teamcity-axi", "config.json")
	before, _ := os.ReadFile(configPath)
	gitfile, _ := os.ReadFile(filepath.Join(second, ".git"))
	if !bytes.HasPrefix(gitfile, []byte("gitdir:")) {
		t.Fatal("fixture is not linked worktree")
	}
	for _, flags := range nativeFormats() {
		results := make([]nativeCall, 2)
		var wg sync.WaitGroup
		for i, f := range []*nativeStatusFixture{a, b} {
			wg.Add(1)
			go func(i int, f *nativeStatusFixture) {
				defer wg.Done()
				results[i] = f.callStatus(t, append([]string{"status", "--check"}, flags...)...)
			}(i, f)
		}
		wg.Wait()
		requireNativeCode(t, results[0], 0)
		requireNativeCode(t, results[1], 1)
		for i, r := range results {
			alias, project, job, root, branch, revision, assessment := "work", "Payments", "Payments_Build", "Payments_Git", "feature/refund", a.head, "passed"
			if i == 1 {
				alias, project, job, root, branch, revision, assessment = "other", "Accounts", "Accounts_Build", "Accounts_Git", "feature/other", head, "failed"
			}
			context := Obj(r.value["context"])
			checkout := Obj(nativeData(r)["checkout"])
			record := Objects(nativeData(r)["jobs"])[0]
			runRevision := Objects(Obj(record["run"])["revisions"])[0]
			if Str(context, "server") != alias || Str(context, "project") != project || Str(record, "jobId") != job || Str(checkout, "vcsRootId") != root || Str(runRevision, "vcsRootId") != root || Str(checkout, "branch") != branch || Str(checkout, "head") != revision || Str(runRevision, "revision") != revision || Str(nativeData(r), "assessment") != assessment || Int(Obj(nativeMeta(r)["counts"]), "childProcesses") != 2 || Int(Obj(nativeMeta(r)["limits"]), "concurrency") != 2 || strings.Contains(r.stdout, token) {
				t.Fatalf("linked worktree binding crossed: %s", r.stdout)
			}
		}
	}
	after, _ := os.ReadFile(configPath)
	if !bytes.Equal(before, after) || a.git(t, "status", "--porcelain") != "" || b.git(t, "status", "--porcelain") != "" {
		t.Fatal("read status changed configuration/checkout")
	}
	for _, scopedServer := range []*nativeMock{a.mock, server} {
		if len(scopedServer.Requests()) != 2 {
			t.Fatal("linked worktree status lost exact bounded request count")
		}
		requireNativeReadOnly(t, scopedServer)
	}
}

// Original: status.test.mjs — checkout checks distinguish exceptional and composite outcomes without false green.
func TestRealCLIStatusExceptionalOutcomes(t *testing.T) {
	f := newNativeStatus(t, 1)
	for _, c := range []struct {
		mode, result, assessment string
		code                     int
	}{{"status-canceled", "canceled", "failed", 1}, {"status-failed-to-start", "failed_to_start", "failed", 1}, {"status-composite", "success", "passed", 0}, {"status-missing-outcome", "unknown", "unverified", 1}} {
		f.mock.SetMode(c.mode)
		for _, flags := range nativeFormats() {
			r := f.callStatus(t, append([]string{"status", "--check"}, flags...)...)
			requireNativeCode(t, r, c.code)
			requireNativeValue(t, Obj(nativeData(r)["check"])["passed"], c.code == 0)
			job := Objects(nativeData(r)["jobs"])[0]
			if Str(Obj(job["run"]), "result") != c.result || Str(job, "assessment") != c.assessment {
				t.Fatal("checkout exceptional outcome hidden")
			}
		}
	}
	requireNativeReadOnly(t, f.mock)
}
