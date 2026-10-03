# Go reimplementation

The user requested a complete Go replacement of the accepted read-only product,
including its test and maintenance tooling, with no executable JavaScript or
TypeScript source files, Node dependencies or Node configuration in the worktree. This supersedes the TypeScript/Node/npm stack choice in section
3.1 of the specification and the old formatter/test commands in AGENTS.md.

The Go executable embeds public schemas and the portable skill. TOON remains the
default serialization; JSON uses the same normalized model. Official TeamCity
operations remain native CLI subprocesses: authentication, HTTP handling and
structured log retrieval belong to `teamcity`. AXI owns trusted context, exact
identity assertions, projection, output budgets, evidence accounting, checkout
assessment and bounded investigation. Native setup, authentication management,
ordinary developer CLI operations and writes stay with `teamcity`; AXI adds no
unrestricted passthrough command.

Go contexts govern cancellation and deadlines, invocation-local semaphores govern
child concurrency, and process groups govern descendant cleanup. Runtime JSON
Schema validation remains offline. External library versions are pinned by
go.mod/go.sum. There are no runtime downloads or package-manager hooks.

The original 133 deterministic, 56 native mock and 18 restricted live contracts
passed on Node 24.14.0 before removal. Their scenario inventory is retained in
`docs/go-test-baseline.json`; all 207 scenarios map to Go in
`docs/go-test-parity.json`. Existing captured native and live wire artifacts
are immutable evidence; they do not by themselves certify the Go implementation.
The Go port requires equivalent behavioral tests, actual native mock execution,
live execution for the recorded supported combination, independent review,
formatting/vet checks and compiled package smoke checks before acceptance.

The Go mock tests use a real `httptest` HTTP server and the checksum-verified
official CLI. Live tests reuse the owned restricted TeamCity fixture; no new
container provisioning is needed for these read scenarios. Container-based setup
should use testcontainers when a new disposable fixture is required.

Comparison, setup/hooks, mutations and additional platform/server certification
remain governed by their original scope gates.
