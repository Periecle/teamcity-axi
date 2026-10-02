# Implementation status

The full objective remains active. This is a development foundation, not a v0.1
release or a live-certified TeamCity integration.

| Milestone | Current evidence | Remaining gate |
|---|---|---|
| 0: upstream contract | v1.5.0 pinned; four Unix checksums; 27 Linux x64 binary/mock fixtures and 45 actual TeamCity 2026.2 restricted-identity captures; summary omission confirmed | Multi-root/hostile-branch/shared-DAG/positive-queue cases, live muted/duplicate tests, live context prefix, actual expired token, other platforms executed |
| 1: executable boundary | Strict registry/parser; local help/version/schema; JSON/TOON equivalence; run-view/list/problems/tests/log and diagnostic payload schemas; redaction and bounded-output error path | Remaining command-specific payload schemas and generated docs |
| 2: context/transport | Trusted configuration, native TOML path scope, worktree Git, origin binding, neutral CWD, restricted environment, shared semaphore/deadline/capture limits, process-group cleanup; read-service signal handlers | Integration into remaining command services, full budget reporting and remaining adversarial cases |
| 3: vertical read | Strict raw-HTTP/run/job/page adapters; exact-ID run view and bounded run list; current policy/scope rechecks, reconstructed continuation and fixed-window query-bound cursors; actual wrapper/native/live tests | Remaining list outcome/time precision contracts, positive revision/branch cases, full command-specific capability probes; full certification |
| 4–6: read product | Independent problem/test/change pages and expansion; bounded structured logs; immediate dependency adapter and exact counts; registry expresses remaining grammar | Graph/investigation integration, job/queue/agents, exact checkout status, watch and generated portable skill |
| 7: evaluation/release | No release claims | Sanitized evaluation corpus/native baseline measurements, CI/platform/package gates and actual support matrix |

Deferred comparisons/setup and separately approved writes remain outside the
initial read-only scope.

Focused verification uses Node 24.14.0. The host Node 26 is not claimed as a
supported runtime. Tests that spawn Node/Git or bind local HTTP require normal
process/network permissions; the current sandbox returns `EPERM` with empty
captured child output. Such failures are rerun outside the sandbox, never skipped.

Current focused evidence: TypeScript build and 55 deterministic unit/executable
tests pass on Node 24.14.0 Linux x64; 14 real-CLI/mock-server tests pass,
including the 27 native wire observations and end-to-end evidence reads. No skipped
tests. Independent review identified and drove fixes for credential previews,
control-sequence reconstruction, missing revision metadata, safe timestamps,
assertion-preserving hints, and Unicode byte budgets. Foundation GitHub CI passed
both jobs on the preceding `f70ef36` checkpoint (run
https://github.com/Periecle/teamcity-axi/actions/runs/37001837205).

Run list now consumes the paging helpers. It performs a bounded exact-job
preflight, validates every row against declared scope and current trusted policy,
and preserves useful rows if continuation is unsafe. Totals remain unknown;
missing continuation under a bounded lookup is partial, never an exact zero.
Default finish-time windows are fixed in cursors and emitted in the response.
Revision filtering emits only exact selected-root tuples from the fetched page;
missing metadata remains partial. Positive live VCS/branch matches, fractional
time boundaries and additional explicit outcome metadata remain open.

Next: integrate bounded graph traversal and investigation, and finish remaining list contracts
from recorded contracts. The user has no existing TeamCity sandbox. A temporary official
2026.2 server (build 238924) is running on localhost with dedicated test volumes
and a resource-bounded official build agent. The user approved its local test
licence. The test reader has project-view permission on `AxiContract`, with no
build-run permission or inherited All Users role. Its native GET for the foreign
project returns 403. Seven live-server tests pass: bounded independent native
reads, permission inventory, and actual wrapper JSON/TOON exact-ID run view with
denied/missing/mismatch/invalid-authentication errors, plus actual list and
cursor continuation, project scope, branch projection and precision rejection,
verified context/doctor, exact problem/test occurrence expansion, bounded logs and positive changes.
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
review caught and corrected global no-hints/require-complete handling. The full
graph/investigation, multi-root and hostile branch gates remain open.
