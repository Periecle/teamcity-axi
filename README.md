# teamcity-axi

A read-only Go CLI for bounded TeamCity evidence. Implementation follows
[IMPLEMENTATION_PLAN.md](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/IMPLEMENTATION_PLAN.md), with
[SPECIFICATION.md](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/SPECIFICATION.md) as the normative contract.

Observe the current checkout with `status`, investigate an exact execution with
`run failure`, or wait for its outcome with `run watch`. The read-only command
surface also includes exact run/job/agent views, bounded run/job/queue/agent
inventories, independent problems/tests/logs/changes/dependency reads, and local
or verified context diagnostics. TOON is the default; optional JSON carries the same logical values.
Typed validation, scope assertions, and query-bound cursors preserve identity.
See the [generated command reference](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/commands.md),
[portable agent skill](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/skills/teamcity-axi/SKILL.md),
[implementation status](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/STATUS.md), and [compatibility](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/compatibility.json).
The [recorded evaluation](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/evaluation.md) compares investigation evidence
against optimized native workflows, including measured costs and limitations.

v0.1.1 includes lazy schema initialization, precise evidence expansion hints,
shared renderer normalization, allocation reductions and shorter portable
instructions. Both schema gates and required independent evidence remain intact.
The [v0.1.1 performance report](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/performance-v0.1.1.md) records all before/after
measurements, model sessions, regressions and remaining opportunities. Historical
TS session time includes model reasoning; measured Go CLI execution is faster.

The standalone executable requires a separately installed official `teamcity` CLI.
Build from source with Go 1.26 or newer; no language runtime is needed after building. The tested
native wire contract is v1.5.0 on Linux x64. Other Unix archives are checksum
recorded but have not been executed. The read-only command services
have been tested on TeamCity 2026.2 build 238924 with a restricted test identity;
Go execution evidence covers that recorded combination and its stated capability
limits. Historical v0.1.0 and TS evaluations remain available alongside the
current release's evidence; they are separate observations, not a controlled
language comparison or a general efficiency guarantee.
The wrapper never downloads native tools during installation.
Use the official `teamcity` CLI directly for ordinary native operations and
authentication management. AXI adds bounded agent-facing evidence, exact checkout
assessment, source accounting and investigation; it does not duplicate native
setup or add an unrestricted command passthrough.
Files in `examples/` use synthetic placeholder identities; live test evidence is
recorded separately in the sanitized fixture corpus.

Install the Linux amd64 build from v0.1.1:

```sh
curl -fLO https://github.com/Periecle/teamcity-axi/releases/download/v0.1.1/teamcity-axi-linux-amd64.tar.gz
curl -fLO https://github.com/Periecle/teamcity-axi/releases/download/v0.1.1/SHA256SUMS
sha256sum --check SHA256SUMS
tar -xzf teamcity-axi-linux-amd64.tar.gz
mkdir -p "$HOME/.local/bin"
install -m 0755 teamcity-axi "$HOME/.local/bin/teamcity-axi"
"$HOME/.local/bin/teamcity-axi" --version
```

Add `$HOME/.local/bin` to your `PATH` to invoke `teamcity-axi` directly. Install
the official TeamCity CLI separately and select the checksum-tested v1.5.0
binary recorded in [compatibility](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/compatibility.json). No Go toolchain is
required to run the compiled wrapper. Only Linux amd64 has execution evidence;
cross-compilation does not establish support for another platform.

For a source checkout, build the executable and run the examples below:

```sh
go mod download
make build
bin/teamcity-axi --version
bin/teamcity-axi --help
bin/teamcity-axi context show
bin/teamcity-axi schema run.view
bin/teamcity-axi status --job Payments_Build --server work --vcs-root Payments_Git --check
bin/teamcity-axi run watch 482193 --server work --check
bin/teamcity-axi run view 482193 --server work
bin/teamcity-axi run tests 482193 --server work --failed
bin/teamcity-axi run log 482193 --server work --tail 80
bin/teamcity-axi job list --project Payments --server work
bin/teamcity-axi job view Payments_Build --server work
bin/teamcity-axi queue list --job Payments_Build --server work
bin/teamcity-axi agent list --job Payments_Build --server work
bin/teamcity-axi agent view 7 --project Payments --server work
make test
make format
make format-check
make docs
```

The examples emit TOON. Add `--json` when a JSON consumer needs it, for example
`teamcity-axi run failure 482193 --server work --json`.

Go source uses gofmt. `make test` enforces formatting and generated help drift
before running deterministic tests. Native mock and live suites are explicit
separate targets; required fixture inputs fail when missing. Captured wire
artifacts are never reformatted.

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
The graph never implies causal attribution. See [graph decisions](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/decisions/0007-run-graph.md).

`run failure` emits bounded observations with retrievable source evidence. It
accounts separately for problems, unmuted tests, muted tests, dependency
expansion and inspected log windows. Source denial retains successful siblings;
unknown totals never become zero. Successful finished runs skip investigation
reads. Diagnosis defaults to three runs, graph reads stop at ten, and non-terminal
roots reserve a final observation. The current live problem/test adapters retain
unknown exhaustion, so failed-run investigations remain partial. Optional changes
are reduced before required evidence when stdout is tight. See
[failure decisions](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/decisions/0008-failure-investigation.md).

`job list` reads one bounded page of jobs directly owned by the selected project.
It defaults to 20 rows and preserves unknown totals and collection exhaustion.
`job view` returns safe exact-ID metadata. Both commands expose nullable paused
state and omit parameters and settings; current project policy applies to every
invocation. See [job decisions](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/decisions/0009-scoped-jobs.md).

`queue list` requires a selected job or project and preserves exact queued
execution IDs, optional branch/time and the provider's wait reason. It defaults
to 20 rows; totals and missing bounded continuation remain unknown. Cursors bind
the resolved scope and current policy, while offset consistency is best effort.
Retrieval hints observe exact executions. A queued run without a reported result
stays unknown with an explicit limitation. See [queue decisions](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/decisions/0010-scoped-queue.md).

`agent list` requires a pool or validated job/project scope; `agent view` reads an
exact numeric ID. Connectivity, enablement and authorization stay separate and
nullable. Safe active-run pointers must pass current project policy. Missing
activity metadata never establishes idleness, and unavailable pool scopes remain
errors. Pages preserve unknown totals and exhaustion. See [agent decisions](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/decisions/0011-safe-agents.md).

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
Use [the schema](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/schemas/user-config.schema.json) and
[synthetic example](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/examples/user-config.json). Set ownership to yourself and
permissions to `0600`. Repository `teamcity.toml` selects registered URLs and
scope; `.teamcity-axi.json` supplies VCS mappings. Never put tokens in either.
Use restricted official-CLI authentication. Inherited `TEAMCITY_TOKEN` requires
matching `TEAMCITY_URL`.

```sh
TEAMCITY_AXI_TEST_BINARY=/absolute/path/teamcity make test-real-cli
```

This suite requires the verified release binary and fails if it is absent or
mismatched. It never silently skips. Read [security](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/security.md),
[fixture provenance](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/tests/fixtures/README.md) and [sources](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/SOURCES.md).

`--debug` writes one JSON diagnostic to stderr with actual child, concurrency,
capture, output and deadline limits. Limit-hit responses carry the same numeric
ceilings, including output reductions that happen during rendering. Normal
responses stay unchanged.

The project uses the [MIT license](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/LICENSE). The [dependency inventory](https://github.com/Periecle/teamcity-axi/blob/v0.1.1/docs/dependencies.md)
records the pinned Go module versions and declared third-party licenses.
