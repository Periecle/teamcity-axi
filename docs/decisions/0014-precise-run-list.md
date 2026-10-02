# Precise run-list windows and normalized unknown results

Status: accepted for the development read-only contract.

Run list accepts all six public outcomes and fractional RFC 3339 finish-time
bounds. Windows are exclusive. Exact requested precision survives UTC
normalization, the default seven-day lookback, output, hints and cursor binding.
Calendar errors and reversed sub-millisecond intervals fail before native reads.

The live server compares millisecond finish timestamps but reports whole seconds
in Build DTOs. Run 1 reports `11:33:14` while strict equality and adjacent native
predicate captures establish its finish at `11:33:14.569`. Comparing only the
reported DTO timestamp would wrongly reject that build from a narrow window
around its true finish. Conversely, rounding the provider filter to whole
seconds could admit it to a window it did not actually match.

For the verified integer-millisecond server model, strict `after(floor-ms(since))`
and `before(ceil-ms(until))` preserve the exact requested exclusive interval.
Native date selectors emit controlled three-digit milliseconds when necessary.
They never send arbitrary user fractions into the server parser. Strict
build-relative equality probes and absolute millisecond equality/adjacent probes
verify the comparison semantics. A requested two-nanosecond interval around the
actual finish includes run 1; moving either bound across that finish excludes it.
The server model is also documented by the
[TeamCity SFinishedBuild API](https://javadoc.jetbrains.net/teamcity/openapi/current/jetbrains/buildServer/serverSide/SFinishedBuild.html),
whose finish timestamp is a `java.util.Date`.

Provider predicates establish window membership. The adapter still validates
all row identities, scope, outcome metadata and finish timestamps. A reported
second bounds possible server instants from that second through its last
millisecond; disjoint intervals are rejected locally with a scoped limitation.
Output declares provider membership, reported second precision and filter
millisecond precision. It does not fabricate a precise finishedAt value.
At the upper RFC 3339 year limit, a rounded threshold outside the representable
native format is omitted; valid four-digit reported timestamps remain bounded
and verified against the requested window.

`--result unknown` reads the bounded candidate page without a raw-status filter.
A future status, missing explicit outcome metadata, or a queued execution without
a result can normalize to unknown. Canceled executions do not qualify merely
because TeamCity labels them UNKNOWN. Every candidate is scope-checked before
filtering. Provider candidate counts and page positions remain separate from
returned matches. Empty filtered pages preserve valid continuation. All result
selectors, including unknown versus no selector, participate in the cursor hash.

Exact bounds can exceed the four-KiB cursor budget even when valid CLI arguments
and output fit their own budgets. The service then preserves useful rows and
bounds, emits CURSOR_LIMIT_EXCEEDED, and supplies no cursor or continuation hint.
The response is partial; --require-complete fails. It never rounds input bounds
to make a cursor fit or converts this acquisition into a late usage error.

Evidence: 39 native/mock captures and 109 restricted TeamCity 2026.2 captures,
plus deterministic precision/cursor/page tests and released-native/live CLI
checks in both serializers. Historical wire observations are preserved.
At this checkpoint, missing nextHref under lookupLimit still did not prove
collection exhaustion. The subsequent [verified exhaustion contract](0015-verified-run-exhaustion.md) closes that acceptance gate for the exact tested server build. See [official locator documentation](https://www.jetbrains.com/help/teamcity/rest/get-build-details.html).
