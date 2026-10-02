# Implementation status

The full objective remains active. This is a development foundation, not a v0.1
release or a live-certified TeamCity integration.

| Milestone              | Current evidence                                                                                                                                                                                                           | Remaining gate                                                                                                                                             |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 0: upstream contract   | v1.5.0 pinned; four Unix checksums; 27 Linux x64 binary/mock fixtures and 45 actual TeamCity 2026.2 restricted-identity captures; summary omission confirmed                                                               | Multi-root/hostile-branch/shared-DAG/positive-queue cases, live muted/duplicate tests, live context prefix, actual expired token, other platforms executed |
| 1: executable boundary | Strict registry/parser; local help/version/schema; JSON/TOON equivalence; run-view/list/problems/tests/log and diagnostic payload schemas; redaction and bounded-output error path                                         | Remaining command-specific payload schemas and generated docs                                                                                              |
| 2: context/transport   | Trusted configuration, native TOML path scope, worktree Git, origin binding, neutral CWD, restricted environment, shared semaphore/deadline/capture limits, process-group cleanup; read-service signal handlers            | Integration into remaining command services, full budget reporting and remaining adversarial cases                                                         |
| 3: vertical read       | Strict raw-HTTP/run/job/page adapters; exact-ID run view and bounded run list; current policy/scope rechecks, reconstructed continuation and fixed-window query-bound cursors; actual wrapper/native/live tests            | Remaining list outcome/time precision contracts, positive revision/branch cases, full command-specific capability probes; full certification               |
| 4–6: read product      | Independent problem/test/change pages and expansion; bounded structured logs; bounded concrete run graphs and source-accounted failure investigation with reserved final-state reads; registry expresses remaining grammar | Failure exhaustion/reference/outcome certification, job/queue/agents, exact checkout status, watch and generated portable skill                            |
| 7: evaluation/release  | No release claims                                                                                                                                                                                                          | Sanitized evaluation corpus/native baseline measurements, CI/platform/package gates and actual support matrix                                              |

Deferred comparisons/setup and separately approved writes remain outside the
initial read-only scope.

Focused verification uses Node 24.14.0. The host Node 26 is not claimed as a
supported runtime. Tests that spawn Node/Git or bind local HTTP require normal
process/network permissions; the current sandbox returns `EPERM` with empty
captured child output. Such failures are rerun outside the sandbox, never skipped.

Current focused evidence: TypeScript build and 81 deterministic unit/executable
tests pass on Node 24.14.0 Linux x64; 21 real-CLI/mock-server tests pass,
including the 27 native wire observations and end-to-end evidence reads. No skipped
tests. Independent review identified and drove fixes for credential previews,
control-sequence reconstruction, missing revision metadata, safe timestamps,
assertion-preserving hints, and Unicode byte budgets. Foundation GitHub CI passed
both jobs on the preceding `2e2bbcf` checkpoint (run
https://github.com/Periecle/teamcity-axi/actions/runs/37007881350).

Run list now consumes the paging helpers. It performs a bounded exact-job
preflight, validates every row against declared scope and current trusted policy,
and preserves useful rows if continuation is unsafe. Totals remain unknown;
missing continuation under a bounded lookup is partial, never an exact zero.
Default finish-time windows are fixed in cursors and emitted in the response.
Revision filtering emits only exact selected-root tuples from the fetched page;
missing metadata remains partial. Positive live VCS/branch matches, fractional
time boundaries and additional explicit outcome metadata remain open.

Next: finish failure/list contract gates and implement the remaining read services
from recorded contracts. The user has no existing TeamCity sandbox. A temporary official
2026.2 server (build 238924) is running on localhost with dedicated test volumes
and a resource-bounded official build agent. The user approved its local test
licence. The test reader has project-view permission on `AxiContract`, with no
build-run permission or inherited All Users role. Its native GET for the foreign
project returns 403. Nine live-server tests pass: bounded independent native
reads, permission inventory, and actual wrapper JSON/TOON exact-ID run view with
denied/missing/mismatch/invalid-authentication errors, plus actual list and
cursor continuation, project scope, branch projection and precision rejection,
verified context/doctor, exact problem/test occurrence expansion, bounded logs, positive changes, concrete bounded run-tree expansion and source-accounted failure reports.
No skipped tests.

Pinned Prettier 3.9.9 formats source, tests, scripts, schemas and configuration.
Pinned ESLint Stylistic 5.10.0 automatically adds and enforces TypeScript blank
lines between import groups, definitions, methods, control blocks and returns.
The npm pretest formatting gate runs in existing CI. Independent review drove
fixes for all-branch projection, original sub-millisecond precision, unknown
outcome diagnostics and privacy/provenance in the live recorder. Explicitly
forwarded header values are redacted before previews and final rendering.
LSP requests either timed out or reported cached declarations inconsistent with
the current source; current TypeScript compilation supplies semantic verification. The bridge does not support `.mjs` files.

Live evidence corrected the dependency contract: the build subresource rejects
JSON with 406; the released CLI's `snapshotDependency:(to:(id:...),recursive:false)`
build locator returns a bounded JSON collection. Positive restricted reads now prove immediate root-to-prerequisite direction;
scoped counts distinguish one dependency and a zero-dependency child. The native `--tail 20` returns 21 log entries;
the public log command enforces its own bound and declares retained message IDs. Agent metadata reports
default pool ID 0, but a pool-0 locator returns 404; scoped pool reads remain
unverified. Positive change pages now verify three commits under a selected VCS root and optional
file names. Queue responses remain empty and do not prove positive queued-build behavior. Full server certification remains open.

Verified context uses exact selected job/project reads and a current-user
fingerprint; it never enumerates all projects. Local context remains no-network.
Offline doctor invokes only the local version command. Online doctor verifies
selected scope, server metadata, a one-row run page, exact sample detail and the
optional structured log capability. Required capabilities whose scoped adapters
are pending remain `not_probed` and the result stays partial. Live GET captures
and wrapper tests verify these behaviors under the restricted reader. Trusted
project roots admit descendants only through cached observed ancestry, with an
eight-link bound and fail-closed cycle/unknown handling.

Independent review drove fixes for credential-free offline probes, preservation
of acquired diagnostics after optional log budget failures, the eight-child
simple-read ceiling, the eighth ancestry link, and long-branch schema alignment.

Independent problem/test reads preserve compound occurrence IDs bound to the
selected run, string test definition IDs above JavaScript's safe-integer range,
muted/ignored/unknown categories, query-bound cursors and exact-item expansion.
Malformed continuation retains valid rows with partial coverage. Default text
previews and full-text ceilings apply after redaction; byte-budget overflow is
explicit. Positive live muted/duplicate cases remain open, with mock coverage.

Log failure evidence uses three independent bounded reads rather than native
combined summaries, preserving useful siblings when one source is unavailable.
Literal matching precedes text previews and declares its retained tail window.
Messages require increasing IDs, timestamps normalize to UTC, and missing or
invalid optional timestamps remain explicit. Independent review drove fixes for
continuation types, misleading overdelivery expansion hints, message ordering,
and lone Unicode surrogates; JSON/TOON equivalence and regressions pass.

Bounded changes now preserve commit/root identity, first-line previews, explicit full
page expansion, optional capped file names and file omission counts. Query-bound
cursors preserve selected run, current policy and file-fetch semantics. Positive
live fixture builds verify change paging, message expansion and file data; direct
counts and immediate-dependency pages provide the next graph slice with recorded
contracts. Missing change roots remain explicit partial evidence. Independent
review caught and corrected global no-hints/require-complete handling. Full failure-investigation certification, live shared-DAG/cycle cases, multi-root and hostile branch gates remain open.

Wrapper-owned breadth-first `run tree` now retains unique execution nodes and all
retained directed edges, detects cycles separately from shared prerequisites,
and declares every expansion boundary. Independent scoped counts reconcile
bounded pages; a zero count establishes a known leaf. Non-terminal roots reserve
one final observation through an atomic transport launch ceiling, including
ancestry and overlapping reads. Failed or changed final observations invalidate
root expansion; still-running graphs are provisional. Shared limit diagnostics
are grouped while affected nodes retain their explicit states.

Nine planner cases, an executable reservation regression and four actual
released-CLI mock tests verify shared DAGs, cycles, denied/foreign/conflicting
observations, safe empty continuation, limits, metadata/state changes and
JSON/TOON equivalence. Independent review corrected child diagnostic attribution
and excluded rejected child metadata, including repeated retained IDs. The live
restricted reader proves root 9 to dependency 8, scoped counts one and zero,
known-leaf completion and bounded partial expansion. Live shared-DAG/cycle and
non-terminal state-change cases remain open; focused tree evidence does not
certify the full failure-investigation product.

Source-accounted `run failure` now short-circuits successful finished roots,
collects independent problems and unmuted tests, requests declared log windows
when direct evidence is absent/generic, and keeps independently fetched muted
failures separate. Diagnosis is bounded and deterministic; failed dependencies
are observations rather than causal conclusions. Primary evidence and final
reads reserve capacity that child ancestry cannot consume. Optional changes run
last and are reduced before required evidence under actual JSON/TOON byte
measurement, with source counts and omitted rows reconciled.

Fifteen focused planner cases plus a parser literal-value regression, three
released-CLI failure cases and one live case verify failed-root occurrences,
source denial, budgets, previews, redacted fingerprints, quote/backslash
credentials, Unicode retrieval and success short circuit. Independent review
drove these corrections. Source scopes distinguish execution, bounded page,
immediate dependencies and retained tail windows. The current live problem/test
adapters preserve unknown exhaustion, so failed-root investigations remain
partial. Explicit dependency-problem references are unavailable in the verified
DTO and are declared as such; the service does not infer links from prose.
Broader live outcome/topology fixtures and complete-source proof remain gates,
alongside job/queue/agent/status/watch delivery and release evaluation.
