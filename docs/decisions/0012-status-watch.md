# Exact-checkout status and fixed-run watch

Status answers whether each required tracked job has a verified successful
execution for the current clean Git checkout. Watch answers whether one frozen
execution has reached a successful terminal outcome. They are separate assertions.

The status adapter selects up to five exact job IDs through a BuildTypeLocator
union. A bounded nested build projection includes all lifecycle states, selects
the resolved logical branch, returns at most twenty candidates per job, and uses
a lookup limit of 5,000. Native v1.5.0 and the restricted TeamCity 2026.2 sandbox
confirm this request shape. Recorded inputs and outputs are `status-one`,
`status-five`, `status-branch`, `status-missing-job`, and `status-denied` in the
live corpus, plus `status-snapshot` in the native/mock corpus.

Returned job/run/project/branch identities must agree. Repeated job/run/root IDs
and candidates outside descending run-ID order fail validation. Select the
newest exact selected-root candidate from the bounded window; a newer different
revision does not replace it. A newer candidate with unknown selected-root
coverage prevents a passing assertion. Queued, running, and red exact candidates
cannot be replaced by older green executions. Other root identities are retained,
and unverified multi-root coverage cannot certify the local checkout. Personal
or unspecified checkout state is also unverified.

Every binding job remains required even when the home display omits jobs beyond
five. Missing/denied jobs can be omitted by the provider and therefore remain
unavailable; an empty union is not proof of project-wide absence. Nested provider
continuations are checked for scope/bounds and never followed. No continuation
does not prove history exhaustion. Activity counts refer to returned candidates.

The shared status budget is five seconds, six child launches including the
native probe, concurrency two, a 1 MiB child stdout capture, and 6 KiB rendered
output. Read the bulk snapshot before project ancestry so primary acquisition
has reserved capacity. Only policy-admitted snapshots reach output. The bulk
read timestamp, rather than the later policy timestamp, describes observation.

Watch reuses the recorded exact-run adapter and freezes server/run ID throughout
the loop. Each observation reasserts optional requested job/project scope and
trusted project policy. The final context describes the observed execution,
not the caller's unrelated branch or tracked-job set. Keep the latest verified
source limitations when a later read is unavailable. Optional summary text is
outside the compact watch projection.

Watch defaults to ten-second intervals (minimum five), a 120-second deadline,
32 child launches including the probe, concurrency one, 1 MiB child capture,
and an 8 KiB final document. Deadline expiration preserves the latest observation
as partial. Missing, inaccessible, and other unavailable reads remain distinct.
SIGINT/SIGTERM abort sleep and terminate children without changing the TeamCity
execution. Ordinary completed-red observations exit zero; `--check` requires a
terminal success. `--require-complete` rejects a partial status or watch result.

Focused unit, native/mock, and live evidence proves the implemented contracts.
Broader live multi-root/branch/lifecycle cases and release evaluation remain
separate acceptance gates.
