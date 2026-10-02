# Implementation status

The full objective remains active. This is a development foundation, not a v0.1
release or a live-certified TeamCity integration.

| Milestone              | Current evidence                                                                                                                                                                                                                                                                   | Remaining gate                                                                                                                                              |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 0: upstream contract   | v1.5.0 pinned; four Unix checksums; 33 Linux x64 binary/mock fixtures and 69 actual TeamCity 2026.2 restricted-identity captures; summary omission confirmed                                                                                                                       | Multi-root/hostile-branch/shared-DAG/queue lifecycle cases, live muted/duplicate tests, live context prefix, actual expired token, other platforms executed |
| 1: executable boundary | Strict registry/parser; local help/version/schema; JSON/TOON equivalence; run-view/list/problems/tests/log and diagnostic payload schemas; redaction and bounded-output error path                                                                                                 | Remaining command-specific payload schemas and generated docs                                                                                               |
| 2: context/transport   | Trusted configuration, native TOML path scope, worktree Git, origin binding, neutral CWD, restricted environment, shared semaphore/deadline/capture limits, process-group cleanup; read-service signal handlers                                                                    | Integration into remaining command services, full budget reporting and remaining adversarial cases                                                          |
| 3: vertical read       | Strict raw-HTTP/run/job/page adapters; exact-ID run view and bounded run list; current policy/scope rechecks, reconstructed continuation and fixed-window query-bound cursors; actual wrapper/native/live tests                                                                    | Remaining list outcome/time precision contracts, positive revision/branch cases, full command-specific capability probes; full certification                |
| 4–6: read product      | Independent problem/test/change pages and expansion; bounded structured logs; bounded concrete run graphs and source-accounted failure investigation with reserved final-state reads; safe exact job metadata, bounded direct-project job pages, scoped queue and safe agent reads | Failure/job/queue/agent certification, exact checkout status, watch and generated portable skill                                                            |
| 7: evaluation/release  | No release claims                                                                                                                                                                                                                                                                  | Sanitized evaluation corpus/native baseline measurements, CI/platform/package gates and actual support matrix                                               |

Deferred comparisons/setup and separately approved writes remain outside the
initial read-only scope.

Focused verification uses Node 24.14.0. The host Node 26 is not claimed as a
supported runtime. Tests that spawn Node/Git or bind local HTTP require normal
process/network permissions; the current sandbox returns `EPERM` with empty
captured child output. Such failures are rerun outside the sandbox, never skipped.

Current focused evidence: TypeScript build and 99 deterministic unit/executable
tests pass on Node 24.14.0 Linux x64; 34 real-CLI/mock-server tests pass,
including the 33 native wire observations and end-to-end evidence reads. No skipped
tests. Independent review identified and drove fixes for credential previews,
control-sequence reconstruction, missing revision metadata, safe timestamps,
assertion-preserving hints, and Unicode byte budgets. Foundation GitHub CI passed
both jobs on the preceding `e9dcc50` checkpoint (run
https://github.com/Periecle/teamcity-axi/actions/runs/37017119487).

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
project returns 403. Twelve live-server tests pass: bounded independent native
reads, permission inventory, and actual wrapper JSON/TOON exact-ID run view with
denied/missing/mismatch/invalid-authentication errors, plus actual list and
cursor continuation, project scope, branch projection and precision rejection,
verified context/doctor, exact problem/test occurrence expansion, bounded logs, positive changes, concrete bounded run-tree expansion source-accounted failure reports, scoped safe job reads, positive queue pages and safe scoped agent reads.
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
the public log command enforces its own bound and declares retained message IDs. Historical agent metadata reports
default pool ID 0; the controlled fixture now uses dedicated pool ID 1. Restricted
pool locators return 404; positive pool scope remains unverified. Positive change pages now verify three commits under a selected VCS root and optional
file names. Positive scoped queue responses now prove queued execution IDs and provider wait reasons; broader lifecycle transitions remain unverified. Full server certification remains open.

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
alongside status/watch delivery and release evaluation.

Scoped `job list` and exact `job view` now expose safe metadata with nullable
paused state, current policy admission and typed retrieval actions. The list
uses direct project membership and query-bound offset cursors; missing bounded
continuation retains unknown exhaustion and total. Invalid local project input
and mismatched cursors fail before any child launch. Four job adapter cases,
a live-recorder identity regression, four released-native command cases and one
restricted live case verify safe fields, foreign scope, permission/capability
failures, redaction, empty continuation, output bounds, hundred-row diagnostic
bounds and dash-leading retrieval IDs. That job checkpoint replayed all 49 actual
restricted read observations. Independent review drove local input classification
and confirmed the diagnostic/hint fixes. Full job policy/topology and paused-state
live certification remain open; status/watch and release evaluation
remain required by the full objective.

Scoped `queue list` now derives and admits exact job/project scope, reads one
bounded page and retains observed execution IDs, lifecycle, optional branch/time
and provider wait reason. Cursors bind the resolved scope, server, page size and
current policy. Totals and bounded exhaustion remain unknown, including empty
pages; unsafe continuation preserves useful rows. Queue movement is best effort
and changed/unknown lifecycle is explicit. Hints either continue the query or
observe the exact execution with job/project assertions.

Five queue adapter cases, one recorded queued-result case, one recorder identity
regression, four released-CLI queue cases and one restricted live queue case
verify positive pages, cursor/hint execution, nullable reason, denial/capability
errors, malformed identity, sanitation and input/output/diagnostic bounds. The
recorder replayed all 56 restricted reads. Independent review identified and
drove rejection of ill-formed Unicode, C1 and bidi identity controls so sanitation
cannot change accepted identities. Actual queued-run detail omits result status;
the run adapter preserves unknown result and an explicit limitation, while
finished/running omissions and null status remain invalid. All 133 tests pass
(93 deterministic/executable, 29 released-CLI/mock, 11 live), with no skips.
Formatting gates and current TypeScript compilation pass; LSP timed out with
stale diagnostics. Remaining status/watch services, broader certification,
portable skill/help generation and evaluation/release gates remain required.

Safe `agent list`/`agent view` now expose separate nullable connectivity,
enablement and authorization, safe pool metadata and admitted exact active-run
pointers. Lists require an explicit pool or validated job/project scope. Exact
detail retains supplied assertions and rejects a substituted agent. Compatibility
filters and best-effort cursor bindings preserve resolved scope and current
policy. Missing active-build data stays not_reported; it does not prove idleness.
Totals and bounded exhaustion remain unknown, including empty compatible pages.

Five agent adapter cases, one recorder guard case, five native/mock command cases
and one restricted live case verify safe metadata, exact scope, retrieval hints,
separate false/unknown states, permission/capability/schema errors, pool ID zero,
continuation, sanitation and byte/diagnostic bounds. Independent review drove
interruption propagation during optional active-pointer admission and a single
clock value for cursor expiry/encoding. Executable regressions prove exit 130 and
retention of rows when a cursor expires during acquisition. LSP diagnostics for
the agent adapter, command and current reader were clean; current compilation and
formatting gates pass. Positive live pool access, active pointers, explicit idle
and mixed availability remain gates; the restricted reader was not broadened.
The recorder replayed all 69 actual reads. Current full verification passes 145
tests (99 deterministic/executable, 34 native/mock, 12 live), with no skips.
Exact checkout status, watch, generated portable skill/help, broader certification
and evaluation/release remain required by the full objective.
