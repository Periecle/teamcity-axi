//go:build live

package liveserver

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Periecle/teamcity-axi/internal/axi"
)

func TestLiveStatusExactCleanCheckoutDirtyStaleMissingAndUnmatchedRoot(t *testing.T) {
	source := os.Getenv("TEAMCITY_AXI_LIVE_CHECKOUT")
	if !filepath.IsAbs(source) {
		t.Fatal("Live exact-checkout tests require an absolute TEAMCITY_AXI_LIVE_CHECKOUT fixture path; no skips")
	}
	f := fixture(t)
	checkout := filepath.Join(f.Dir, "checkout")
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + f.Dir, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
	clone := exec.Command("git", "-c", "core.hooksPath=/dev/null", "clone", "--no-hardlinks", "--no-local", "--", source, checkout)
	clone.Env = env
	if result, e := clone.CombinedOutput(); e != nil {
		t.Fatalf("clone owned checkout: %v: %s", e, result)
	}
	git := exec.Command("git", "rev-parse", "HEAD")
	git.Dir = checkout
	git.Env = env
	raw, e := git.Output()
	if e != nil {
		t.Fatal(e)
	}
	head := strings.TrimSpace(string(raw))
	p := f.Contract.Fixture
	args := []string{"status", "--job", axi.Str(p, "vcsJobId"), "--vcs-root", axi.Str(p, "vcsRootId"), "--all-branches", "--cwd", checkout, "--check"}
	for _, format := range []string{"json", "toon"} {
		result := wire(t, f, append(append([]string{}, args...), "--format", format), 0)
		value := decode(t, result, format)
		equal(t, at(value.Data, "check", "passed"), true)
		equal(t, at(value.Data, "checkout", "head"), head)
		run := axi.Obj(objects(value.Data["jobs"])[0]["run"])
		equal(t, run["id"], p["vcsRunId"])
		equal(t, objects(run["revisions"])[0]["revision"], head)
		equal(t, at(value.Meta, "counts", "childProcesses"), 2)
		require(t, len(result.Stdout) <= 6144, "status byte budget exceeded")
	}
	path := filepath.Join(checkout, "owned-dirty-test.txt")
	if e = os.WriteFile(path, []byte("Owned dirty checkout fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	dirty := jsonCall(t, f, args, 1)
	equal(t, at(dirty.Data, "checkout", "dirty"), true)
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	for _, extra := range [][]string{{"--revision", strings.Repeat("0", 40)}, {"--vcs-root", "Unmatched_Git"}} {
		filtered := append([]string{}, args...)
		if extra[0] == "--vcs-root" {
			filtered = append(append([]string{}, args[:3]...), args[5:]...)
		}
		value := jsonCall(t, f, append(filtered, extra...), 1)
		equal(t, at(value.Data, "check", "passed"), false)
		require(t, value.Data["assessment"] != "passed", "stale/unmatched root certified")
	}
	for _, name := range []string{"status-one", "status-five", "status-branch", "status-missing-job", "status-denied"} {
		body := api(t, native(t, f, name))
		if name == "status-missing-job" || name == "status-denied" {
			equal(t, body["count"], 0)
		}
		if name == "status-five" {
			equal(t, body["count"], 5)
		}
	}
	missing := jsonCall(t, f, []string{"status", "--job", "AxiContract_Missing", "--all-branches", "--cwd", checkout, "--check"}, 1)
	equal(t, objects(missing.Data["jobs"])[0]["availability"], "unavailable")
}
