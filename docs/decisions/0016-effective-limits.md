# Effective command limits

Status: accepted for the development read-only product.

Section 15.3 requires actual ceilings in debug diagnostics and responses that hit
limits. Command profiles now share one resolver for child count, concurrency,
stdout/stderr capture and absolute deadline. Output ceilings include tighter
trusted configuration and caller requests. Existing hard maxima and profile
budgets remain unchanged.

`--debug` emits one JSON stderr document with numeric ceilings and child count.
Public limit metadata is additive. Clean ordinary response payloads remain
unchanged. Renderer-only hint removal, optional-data reduction and bounded error
fallback retain effective ceilings. Limit errors identify the actual ceiling and
observed count or bytes when available; no argv, environment or credential values
are added.

Verification includes profile minima, child/deadline/capture enforcement,
renderer-only reduction, JSON/TOON equivalence, tighter trusted configuration,
redaction and packaged debug behavior. Full checks: 133 deterministic, 56 pinned
native and 18 restricted live tests; production-only package smoke checks: 18.
Independent review found no remaining material issue. Some LSP requests timed out;
fresh TypeScript compilation supplied semantic verification.
