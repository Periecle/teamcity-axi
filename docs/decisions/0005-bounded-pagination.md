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

The run-list service now consumes these helpers with exact-job preflight,
scope/policy checks on every row, fixed finish-time windows, cursor binding and
independent review. It preserves useful rows when continuation is unsafe and
reports unknown totals. Missing continuation under a bounded scan does not prove
exhaustion: the response is partial with unknown continuation, including empty
pages. Known continuation after an empty page remains usable. Offset consistency
is reported explicitly.

Live root-context reads on TeamCity 2026.2 build 238924 verify encoded job IDs,
project/all-branch pages, finish-time filters and relative continuation. Positive
hostile branch names, exact VCS revision matches, live deployment prefixes remain independent gates. Explicit outcomes and
sub-second boundaries are now verified in
[the outcome](0013-execution-outcomes.md) and
[precise list](0014-precise-run-list.md) contracts. Unknown
result diagnostics and branch identity survive public projection.
