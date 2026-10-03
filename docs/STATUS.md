# Go implementation status

All eighteen accepted read-only services and the recorder, documentation,
evaluation and package tooling are implemented in Go. The worktree contains no
JS/TS source files, Node modules or Node configuration. TOON is the default; JSON is
optional. Official TeamCity operations continue through the native CLI.
Comparison, setup/hooks and mutations remain deferred.

The original implementation was executed before removal on Node 24.14.0:
133 deterministic, 56 official-binary mock and 18 restricted live scenarios
passed with no skips. The frozen inventory is [go-test-baseline.json](go-test-baseline.json).
All 207 behaviors map to executable Go tests in [go-test-parity.json](go-test-parity.json).
Captured native/live JSON wire artifacts are unchanged. Both Go recorders also
executed successfully into new private temporary files: 39 synthetic native
records and 118 restricted live records; identity and credential guards passed.

Current release verification passes 163 deterministic top-level tests, the 56
additional official-binary contract scenarios, and all 18 restricted live tests.
The native tagged run includes deterministic tests (219 total top-level passes).
Go race detection, vet, formatting, generated documentation and the Go-only
source gate pass. The standalone archive passes content/license inspection and
56 offline command executions without language runtimes or native credentials.
No checks silently skip. See [the release audit](release-audit.md) and
[current release verification](release-verification.json).

Independent reviews covered trusted context, process authority, adapters,
commands, planners, redaction and evidence accounting. Review fixes include
explicit empty-binding identity rejection, JSON integral-number compatibility,
and bounded capture writer handling. LSP could not start because gopls is absent;
Go compilation, tests, race detection and vet supply the verification fallback.

The release-binary scripted Go benchmark records all 96 observations; both wrapper
formats retain required evidence without identity/completeness errors or secret
exposures. The 16-session actual model comparison passes independent answer and
trace grading: both conditions succeed on 8/8 tasks without identity,
completeness, unsupported-causal or credential-exposure errors. Median tool calls
tie at three; total calls are 30 versus 22, and the secret task regresses to nine
versus three. Shared-dependency and cycle tasks improve to two versus four.
The lower-median performance target remains unmet; no general efficiency claim
is made. See [the complete comparison](evaluation.md).

Review found an absent aggregate protocol capture ceiling; the first attempt
was stopped after one completed session, then the runner was fixed and tested
before restarting all 16 sessions. Raw, graded, independent-review and aborted
records remain published as separate sanitized synthetic evidence. Historical
TypeScript results remain unchanged. The owner has authorized Go v0.1.0
publication; final archive and remote CI/publication verification follow these
local execution records.
