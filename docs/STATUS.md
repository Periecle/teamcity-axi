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
executed successfully into new private temporary files:39synthetic native
records and118restricted live records; identity and credential guards passed.

Current Go verification passes 162 deterministic top-level tests, the 56
additional official-binary contract scenarios, and all 18 restricted live tests.
The native tagged run includes deterministic tests (218 total top-level passes).
Go race detection, vet, formatting, generated documentation and the Go-only
source gate pass. The standalone archive passes content/license inspection and
56 offline command executions without language runtimes or native credentials.
No checks silently skip. See [the release audit](release-audit.md).

Independent reviews covered trusted context, process authority, adapters,
commands, planners, redaction and evidence accounting. Review fixes include
explicit empty-binding identity rejection, JSON integral-number compatibility,
and bounded capture writer handling. LSP could not start because gopls is absent;
Go compilation, tests, race detection and vet supply the verification fallback.

The fresh scripted Go benchmark records all 96 observations; both wrapper
formats retain required evidence without identity/completeness errors or secret
exposures. The model runner and isolation/cleanup checks pass, but the new
16-session model evaluation awaits explicit credential/account-use approval
after automatic review rejected its launch. Historical model results do not
certify Go, and the model release gate remains open. No Go release publication
or remote CI result is claimed.
