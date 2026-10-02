# Development instructions

`SPECIFICATION.md` is normative; implement the read-only milestones in
`IMPLEMENTATION_PLAN.md`. Keep `docs/STATUS.md` honest about actual evidence.
Never infer live support from synthetic/mock contract tests. Defer comparison,
setup and all mutations according to their separate scope gates.

Use the supplied Context7 instructions for library/CLI documentation. Use LSP
diagnostics after edits; when stale or unavailable, use `tsc --noEmit` and report
the limitation. Preserve exact run/server/job/root identity and all unknown or
partial semantics. No raw upstream objects reach public output.

Run `npm test` on Node 24. For the official-binary mock contract suite, supply a
checksum-verified binary through `TEAMCITY_AXI_TEST_BINARY` and run
`npm run test:real-cli`. Missing fixtures/binaries are failures, never skips.
Use independent review of security/evidence handling as required by the plan.
Do not sign commits. The user authorizes pushes to `Periecle/teamcity-axi`.
