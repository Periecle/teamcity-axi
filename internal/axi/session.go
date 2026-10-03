package axi

import (
	"context"
	"path/filepath"
	"regexp"
	"sync"
)

type ReadSession struct {
	Reader            Reader
	Transport         *ProcessTransport
	NativeVersion     string
	MaxChildProcesses int
	Binary            string
}

func (s *ReadSession) Close() error {
	if s.Transport == nil {
		return nil
	}
	return s.Transport.Close()
}
func OpenReadSession(ctx context.Context, ec ExecutionContext, profile string) (*ReadSession, error) {
	env := Environment()
	path := ""
	allow := false
	if ec.Config != nil {
		path = ec.Config.BinaryPath
		allow = ec.Config.AllowWorkspaceBinary
	}
	binary, err := ResolveBinary(path, ec.RepositoryRoot, allow, env)
	if err != nil {
		return nil, err
	}
	server := ServerConfig{}
	if ec.Config != nil {
		server = ec.Config.Servers[ec.Server]
	}
	serverURL := ec.ServerURL
	if profile == "offline" {
		for k := range env {
			if len(k) >= 9 && k[:9] == "TEAMCITY_" {
				delete(env, k)
			}
		}
		serverURL = "https://offline.invalid"
		server.ForwardHeaderEnvNames = nil
	}
	limits := ReadLimits(ec, profile)
	transport, err := NewProcessTransport(TransportOptions{Binary: binary, ServerURL: serverURL, Env: env, Limits: limits, HeaderNames: server.ForwardHeaderEnvNames})
	if err != nil {
		return nil, err
	}
	capture, err := transport.Execute(ctx, Operation{Kind: "version"}, -1)
	if err != nil {
		transport.Close()
		return nil, err
	}
	match := regexp.MustCompile(`^teamcity version ([a-zA-Z0-9.+-]{1,80})\r?\n$`).FindSubmatch(capture.Stdout)
	if capture.ExitCode != 0 || capture.Signal != "" || match == nil {
		transport.Close()
		return nil, NewError("DEPENDENCY_UNSUPPORTED", "Native executable has an unsupported version response", 1)
	}
	reader := NewNativeReader(transport, serverURL, KnownSecrets(env, secretPatterns(ec), server.ForwardHeaderEnvNames))
	return &ReadSession{Reader: reader, Transport: transport, NativeVersion: string(match[1]), MaxChildProcesses: limits.MaxChildren, Binary: filepath.Base(binary)}, nil
}
func secretPatterns(ec ExecutionContext) []string {
	if ec.Config != nil {
		return ec.Config.SecretNamePatterns
	}
	return nil
}

type policyObservation struct {
	done    chan struct{}
	project Object
	err     error
}
type ProjectPolicy struct {
	reader   Reader
	roots    []string
	budget   Budget
	mu       sync.Mutex
	projects map[string]*policyObservation
}

func NewProjectPolicy(reader Reader, roots []string, budget Budget) *ProjectPolicy {
	return &ProjectPolicy{reader: reader, roots: roots, budget: budget, projects: map[string]*policyObservation{}}
}
func (p *ProjectPolicy) Project(ctx context.Context, id string) (Object, error) {
	return p.ProjectBudget(ctx, id, p.budget)
}
func (p *ProjectPolicy) ProjectBudget(ctx context.Context, id string, budget Budget) (Object, error) {
	p.mu.Lock()
	observation, found := p.projects[id]
	if !found {
		observation = &policyObservation{done: make(chan struct{})}
		p.projects[id] = observation
	}
	p.mu.Unlock()
	if !found {
		read := p.reader.Read(ctx, ReadRequest{Kind: "project.detail", ID: id}, budget)
		if read.State == "available" {
			observation.project = read.Value
		} else {
			observation.err = read.Error
		}
		close(observation.done)
	} else {
		select {
		case <-observation.done:
		case <-ctx.Done():
			return nil, NewError("INTERRUPTED", "Invocation interrupted", 1)
		}
	}
	return observation.project, observation.err
}
func (p *ProjectPolicy) Assert(ctx context.Context, id string) error {
	return p.AssertBudget(ctx, id, p.budget)
}
func (p *ProjectPolicy) AssertBudget(ctx context.Context, id string, budget Budget) error {
	if p.roots == nil {
		return nil
	}
	if id == "" {
		return NewError("POLICY_DENIED", "Project identity is unavailable for trusted policy", 1)
	}
	seen := map[string]bool{}
	current := id
	for depth := 0; current != "" && depth < 8; depth++ {
		if contains(p.roots, current) {
			return nil
		}
		if seen[current] {
			break
		}
		seen[current] = true
		project, err := p.ProjectBudget(ctx, current, budget)
		if err != nil {
			return err
		}
		current = Str(project, "parentProjectId")
	}
	if current != "" && contains(p.roots, current) {
		return nil
	}
	return NewError("POLICY_DENIED", "Project ancestry does not establish an allowed subtree", 1)
}
