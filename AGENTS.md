# Development instructions

`SPECIFICATION.md` is normative; implement the read-only milestones in
`IMPLEMENTATION_PLAN.md`. Keep `docs/STATUS.md` honest about actual evidence.
Never infer live support from synthetic/mock contract tests. Defer comparison,
setup and all mutations according to their separate scope gates.

Use the supplied Context7 instructions for library/CLI documentation. Use LSP
diagnostics after edits; when stale or unavailable, use Go compilation/vet and report
the limitation. Preserve exact run/server/job/root identity and all unknown or
partial semantics. No raw upstream objects reach public output.

Run `make test` and `go test -race ./...`. For the official-binary mock contract
suite, supply a checksum-verified binary through `TEAMCITY_AXI_TEST_BINARY` and
run `make test-real-cli`. Missing fixtures/binaries are failures, never skips.
Use `make test-live` with the private restricted-reader fixture for live checks.
Use independent review of security/evidence handling as required by the plan.
Do not sign commits. The user authorizes pushes to `Periecle/teamcity-axi`.

Keep Go source readable with gofmt: run `make format` after edits and
`make format-check` before handoff. `make test` enforces formatting and generated
documentation drift. Preserve captured wire artifacts; never reformat captures.
The full Go rewrite supersedes the previous Node/npm/TypeScript formatter rules.
When Go LSP is unavailable, use Go compilation/tests/vet and report the limitation.
