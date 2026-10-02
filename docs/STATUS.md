# Product delivery and acceptance

The read-only v0.1.0 product is implemented and accepted for the recorded tested
combination. Milestones 0–7, all 18 registered command services, and all 32 mandatory
scenario behaviors are complete. Comparison, setup/hooks and mutations remain
outside this release according to the implementation plan.

The product observes exact checkout status and execution outcomes, investigates
failures through independent sources and bounded graph traversal, exposes scoped
jobs/queue/agents, and packages local diagnostics, help, schemas and the portable
skill. Identity, unknown/partial semantics, restricted read transport, resource
limits and JSON/TOON validity have executable evidence.

## Executed verification

On Linux x64 / Node 24.14.0 / official TeamCity CLI 1.5.0:

- 133 deterministic tests, including pinned formatter and generated-doc drift gates.
- 56 released-native mock contracts.
- 18 restricted live tests on TeamCity 2026.2 build 238924.
- 21 production-only installed-package smoke checks.
- 16 actual model-agent sessions, all independently graded successful, with zero
  identity, completeness, unsupported-cause or secret-exposure errors.
- No skips. Compiler, schema, portable-skill and diff checks pass.

The [scenario audit](acceptance.md), [release audit](release-audit.md),
[compatibility matrix](compatibility.json), [changelog](../CHANGELOG.md), and
[dependency/license inventory](dependencies.md) retain acceptance evidence and
limits. Independent evidence/security review found no remaining material issue
in the accepted scope. LSP returned stale data/timeouts for some TypeScript files;
fresh compilation served as fallback. The .mjs evaluator is unsupported by LSP
and has syntax plus actual execution/cleanup checks.

Exact-head CI passed both jobs at the effective-limit checkpoint `8c0e727`
([run 37044990473](https://github.com/Periecle/teamcity-axi/actions/runs/37044990473));
Final product CI passed both jobs at `a88ddef`
([run37049362753](https://github.com/Periecle/teamcity-axi/actions/runs/37049362753)). The compiled package ships
MIT licensing, changelog, schemas, skill and generated reference. It excludes
test/setup/evaluation tools, wire captures and credentials. Native installation
is separate; there is no postinstall download, hidden update or telemetry.

## Measured results and support limits

The [scripted benchmark](evaluation.md) preserves required wrapper evidence in
48/48 observations without canary exposure. Its optimized native baseline is
smaller and faster. The actual eight-task agent sample passes 8/8 in each
condition: wrapper median 3 versus native 3.5 tool calls, but totals 30 versus 26 and
regressions on shared-dependency, exact-green and secret tasks. Output is larger.
No statistical, token-saving or broad superiority claim is made.

Support is limited to the recorded platform/native/server combination and
capability limits. Other platform binaries are checksum-recorded but unexecuted.
Logs are retained tails. Unknown provider exhaustion remains unknown; exact
first-page run-list zero depends on the verified server pagination contract.
Restricted pool reads remain unavailable, and missing activity never proves
idleness. Optional broader live fixtures and new platform/server versions need
separate execution evidence.

The owned localhost test server and restricted reader remain available for
reproducible integration testing. The reader has project-view scope on
AxiContract, no inherited All Users roles and no build-run permission. Evaluation
sessions and their temporary model-auth copies are cleaned up; ordinary product
commands never persist transcripts or credentials.
