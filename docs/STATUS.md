# Implementation status

The full objective remains active. This is a development foundation, not a v0.1
release or a live-certified TeamCity integration.

| Milestone | Current evidence | Remaining gate |
|---|---|---|
| 0: upstream contract | v1.5.0 release/source pinned; archive and executable SHA-256 for four Unix targets; 19 released Linux x64 binary/mock-server fixtures; summary omission confirmed; ADRs and compatibility manifest | Controlled real TeamCity version/build, restricted identity, live endpoint/permission/locator/pagination evidence; other platforms executed |
| 1: executable boundary | Strict registry/parser; local help/version/schema; JSON/TOON equivalence and seed schema validation; redaction and bounded-output error path | Typed remote vertical slice, all command-specific payload schemas and generated docs |
| 2: context/transport | Trusted configuration, native TOML path scope, worktree Git, origin binding, neutral CWD, restricted environment, shared semaphore/deadline/capture limits, process-group cleanup | Integration into all command services, invocation signal handlers, full budget reporting and remaining adversarial cases |
| 3–6: read product | Registry expresses required command grammar; remote handlers explicitly return unsupported instead of fabricated data | DTO adapters, pages/cursors, primitive commands, graph/investigation, exact checkout status, watch and generated portable skill |
| 7: evaluation/release | No release claims | Sanitized evaluation corpus/native baseline measurements, CI/platform/package gates and actual support matrix |

Deferred comparisons/setup and separately approved writes remain outside the
initial read-only scope.

Focused verification uses Node 24.14.0. The host Node 26 is not claimed as a
supported runtime. Tests that spawn Node/Git or bind local HTTP require normal
process/network permissions; the current sandbox returns `EPERM` with empty
captured child output. Such failures are rerun outside the sandbox, never skipped.

Current focused evidence: TypeScript build and 23 deterministic unit/executable
tests pass on Node 24.14.0 Linux x64; one real-CLI/mock-server contract suite
passes, covering 19 native observations. No skipped tests. Independent review
identified and drove fixes for credential/URL/configuration/output edge cases.

Next: complete context/transport review, build strict raw-response and DTO adapters
from the recorded fixture shapes, and deliver the exact-ID `run view` vertical
slice. Keep live support unverified until controlled server evidence exists.
