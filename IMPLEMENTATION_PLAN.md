# Implementation plan and handoff

This plan implements `SPECIFICATION.md`. It retains milestone contracts and
acceptance gates, not calendar estimates or a current completion report. The
accepted read-only scope is implemented in Go. See
[implementation status](docs/STATUS.md) and
[the current release verification](docs/release-v0.1.1-verification.json)
and [historical v0.1.0 verification](docs/release-verification.json) for executed checks
and release evidence. Comparison, setup/hooks and writes retain their separate
deferred gates below.

## Milestone 0 — Verify and freeze the upstream contract

Deliverables:

- One selected released native CLI binary for each development platform; version and SHA-256 recorded.
- A documented test server version/build and a restricted test identity.
- Sanitized fixtures for run detail/list, problems, tests, structured log tail, immediate snapshot dependencies, changes, queue, jobs and agents.
- Error fixtures for permission denial, missing data, expired authentication, malformed response and unsupported log capability.
- A compatibility manifest identifying each required/optional feature.
- ADRs covering read-only scope, trusted server/context, response/completeness, serializer and subprocess/API boundary.

Verify the native failure-summary omission behavior against the chosen binary rather than assuming `main` exactly matches the release. Confirm whether raw API header output can be parsed as specified and whether server context paths are retained correctly. Establish which independent read endpoints and locator dimensions are supported.

**Gate:** no adapter code is considered complete without a recorded input/output fixture for the native operation it relies on. Unsupported capabilities are represented, not mocked into existence.

## Milestone 1 — Build the executable boundary

Implement the command registry, strict argument validation, version/help fast paths, base response models, errors, TOON/JSON renderer, runtime schema validation and minimal `schema` command.

Required checks:

- Unknown/misspelled flags and duplicate conflicting flags produce exit 2 without any child process.
- No-argument local/unconfigured view is small and valid.
- Version/help require no credentials, Git access, network or heavy initialization.
- JSON and TOON preserve the same logical values; quoted numeric strings stay strings.
- Oversized input, Unicode and malicious terminal controls cannot create malformed output.

**Gate:** a new domain command can return a typed fixture through the entire executable/rendering path without touching a real server.

## Milestone 2 — Implement context and restricted process transport

Implement trusted user config and optional repository overlay, native TOML binding reader, worktree-aware Git context, server selection, token-origin checks, safe executable resolution, environment construction, neutral child CWD, timeout/capture/concurrency controls and process-group cleanup.

Build a fake native executable test fixture. It must be able to log argv/environment safely, hang, emit controlled byte streams and return selected exit statuses. Tests use secret canaries but must not log their values in normal failure reports.

Required adversarial cases: repository selects an attacker URL, working directory contains a fake `teamcity`, `.git` is a worktree file, two worktrees differ in branch, inherited token belongs to another server, locator-like branch names, and child process exceeds capture limit.

**Gate:** every child uses the frozen target/read-only environment, and cancellation leaves no orphan. Actual protection still relies on restricted server credentials, as documented.

## Milestone 3 — Deliver one vertical read slice

Start with `run view`, then `run list`, then `context show`/`doctor`.

Deliver typed DTO adapters, exact identity assertions, pagination model, safe locators, safe raw-API parser and a first real-CLI/mock-server suite. Add log-tail capability probing, but do not make unsupported endpoints mandatory.

**Gate:** end-to-end tests demonstrate that a failed build is reported as a successful observation, an unrelated green run cannot replace the requested ID, and permission/malformed responses cannot become empty success.

## Milestone 4 — Add independent evidence primitives

Implement problems, tests, changes, log-tail and bounded dependency reads. Add selected-item expansion and accurate source availability. Ensure muted tests and duplicate names remain distinct. Do not trust a combined summary to establish missing-source completeness.

**Gate:** all primitive commands have positive, permission-denied, missing-capability, truncated-page and oversized-output cases. Every emitted next-action argv must parse under the command registry.

## Milestone 5 — Build failure investigation

Implement wrapper-owned graph traversal, deterministic candidate selection, source coverage, observation/hypothesis separation, evidence references, reserved budgets and final-state recheck.

Use fixed fixtures with a shared-dependency DAG, a cycle, independent failures, root-only failures, canceled/composite/failed-to-start builds, partial permissions and missing logs. No LLM integration or automatic remediation is added.

**Gate:** no fixture produces an unsupported root-cause assertion, false global zero, false leaf or complete report after a required-source failure. Useful sibling evidence survives another source failing.

## Milestone 6 — Complete the read-only product

Add scoped status, required exact-revision matching for the selected job, watch, safe job/queue/agent views, and generated portable skill/help documentation.

A status assertion must fail or remain unverified for a stale branch build, missing required job, unmatched VCS root or dirty worktree. Watch remains read-only and single-document.

**Gate:** two developers with several worktrees/agents can run concurrent read scenarios without shared state contamination, while each invocation respects its configured child concurrency and deadline.

## Milestone 7 — Evaluate and release

Publish a sanitized evaluation corpus and optimized native baseline. Include native selected-field JSON and failure diagnostics. Record task success, tool turns, output tokens with tokenizer identity, latency, subprocess count, identity mistakes, completeness mistakes and secret canary results.

Run package-content checks and platform smoke tests; verify schemas and generated docs; publish the actual compatibility matrix. State support only for tested combinations.

**Gate:** no correctness/security regression is accepted for a token/call-count improvement. Claims in release notes are backed by measured results, not extrapolated AXI benchmarks.

## Deferred milestone — Richer comparisons and opt-in integration

Implement `run diff` with explicit baseline identity, bounded same-branch previous-success search, configuration/revision limitations and missing-retained-evidence handling. Extend exact-revision status to richer multi-job settings.

Verify current official agent harness contracts before adding explicit setup/uninstall. Do not automatically install hooks or capture transcripts.

## Separate future milestone — Guarded writes

Implement only after the read-only product is accepted and a separate scope decision approves the mutation extension. Follow Section 19's immutable plan, policy, explicit apply and unknown-outcome rules. No write command ships as simple passthrough.

## Parallel ownership lanes

| Lane | Owned modules | Stable input/output contract | Must not change alone |
|---|---|---|---|
| A: boundary and context | cli, context, transport | ExecutionContext, command descriptor, captured process result | Public envelope or write policy |
| B: TeamCity adapter | DTO validators, locators, raw parser | TeamCityReader and ReadResult | Completeness meanings |
| C: investigation | graph, coverage, findings | Failure payload and evidence references | Transport concurrency/environment |
| D: output and contracts | schemas, projection, budgeting, redaction | Valid normalized object -> bounded document | Entity identity fields |
| E: verification | fake executable, mock server, live sandbox, evals | Versioned fixtures and task definitions | Golden outputs without review |

Start lanes B–E after the corresponding contracts are approved. Small vertical PRs are preferable to five giant horizontal implementations. One integration owner reviews cross-lane assumptions and schema changes; the authoring agent should not be the only reviewer of its evidence/permission handling.

## Coding-agent handoff instruction

> Implement the read-only milestones in order. Treat SPECIFICATION.md as normative. Start by verifying a released official TeamCity CLI and recording actual fixtures; do not invent endpoint schemas or minimum supported versions. Use the supplied core schemas/examples as contract seeds and add every missing command-specific schema before release. Keep all TeamCity interaction behind TeamCityReader and the restricted subprocess transport. Add negative tests before broadening the command surface. Do not implement writes, unrestricted API passthrough, raw full-log fallback, hidden self-updates, agent-hook installation or LLM-based diagnosis in the initial release. Report any unsupported upstream capability explicitly and preserve all partial-data/unknown semantics.
