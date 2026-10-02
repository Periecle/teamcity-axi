# Fixture provenance

`mock-server.mjs` contains invented, sanitized public sample data shaped from
the official v1.5.0 CLI source. It is not a captured private/live server response.
`native-operations.mjs` lists the exact released-binary argv exercised against
that local mock. `native-v1.5.0/contract.json` records resulting native stdout,
stderr, exit codes and mock request method/path/query/authentication-presence.

The recorder checks the binary SHA-256 and version against
`docs/compatibility.json` before attributing observations to the pinned release.
Temporary paths and mock port are normalized; response Date is fixed. The mock
token is only an inert fixture sentinel and never a usable credential. Tests
assert it is absent from captured output. Fixture content is original project
test data; official binaries/source are downloaded only into temporary storage
and are not bundled in the package.

```sh
TEAMCITY_AXI_TEST_BINARY=/absolute/path/teamcity npm run fixtures:record
TEAMCITY_AXI_TEST_BINARY=/absolute/path/teamcity npm run test:real-cli
```

The raw operations verify CLI request construction and response transport. They
do not prove that a real server implements locator/field/continuation semantics.
No minimum supported server version is inferred from them.

`teamcity-2026.2-native-1.5.0/contract.json` contains 56 captured reads through
the same released Linux x64 CLI against actual TeamCity 2026.2 build 238924.
Its projects, builds, tests and user are original synthetic test data. The
permission inventory proves project viewing without build-run permission;
an exact foreign-project build read returns 403. Known test credentials,
Set-Cookie headers and session path identifiers are redacted before writing.
New context/doctor contracts include exact encoded project/root metadata, the
current user ID, a one-row job/project sample page and native `--tail 1` returning
two entries. The public adapter enforces the requested log cap and exposes only
an identity fingerprint.
Independent evidence captures include bounded count/start problem/test pages,
unmuted/muted failure filters and exact occurrence expansion with duration and
string test definition IDs. Encoding an entire compound occurrence locator as
base64 is rejected by the server; adapters reconstruct structural syntax from
validated numeric components. Actual muted results are empty; positive muted
failures and duplicate names are covered by invented mock data and remain live
fixture gates.
JSON payloads are decoded before redaction so JSON escaping cannot hide a
known credential. Unparseable payloads are replaced with an explicit omission
marker; native stderr is retained only as a presence marker. No arbitrary raw
native diagnostics are published by the live recorder.
Image digests describe the provisioned temporary containers, rather than an
attestation obtained from the REST endpoint. The user approved the temporary
localhost test server's license before first-start setup. Administrator setup
and fixture mutations are separate from the restricted capture identity.

An invalid synthetic token yields 401; this does not prove actual token expiry.
The capture includes the rejected dependency subresource and the supported
immediate-dependency locator separately. Positive change and immediate-dependency captures verify three contextual commits,
optional files, root-to-prerequisite direction and scoped counts one/zero. Positive
queue pages now preserve exact execution/job/project identity and provider wait
reasons. Shared-DAG/cycle traversal, multi-root identity, branch escaping,
live context prefixes and scoped agent/pool behavior remain open gates.

Live tooling is test-only, requires its pinned binary and a private, owned
regular JSON credential file with `serverUrl` and `token` (optional `password`
is also redacted), and fails when inputs are absent. Keep that file outside the
repository. Replaying fixtures is included in `npm test`; live execution is
explicit and never silently skipped:

```sh
TEAMCITY_AXI_TEST_BINARY=/absolute/path/teamcity \
TEAMCITY_AXI_LIVE_CREDENTIALS=/private/path/reader.json npm run test:live
TEAMCITY_AXI_TEST_BINARY=/absolute/path/teamcity \
TEAMCITY_AXI_LIVE_CREDENTIALS=/private/path/reader.json \
TEAMCITY_AXI_LIVE_OUTPUT=/tmp/new-sanitized-contract.json npm run fixtures:record-live
```

The recorder refuses an existing output file and checks the captured server
build, exact identities, permission inventory and expected denial/error codes
before attributing the new capture to this controlled fixture. Review any new
capture before replacing checked-in evidence. None of this certifies the full
read-only product.

The controlled VCS/dependency fixture and its exact run inventory are documented
in [SANDBOX.md](SANDBOX.md). Positive adapter contracts do not certify graph
traversal or failure investigation.

Four additional restricted job-page observations verify encoded direct-project
scope, bounded count/start/lookupLimit, nextHref, an empty bounded page and
foreign-project denial. The recorder validates exact job identities before
accepting repeated captures. The native/mock corpus also records bounded job
pages and exact job metadata; wrapper tests cover unsafe continuations, local
cursor rejection, nullable paused state, bounded diagnostics and dash-leading
retrieval identities. These focused cases do not certify remaining read services.

Seven additional actual queue observations verify project, job and intersected
scopes, positive continuation, an empty bounded page, foreign-project 403 and exact
queued-run detail. The recorder binds them to the controlled execution IDs 10/11
and owning jobs; it rejects moved identities or changed lifecycle. The queued
detail omits status, which the wrapper preserves as an unknown result. The
native/mock corpus now has 30 observations, including the bounded queue GET.
Negative executable cases reject malformed identity Unicode/controls, unsafe
continuations, unsupported reads and oversized input/output without false empty
success. Broader live state transitions and queue scope certification remain open.
