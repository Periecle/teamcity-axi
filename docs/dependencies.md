# Dependency and license inventory

The Go implementation pins its module dependencies in go.mod/go.sum. Product
binaries embed the schema and TOON dependencies; development evaluation tooling
also uses an ordinary-text o200k_base tokenizer. The official TeamCity CLI is
installed separately and retains its own JetBrains terms. There are no install
hooks, automatic native downloads or runtime dependency downloads.

The project uses the [MIT license](../LICENSE). Third-party license texts are included in THIRD_PARTY_LICENSES.txt and the
standalone archive; refresh both when module versions change.

| Module | Version | License | Use |
| --- | --- | --- | --- |
| github.com/BurntSushi/toml | v1.6.0 | MIT | Native repository bindings |
| github.com/santhosh-tekuri/jsonschema/v6 | v6.0.3 | Apache-2.0 | Offline public and config schema validation |
| github.com/toon-format/toon-go | v0.1.0 | MIT | Primary output serializer |
| golang.org/x/text | v0.14.0 | BSD-3-Clause | Schema dependency |
| github.com/tiktoken-go/tokenizer | v0.8.1 | MIT | Release evaluation token counting |
| github.com/dlclark/regexp2/v2 | v2.5.1 | MIT | Tokenizer dependency |

Go's standard library supplies process execution, cancellation, concurrency,
embedded assets, HTTP test servers, JSON, checksums and package archives. Native
mock functional tests use a real local HTTP server and the checksum-pinned
released CLI. Live read tests reuse the owned restricted fixture. New disposable
container provisioning, when required, should use testcontainers.
