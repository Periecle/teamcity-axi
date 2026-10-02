# 0004: Exact run reads preserve identity and unavailable metadata

The first remote service reads one explicitly requested run through
`TeamCityReader` and the restricted native transport. Its field projection and
raw HTTP framing have a released v1.5.0 binary/mock fixture; this is development
contract evidence, not real-server certification.

The adapter validates required lifecycle/result and safe identities, asserts the
returned run ID, and ignores additive fields. Job/project flags are assertions.
Trusted project scope remains an exact allowlist until ancestry reads have
separate evidence; unknown ancestry cannot widen it.

Unknown enum values remain unknown. Invalid optional timestamps and omitted
revision metadata are explicit limitations; an omitted revision collection does
not become an empty successful observation. A failed build is a successful read.
Non-2xx responses classify by HTTP status and preserve status/Retry-After inside
the typed error, without exposing untrusted response bodies or headers.

Known credentials are masked before and after terminal-control normalization,
before diagnostic or summary previews. Expansion hints preserve explicit scope
assertions. Full text remains subject to the global serialized byte budget.
Cancellation terminates the transport before emitting one safe final envelope.
