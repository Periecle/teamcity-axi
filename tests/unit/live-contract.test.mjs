import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { parseRaw } from '../../dist/adapter/raw.js';
import { normalizeRun } from '../../dist/adapter/run.js';
import { nextPosition } from '../../dist/adapter/continuation.js';
const contract = JSON.parse(
  readFileSync('tests/fixtures/teamcity-2026.2-native-1.5.0/contract.json', 'utf8'),
);
const raw = (name) => {
  const r = contract.records[name];
  return {
    stdout: Buffer.from(r.stdout),
    stderr: Buffer.from(r.stderr),
    exitCode: r.code,
    signal: r.signal,
  };
};
const body = (name) => parseRaw(raw(name)).body;
test('live restricted identity and exact failed/green DTOs are preserved independently of HTTP failures', () => {
  assert.equal(body('server').buildNumber, contract.server.buildNumber);
  const permissions = body('permissions').permissionAssignment;
  assert.deepEqual(permissions.map((p) => p.permission.id).sort(), [
    'change_own_profile',
    'view_project',
    'view_project',
  ]);
  assert.ok(
    permissions
      .filter((p) => p.permission.id === 'view_project')
      .every((p) => !p.isGlobalScope && ['AxiContract', '_Root'].includes(p.project.id)),
  );
  for (const [name, id, result] of [
    ['outcome-normal-failed', '1', 'failure'],
    ['outcome-normal-green', '2', 'success'],
  ]) {
    const n = normalizeRun(body(name), 'http://127.0.0.1:32768');
    assert.equal(n.run.id, id);
    assert.equal(n.run.result, result);
    assert.equal(n.projectId, 'AxiContract');
    assert.deepEqual(n.run.revisions, []);
    assert.deepEqual(n.limitations, []);
  }
  for (const [name, code] of [
    ['denied', 'PERMISSION_DENIED'],
    ['missing', 'NOT_FOUND'],
    ['invalid-auth', 'AUTH_REQUIRED'],
    ['dependencies-unsupported', 'UPSTREAM_FAILURE'],
  ])
    assert.throws(
      () => body(name),
      (e) => e.code === code,
      name,
    );
});
test('live continuation reconstructs encoded scope and finish-time filters without forwarding hrefs', () => {
  for (const name of ['pages', 'encoded-page']) {
    const path = contract.records[name].args[1],
      query = new URL(path, 'http://fixture.invalid').searchParams;
    // Nested date conditions contain commas; preserve the adapter-owned pieces.
    const actualFilters =
      name === 'pages'
        ? ['buildType:(id:AxiContract_Fail)', 'defaultFilter:false']
        : [
            'buildType:(id:($base64:QXhpQ29udHJhY3RfRmFpbA))',
            'defaultFilter:false',
            'state:finished',
            'finishDate:(date:20261001T000000+0000,condition:after)',
            'finishDate:(date:20261003T000000+0000,condition:before)',
          ];
    assert.equal(
      nextPosition(body(name).nextHref, {
        serverUrl: 'http://127.0.0.1:32768',
        resource: 'builds',
        filters: actualFilters,
        fields: query.get('fields'),
        count: 1,
        start: 0,
        scanLimit: 5000,
      }),
      1,
    );
  }
  assert.deepEqual(body('empty-page'), { build: [], count: 0 });
});
test('live independent evidence and optional structured log have distinct native shapes', () => {
  assert.equal(body('problems').count, 3);
  assert.equal(body('tests').testOccurrence[0].id, 'build:(id:1),id:2000000000');
  assert.deepEqual(body('dependencies'), { build: [], count: 0 });
  assert.deepEqual(body('changes'), { change: [], count: 0 });
  assert.deepEqual(body('queue'), { build: [], count: 0 });
  assert.equal(body('agents').agent[0].pool.id, 0);
  // The native tail includes 21 entries for --tail 20 on this server. A public
  // adapter must enforce its own window rather than assuming an exact count.
  const log = JSON.parse(contract.records.log.stdout);
  assert.equal(log.run_id, '1');
  assert.equal(log.messages.length, 21);
  assert.equal(log.messages[0].id, 35);
  assert.match(log.messages[0].timestamp, /\+0000$/);
});
