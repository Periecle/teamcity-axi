# Primary source register

Consulted on **1 October 2026**. Links to `main` are mutable; the implementation must record immutable revisions and test released binaries. No TeamCity tenant belonging to Roman was queried.

## S1 — Official CLI scripting and JSON contract

`https://www.jetbrains.com/help/teamcity/teamcity-cli-scripting.html`

Basis: list field selection, structured inspection, non-interactive mode and read-only behavior. Do not assume every command accepts every output flag.

## S2 — Official run operations

`https://www.jetbrains.com/help/teamcity/teamcity-cli-managing-runs.html`

Basis: run/log/test/change/watch/diff operations and native failure diagnostics. Wrapper semantics and limits in the specification are separate design decisions.

## S3 — Native log implementation

`https://raw.githubusercontent.com/JetBrains/teamcity-cli/main/internal/cmd/run/log.go`

Inspected areas: `failureSummaryJSON`, `runLogFull`, `runLogTail`, `runLogFailed`. The researched `runLogFailed` populates problems/tests only when subsidiary fetches succeed, without emitting their failures in the summary. This is a version-specific source observation, not a claim about every released binary.

## S4 — Official raw REST command documentation

`https://www.jetbrains.com/help/teamcity/teamcity-cli-rest-api-access.html`

Basis: explicit methods, headers, raw response, response headers and pagination controls.

## S5 — Native raw API implementation

`https://raw.githubusercontent.com/JetBrains/teamcity-cli/main/internal/cmd/api/api.go`

Inspected areas: command flags and `outputAPIResponse`. Basis: absence of a declared `--json` flag on the inspected command, raw response/header envelope, and separate error handling. Pin and verify before depending on this output shape.

## S6 — Native run-tree implementation

`https://raw.githubusercontent.com/JetBrains/teamcity-cli/main/internal/cmd/run/tree.go`

Basis: recursive snapshot-dependency expansion; native depth zero is unlimited. The wrapper deliberately uses controlled traversal and different zero-depth semantics.

## S7 — Native repository linking

`https://www.jetbrains.com/help/teamcity/teamcity-cli-linking.html`

Basis: `teamcity.toml`, multiple server entries, path scopes and context precedence. The wrapper's trusted-server restriction is an additional product constraint.

## S8 — REST build locators

`https://www.jetbrains.com/help/teamcity/rest/buildlocator.html`

Basis: default filtering, state dimensions, count/start and lookupLimit. Safely encoding arbitrary branch names must be verified against the selected server versions.

## S9 — REST collection pagination

`https://www.jetbrains.com/help/teamcity/rest/builds.html`

Additional implementation reference:

`https://raw.githubusercontent.com/JetBrains/teamcity-cli/main/api/builds.go`

Basis: collection continuation and native scan behavior. A returned page count is not sufficient evidence of a global total.

## S10 — AXI principles

`https://github.com/kunchenguid/axi`

`https://raw.githubusercontent.com/kunchenguid/axi/main/.agents/skills/axi/SKILL.md`

Basis: agent-oriented CLI behavior, compact structured output, explicit state/errors and context-sensitive discovery. No AXI benchmark result is claimed for teamcity-axi. The specification's own contract and declared deviations take precedence for this project.

## S11 — TOON reference implementation and specification

`https://github.com/toon-format/toon`

`https://toonformat.dev/reference/spec`

Basis: use of a tested serializer rather than hand-built output. Exact package/spec compatibility is a release gate.

## S12 — Node.js release table

`https://nodejs.org/en/about/previous-releases`

Basis: Node.js 24 listed as LTS at research time. Runtime selection is a project decision, not a requirement imposed by TeamCity or AXI.

## S13 — AXI JavaScript SDK

`https://raw.githubusercontent.com/kunchenguid/axi/main/packages/axi-sdk-js/README.md`

Basis: shared CLI runtime and documented built-in self-update behavior. The initial specification avoids adopting an executable boundary with undesired inherited commands.

## S14 — Official configuration and authentication

`https://www.jetbrains.com/help/teamcity/teamcity-cli-configuration.html`

`https://www.jetbrains.com/help/teamcity/teamcity-cli-authentication.html`

Basis: server/token environment variables, stored authentication, read-only mode, color and update settings. The wrapper's origin binding and environment narrowing are additional security requirements.

## S15 — Official telemetry behavior

`https://www.jetbrains.com/help/teamcity/teamcity-cli-analytics.html`

Basis: documented opt-out through `DO_NOT_TRACK`. The wrapper should set it for child invocations without changing global user preferences.

## S16 — Official CLI contribution/JSON-error contract

`https://raw.githubusercontent.com/JetBrains/teamcity-cli/main/CONTRIBUTING.md`

Basis: JSON success on stdout and structured errors on stderr for JSON-enabled data commands. The wrapper deliberately re-emits its own errors on stdout.

## S17 — REST discovery and versioning

`https://www.jetbrains.com/help/teamcity/rest/teamcity-rest-api-documentation.html`

Basis: server information, `/app/rest/swagger.json`, protocol-version considerations and endpoint discovery. Discovery must remain bounded and must not expose the full server schema to an agent by default.
