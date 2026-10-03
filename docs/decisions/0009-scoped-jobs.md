# Scoped job reads

`job list` requires an exact selected project from a flag or validated repository
context. It never enumerates all build configurations. The selected project is
read and admitted by the current trusted subtree policy before the job page is
requested. `job view` reads one exact job ID, checks any explicit project assertion,
and admits its observed project through the same policy.

Both commands expose only ID, name, project ID and nullable paused state. Parameters,
settings, build steps and credentials are absent from the requested fields and
public DTO. Missing paused state remains null with an explicit limitation; it
never becomes false. Repeated missing-state diagnostics are grouped so a full
hundred-row page stays within the envelope's diagnostic ceiling.

The list uses the direct-parent `project` locator, not descendant membership.
The official [BuildTypeLocator reference](https://www.jetbrains.com/help/teamcity/rest/buildtypelocator.html)
and [build configuration guide](https://www.jetbrains.com/help/teamcity/rest/manage-build-configuration-details.html)
distinguish `project` from `affectedProject`. Current documentation was retrieved
through Context7; actual TeamCity 2026.2 reads independently verified the selected
project literal encoding, count/start/lookupLimit dimensions, safe field selection,
continuation and foreign-project denial. These observations do not establish
support for untested versions or platforms.

The default page size is 20, maximum 100, with a 5,000-position bounded lookup.
Every row must belong to the exact selected project, and duplicate IDs or invalid
metadata fail closed. Useful rows survive an unsafe continuation. The wrapper
reconstructs approved offsets rather than following a server-supplied URL.
Collection count is the returned page count. Missing nextHref under the bounded
lookup leaves hasMore and total unknown, including empty pages; it cannot prove
that a project has no jobs.

Cursors bind the command, frozen server alias and URL, exact project, page size
and current trusted project policy. Invalid, expired or mismatched cursors fail
before a native child launches. Each continuation repeats project observation
and policy admission, and offset consistency remains best effort. Typed retrieval
hints preserve exact project/job scope. Equals-form option values and an option
terminator protect dash-leading identities from being reinterpreted as flags.

The commands share the eight-child simple-read ceiling and invocation deadline.
Both serializers enforce their actual UTF-8 byte ceiling. Required metadata is
not silently removed to fit; an explicit bounded output error is returned.
No-hints and require-complete flags apply to both commands. Focused unit,
released-native/mock and restricted live tests verify this slice. Broader live
policy/topology and paused-state cases, the remaining queue/agent/status/watch
services, evaluation and release certification were still open at that original
implementation checkpoint. Current Go service coverage and remaining
compatibility limits are tracked in [implementation status](../STATUS.md).
