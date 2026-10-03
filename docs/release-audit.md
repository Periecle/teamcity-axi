# Go read-only implementation audit

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
| Functional/live compatibility | 56 checksum-pinned native scenarios and 18 restricted live tests pass. Native/mock contracts do not imply broader live support. Both Go recorders produced new guarded39/118-record captures without altering frozen fixtures. |
| Standalone distribution | Product Go executable, MIT/third-party licenses, README/changelog/reference/inventory; embedded schemas/skill; archive excludes fixtures, development tooling and credentials. 56 offline command executions pass. |
| Scripted evaluation | Fresh 96 observations, both wrapper conditions retain24/24 tasks each, zero exposures or identity/completeness mistakes. Native baseline regressions and all rows retained. |
| Model comparison release gate | Go runner and protocol/isolation/cleanup tests implemented and pass. Actual16 credential-backed sessions remain unexecuted pending explicit user approval; historical TypeScript model results are separate. |

Verification passed 162 deterministic top-level tests, 56 additional native
contract scenarios and 18 live tests, with no skips. The native tagged command
passes218 top-level tests because it also runs deterministic coverage. Go race,
vet, gofmt, generated docs, source selection and package checks pass. The
[verification record](go-verification.json) binds exact test lists, commands and
artifact hashes. These are local execution results; no remote CI claim is made.

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

The Go implementation and automated parity checks are complete. Full release
certification still requires the new model comparison and independent answer
and tool-trace grading. Automatic approval review rejected the credential-backed
launch because explicit credential/account-use authorization was absent; no
inference sessions were started. No GitHub release is published by this work.
