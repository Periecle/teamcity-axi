# teamcity-axi

A read-only TypeScript CLI for bounded TeamCity evidence. Implementation follows
[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md), with
[SPECIFICATION.md](SPECIFICATION.md) as the normative contract.

The development build implements strict arguments, local context,
help/schema/version, JSON/TOON output, the restricted process transport, and
exact-ID `run view`, bounded `run list`, independent `run problems` and `run tests`,
bounded `run log` and `run changes`, verified `context show` and scoped `doctor`. Reads use typed
validation, scope assertions and query-bound cursors. Other remote services remain
gated pending adapter implementation. See
[implementation status](docs/STATUS.md) and [compatibility](docs/compatibility.json).

Requires Node 24 and a separately installed official `teamcity` CLI. The tested
native wire contract is v1.5.0 on Linux x64. Other Unix archives are checksum
recorded but have not been executed. Focused live contracts and exact run view
have been tested on TeamCity 2026.2 build 238924 with a restricted test identity;
the full read-only product is not yet certified.
The wrapper never downloads native tools during installation.

```sh
npm ci --ignore-scripts
npm run build
node bin/teamcity-axi.mjs --version
node bin/teamcity-axi.mjs --help
node bin/teamcity-axi.mjs context show --json
node bin/teamcity-axi.mjs schema run.view --json
node bin/teamcity-axi.mjs run view 482193 --server work --json
node bin/teamcity-axi.mjs run tests 482193 --server work --failed --json
node bin/teamcity-axi.mjs run log 482193 --server work --tail 80 --json
npm test
npm run format
npm run format:check
```

Code uses pinned ESLint Stylistic and Prettier. ESLint adds blank lines between
import groups, definitions, methods, control blocks and returns. Prettier applies
two-space indentation, semicolons, single quotes, trailing commas and LF endings. `npm test` checks formatting before compiling
and running tests, so the same rules are enforced in CI. Captured wire artifacts
are excluded from automatic rewriting.

Run list defaults to finished runs in an emitted seven-day finish-time window.
It keeps page totals unknown and marks missing continuation as partial when
bounded scan exhaustion cannot be proved. Exact contextual branches can be
elided from projected rows; all-branch rows retain branch identity. Fractional
finish-time filters and canceled/failed-to-start/unknown outcome filters remain
explicitly gated while their contracts are verified.

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

`context show` makes no server calls unless `--verify` is supplied. Verification
reads only selected jobs/projects and the current identity; it reports a safe
identity fingerprint. `doctor --offline` probes the local executable version
without HTTP. Online doctor requires a project or job, probes bounded core reads
and optional structured logs, and reports remaining capabilities as unverified.
Its current result is partial until the remaining adapters are delivered.
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
