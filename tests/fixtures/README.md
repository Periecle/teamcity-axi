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
