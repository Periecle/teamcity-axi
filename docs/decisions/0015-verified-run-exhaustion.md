# Verified run-list exhaustion

Status: accepted for the development read-only contract.

A genuinely exhausted empty query must return scoped exact zero without another
discovery hint. A search stopped by the lookup cap must retain uncertainty even
when it returns zero rows. Missing nextHref alone is insufficient on an
unverified server.

The immutable public TeamCity REST implementation at revision
`fc730e618ccd4b57dbbaf03425bb79a9580d19d2` carries the lookup-limit-reached bit
from its finder into the pager. The
[PagerDataImpl implementation](https://github.com/JetBrains/teamcity-rest/blob/fc730e618ccd4b57dbbaf03425bb79a9580d19d2/rest-api/src/jetbrains/buildServer/server/rest/model/PagerDataImpl.java)
creates nextHref whenever that limit is reached. An empty capped page keeps its
start and increases lookupLimit; a positive capped page advances by the actual
returned count and increases lookupLimit. An ordinary undersized page without a
reached cap has no continuation. Collection count describes the returned page;
it is not an authoritative total for arbitrary multi-page searches.

Restricted live reads on TeamCity 2026.2 build 238924 corroborate both paths.
A direct-project failure search with lookupLimit one returns no rows and a
continuation with lookupLimit two. A positive capped search returns one row and
the increased-limit continuation. The owned QueueA job has queued work but no
finished executions: its bounded finished query over the recorded window returns
zero rows without continuation. These observations use the same GET, HTTP
headers, selected fields, structural locators and DTO normalization as the
production adapter. The nine additional reads bring the restricted corpus to
118 observations.

Only an undersized raw provider page with absent nextHref can seek this proof.
The adapter performs one additional bounded server metadata read and certifies
exhaustion only for the exact observed version label `2026.2 (build 238924)` and
build number `238924`. Other builds, unavailable metadata, failed probes and
insufficient budget preserve rows and SCAN_COVERAGE_UNKNOWN. Interruption
propagates. Full pages without continuation and malformed/unsafe continuations
cannot be certified. No lookup limit is increased to follow the server's cap
continuation.

An exhausted first page with no unresolved candidate limitations reports an
exact total, including zero. Exhausted later pages keep the query total unknown;
previous-page results are not stored or inferred. Discarded candidates with
unverified finish times or selected-root revisions also prevent an exact total.
Empty-scope wording is reserved for an exact total. Otherwise the response says
there are no retained matches in this page and the query total remains unknown.
The selection declares its exhaustion basis and preserves best-effort offset
consistency; no snapshot consistency is asserted.

This closes mandatory scenario A02 for the verified server combination. Tests
cover exact empty scope in JSON and TOON, strict completeness, lack of discovery
hints, cap escalation rejection, unknown versions, denied/budget/capture/deadline
probes, interruption, malformed continuation, full-page omissions and later-page
or unverifiable-candidate wording. Native source proof and actual server captures
remain distinct from synthetic negative scenarios. Other collection adapters
keep their independently verified or unknown coverage contracts.
