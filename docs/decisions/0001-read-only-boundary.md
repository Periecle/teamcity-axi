# ADR 0001: read-only executable and authority boundary

Accepted for the initial implementation, 2026-10-02.

The wrapper owns a strict command registry. No API passthrough, mutation, hook,
self-update, log download or shell operation is exposed. Native operations are
typed as bounded GET, structured tail, or local version. They run through one
invocation-owned transport with a frozen server, argument arrays, closed stdin,
empty temporary working directory, capture limits, child ceiling, concurrency
ceiling and overall deadline. POSIX process groups are killed and the native
parent is reaped before cleanup. Windows is unsupported.

The child forces `TEAMCITY_RO=1`, `TEAMCITY_NO_UPDATE=1`, `DO_NOT_TRACK=1`,
`NO_COLOR=1` and `TERM=dumb`. Unrecognized TeamCity variables and build-auth
fallback variables are removed. Trusted explicitly configured custom headers
cannot override authentication, host or connection handling.

These controls limit the wrapper, not the authority of another program using the
same token. Real deployments require a restricted server identity. A client-side
project policy is additional narrowing and never substitutes for server RBAC.
