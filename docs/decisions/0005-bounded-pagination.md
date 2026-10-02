# 0005: Pagination reconstructs typed queries

Literal values use explicit base64url conditions derived from the pinned native
CLI and public REST documentation. Nested branch names use the explicit
`name:(value:...)` condition rather than letting a decoded name become a locator.
Malformed Unicode is rejected before UTF-8 encoding can change selector identity.

Provider links are untrusted. The adapter reduces a validated link to a numeric
position and rebuilds the next request. It rejects foreign origins, absolute
URLs outside the configured deployment context, traversal, extra query keys,
changed fields/filters, repeated positions, larger pages and increased scan
limits. Bare relative REST hrefs may be normalized against the frozen context;
that convention remains a separate live evidence gate.

Public cursors are bounded, versioned, expiring data containing command, trusted
alias, filter hash, page size, numeric position and optional fixed time window.
They contain no provider URL or authentication credential and confer no
authority. The consuming query must revalidate policy and recompute the filter
hash from the effective query, including its window, before checking binding.
Cursor binding separately checks fixed window dates.

These helpers have pure adversarial tests and independent review. They are not
yet a complete run-list adapter or a live pagination support claim.
