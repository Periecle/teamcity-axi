# Bounded changes and immediate snapshot dependencies

Changes are contextual evidence associated with an exact run. They retain commit
ID, revision and VCS-root identity; they do not attribute a failure to a commit.
The service verifies the primary run and trusted project policy before querying
changes, and rejects roots absent from available run revision metadata. Missing
change-root metadata excludes that row with an explicit limitation and preserved
provider count. It never substitutes a default root.

Default messages show the first line, up to 200 code points. `--full` expands the
same bounded page, preserving server/run/job/project and cursor; text still has
an explicit hard ceiling and the global UTF-8 byte budget. File names are fetched
only with `--files`, normalized and redacted before output, and capped at 100 per
change. Returned/omitted counts and limitations disclose this display cap. The
transport's two-MiB capture ceiling also applies to requested file payloads;
oversized input returns an explicit error rather than streaming or silently
trimming a response. Projection retains mandatory commit and root identity.

Changes and dependencies use wrapper-owned count/start/lookupLimit selectors.
Continuation is validated and reconstructed, never passed through. Cursors bind
the frozen server, run, scope, trusted project roots and query dimensions,
including file fetching. Missing continuation does not establish exhaustion of a
bounded lookup.

Immediate snapshot dependencies use the build collection's
`snapshotDependency:(to:(id:...),recursive:false)` locator. An edge runs from the
selected run to a prerequisite. Actual positive restricted-identity reads on
TeamCity 2026.2 verify that direction; the alternate build subresource rejects
JSON. A separate exact-run read of `snapshot-dependencies(count)` distinguishes
zero prerequisites from an unexecuted or incomplete query. Graph traversal must
reconcile this count with acquired pages and preserve permission, budget and
state-change uncertainty before claiming complete expansion or a leaf. The count
primitive alone does not certify an investigation.

Contracts and optional file properties were checked against the official
[Change](https://www.jetbrains.com/help/teamcity/rest/change.html),
[FileChange](https://www.jetbrains.com/help/teamcity/rest/filechange.html) and
[build details](https://www.jetbrains.com/help/teamcity/rest/get-build-details.html)
documentation, then tested with the pinned native CLI. No minimum server version
is inferred from documentation or mocks.
