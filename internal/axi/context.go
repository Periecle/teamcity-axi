package axi

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

var unsafeURLEncoding = regexp.MustCompile(`(?i)%2f|%5c|%2e`)

func CanonicalURL(input string, allowLoopback bool) (string, error) {
	u, err := url.Parse(input)
	if err != nil || u.Host == "" || u.Opaque != "" {
		return "", Usage("Invalid server URL")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(input, "\r\n\x00\\") || unsafeURLEncoding.MatchString(input) {
		return "", Usage("Unsafe server URL")
	}
	for _, p := range strings.FieldsFunc(input, func(r rune) bool { return r == '/' || r == '?' || r == '#' }) {
		if p == "." || p == ".." {
			return "", Usage("Unsafe server URL")
		}
	}
	host := strings.ToLower(u.Hostname())
	loop := contains([]string{"localhost", "127.0.0.1", "::1"}, host)
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "https" && !(allowLoopback && loop && u.Scheme == "http") {
		return "", NewError("UNTRUSTED_SERVER", "HTTPS is required except explicitly trusted loopback development servers", 1)
	}
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	u.Host = host
	if port != "" {
		u.Host += ":" + port
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}
func validIdentity(v string) bool {
	return len([]rune(v)) > 0 && len([]rune(v)) <= 256 && !identifierControls.MatchString(v)
}
func Environment() map[string]string {
	r := map[string]string{}
	for _, s := range os.Environ() {
		k, v, ok := strings.Cut(s, "=")
		if ok {
			r[k] = v
		}
	}
	return r
}
func envList(env map[string]string) []string {
	r := make([]string, 0, len(env))
	for k, v := range env {
		r = append(r, k+"="+v)
	}
	return r
}
func boundedRead(path string, trusted bool) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, Usage("Cannot read configuration")
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil, Usage("Configuration must be a regular file")
	}
	if trusted {
		sys, ok := st.Sys().(*syscall.Stat_t)
		if st.Mode().Perm()&0022 != 0 || (ok && int(sys.Uid) != os.Getuid()) {
			return nil, NewError("POLICY_DENIED", "Trusted configuration must be owned by the current user and not writable by other users", 1)
		}
	}
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return nil, Usage("Cannot read configuration")
	}
	if len(b) > 65536 {
		return nil, Usage("Configuration exceeds the 64 KiB input limit")
	}
	if !utf8.Valid(b) {
		return nil, Usage("Configuration must be UTF-8")
	}
	return b, nil
}
func deadlineError(deadline int64) *DomainError {
	return &DomainError{Code: "DEADLINE_EXCEEDED", Message: "Overall deadline exceeded", ExitCode: 1, Retryable: true, Details: Object{"limit": "deadline", "ceiling": deadline, "observed": time.Now().UnixMilli()}}
}
func gitRead(ctx context.Context, cwd string, args []string, env map[string]string, deadline int64) (string, bool, error) {
	remaining := time.Until(time.UnixMilli(deadline))
	if remaining <= 0 {
		return "", false, deadlineError(deadline)
	}
	child, cancel := context.WithTimeout(ctx, min(remaining, time.Second))
	defer cancel()
	path, err := exec.LookPath("git")
	if err != nil {
		return "", false, nil
	}
	if env["PATH"] != "" {
		path = ""
		for _, dir := range filepath.SplitList(env["PATH"]) {
			candidate := filepath.Join(dir, "git")
			if st, e := os.Stat(candidate); e == nil && st.Mode().IsRegular() && st.Mode().Perm()&0111 != 0 {
				path = candidate
				break
			}
		}
		if path == "" {
			return "", false, nil
		}
	}
	cmd := exec.CommandContext(child, path, append([]string{"-c", "core.fsmonitor=false"}, args...)...)
	cmd.Dir = cwd
	cmd.Env = envList(map[string]string{"PATH": env["PATH"], "HOME": env["HOME"], "LANG": "C", "GIT_TERMINAL_PROMPT": "0", "GIT_CONFIG_NOSYSTEM": "1"})
	out := &limitedBuffer{limit: 131072}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	err = cmd.Run()
	if time.Now().UnixMilli() >= deadline {
		return "", false, deadlineError(deadline)
	}
	if err != nil {
		return "", false, nil
	}
	return strings.TrimSpace(out.String()), true, nil
}

type bindingScope struct {
	Project string   `toml:"project"`
	Job     string   `toml:"job"`
	Jobs    []string `toml:"jobs"`
}
type nativeBinding struct {
	URL     string                  `toml:"url"`
	Project string                  `toml:"project"`
	Job     string                  `toml:"job"`
	Jobs    []string                `toml:"jobs"`
	Paths   map[string]bindingScope `toml:"paths"`
}

func nativeBindings(b []byte) ([]nativeBinding, error) {
	var n struct {
		Server []nativeBinding `toml:"server"`
	}
	md, err := toml.Decode(string(b), &n)
	if err != nil {
		return nil, Usage("Malformed teamcity.toml")
	}
	if len(md.Undecoded()) > 0 || len(n.Server) > 100 {
		return nil, Usage("Invalid native repository binding")
	}
	// Typed TOML decoding loses the distinction between absent and empty selectors.
	// Validate field presence before an empty value could erase a bound identity.
	var raw map[string]any
	if _, err := toml.Decode(string(b), &raw); err != nil {
		return nil, Usage("Malformed teamcity.toml")
	}
	for _, binding := range Objects(raw["server"]) {
		scopes := []Object{binding}
		for _, scope := range Obj(binding["paths"]) {
			scopes = append(scopes, Obj(scope))
		}
		for _, scope := range scopes {
			for _, key := range []string{"project", "job"} {
				if value, present := scope[key]; present {
					identity, ok := value.(string)
					if !ok || !validIdentity(identity) {
						return nil, Usage("Invalid binding scope identifiers")
					}
				}
			}
		}
	}
	for _, v := range n.Server {
		if len(v.URL) == 0 || len(v.URL) > 2048 || len(v.Paths) > 100 {
			return nil, Usage("Invalid native server binding")
		}
		scopes := []bindingScope{{v.Project, v.Job, v.Jobs}}
		for p, s := range v.Paths {
			if filepath.IsAbs(p) || strings.ContainsAny(p, "\\\x00\r\n") || identifierControls.MatchString(p) {
				return nil, Usage("Invalid native binding path")
			}
			for _, x := range strings.Split(p, "/") {
				if x == "." || x == ".." {
					return nil, Usage("Invalid native binding path")
				}
			}
			scopes = append(scopes, s)
		}
		for _, s := range scopes {
			if (s.Project != "" && !validIdentity(s.Project)) || (s.Job != "" && !validIdentity(s.Job)) || len(s.Jobs) > 100 {
				return nil, Usage("Invalid binding scope identifiers")
			}
			for _, j := range s.Jobs {
				if !validIdentity(j) {
					return nil, Usage("Invalid binding scope identifiers")
				}
			}
		}
	}
	return n.Server, nil
}
func inside(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || (!filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}
func repositoryFile(path, root string, symlinks bool) ([]byte, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, Usage("Cannot resolve repository configuration")
	}
	if !symlinks && !inside(resolved, root) {
		return nil, NewError("POLICY_DENIED", "Repository configuration escapes the worktree", 1)
	}
	return boundedRead(resolved, false)
}
func defaultTimeout(name string) int {
	switch name {
	case "status":
		return 5000
	case "run.watch":
		return 120000
	case "run.tree", "run.failure":
		return 20000
	}
	return 10000
}
func ResolveContext(ctx context.Context, p Parsed, env map[string]string) (ExecutionContext, error) {
	ec := ExecutionContext{Deadline: time.Now().UnixMilli() + int64(p.Int("timeout", defaultTimeout(p.Descriptor.Name))), Jobs: []string{}, Sources: Object{}}
	cwd := p.String("cwd")
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	abs, err := filepath.Abs(cwd)
	if err == nil {
		ec.Cwd, err = filepath.EvalSymlinks(abs)
	}
	if err != nil {
		return ec, Usage("Cannot resolve --cwd")
	}
	st, err := os.Stat(ec.Cwd)
	if err != nil || !st.IsDir() {
		return ec, Usage("--cwd is not a directory")
	}
	configHome := env["XDG_CONFIG_HOME"]
	if configHome == "" {
		home := env["HOME"]
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		configHome = filepath.Join(home, ".config")
	}
	raw, err := boundedRead(filepath.Join(configHome, "teamcity-axi", "config.json"), true)
	if err != nil {
		return ec, err
	}
	if raw != nil {
		var value any
		if json.Unmarshal(raw, &value) != nil {
			return ec, Usage("Invalid trusted configuration JSON")
		}
		if err := ValidateConfig("user-config", value); err != nil {
			return ec, err
		}
		ec.Config = &UserConfig{}
		if json.Unmarshal(raw, ec.Config) != nil {
			return ec, Usage("Invalid trusted configuration JSON")
		}
		if _, err := SecretMatchers(ec.Config.SecretNamePatterns); err != nil {
			return ec, Usage("Unsafe trusted secret-name pattern")
		}
	}
	git := func(a ...string) (string, bool, error) { return gitRead(ctx, ec.Cwd, a, env, ec.Deadline) }
	root, _, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return ec, err
	}
	ec.RepositoryRoot = root
	branchRef := ""
	if root != "" {
		ec.Head, _, err = git("rev-parse", "--verify", "HEAD")
		if err != nil {
			return ec, err
		}
		branchRef, _, err = git("symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil {
			return ec, err
		}
		changes, ok, e := git("status", "--porcelain=v1", "--untracked-files=normal")
		if e != nil {
			return ec, e
		}
		if ok {
			dirty := changes != ""
			ec.Dirty = &dirty
		}
	}
	bindings := []nativeBinding{}
	var overlay struct {
		VCSRoots []struct{ Server, Remote, RootID string }
	}
	allowSymlinks := ec.Config != nil && ec.Config.AllowConfigSymlinks
	if root != "" {
		for dir := ec.Cwd; ; dir = filepath.Dir(dir) {
			raw, e := repositoryFile(filepath.Join(dir, "teamcity.toml"), root, allowSymlinks)
			if e != nil {
				return ec, e
			}
			if raw != nil {
				bindings, e = nativeBindings(raw)
				if e != nil {
					return ec, e
				}
				rel, _ := filepath.Rel(dir, ec.Cwd)
				rel = filepath.ToSlash(rel)
				for i, b := range bindings {
					longest := -1
					var best bindingScope
					for path, s := range b.Paths {
						if (rel == path || strings.HasPrefix(rel, path+"/")) && len(path) > longest {
							longest = len(path)
							best = s
						}
					}
					if longest >= 0 {
						if best.Project != "" {
							bindings[i].Project = best.Project
						}
						if best.Job != "" {
							bindings[i].Job = best.Job
						}
						if len(best.Jobs) > 0 {
							bindings[i].Jobs = best.Jobs
						}
					}
				}
				break
			}
			if dir == root || dir == filepath.Dir(dir) {
				break
			}
		}
		raw, e := repositoryFile(filepath.Join(root, ".teamcity-axi.json"), root, allowSymlinks)
		if e != nil {
			return ec, e
		}
		if raw != nil {
			var value any
			if json.Unmarshal(raw, &value) != nil {
				return ec, Usage("Malformed repository overlay JSON")
			}
			if err := ValidateConfig("repository-config", value); err != nil {
				return ec, err
			}
			if json.Unmarshal(raw, &overlay) != nil {
				return ec, Usage("Malformed repository overlay JSON")
			}
		}
	}
	registered := map[string]string{}
	if ec.Config != nil {
		seen := map[string]bool{}
		for alias, s := range ec.Config.Servers {
			u, e := CanonicalURL(s.URL, s.AllowHTTPLoopback)
			if e != nil {
				return ec, e
			}
			if seen[u] {
				return ec, NewError("AMBIGUOUS_CONTEXT", "Trusted aliases must not register the same canonical server URL", 2)
			}
			seen[u] = true
			registered[alias] = u
		}
	}
	selectURL := func(u string) bool {
		for alias, registeredURL := range registered {
			if u == registeredURL {
				ec.Server = alias
				ec.ServerURL = u
				return true
			}
		}
		return false
	}
	explicit := p.String("server")
	_, explicitServer := p.Flags["server"]
	source := "flag"
	if !explicitServer {
		explicit, explicitServer = env["TEAMCITY_AXI_SERVER"]
		source = "TEAMCITY_AXI_SERVER"
	}
	if explicitServer {
		ec.ServerURL = registered[explicit]
		if ec.ServerURL == "" {
			return ec, NewError("UNTRUSTED_SERVER", "Server alias is not registered", 1)
		}
		ec.Server = explicit
		ec.Sources["server"] = source
	} else if env["TEAMCITY_URL"] != "" {
		u, e := CanonicalURL(env["TEAMCITY_URL"], true)
		if e != nil {
			return ec, e
		}
		if !selectURL(u) {
			return ec, NewError("UNTRUSTED_SERVER", "Inherited server URL is not registered", 1)
		}
		ec.Sources["server"] = "TEAMCITY_URL"
	} else if len(bindings) > 0 {
		if len(bindings) > 1 {
			return ec, NewError("AMBIGUOUS_CONTEXT", "Repository has multiple applicable servers; select --server", 2)
		}
		u, e := CanonicalURL(bindings[0].URL, true)
		if e != nil {
			return ec, e
		}
		if !selectURL(u) {
			return ec, NewError("UNTRUSTED_SERVER", "Repository selects an unregistered server", 1)
		}
		ec.Sources["server"] = "repository"
	} else if ec.Config != nil && ec.Config.DefaultServer != "" {
		ec.Server = ec.Config.DefaultServer
		ec.ServerURL = registered[ec.Server]
		if ec.ServerURL == "" {
			return ec, Usage("defaultServer is not registered")
		}
		ec.Sources["server"] = "default"
	}
	if !(p.Descriptor.Name == "doctor" && p.Bool("offline")) && env["TEAMCITY_TOKEN"] != "" {
		u, e := CanonicalURL(env["TEAMCITY_URL"], ec.Config != nil && ec.Config.Servers[ec.Server].AllowHTTPLoopback)
		if env["TEAMCITY_URL"] == "" || e != nil || ec.Server == "" || u != ec.ServerURL {
			return ec, NewError("AUTH_CONTEXT_MISMATCH", "Inherited token is not bound to the selected server", 1)
		}
	}
	var binding *nativeBinding
	for i, b := range bindings {
		u, e := CanonicalURL(b.URL, true)
		if e != nil {
			return ec, e
		}
		if u == ec.ServerURL && ec.Server != "" {
			if binding != nil {
				return ec, NewError("AMBIGUOUS_CONTEXT", "Duplicate repository bindings for the selected server", 2)
			}
			binding = &bindings[i]
		}
	}
	if binding != nil {
		ec.Project = binding.Project
		ec.Job = binding.Job
	}
	for _, key := range []string{"project", "job"} {
		val := p.String(key)
		source := "repository"
		if _, ok := p.Flags[key]; ok {
			if !validIdentity(val) {
				return ec, Usage("Invalid project/job identity")
			}
			source = "flag"
			if key == "project" {
				ec.Project = val
			} else {
				ec.Job = val
			}
		}
		if key == "project" {
			val = ec.Project
		} else {
			val = ec.Job
		}
		if val != "" {
			if !validIdentity(val) {
				return ec, Usage("Invalid project/job identity")
			}
			ec.Sources[key] = source
		}
	}
	ec.Branch = branchRef
	ec.BranchDefined = branchRef != ""
	if p.Bool("all-branches") {
		ec.Branch = ""
		ec.BranchDefined = false
	} else if _, ok := p.Flags["literal-branch"]; ok {
		ec.Branch = p.String("literal-branch")
		ec.BranchDefined = true
	} else if _, ok := p.Flags["branch"]; ok {
		ec.Branch = p.String("branch")
		ec.BranchDefined = true
		if ec.Branch == "@this" {
			if branchRef == "" {
				return ec, NewError("CONTEXT_REQUIRED", "No current logical branch is available", 2)
			}
			ec.Branch = branchRef
		}
	}
	if ec.BranchDefined {
		ec.Sources["branch"] = "git"
		if p.String("branch") != "" || p.String("literal-branch") != "" {
			ec.Sources["branch"] = "flag"
		}
	}
	ec.Revision = ec.Head
	if _, ok := p.Flags["revision"]; ok {
		ec.Revision = p.String("revision")
		if !validIdentity(ec.Revision) {
			return ec, Usage("Invalid revision identity")
		}
	}
	if ec.Revision == "@head" {
		if ec.Head == "" {
			return ec, NewError("CONTEXT_REQUIRED", "No committed Git HEAD is available", 2)
		}
		ec.Revision = ec.Head
	}
	ec.VCSRootID = p.String("vcs-root")
	_, explicitVCSRoot := p.Flags["vcs-root"]
	if ec.VCSRootID == "" && ec.Server != "" {
		matches := []int{}
		for i, m := range overlay.VCSRoots {
			if m.Server == ec.Server {
				matches = append(matches, i)
			}
		}
		if len(matches) == 1 && root != "" {
			m := overlay.VCSRoots[matches[0]]
			remote, _, err := git("config", "--get", "remote."+m.Remote+".url")
			if err != nil {
				return ec, err
			}
			if remote != "" {
				ec.VCSRootID = m.RootID
			}
		}
	}
	if (ec.VCSRootID != "" || explicitVCSRoot) && !validIdentity(ec.VCSRootID) {
		return ec, Usage("Invalid VCS root identity")
	}
	if ec.Revision != "" && !validIdentity(ec.Revision) {
		return ec, Usage("Invalid revision identity")
	}
	if p.String("job") != "" {
		ec.Jobs = []string{ec.Job}
	} else if binding != nil && len(binding.Jobs) > 0 {
		seen := map[string]bool{}
		for _, j := range binding.Jobs {
			if !seen[j] {
				ec.Jobs = append(ec.Jobs, j)
				seen[j] = true
			}
		}
	} else if ec.Job != "" {
		ec.Jobs = []string{ec.Job}
	}
	if time.Now().UnixMilli() >= ec.Deadline {
		return ec, deadlineError(ec.Deadline)
	}
	return ec, nil
}
func PublicContext(ec ExecutionContext) Object {
	if ec.Server == "" {
		return nil
	}
	o := Object{"server": ec.Server}
	for k, v := range map[string]string{"project": ec.Project, "job": ec.Job, "revision": ec.Revision, "vcsRootId": ec.VCSRootID} {
		if v != "" {
			o[k] = v
		}
	}
	if len(ec.Jobs) > 0 {
		o["jobs"] = append([]string{}, ec.Jobs...)
	}
	if ec.BranchDefined || ec.Branch != "" {
		o["branch"] = ec.Branch
	}
	return o
}
