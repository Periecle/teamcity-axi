# Implementation status

The full objective remains active. This is a development foundation, not a v0.1
release or a live-certified TeamCity integration.

| Milestone | Current evidence | Remaining gate |
|---|---|---|
| 0: upstream contract | v1.5.0 pinned; four Unix checksums; 19 Linux x64 binary/mock fixtures and 26 actual TeamCity 2026.2 restricted-identity captures; summary omission confirmed | Positive VCS/branch/dependency-direction/queue cases, live context prefix, actual expired token, other platforms executed |
| 1: executable boundary | Strict registry/parser; local help/version/schema; JSON/TOON equivalence; run-view payload schema; redaction and bounded-output error path | All remaining command-specific payload schemas and generated docs |
| 2: context/transport | Trusted configuration, native TOML path scope, worktree Git, origin binding, neutral CWD, restricted environment, shared semaphore/deadline/capture limits, process-group cleanup; run-view signal handlers | Integration into remaining command services, full budget reporting and remaining adversarial cases |
| 3: vertical read | Strict raw-HTTP/run/job/page adapters; exact-ID run view and bounded run list; current policy/scope rechecks, reconstructed continuation and fixed-window query-bound cursors; actual wrapper/native/live tests | Remaining list outcome/time precision contracts, positive revision/branch cases, verified context/doctor, log capability probe; full certification |
| 4–6: read product | Registry expresses required command grammar; unimplemented remote handlers explicitly return unsupported | Primitive commands, graph/investigation, exact checkout status, watch and generated portable skill |
| 7: evaluation/release | No release claims | Sanitized evaluation corpus/native baseline measurements, CI/platform/package gates and actual support matrix |

Deferred comparisons/setup and separately approved writes remain outside the
initial read-only scope.

Focused verification uses Node 24.14.0. The host Node 26 is not claimed as a
supported runtime. Tests that spawn Node/Git or bind local HTTP require normal
process/network permissions; the current sandbox returns `EPERM` with empty
captured child output. Such failures are rerun outside the sandbox, never skipped.

Current focused evidence: TypeScript build and 43 deterministic unit/executable
tests pass on Node 24.14.0 Linux x64; seven real-CLI/mock-server tests pass,
including the 19 native wire observations and end-to-end run view. No skipped
tests. Independent review identified and drove fixes for credential previews,
control-sequence reconstruction, missing revision metadata, safe timestamps,
assertion-preserving hints, and Unicode byte budgets. Foundation GitHub CI passed
both jobs on `0227c12` (run
https://github.com/Periecle/teamcity-axi/actions/runs/36993119503).

Run list now consumes the paging helpers. It performs a bounded exact-job
preflight, validates every row against declared scope and current trusted policy,
and preserves useful rows if continuation is unsafe. Totals remain unknown;
missing continuation under a bounded lookup is partial, never an exact zero.
Default finish-time windows are fixed in cursors and emitted in the response.
Revision filtering emits only exact selected-root tuples from the fetched page;
missing metadata remains partial. Positive live VCS/branch matches, fractional
time boundaries and additional explicit outcome metadata remain open.

Next: deliver verified context/doctor and finish remaining list contracts from recorded
contracts. The user has no existing TeamCity sandbox. A temporary official
2026.2 server (build 238924) is running on localhost with dedicated test volumes
and a resource-bounded official build agent. The user approved its local test
licence. The test reader has project-view permission on `AxiContract`, with no
build-run permission or inherited All Users role. Its native GET for the foreign
project returns 403. Three live-server tests pass: bounded independent native
reads, permission inventory, and actual wrapper JSON/TOON exact-ID run view with
denied/missing/mismatch/invalid-authentication errors, plus actual list and
cursor continuation, project scope, branch projection and precision rejection.
No skipped tests.

Pinned Prettier 3.9.9 formats source, tests, scripts, schemas and configuration.
The npm pretest formatting gate runs in existing CI. Independent review drove
fixes for all-branch projection, original sub-millisecond precision, unknown
outcome diagnostics and privacy/provenance in the live recorder. Explicitly
forwarded header values are redacted before previews and final rendering.
LSP has stale imported declarations; current TypeScript compilation is the
verification fallback. The LSP bridge does not support `.mjs` files.

Live evidence corrected the dependency contract: the build subresource rejects
JSON with 406; the released CLI's `snapshotDependency:(to:(id:...),recursive:false)`
build locator returns a bounded JSON collection. Empty dependencies do not yet
prove positive graph direction. The native `--tail 20` returns 21 log entries;
the future public adapter must enforce its own bound. Agent metadata reports
default pool ID 0, but a pool-0 locator returns 404; scoped pool reads remain
unverified. Empty changes/queue responses are successful endpoint observations,
not positive VCS/queued-build evidence. Full server certification remains open.
