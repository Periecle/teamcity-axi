# Implementation status

The full objective remains active. This is a development foundation, not a v0.1
release or a live-certified TeamCity integration.

| Milestone | Current evidence | Remaining gate |
|---|---|---|
| 0: upstream contract | v1.5.0 release/source pinned; archive and executable SHA-256 for four Unix targets; 19 released Linux x64 binary/mock-server fixtures; summary omission confirmed; ADRs and compatibility manifest | Controlled real TeamCity version/build, restricted identity, live endpoint/permission/locator/pagination evidence; other platforms executed |
| 1: executable boundary | Strict registry/parser; local help/version/schema; JSON/TOON equivalence; run-view payload schema; redaction and bounded-output error path | All remaining command-specific payload schemas and generated docs |
| 2: context/transport | Trusted configuration, native TOML path scope, worktree Git, origin binding, neutral CWD, restricted environment, shared semaphore/deadline/capture limits, process-group cleanup; run-view signal handlers | Integration into remaining command services, full budget reporting and remaining adversarial cases |
| 3: vertical read | Strict raw-HTTP and run DTO adapters; exact-ID run view; asserted job/project and trusted policy; actual wrapper/native tests for wrong identity, denied/malformed reads, previews and cancellation | Run list/pages/cursors/locators, verified context/doctor, log capability probe; real-server certification |
| 4–6: read product | Registry expresses required command grammar; unimplemented remote handlers explicitly return unsupported | Primitive commands, graph/investigation, exact checkout status, watch and generated portable skill |
| 7: evaluation/release | No release claims | Sanitized evaluation corpus/native baseline measurements, CI/platform/package gates and actual support matrix |

Deferred comparisons/setup and separately approved writes remain outside the
initial read-only scope.

Focused verification uses Node 24.14.0. The host Node 26 is not claimed as a
supported runtime. Tests that spawn Node/Git or bind local HTTP require normal
process/network permissions; the current sandbox returns `EPERM` with empty
captured child output. Such failures are rerun outside the sandbox, never skipped.

Current focused evidence: TypeScript build and 28 deterministic unit/executable
tests pass on Node 24.14.0 Linux x64; five real-CLI/mock-server tests pass,
including the 19 native wire observations and end-to-end run view. No skipped
tests. Independent review identified and drove fixes for credential previews,
control-sequence reconstruction, missing revision metadata, safe timestamps,
assertion-preserving hints, and Unicode byte budgets. Foundation GitHub CI passed
both jobs on `b92ac0a`; this slice must pass CI after its push.

Next: deliver bounded run list and verified context/doctor from recorded
contracts. The user has no existing TeamCity sandbox. A temporary official
2026.2 server image is being prepared on localhost to establish controlled live
evidence; it is not yet configured or certified. Keep live support unverified
until the server version/build, restricted identity and endpoint evidence exist.
