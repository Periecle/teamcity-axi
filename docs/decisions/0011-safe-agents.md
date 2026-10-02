# Safe scoped agent reads

`agent list` requires a pool ID or validated job/project selection. A selected job
is read exactly, its owning project is asserted, and current trusted project
policy is checked before agent metadata is requested. Project-only selection
reads and admits that exact project. A pool-only selection is an explicit shared
agent scope; it does not authorize project or build metadata. `agent view` reads
one exact numeric agent ID and preserves any supplied job/project/pool assertions.
An unrelated returned agent ID or a conflicting reported pool fails closed.

Connectivity, enablement and authorization are independent nullable booleans.
Missing states never become false or an aggregate readiness claim. Public fields
are limited to exact ID, name, these states, safe pool ID/name and an optional
active-execution pointer. Parameters, environment, hosts, remote execution,
authorization controls and reboot operations are absent from requested fields
and public DTOs. Pool ID zero is valid; other numeric identities must be positive
safe integers. Invalid Unicode and control characters cannot change identity
spelling during sanitation.

The official [AgentLocator reference](https://www.jetbrains.com/help/teamcity/rest/agentlocator.html)
and [agent guide](https://www.jetbrains.com/help/teamcity/rest/manage-agents.html)
document compatibility and pool filtering. Current docs were fetched through
Context7 and checked against official references. Actual restricted TeamCity
2026.2 reads verified `compatible:(buildType:(id:...))` and project-selected build
types, count/start/lookupLimit, continuation, positive and empty pages and foreign
project denial. With a job selected, one compatibility filter is used after exact
job/project preflight; duplicate compatibility dimensions are rejected by the
server. List reads use `defaultFilter:false` to avoid silently excluding
unauthorized/disconnected/disabled agents. Exact scoped detail rejects that
dimension, so detail locators omit it.

The controlled test agent was moved, while idle, from Default into a dedicated
pool assigned to the synthetic project. Its restricted project-view reader can
read the agent and compatibility pages but still receives 404 when resolving the
pool in a locator. That is an unavailable scope, not a zero-agent result. Product
code never retries with broader credentials or falls back to an unscoped list.
Positive pool listing is verified through native/mock contracts; positive live
pool access with an appropriate read-only identity remains a certification gate.
The existing test reader's permissions were not broadened.

The official [Agent model](https://www.jetbrains.com/help/teamcity/rest/agent.html)
defines the `build` pointer. Reported pointers require a consistent exact run/job/
project tuple. A pointer outside the selected project is withheld with a
limitation. For unscoped or pool reads, current project policy must admit its
owner before it can be public. Optional policy denial or budget failure retains
safe agent metadata with an unavailable pointer; invocation interruption is
rethrown and preserves exit 130. Pointer retrieval hints carry exact project/job
assertions. No parameters or full build objects are exposed.

The live idle fixture omits `build`; it does not establish that omission means
idle. The public pointer is null with state `not_reported` and an explicit
limitation. Only explicit upstream null yields `idle`; a reported pointer yields
`reported`, and a withheld pointer yields `unavailable`. Positive live active-run
and explicit-idle shapes remain gates, with native/mock coverage in place.

Pages default to 20 agents, maximum 100, with a 5,000-position lookup. Duplicate
identities and malformed collections are rejected. Totals and absent bounded
continuation remain unknown, including empty pages. Unsafe continuation preserves
valid rows. Approved offsets are reconstructed against the frozen target; no
arbitrary URL is followed. Agent states and compatibility may change between
pages, so consistency is best effort. Cursors bind server alias/URL, exact
resolved scopes, page size and current project policy. A single clock value
decides expiry and encoding; a cursor expiring during acquisition preserves rows
with an explicit limitation.

Both serializers use measured UTF-8 output bounds. Required metadata is not
silently dropped to fit. Missing-state diagnostics are grouped across hundred-row
pages, and invocation deadline/concurrency/eight-child limits are shared. Typed
hints preserve pool/job/project scope; no-hints and require-complete apply to
both commands. Focused adapter, executable, native/mock and live tests support
this slice. Broader agent permission/availability certification, checkout status,
watch, generated portable skill/help and evaluation/release gates remain required.
