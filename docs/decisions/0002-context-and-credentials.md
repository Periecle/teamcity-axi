# ADR 0002: trusted configuration and worktree context

Accepted for the initial implementation, 2026-10-02.

User configuration resides at `${XDG_CONFIG_HOME:-~/.config}/teamcity-axi/config.json`.
It must validate against the packaged schema and be owned by the invoking user
without group/other write permission. Repository TOML selects existing registered
servers and exact scope identifiers; it cannot register credential destinations.
Its deepest path binding overlays server defaults as in CLI v1.5.0's pinned
`internal/link/link.go`. The nearest TOML is found without crossing the worktree
root. Wrapper overlays have a separate deny-unknown schema and escaping symlinks
are rejected unless trusted user configuration explicitly allows them.

Resolve Git with local bounded commands, no remote fetch, fsmonitor or inherited
Git execution overrides. A Git worktree file is supported by `rev-parse`; branch,
HEAD and dirty state belong to each checkout. Detached HEAD does not invent a
branch. A configured VCS mapping names a local Git remote; an absent remote is
not used to claim a verified mapping.

URLs preserve context paths. URL credentials, queries, fragments, dot traversal
and encoded path separators/dots are rejected before URL normalization. HTTPS
is required except explicitly allowed loopback development servers. Inherited
tokens require an explicitly supplied matching URL including context path. This
is checked both during resolution and at the subprocess boundary.
