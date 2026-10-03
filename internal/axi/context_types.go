package axi

type ServerConfig struct {
	URL                   string   `json:"url"`
	AllowedProjects       []string `json:"allowedProjects,omitempty"`
	AllowHTTPLoopback     bool     `json:"allowHttpLoopback,omitempty"`
	ForwardHeaderEnvNames []string `json:"forwardHeaderEnvNames,omitempty"`
}
type UserLimits struct {
	Concurrency       int `json:"concurrency,omitempty"`
	MaxBytes          int `json:"maxBytes,omitempty"`
	MaxChildProcesses int `json:"maxChildProcesses,omitempty"`
}
type UserConfig struct {
	SchemaVersion        string                  `json:"schemaVersion"`
	ReadOnly             bool                    `json:"readOnly"`
	Servers              map[string]ServerConfig `json:"servers"`
	DefaultServer        string                  `json:"defaultServer,omitempty"`
	BinaryPath           string                  `json:"binaryPath,omitempty"`
	AllowWorkspaceBinary bool                    `json:"allowWorkspaceBinary,omitempty"`
	AllowConfigSymlinks  bool                    `json:"allowConfigSymlinks,omitempty"`
	SecretNamePatterns   []string                `json:"secretNamePatterns,omitempty"`
	Limits               UserLimits              `json:"limits,omitempty"`
}
type ExecutionContext struct {
	Deadline                                                                                int64
	Cwd, RepositoryRoot, Head, Server, ServerURL, Project, Job, Branch, Revision, VCSRootID string
	Dirty                                                                                   *bool
	BranchDefined                                                                           bool
	Jobs                                                                                    []string
	Sources                                                                                 Object
	Config                                                                                  *UserConfig
}
type ProcessLimits struct {
	Deadline                                           int64
	Concurrency, MaxChildren, StdoutBytes, StderrBytes int
}
