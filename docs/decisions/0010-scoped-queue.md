# Scoped queue reads

`queue list` requires an exact selected job or project. A job-only selection first
reads that exact job and derives its owning project. An explicit project assertion
must match; the current trusted project policy admits the observed scope before
any queue page is read. Project-only selection reads that exact project. No global
queue enumeration is provided.

The queue projection contains execution ID, job ID, lifecycle, optional branch,
queued timestamp and provider wait reason. Nested job/project identities establish
scope and must agree with the selected filters. Duplicate IDs, conflicting scope,
invalid numeric execution IDs, ill-formed Unicode and identity controls fail
closed. Display sanitation must never change the spelling of an accepted identity.
Absent branch, timestamp or wait reason stays null; the wrapper never invents a
waiting cause. Text is sanitized before public rendering. Unknown lifecycle or a
row that became running/finished remains useful but explicitly partial.

The official [BuildQueueLocator reference](https://www.jetbrains.com/help/teamcity/rest/buildqueuelocator.html)
describes the project and build-type dimensions. Current documentation was fetched
through Context7. Actual restricted TeamCity 2026.2 reads separately verified
encoded job, project and intersected locators, selected fields, count/start/
lookupLimit, positive queue pages, continuation and foreign-project denial.
Two synthetic jobs use an impossible exact agent-name requirement to retain
queued executions without altering agent availability. Administrator fixture
setup is separate from product code and the restricted reader.

The default page size is 20, maximum 100, with a 5,000-position lookup bound. The
returned count is a page count; totals remain unknown. Missing continuation,
including an empty bounded page, does not prove queue exhaustion. Unsafe
continuation preserves valid rows with an explicit limitation. Approved offsets
are reconstructed against the frozen server rather than following arbitrary
URLs. Offset pagination is best effort because queue entries can move or start
between observations.

Cursors bind the command, server alias/URL, exact resolved filters, page size and
current project policy. Project-scoped mismatches fail locally; job-only cursors
also bind the project freshly observed from the exact job. Every continuation
repeats policy admission. Hints either continue the same query or observe an
exact queued execution with explicit job/project assertions. Equals-form values
preserve dash-leading scope identities.

The positive live queued-run detail has lifecycle `queued` and omits `status`.
That recorded shape normalizes to result `unknown`, null raw status and an explicit
`RESULT_UNAVAILABLE` limitation. Missing results on running/finished runs and
explicit null status remain schema errors. A queue read never establishes build
success or completion.

The service shares the eight-child simple-read ceiling and invocation deadline.
Both serializers enforce actual UTF-8 output bounds; required queue rows are not
silently removed to fit. Source diagnostics are grouped by code so a hundred-row
page cannot overflow the diagnostic collection. No-hints and require-complete
flags apply. Focused unit, native/mock and restricted live evidence proves this
slice. At that original implementation checkpoint, broader queue
lifecycle/permission/topology certification, agents, checkout status, watch and
evaluation/release gates were still open. Current Go service coverage and
remaining compatibility limits are tracked in [implementation status](../STATUS.md).
