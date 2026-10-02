# Explicit execution outcomes

Status: accepted for the development read-only contract.

TeamCity reports a canceled build as `UNKNOWN` and a failed-to-start build as
`FAILURE`. Those strings alone cannot identify the normalized outcome. The
adapter projects `failedToStart` and only `canceledInfo(timestamp)` on detail,
list, dependency and bulk status reads. Cancellation actor and comment fields
are excluded from requests and public models.

Normalization requires a boolean failed-to-start flag. A valid cancellation
record selects `canceled`; a true failed-to-start flag selects `failed_to_start`.
Absent cancellation records are the observed normal DTO shape. Missing or
malformed outcome information remains unknown or fails schema validation;
conflicting exceptional flags, exceptional nominal success, and failed start
outside a finished lifecycle remain unknown with a scoped limitation. Lifecycle
and composite properties remain independent. Running nominal success does not
satisfy a terminal execution or checkout check.

List selectors for ordinary success/failure/error include `canceled:false` and
`failedToStart:false`. Exceptional selectors use their explicit dimensions.
Every returned row is verified against the requested normalized result. Cursor
bindings include the reconstructed selectors. Returned-page aggregates count
all six public results independently. The [unknown-result selector](0014-precise-run-list.md) now filters normalized
candidates with separate provider coverage.

Restricted native/live captures prove ordinary failure/success, queued and
running states, failed start, cancellation before and after execution starts,
and successful composite execution. A live two-poll watch followed exact run
18 from running to canceled without passing its check. Fixture administration
uses separate temporary test tooling; the shipped product remains read-only.
The reader retains its parent project viewer role, with inherited access to the
owned lifecycle child project and no build-run or global project permission.

Large pages or graphs can generate more than 100 identical scoped limitations.
Before serialization, repeated diagnostics are grouped when the note count
approaches the schema ceiling. The grouped note retains code, source, message,
and the exact distinct execution count. Singletons remain scoped; outcome rows
and completeness flags are preserved. No array of evidence is discarded.

Evidence: 37 actual native/mock captures and 95 restricted TeamCity 2026.2
captures in the versioned fixture contracts; deterministic normalization,
released-native command-service checks, and restricted live view/list/watch/
investigation tests. Historical captures are retained beside current projections.
This is focused contract evidence, not full release certification.

Official contracts: [Build](https://www.jetbrains.com/help/teamcity/rest/build.html)
and [BuildLocator](https://www.jetbrains.com/help/teamcity/rest/buildlocator.html).
