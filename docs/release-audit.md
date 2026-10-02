# Read-only release audit

The accepted scope is milestones 0–7 of the implementation plan. Comparison,
setup/hooks and mutations retain their separate deferred gates. Evidence covers
Linux x64, Node 24.14.0, released TeamCity CLI 1.5.0, and TeamCity 2026.2 build
238924 with restricted project-view credentials. No other platform or server
version is certified by this audit.

| Specification section 21 gate                               | Evidence                                                                                                                                                                                      |
| ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Required command/flag/output contracts and negative cases   | Eighteen registry descriptors, generated command reference and examples; deterministic parser/boundary/command tests and native contract suite.                                               |
| Exact identity and completeness/unknown invariants          | Adapter, context, policy, run-page, status/watch and graph checks; all 32 scenarios mapped in acceptance.md.                                                                                  |
| Restricted raw API and hostile continuations                | Fixed GET argv, field projections, raw-preamble validation, context-prefix mocks, locator/cursor/continuation adversarial tests.                                                              |
| Independent sources rather than native-summary completeness | Problems/tests/logs/changes/dependencies use independent reads; native swallowed-error and source-denial fixtures preserve available evidence.                                                |
| Read-only command and process surface                       | Strict registry, restricted transport environment, neutral child CWD, origin-bound inherited token and GET-only native/live requests.                                                         |
| Restricted credentials and redaction                        | Published security boundary, actual reader permission inventory and foreign-project denial; canaries across stdout/stderr/logs and evaluation.                                                |
| Registry-aligned help/schemas/skill                         | Generated documentation drift gate, public runtime schema validation, JSON/TOON equivalence and portable skill validation.                                                                    |
| Valid bounded output under error/interruption               | Capture/deadline/child/output limits, renderer-only reductions, cancellation/group cleanup and useful partial observations.                                                                   |
| Actual compatibility matrix                                 | Native checksums and immutable source; 39 native/mock and 118 restricted live captures. Other artifacts are explicitly unexecuted.                                                            |
| Realistic agent comparison with failures reported           | Scripted 96-observation benchmark plus isolated actual model sessions; independent answer and tool-trace grading. All 16 sessions passed independent grading; no correctness/security errors. |

Full product verification at the effective-limit checkpoint passed 133
deterministic checks, 56 native contracts, 18 restricted live tests and 18
production-only installed-package smoke checks, with no skips. Final v0.1.0 checks also passed 133/56/18; the installed package
passes 21 smoke checks, including the MIT license, changelog and inventory. Exact-head GitHub
CI passed at `8c0e727`. Final packaging/documentation changes received separate verification; no behavior result is inferred from a package listing.

Package inspection excludes fixtures, setup/evaluation tools, local credentials
and development dependencies. The compiled package includes its executable,
schemas, skill, reference, changelog, MIT license and dependency inventory. The
inventory matches all 106 committed lockfile package entries. No lifecycle hook
downloads the official CLI. It must be installed separately.

Structured logs remain bounded retained tails. Lookup-cap continuation never
increases scan authority. Scoped exact run-list zero is supported only by the
verified pagination contract; unknown builds retain uncertainty. Restricted pool
reads remain unavailable on the fixture, and missing activity never proves an
agent is idle. Wider live fixture coverage and new platforms require additional
execution evidence. No token or latency saving claim is made: the optimized
native scripted baseline is smaller and faster.

Independent review found no additional release-blocking product defect under
this scope. It also reviewed evaluation isolation, baseline operation guidance,
process-count trace integrity and setup/protocol cleanup. The shell process
logger is auditable instrumentation, not a tamper-proof security boundary; rows
with bypass or metric tampering are invalid. Failed setup and stalled initialize
cleanup have explicit execution evidence. LSP cannot diagnose the .mjs evaluator;
Node syntax checks and execution provide its fallback.

v0.1.0 implementation and package acceptance are complete. The compiled GitHub
release asset provides distribution; npm registry publication is not claimed.
