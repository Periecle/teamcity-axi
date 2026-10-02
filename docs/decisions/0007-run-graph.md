# Bounded concrete execution graphs

`run tree` traverses immediate snapshot dependencies through `TeamCityReader`.
It uses breadth-first discovery so each retained execution has its minimum
observed depth. Nodes are unique within the frozen server; all retained parent
edges remain visible. A separate current-path traversal detects cycles, without
mistaking shared dependencies for cycles. The native recursive tree command is
never invoked.

Each expanded execution has an independent scoped dependency count followed by
bounded pages of at most 100 rows. A zero count establishes a leaf. Reconciled
unique rows and an independent count establish complete expansion under a
best-effort observation model. Missing counts, conflicting counts, repeated
rows, unsafe continuation and required read failures remain incomplete. A depth
boundary performs no dependency reads and keeps its counts unknown. Empty pages
with safe continuation do not establish exhaustion.

Default limits are depth four, 30 unique nodes, 24 child processes including
version and scope reads, a 20-second shared deadline and 24 KiB serialized
output. Hard depth and node maxima remain 12 and 200. Every edge references a
retained node. Known targets omitted at the node ceiling are counted separately
from unknown unexpanded work. Shared limit diagnostics are grouped by code;
every affected retained node still carries its expansion state.

Policy checks precede node, edge and child-diagnostic retention. A repeated ID
with conflicting job or project identity cannot replace the existing execution.
Lifecycle, result, branch, revisions and optional execution flags changing
between observations are explicit limitations. Unknown enum values remain
unknown. Child metadata limitations identify the observed child, and rejected
child observations cannot publish their metadata through limitations.

A non-terminal primary execution reserves one final exact-ID read before graph
discovery. The transport atomically enforces an absolute launch ceiling for
unreserved reads, including ancestry reads and overlapping children. Final
scope substitution is an error; final read failure or changed root observation
invalidates root expansion. A root remaining non-terminal is explicitly
provisional. Observations do not claim an atomic server snapshot.

Nine planner tests, one executable reservation regression and four released-CLI
mock tests cover DAGs, cycles, bounds, denied/foreign/conflicting observations,
continuation, state changes and renderer equivalence. The actual restricted
TeamCity 2026.2 fixture additionally proves concrete root 9 to prerequisite 8,
scoped counts one and zero, JSON/TOON equivalence and bounded partial expansion.
Live shared-DAG/cycle/state-change evidence and full release certification remain
open. `run failure` candidate selection and source-accounted diagnosis remain the
next implementation slice.
