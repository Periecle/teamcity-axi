# teamcity-axi

A read-only TypeScript CLI for bounded TeamCity evidence. Implementation follows
[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md), with
[SPECIFICATION.md](SPECIFICATION.md) as the normative contract.

Observe the current checkout with `status`, investigate an exact execution with
`run failure`, or wait for its outcome with `run watch`. The read-only command
surface also includes exact run/job/agent views, bounded run/job/queue/agent
inventories, independent problems/tests/logs/changes/dependency reads, and local
or verified context diagnostics. JSON and TOON carry the same logical values;
typed validation, scope assertions, and query-bound cursors preserve identity.
See the [generated command reference](docs/commands.md),
[portable agent skill](skills/teamcity-axi/SKILL.md),
[implementation status](docs/STATUS.md), and [compatibility](docs/compatibility.json).
The [recorded evaluation](docs/evaluation.md) compares investigation evidence
against optimized native workflows, including measured costs and limitations.

Requires Node 24 and a separately installed official `teamcity` CLI. The tested
native wire contract is v1.5.0 on Linux x64. Other Unix archives are checksum
recorded but have not been executed. The read-only command services
have been tested on TeamCity 2026.2 build 238924 with a restricted test identity;
v0.1.0 acceptance covers that recorded combination and its stated capability limits.
The wrapper never downloads native tools during installation.
Files in `examples/` use synthetic placeholder identities; live test evidence is
recorded separately in the sanitized fixture corpus.

```sh
npm ci --ignore-scripts
npm run build
node bin/teamcity-axi.mjs --version
node bin/teamcity-axi.mjs --help
node bin/teamcity-axi.mjs context show --json
node bin/teamcity-axi.mjs schema run.view --json
node bin/teamcity-axi.mjs status --job Payments_Build --server work --vcs-root Payments_Git --check --json
node bin/teamcity-axi.mjs run watch 482193 --server work --check --json
node bin/teamcity-axi.mjs run view 482193 --server work --json
node bin/teamcity-axi.mjs run tests 482193 --server work --failed --json
node bin/teamcity-axi.mjs run log 482193 --server work --tail 80 --json
node bin/teamcity-axi.mjs job list --project Payments --server work --json
node bin/teamcity-axi.mjs job view Payments_Build --server work --json
node bin/teamcity-axi.mjs queue list --job Payments_Build --server work --json
node bin/teamcity-axi.mjs agent list --job Payments_Build --server work --json
node bin/teamcity-axi.mjs agent view 7 --project Payments --server work --json
npm test
npm run format
npm run format:check
npm run docs:generate
```

Code uses pinned ESLint Stylistic and Prettier. ESLint adds blank lines between
import groups, definitions, methods, control statements (including single-line
guards) and returns. Prettier applies
two-space indentation, semicolons, single quotes, trailing commas and LF endings. `npm test` checks formatting before compiling
and running tests, so the same rules are enforced in CI. Captured wire artifacts
are excluded from automatic rewriting.

`status` reads at most five required tracked jobs and displays one relevant run
per job. Each bounded candidate window includes queued, running, and finished
executions. The newest exact selected-root candidate determines the job's check;
an older green cannot replace a newer exact queued, running, or red run. Newer
unknown revision coverage remains unverified. `--check` requires every tracked
job to have an exact completed successful non-personal run, plus a verified clean
Git HEAD. Dirty worktrees, missing jobs/roots, omitted required jobs, and other
unverified roots cannot pass. Ordinary red observation exits zero. Status history
and activity counts describe only the returned candidates, not a server-wide
inventory. With no binding, the no-argument home view stays local and unconfigured.

`run watch ID` polls one frozen execution and emits one final document. Its
default deadline is 120 seconds, interval ten seconds (minimum five), child
ceiling 32, concurrency one, and stdout budget 8 KiB. A deadline retains the last
verified queued/running observation as partial. Vanished and inaccessible runs
remain distinct; stopping the watcher never cancels TeamCity work. `--check`
requires a terminal success for that execution. Use status to assert the local
checkout; use `--require-complete` to reject incomplete observations.

Run list defaults to finished runs in an emitted seven-day finish-time window.
The verified server combination can prove undersized-page exhaustion and report
exact first-page totals, including scoped zero. Other server builds, unresolved
candidate membership and later-page totals retain uncertainty. Lookup-cap
continuations never increase the configured scan budget. Exact contextual branches can be
elided from projected rows; all-branch rows retain branch identity. Fractional
RFC 3339 bounds retain their exact precision in windows and cursors. Server
predicates establish membership at millisecond precision; reported finish
timestamps remain at their original second precision. `--result unknown` filters
normalized candidates, retaining provider counts and empty-page continuation.
`--result canceled` and `--result failed_to_start` use explicit server metadata;
ordinary success/failure/error filters exclude those exceptional outcomes.
Missing or conflicting outcome metadata stays unknown. Composite execution is
reported independently from lifecycle and result.

Problems and tests retain exact run-bound occurrence IDs, including duplicate
test names. `run tests --failed` excludes muted failures unless `--include-muted`
is supplied; `--muted` selects muted failures. Pages keep totals unknown when
bounded exhaustion is unproven. Select an emitted occurrence ID with `--problem`
or `--test` for exact expansion; test definition IDs are not occurrence IDs.

Logs expose a declared retained tail window and stable message IDs. `--contains`
matches literal text within the full retained messages before display previews.
`run log --failed` combines independent problem, unmuted failed-test and bounded
log reads; unavailable sources remain explicit while available siblings survive.
It does not imply causal attribution or complete-log coverage. `--full` expands
bounded text previews, subject to the output byte budget and a text ceiling.

`run changes` defaults to ten contextual commits, retaining commit and VCS-root
identity. Messages show their first line by default; `--full` expands the bounded
page. File names are fetched only with `--files`, capped at 100 per commit with
explicit omission counts. Changes are contextual evidence, without causal claims.

`run tree` inspects concrete immediate snapshot dependencies with shared budgets.
Shared prerequisites appear once while retaining every parent edge; cycles are
reported separately. Scoped counts distinguish known leaves from depth, node,
call and permission boundaries. Defaults are depth four, 30 nodes, 24 child
processes, 20 seconds and 24 KiB output. `--depth 0` means root only. Non-terminal
roots reserve a final observation, with provisional or changed evidence explicit.
The graph never implies causal attribution. See [graph decisions](docs/decisions/0007-run-graph.md).

`run failure` emits bounded observations with retrievable source evidence. It
accounts separately for problems, unmuted tests, muted tests, dependency
expansion and inspected log windows. Source denial retains successful siblings;
unknown totals never become zero. Successful finished runs skip investigation
reads. Diagnosis defaults to three runs, graph reads stop at ten, and non-terminal
roots reserve a final observation. The current live problem/test adapters retain
unknown exhaustion, so failed-run investigations remain partial. Optional changes
are reduced before required evidence when stdout is tight. See
[failure decisions](docs/decisions/0008-failure-investigation.md).

`job list` reads one bounded page of jobs directly owned by the selected project.
It defaults to 20 rows and preserves unknown totals and collection exhaustion.
`job view` returns safe exact-ID metadata. Both commands expose nullable paused
state and omit parameters and settings; current project policy applies to every
invocation. See [job decisions](docs/decisions/0009-scoped-jobs.md).

`queue list` requires a selected job or project and preserves exact queued
execution IDs, optional branch/time and the provider's wait reason. It defaults
to 20 rows; totals and missing bounded continuation remain unknown. Cursors bind
the resolved scope and current policy, while offset consistency is best effort.
Retrieval hints observe exact executions. A queued run without a reported result
stays unknown with an explicit limitation. See [queue decisions](docs/decisions/0010-scoped-queue.md).

`agent list` requires a pool or validated job/project scope; `agent view` reads an
exact numeric ID. Connectivity, enablement and authorization stay separate and
nullable. Safe active-run pointers must pass current project policy. Missing
activity metadata never establishes idleness, and unavailable pool scopes remain
errors. Pages preserve unknown totals and exhaustion. See [agent decisions](docs/decisions/0011-safe-agents.md).

`context show` makes no server calls unless `--verify` is supplied. Verification
reads only selected jobs/projects and the current identity; it reports a safe
identity fingerprint. `doctor --offline` probes the local executable version
without HTTP. Online doctor requires a project or job, probes bounded core reads
and optional structured logs, and reports remaining capabilities as unverified.
Its result stays partial for capabilities outside that bounded probe set;
command implementation does not imply that doctor verified every capability.
Project-subtree policy follows at most eight observed parent links; unknown or
cyclic ancestry cannot authorize access.

Trusted configuration is `${XDG_CONFIG_HOME:-~/.config}/teamcity-axi/config.json`.
Use [the schema](schemas/user-config.schema.json) and
[synthetic example](examples/user-config.json). Set ownership to yourself and
permissions to `0600`. Repository `teamcity.toml` selects registered URLs and
scope; `.teamcity-axi.json` supplies VCS mappings. Never put tokens in either.
Use restricted official-CLI authentication. Inherited `TEAMCITY_TOKEN` requires
matching `TEAMCITY_URL`.

```sh
TEAMCITY_AXI_TEST_BINARY=/absolute/path/teamcity npm run test:real-cli
```

This suite requires the verified release binary and fails if it is absent or
mismatched. It never silently skips. Read [security](docs/security.md),
[fixture provenance](tests/fixtures/README.md) and [sources](SOURCES.md).

`--debug` writes one JSON diagnostic to stderr with actual child, concurrency,
capture, output and deadline limits. Limit-hit responses carry the same numeric
ceilings, including output reductions that happen during rendering. Normal
responses stay unchanged.

The project uses the [MIT license](LICENSE). The [dependency inventory](docs/dependencies.md)
records the exact lockfile versions and declared third-party licenses.
