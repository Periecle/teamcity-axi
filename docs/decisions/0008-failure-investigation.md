# Source-accounted failure investigation

`run failure` starts with the exact primary execution and current trusted project
policy. Finished success returns `not_failed` without dependency, test, problem,
log or change reads. Other outcomes retain their lifecycle and visible result;
non-terminal executions remain provisional and unknown outcomes are inconclusive.

Root problems precede bounded breadth-first graph discovery. The graph consumes
at most ten adapter reads and shares the 24-child invocation ceiling, 20-second
deadline and 24 KiB output default. Primary test/log capacity and a non-terminal
root's final read are reserved. Graph policy ancestry reads use the narrower
graph launch ceiling while retaining invocation-local ancestry caching.

Diagnosis considers the primary execution first, then observed failed or unknown
prerequisites. The default diagnosis cap is three including the root; the hard
maximum is ten. Depth and exact numeric execution ID break ties deterministically.
The pure selector accepts independently established dependency references, but
the currently verified problem DTO has no such field. The service explicitly
reports `dependencyReferences=unavailable`; it never extracts causal links from
problem prose or treats an unexpanded node as a failed leaf.

Each diagnosed run has independent problem and unmuted failed-test reads. Logs
are required when direct descriptions are absent or generic. Each log coverage
record declares its tail window, first/last retained message IDs and provider
omissions, with an unknown full-log total. Independently fetched muted failures
use separate source IDs. Optional changes are read last and retain exact root
identity, timestamp and bounded contextual previews.

Coverage records declare required versus optional sources, the inspected scope,
returned/provider counts, nullable totals and a reason for partial, unavailable,
skipped or exhausted work. Complete bounded pages require actual exhaustion
proof; the current live primitive adapters retain unknown bounded exhaustion,
so their failure reports remain partial. Both denied count/page graph reads yield
unavailable source coverage rather than invented partial content. Known skipped
candidates and graph boundaries remain visible. The bounded source schema admits
up to 900 records to cover the hard 200-node graph and skipped-source accounting.

Findings contain observations and source-bound retrieval actions. Duplicate test
names and separate execution/occurrence IDs remain distinct. A failed prerequisite
is an observation, not a causal assertion. Text matching describes only the
inspected tail. Fingerprints sanitize all inputs before hashing or encoding;
dependency evidence uses a fixed lifecycle/result sentence. Default text previews
and full excerpt ceilings report truncation. Full failure excerpts are capped at
8,192 code points, and the whole response still respects its byte ceiling.

The renderer first drops optional hints, then removes optional change rows from
the end of their returned order until the actual JSON/TOON document fits.
`selection.omittedChanges` and change-source returned/provider counts remain
reconciled. Required findings and graph nodes are retained. If core evidence still
cannot fit, an explicit bounded output error is returned. The hundred-finding
ceiling separately counts omitted observed findings and makes coverage partial.

A reserved final read never replaces the frozen server/job/project scope.
Unavailable or changed final observations invalidate root graph expansion and
remain explicit. Evidence collected before a root transition is preserved rather
than rewritten to imply an atomic snapshot. A transition to completed success
after investigation yields an inconclusive assessment.

Focused planner, released-native/mock and restricted live tests cover these
contracts. Live evidence proves failed-root source identities, occurrence
retrieval, unknown totals, separate muted coverage, success short circuit and
foreign-project denial. Broader live cancellation/composite/state-change/shared-DAG
fixtures, explicit problem-to-dependency references, exhaustion proofs and full
milestone certification remain open. Job/queue/agent/status/watch services and
release evaluation remain required by the full objective.
