# Changelog

## Unreleased Go reimplementation

Reimplements all eighteen accepted read-only commands and development tooling in
Go. The standalone executable embeds offline schemas and the portable skill.
TOON remains the default; JSON is optional. Ordinary native operations and auth
management stay with the official TeamCity CLI. No JS/TS runtime is required.

The migration retains the original adversarial test behaviors and captured wire
artifacts, adds Go race checks, and separately reruns native and restricted live
contracts. See docs/go-test-baseline.json and the current release audit.

## 0.1.0 historical TypeScript release

The initial read-only product implements eighteen command services: exact-checkout
status, fixed-execution watch, independent run evidence and failure investigation,
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
Publication awaits owner approval; no GitHub or npm registry release is claimed.
