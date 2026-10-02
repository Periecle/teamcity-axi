# Product delivery and acceptance

The development product now implements all eighteen registered read-only command
services: checkout status, exact execution watch and investigation, independent
run evidence, scoped jobs/queue/agents, context diagnostics, and local schema/help.
The portable skill and command reference are packaged. This completes the command
implementation portion of milestone 6; the full project goal remains active until
remaining contract, evaluation, and release gates are proved.

| Milestone                | Delivered                                                                                                                                                                                                                                                                                        | Remaining acceptance work                                                                                                                                    |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 0: upstream contract     | Native v1.5.0 and immutable source pinned; four Unix checksums; 39 Linux x64 native/mock captures and 109 restricted TeamCity 2026.2 captures; boundary/context/read-only ADRs                                                                                                                   | Broader live multi-root/branch/DAG/queue/agent fixtures, live deployment context prefix, actual expired-token evidence, execution of other claimed platforms |
| 1: executable boundary   | Strict registry/parser, local help/version/schema, every registered command service, command-specific schemas, equivalent JSON/TOON, bounded/redacted output                                                                                                                                     | Release-wide schema/examples/package audit and remaining flag capability gates                                                                               |
| 2: context/transport     | Trusted user config, repository binding/root mapping, worktree-aware Git, token-origin binding, neutral child CWD, restricted environment, shared budgets, cancellation/process-group cleanup                                                                                                    | Final adversarial/concurrency acceptance audit across the complete product                                                                                   |
| 3: vertical reads        | Exact run view, bounded scoped run pages, local/verified context and doctor, reconstructed cursors and scope/policy assertions                                                                                                                                                                   | Broader positive live branch/revision cases and exact exhausted-empty collection proof                                                                       |
| 4: evidence              | Independent problem/test/change/log/dependency reads and exact occurrence expansion, explicit availability/unknown totals                                                                                                                                                                        | Broader live muted/duplicate/source-denial/retention cases and complete primitive gate audit                                                                 |
| 5: failure investigation | Bounded DAG/cycle traversal, deterministic evidence references, independent source coverage, reserved final reads, no causal overclaims                                                                                                                                                          | Broader live shared graphs and final investigation acceptance audit                                                                                          |
| 6: read-only product     | Exact-checkout status, fixed-run watch, safe job/queue/agent reads, versioned portable skill, registry-generated help/examples with CI drift gate                                                                                                                                                | Broader live status/queue/agent lifecycle and multi-worktree scenarios, final product acceptance audit                                                       |
| 7: evaluation/release    | Eight-task sanitized corpus; actual optimized native selected-field JSON and failure diagnostics; 96 recorded workflow observations; pinned token/latency/process/HTTP/identity/completeness/canary metrics; paired startup timing; package-content check and actual tested compatibility matrix | Isolated model-agent task-success/tool-turn/causal-claim evaluation; release notes and final platform/package/acceptance audit                               |

Comparison, harness setup/hooks, and mutation commands remain in the plan's
separate deferred scopes. They are not silently folded into the read-only product.

## Verification

Current full local checks pass on Node 24.14.0 / Linux x64:

- 128 deterministic unit and executable tests, including formatting and generated
  help/examples for all eighteen registry descriptors.
- 53 released-native CLI/mock-server tests, including the evaluation corpus.
- 17 actual restricted-server tests on TeamCity 2026.2, build 238924.
- No tests skipped. TypeScript compilation, diff validation, and portable skill
  validation pass. Package inspection includes status/watch services and schemas,
  the skill, and generated reference; excludes test/setup scripts and credentials.
  The production-only installed package passes fourteen smoke checks, including precise fractional windows, normalized unknown filtering, and explicit
  canceled, failed-to-start, and composite results through the pinned native CLI.

Independent evidence/security review found no remaining material run-list or status/watch
issues after fixes to strict completeness, scoped hints, continuation safety,
retained source limitations, observed execution context, and explicit outcome
classification. A full unknown-outcome page retains all rows after repeated
diagnostics are grouped before schema validation. LSP returned cached
cross-file types or timed out on some files; current TypeScript compilation is
the authoritative semantic fallback. Full release certification is still separate.

GitHub CI passed both jobs on the outcome checkpoint `3500d1b`
([run 37035063651](https://github.com/Periecle/teamcity-axi/actions/runs/37035063651)).
The existing workflow runs the generated-documentation drift gate through
`npm test`; no workflow authorization expansion is needed.

## Product behavior

Status reads an exact bounded union of up to five tracked jobs and their twenty
most recent run candidates. Native/live captures prove queued and finished
candidates, exact literals, five-job union, filtered-empty branch and unavailable
job omission. The newest selected-root exact candidate controls the job check;
newer unknown revision evidence cannot pass, and older green never replaces newer
exact running/queued/red work. Dirty worktrees, missing roots/jobs, personal runs,
unverified other roots, and omitted required jobs cannot assert green. Activity
counts and history coverage remain bounded rather than global.

Watch polls one frozen execution, emits one final document, and rechecks exact
identity and policy. It retains the latest verified observation and source notes
at deadline or after vanished/inaccessible reads. The final context belongs to
that observed execution. Native tests cover terminal transition, vanished/access
loss, deadline, both serializers, scoped identity, and SIGINT/SIGTERM. Live tests
cover ordinary/composite success, red/canceled/failed-to-start terminal assertions, queued deadline retention, and an actual two-poll running-to-canceled transition. Neither status
nor watch mutates TeamCity.

The [portable skill](../skills/teamcity-axi/SKILL.md) teaches exact checkout versus
execution checks, untrusted evidence, partial/unknown semantics, and narrowed
read-only expansion. [Command help/examples](commands.md) are generated from the
same registry used by parsing; CI rejects drift. Captured wire artifacts remain
excluded from automatic formatting. Pinned ESLint Stylistic inserts TypeScript
spacing, including unbraced guards, while pinned Prettier formats layout.

## Evaluation and next delivery

The [recorded evaluation](evaluation.md) retained required evidence in all 48
wrapper observations, with no canary exposure or response-budget failure.
The native selected-field baseline is smaller and faster; there is no token or
latency saving claim. Standalone native failure diagnostics lack required
availability and graph evidence for this compound rubric. Independent review
verified projection fairness, schema/exit checks, exact evidence attribution,
and rejection of false-completeness claims. Model-agent task success, actual
tool turns, and causal-claim metrics remain null rather than inferred from
scripted launches. CI enforces the new evidence benchmark in the native suite.

Explicit canceled and failed-to-start outcomes now propagate through view, list,
status, watch and investigation. Ordinary result selectors exclude exceptional
outcomes; missing or conflicting metadata cannot pass a check. Composite remains
independent from lifecycle and result. This closes the focused A11 outcome
contract. See [the decision](decisions/0013-execution-outcomes.md) for captures
and semantics.

Run list now supports normalized unknown-result filtering and precise fractional
finish windows. Live evidence revealed hidden milliseconds in the server
predicate; the adapter preserves exact bounds and declares the coarser reported
timestamp precision. Cursor overflow retains useful partial rows. See
[the list decision](decisions/0014-precise-run-list.md).

Close the model-agent evaluation gate, remaining live contracts and mandatory
acceptance scenarios, then perform the requirement-by-requirement release audit.
Prioritize demonstrated identity/completeness/security defects that affect the
product, rather than unrelated cleanup.

The temporary localhost sandbox remains available for that work. Its test reader
has only project-view scope inherited from `AxiContract`, including its owned
lifecycle child and root, and no inherited All Users roles or build-run
permission. Positive pool and active/idle/mixed agent
availability remain unproved under that identity; do not broaden permissions to
hide unavailable reads. Fixture setup writes belong only to owned test resources
and are absent from the package. Cleanup of the owned sandbox waits until its
remaining integration evidence is collected.
