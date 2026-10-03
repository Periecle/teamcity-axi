# Go read-only implementation audit

This document records the immutable published v0.1.0 release checkpoint.
[Post-release performance work](performance.md) has separate source, test and
evaluation evidence; it does not replace this release's reports or public assets.

The Go reimplementation covers the accepted eighteen-service read-only scope,
including its test, capture, documentation, evaluation and package tooling.
Comparison, setup/hooks and mutations retain separate deferred gates. Executed
compatibility is Linux amd64, Go 1.27.1, official TeamCity CLI 1.5.0, and
restricted TeamCity 2026.2 build 238924 project-view reads. No other platform or
server version is certified. Darwin arm64/amd64 and Linux arm64 cross-compilation are build checks,
not execution evidence.

| Gate | Current Go evidence |
| --- | --- |
| Command/flag/output contracts | All eighteen registry descriptors, generated help/examples, embedded offline schemas, strict parser and command tests. |
| Original behavioral coverage | Every original 133 deterministic, 56 native and 18 live scenario maps to Go in go-test-parity.json; no removed behavioral gate. |
| Exact identity and unknown semantics | DTO, context, policy, paging, status/watch and graph tests; all 32 mandatory scenarios mapped in acceptance.md. |
| Restricted native operations | Literal GET argv, projected fields, raw protocol/status validation, neutral CWD and narrow inherited environment. |
| Independent evidence sources | Problems/tests/logs/changes/dependencies acquired separately; swallowed summary errors and source denials remain explicit. |
| Scoped authority and credentials | Trusted server selection, origin-bound tokens, project ancestry, actual reader permission inventory and foreign-project denial; canaries sanitized before previews/fingerprints. |
| Bounded output and cleanup | Capture/launch/concurrency/deadline/output caps, reserved capacity, group cancellation including resistant descendants, valid bounded errors and retained watch observations. |
| Functional/live compatibility | 56 checksum-pinned native scenarios and 18 restricted live tests pass. Native/mock contracts do not imply broader live support. Both Go recorders produced new guarded 39/118-record captures without altering frozen fixtures. |
| Standalone distribution | Product Go executable, MIT/third-party licenses, README/changelog/reference/inventory; embedded schemas/skill; archive excludes fixtures, development tooling and credentials. 56 offline command executions pass. |
| Scripted evaluation | 96 observations, both wrapper conditions retain 24/24 tasks each, zero exposures or identity/completeness mistakes. Native baseline regressions and all rows retained. |
| Model comparison release gate | 16 actual sessions independently graded: both conditions pass 8/8 tasks with zero correctness/security errors. Median calls tie at three; totals 30 versus 22 and secret task nine versus three remain explicit. Shared/cycle two-versus-four benefits are scoped to those tasks. Lower-median target is unmet. |

Release verification passed 163 deterministic top-level tests, 56 additional native
contract scenarios and 18 live tests, with no skips. The native tagged command
passes 219 top-level tests because it also runs deterministic coverage. Go race,
vet, gofmt, generated docs, source selection and package checks pass. The
[release verification record](release-verification.json) binds exact test lists, commands and
artifact hashes. Remote [Go CI](https://github.com/Periecle/teamcity-axi/actions/runs/37117635743)
also passes on Go 1.26.0 at `890bdb5`: deterministic/vet/format/docs checks,
race checks, offline archive execution and checksum-pinned native contracts.
Restricted live evidence remains the separately recorded local execution.

Structured logs remain retained tails; no complete-log coverage is claimed.
Exact first-page zero requires the recorded server's verified pagination
contract. Unknown builds, unsupported pool availability, missing activity,
source denials and other roots retain uncertainty. Wider live fixture coverage
and platforms need their own execution evidence. No token saving or general
performance claim follows from the synthetic evaluation.

Independent reviews audited source and original behavioral assertions, including
security/evidence boundaries and evaluation process isolation. Capture writers
now prevent ReaderFrom bypasses; empty explicit bindings and integral native
JSON numbers have regression coverage. LSP was unavailable because gopls was
missing; Go compilation, tests, race detection and vet are the fallback.

The Go implementation, parity checks and independent model grading are complete
for the recorded scope. The model trace review reconciles 52 actual tool calls,
221 native launches and 208 authenticated GETs, without unsupported operations
or instrumentation bypass. The lower-median performance target did not pass;
publication does not certify that target or general efficiency. The narrow
shared/cycle fewer-call observations retain task-critical evidence, while every
regressing task and aggregate cost remains visible in [the comparison](evaluation.md).

The first model attempt was interrupted after review found an aggregate protocol
capture gap. The runner now enforces a 64 MiB total stream ceiling with an
overflow/cleanup regression. That attempt is retained separately from the
restarted 16-session comparison. The released executable is the exact
model-evaluated binary; product source is unchanged from its recorded checkpoint.
The first remote CI run exposed unstable error classification when a capture
failure left a partial final JSON line. The later evaluator-only correction
checks the stream error before decoding that fragment; a deterministic fragment
case and the original flood/cleanup case pass. Independent review confirms that
the completed model report and its original harness hash remain valid.
The final archive contains the documents completed before publication.
Go [v0.1.0 is published](https://github.com/Periecle/teamcity-axi/releases/tag/v0.1.0)
at `3cfc504`, with successful [final-commit CI](https://github.com/Periecle/teamcity-axi/actions/runs/37117779999).
The unsigned tag and fresh downloaded assets match the recorded hashes, and the
downloaded archive passes all 56 offline command executions. See
[the publication verification record](release-publication.json).
