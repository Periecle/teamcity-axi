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

`teamcity-2026.2-native-1.5.0/contract.json` contains 31 captured reads through
the same released Linux x64 CLI against actual TeamCity 2026.2 build 238924.
Its projects, builds, tests and user are original synthetic test data. The
permission inventory proves project viewing without build-run permission;
an exact foreign-project build read returns 403. Known test credentials,
Set-Cookie headers and session path identifiers are redacted before writing.
New context/doctor contracts include exact encoded project/root metadata, the
current user ID, a one-row job/project sample page and native `--tail 1` returning
two entries. The public adapter enforces the requested log cap and exposes only
an identity fingerprint.
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
immediate-dependency locator separately. Changes, queue and dependency pages
are currently empty. Positive graph direction, VCS identity, branch escaping,
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
