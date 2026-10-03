# Changelog

## Unreleased performance improvements

- Compile packaged schemas only when their contracts are validated, with a
  synchronized compiler/cache and unchanged offline validation requirements.
- Promote full-detail recovery hints only for truncated retained excerpts;
  preserve every exact, scoped evidence retrieval action.
- Add concrete first-read commands and output-field guidance to the portable
  skill so investigations do not need routine help/schema discovery or rereads.
- Add concurrent fail-closed schema validation and targeted hint regression
  coverage. New performance measurements remain separate from v0.1.0 evidence.
- Alternating cold starts reduce schema/context medians about 34%; all four
  scripted reports retain their 384 observations and unchanged evidence scores.
  See the [performance comparison](docs/performance.md).
- Give the 64 MiB protocol-flood test a longer fixture deadline under CI race
  instrumentation; it still requires overflow and prompt process reaping.
- Fresh independently graded model tasks use one wrapper tool call each, eight
  total, versus native median three and 24 total. Both succeed on 8/8 tasks
  without correctness/security errors. Median Go session time improves from
  55.4 to 44.3 seconds; historical TS's 37.3 seconds remains faster and wrapper
  output remains larger than native. All per-task timing regressions are retained.

## 0.1.0 — 2026-10-03

Reimplements all eighteen accepted read-only commands and development tooling in
Go. The standalone executable embeds offline schemas and the portable skill.
TOON remains the default; JSON is optional. Ordinary native operations and auth
management stay with the official TeamCity CLI. No JS/TS runtime is required.

The migration retains the original adversarial test behaviors and captured wire
artifacts, adds Go race checks, and separately reruns native and restricted live
contracts. See [the preserved baseline](https://github.com/Periecle/teamcity-axi/blob/v0.1.0/docs/go-test-baseline.json) and
[the current release audit](https://github.com/Periecle/teamcity-axi/blob/v0.1.0/docs/release-audit.md).

The Go release passes 163 deterministic tests, 56 additional native contracts,
18 restricted live tests and race/vet/format/documentation checks. Sixteen
independently graded model sessions pass all eight tasks in both conditions
without correctness or credential-exposure errors. Median tool calls tie at
three, total calls are 30 versus 22, and the secret task regresses to nine versus
three. Shared-dependency and cycle tasks each improve to two versus four. The
lower-median target is unmet; no general efficiency claim is made. The final
scripted benchmark retains all wrapper evidence at about 110 ms, with optimized
native workflows smaller and faster. See [measured results](https://github.com/Periecle/teamcity-axi/blob/v0.1.0/docs/evaluation.md).

## Historical TypeScript checkpoint (unpublished)

The initial read-only implementation covered eighteen command services:
exact-checkout status, fixed-execution watch, independent run evidence and failure investigation,
scoped jobs/queue/agents, context diagnostics, and local help/schema commands.

- Preserves exact execution, job, project and VCS-root identity; distinguishes
  observation from assertions and partial evidence from verified absence.
- Reads independent problem/test/change/log/dependency sources. Duplicate and
  muted occurrences, shared DAG nodes, cycles and traversal boundaries remain
  explicit. Failure order does not become a root-cause claim.
- Enforces trusted server selection, origin-bound credentials, neutral child CWD,
  constrained read-only operations, numeric resource ceilings and bounded valid
  JSON/TOON output. Known credentials and terminal controls are sanitized.
- Supports exact fractional run-list windows, normalized unknown/exceptional
  outcomes, conservative continuations and verified first-page exhaustion on
  the recorded TeamCity server build.
- Packages the portable skill, schemas, generated command reference and license
  inventory. Pinned formatters enforce readable TypeScript spacing in CI.

Executed compatibility: Linux x64, Node 24.14.0, official TeamCity CLI 1.5.0 and
TeamCity 2026.2 build 238924 with restricted project-view credentials. Other
platforms/server builds are untested. Structured logs remain retained tails;
positive restricted pool availability is unavailable on this fixture.

The recorded scripted benchmark demonstrates retained typed evidence and
redaction, while the optimized native baseline is smaller and faster. It does
not establish token or latency savings. Sixteen actual model-agent sessions passed independent grading. Median tool
turns were 3 versus 3.5 in this fixed sample, with a secret-task regression of 8
versus 4. There is no general performance guarantee. The final release audit
accepts only the recorded compatibility scope. Comparison, setup/hooks and
mutations remain deferred. The compiled package and checksum are prepared for GitHub distribution.
At that checkpoint publication awaited owner approval. No TypeScript GitHub or
npm registry release was published.
