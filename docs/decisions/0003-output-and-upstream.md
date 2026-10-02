# ADR 0003: normalized output and upstream strategy

Accepted for the initial implementation, 2026-10-02.

Strict TypeScript ESM on Node 24 is the implementation baseline. Public JSON
schemas are runtime-validated by pinned Ajv 2020. Conditional seed schemas use
cross-subschema `required` and property constraints; Ajv's strictRequired,
strictTypes and strictTuples lint checks are disabled without disabling their
actual JSON Schema validation semantics. Other schema lint checks remain strict.

Pinned `@toon-format/toon` encodes the same model as JSON; no dispatcher SDK
with implicit update operations is imported. Redact known environment secrets,
recognizable authorization, credential URLs and private keys before serialization.
Remove ANSI/control sequences and expose bidi controls as literal code points.
Public envelope constants and wrapper-generated timestamps are protocol metadata,
not credential-bearing text; coincidental secret matches do not rewrite them.
Registered protocol keys are likewise preserved. Trusted secret-name patterns
use bounded expressions (anchors, character classes and at most one `.*`;
groups, repetition and backreferences are rejected) for environment collection
and untrusted field redaction.

Measure serialized UTF-8 bytes including the terminal newline. Never slice the
serialized stream or silently drop evidence arrays. Until command-specific
reducers prove page/coverage/reference invariants, oversized output returns an
explicit bounded error retaining minimal trusted server and asserted scope.

All TeamCity operations use the official executable. The v1.5.0 released binary
has been executed against synthetic local HTTP fixtures, with exact raw
status/header/body parsing contract inputs recorded. It accepts raw GET include
and context prefixes. Its combined failed-log summary silently omits subsidiary
test request failures; it is never an authoritative completeness source.
Independent occurrence pages and immediate dependency reads will use constrained
GET operations. Structured tail is optional; unsupported logs never trigger a
full-log fallback. No remote-response cache is introduced.

Mock fixtures prove client wire behavior, not server endpoint semantics or live
support. Live versions, permissions, locator escaping and scan-limit semantics
remain release gates until a controlled server is tested.
