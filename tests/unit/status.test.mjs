import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { normalizeStatus, statusRequest } from '../../dist/adapter/status.js';
import { assessStatusJob } from '../../dist/planner/status.js';
import { parseRaw } from '../../dist/adapter/raw.js';
import { NativeTeamCityReader } from '../../dist/adapter/reader.js';
import { run } from '../fixtures/mock-server.mjs';

const contract = JSON.parse(
  readFileSync('tests/fixtures/teamcity-2026.2-native-1.5.0/contract.json', 'utf8'),
);
const capture = (name) => {
  const record = contract.records[name];
  return {
    stdout: Buffer.from(record.stdout),
    stderr: Buffer.from(record.stderr),
    exitCode: record.code,
    signal: record.signal,
  };
};
const body = (name) => parseRaw(capture(name)).body;
const server = 'http://127.0.0.1:32768';
const query = { jobIds: ['Payments_Build'], branch: 'feature/refund' };
const dto = (builds) => ({
  count: 1,
  buildType: [
    {
      id: 'Payments_Build',
      name: 'Build',
      projectId: 'Payments',
      paused: false,
      builds: { count: builds.length, build: builds },
    },
  ],
});
const normalized = (builds) => normalizeStatus(dto(builds), query, server, [])[0];
const revision = 'a'.repeat(40);
const root = 'Payments_Git';
const assess = (builds) => assessStatusJob('Payments_Build', normalized(builds), revision, root);

test('recorded bulk status preserves five exact jobs and queued lifecycles with bounded activity', () => {
  const input = body('status-five-outcomes');
  const jobs = input.buildType.map((job) => job.id);
  const snapshots = normalizeStatus(input, { jobIds: jobs }, server, []);
  assert.deepEqual(
    snapshots.map((snapshot) => snapshot.job.id),
    jobs,
  );
  assert.deepEqual(
    snapshots
      .filter((snapshot) => snapshot.page.runs[0].state === 'queued')
      .map((snapshot) => snapshot.page.runs[0].id),
    ['10', '11'],
  );
  const vcs = snapshots.find((snapshot) => snapshot.job.id === 'AxiContract_Vcs');
  const selected = assessStatusJob(
    vcs.job.id,
    vcs,
    vcs.page.runs[0].revisions[0].revision,
    'AxiContract_Git',
  );
  assert.equal(selected.match, 'exact');
  assert.equal(selected.assessment, 'passed');
  assert.equal(vcs.page.hasMore, null);
});

test('actual production locators remain identical to recorded unpadded and exact-branch requests', () => {
  assert.equal(
    statusRequest({ jobIds: ['AxiContract_Vcs'] }),
    contract.records['status-one-outcomes'].args[1],
  );
  assert.equal(
    statusRequest({ jobIds: ['AxiContract_Vcs'], branch: 'feature/status,project:AxiDenied' }),
    contract.records['status-branch-outcomes'].args[1],
  );
  const page = normalizeStatus(
    body('status-branch-outcomes'),
    { jobIds: ['AxiContract_Vcs'], branch: 'feature/status,project:AxiDenied' },
    server,
    [],
  );
  assert.equal(page[0].page.runs.length, 0);
  for (const name of ['status-missing-job-outcomes', 'status-denied-outcomes'])
    assert.deepEqual(
      normalizeStatus(body(name), { jobIds: ['AxiContract_Missing'] }, server, []),
      [],
    );
});

test('red status is a successful observation; newer exact red never falls back to green', () => {
  const selected = assess([run, { ...run, id: 482192, status: 'SUCCESS' }]);
  assert.equal(selected.match, 'exact');
  assert.equal(selected.assessment, 'failed');
  assert.equal(selected.run.id, '482193');
  assert.equal(assess([{ ...run, status: 'SUCCESS' }]).assessment, 'passed');
});

test('newest exact running and queued runs block older completed green', () => {
  for (const state of ['queued', 'running']) {
    const active = { ...run, state, status: state === 'queued' ? undefined : 'SUCCESS' };
    const selected = assess([active, { ...run, id: 482192, status: 'SUCCESS' }]);
    assert.equal(selected.assessment, 'in_progress');
    assert.equal(selected.run.id, '482193');
  }
});

test('newer different checkout allows an older exact run; newer unknown revision never certifies it', () => {
  const older = { ...run, id: 482192, status: 'SUCCESS' };
  const different = {
    ...run,
    revisions: {
      revision: [{ version: 'b'.repeat(40), 'vcs-root-instance': { 'vcs-root-id': root } }],
    },
  };
  assert.equal(assess([different, older]).assessment, 'passed');
  const unknown = assess([{ ...run, revisions: { revision: [] } }, older]);
  assert.equal(unknown.run.id, '482192');
  assert.equal(unknown.match, 'exact');
  assert.equal(unknown.assessment, 'unverified');
  assert.equal(unknown.limitations[0].code, 'NEWER_REVISION_UNVERIFIED');
});

test('missing roots, jobs and bounded empty candidates never prove global absence or success', () => {
  for (const value of [
    assess([]),
    assess([{ ...run, revisions: { revision: [] } }]),
    assessStatusJob('Missing', undefined, revision, root),
  ]) {
    assert.equal(value.match, 'unverified');
    assert.equal(value.assessment, 'unverified');
    assert.ok(value.limitations.length);
  }
  const stale = assess([
    {
      ...run,
      revisions: {
        revision: [{ version: 'b'.repeat(40), 'vcs-root-instance': { 'vcs-root-id': root } }],
      },
    },
  ]);
  assert.equal(stale.match, 'different');
  assert.equal(stale.assessment, 'unverified');
});

test('all other root identities are retained; personal and missing personal state cannot certify checkout', () => {
  const multi = {
    ...run,
    status: 'SUCCESS',
    revisions: {
      revision: [
        ...run.revisions.revision,
        { version: 'b'.repeat(40), 'vcs-root-instance': { 'vcs-root-id': 'Other_Git' } },
      ],
    },
  };
  const selected = assess([multi]);
  assert.equal(selected.match, 'exact');
  assert.equal(selected.run.revisions.length, 2);
  assert.equal(selected.assessment, 'unverified');
  assert.equal(selected.limitations[0].code, 'OTHER_ROOTS_UNVERIFIED');
  for (const personal of [true, undefined])
    assert.equal(assess([{ ...run, status: 'SUCCESS', personal }]).assessment, 'unverified');
});

test('status rejects foreign identities, duplicates, reordered candidates and unbounded DTOs', () => {
  const invalid = [
    dto([{ ...run, buildTypeId: 'Foreign' }]),
    dto([{ ...run, buildType: { id: run.buildTypeId, projectId: 'Foreign' } }]),
    dto([{ ...run, branchName: 'Foreign' }]),
    dto([run, run]),
    dto([{ ...run, id: 482192 }, run]),
    dto([
      { ...run, revisions: { revision: [...run.revisions.revision, ...run.revisions.revision] } },
    ]),
    dto(Array.from({ length: 21 }, (_, index) => ({ ...run, id: run.id - index }))),
    { count: 2, buildType: [dto([]).buildType[0], dto([]).buildType[0]] },
    dto([]),
  ];
  delete invalid.at(-1).buildType[0].builds;
  for (const input of invalid) assert.throws(() => normalizeStatus(input, query, server, []));
});

test('status selectors fail locally and cannot inject branch or job locator dimensions', () => {
  assert.doesNotThrow(() => statusRequest({ jobIds: ['Job'], branch: 'b'.repeat(300) }));
  for (const invalid of [
    { jobIds: [] },
    { jobIds: ['A', 'A'] },
    { jobIds: ['A', 'B', 'C', 'D', 'E', 'F'] },
    { jobIds: ['A\u202e'] },
    { jobIds: ['A'], branch: '\ud800' },
  ])
    assert.throws(
      () => statusRequest(invalid),
      (error) => error.code === 'USAGE_ERROR',
    );
  const url = new URL(
    statusRequest({ jobIds: ['job,project:attacker'], branch: 'x,state:finished' }),
    server,
  );
  assert.ok(!url.searchParams.get('locator').includes('attacker'));
  assert.ok(!url.searchParams.get('fields').includes('x,state:finished'));
});

test('status reader preserves recorded operation and transport failures without manufacturing missing jobs', async () => {
  const reader = new NativeTeamCityReader(
    {
      execute: async (operation) => {
        assert.equal(operation.path, contract.records['status-one-outcomes'].args[1]);
        return capture('status-one-outcomes');
      },
    },
    server,
  );
  const read = await reader.readStatus(
    { jobIds: ['AxiContract_Vcs'] },
    { deadline: Date.now() + 1000 },
  );
  assert.equal(read.state, 'available');
  assert.equal(read.provenance.operation, 'status.snapshot');
  const denied = new NativeTeamCityReader({ execute: async () => capture('denied') }, server);
  assert.equal(
    (await denied.readStatus({ jobIds: ['AxiContract_Vcs'] }, { deadline: Date.now() + 1000 }))
      .error.code,
    'PERMISSION_DENIED',
  );
});
