# teamcity-axi

A read-only TypeScript CLI for bounded TeamCity evidence. Implementation follows
[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md), with
[SPECIFICATION.md](SPECIFICATION.md) as the normative contract.

The development build implements strict arguments, local context,
help/schema/version, JSON/TOON output, the restricted process transport, and
exact-ID `run view` with typed validation and scope assertions. Other remote
services remain gated pending adapter implementation. See
[implementation status](docs/STATUS.md) and [compatibility](docs/compatibility.json).

Requires Node 24 and a separately installed official `teamcity` CLI. The tested
native wire contract is v1.5.0 on Linux x64. Other Unix archives are checksum
recorded but have not been executed. No live server is currently certified.
The wrapper never downloads native tools during installation.

```sh
npm ci --ignore-scripts
npm run build
node bin/teamcity-axi.mjs --version
node bin/teamcity-axi.mjs --help
node bin/teamcity-axi.mjs context show --json
node bin/teamcity-axi.mjs schema run.view --json
node bin/teamcity-axi.mjs run view 482193 --server work --json
npm test
```

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
