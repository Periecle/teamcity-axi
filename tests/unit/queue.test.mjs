import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { normalizeQueuePage, queueRequest } from '../../dist/adapter/queue.js';
import { NativeTeamCityReader } from '../../dist/adapter/reader.js';
import { parseRaw } from '../../dist/adapter/raw.js';
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
const query = { projectId: 'AxiContract', count: 1, start: 0, scanLimit: 5000 };
const server = 'http://127.0.0.1:32768';
test('actual positive queue pages preserve execution/project/job identity, wait text, UTC time and continuation', () => {
  const page = normalizeQueuePage(body('queue-project-positive'), query, server, []);
  assert.deepEqual(page.items, [
    {
      id: '10',
      jobId: 'AxiContract_QueueA',
      state: 'queued',
      branch: null,
      queuedAt: '2026-10-02T15:41:26.000Z',
      waitReason: 'There are no idle compatible agents which can run this build',
    },
  ]);
  assert.equal(page.hasMore, true);
  assert.equal(page.position, 1);
  assert.deepEqual(page.limitations, []);
  const next = normalizeQueuePage(body('queue-project-next'), { ...query, start: 1 }, server, []);
  assert.equal(next.items[0].id, '11');
  assert.equal(next.position, 2);
  const job = normalizeQueuePage(
    body('queue-intersection'),
    { ...query, count: 20, jobId: 'AxiContract_QueueA' },
    server,
    [],
  );
  assert.equal(job.items[0].id, '10');
  assert.equal(job.hasMore, null);
  assert.ok(job.limitations.some((l) => l.code === 'SCAN_COVERAGE_UNKNOWN'));
});
test('missing optional queue fields remain unknown and unknown/changed lifecycle is explicit', () => {
  const build = body('queue-project-positive').build[0];
  const { waitReason, queuedDate, ...missing } = build;
  const page = normalizeQueuePage({ count: 1, build: [missing] }, query, server, []);
  assert.equal(page.items[0].waitReason, null);
  assert.equal(page.items[0].queuedAt, null);
  for (const state of ['future-state', 'running', 'finished']) {
    const changed = normalizeQueuePage(
      { count: 1, build: [{ ...build, state }] },
      query,
      server,
      [],
    );
    assert.equal(changed.items[0].state, state === 'future-state' ? 'unknown' : state);
    assert.ok(
      changed.limitations.some(
        (l) =>
          l.code === (state === 'future-state' ? 'UNKNOWN_QUEUE_STATE' : 'QUEUE_STATE_CHANGED'),
      ),
    );
  }
  const invalid = normalizeQueuePage(
    { count: 1, build: [{ ...build, queuedDate: '20260230T000000+0000' }] },
    query,
    server,
    [],
  );
  assert.equal(invalid.items[0].queuedAt, null);
  assert.ok(
    invalid.limitations.some((l) => l.code === 'INVALID_TIMESTAMP' && l.source === 'queue'),
  );
  const secret = 'private-credential';
  const redacted = normalizeQueuePage(
    {
      count: 1,
      build: [{ ...build, state: secret, waitReason: secret + '\x1b[31m', branchName: secret }],
    },
    query,
    server,
    [secret],
  );
  assert.equal(redacted.items[0].rawState, '[REDACTED]');
  assert.equal(redacted.items[0].waitReason, '[REDACTED]');
  assert.equal(redacted.items[0].branch, '[REDACTED]');
});
test('scoped queue adapters reject missing/conflicting/foreign identity, duplicate rows and malformed collections', () => {
  const build = body('queue-project-positive').build[0];
  for (const patch of [
    { id: 9007199254740992 },
    { id: '0' },
    { buildTypeId: 'Foreign' },
    { buildType: { id: 'Other', projectId: 'AxiContract' } },
    { buildType: { id: build.buildTypeId, projectId: 'Forbidden' } },
    { state: null },
    { waitReason: 42 },
    { branchName: 42 },
  ])
    assert.throws(() =>
      normalizeQueuePage(
        { count: 1, build: [{ ...build, ...patch }] },
        { ...query, jobId: build.buildTypeId },
        server,
        [],
      ),
    );
  for (const page of [
    { count: 0, build: [build] },
    { count: 1, build: null },
    { count: 2, build: [build, build] },
  ])
    assert.throws(() => normalizeQueuePage(page, { ...query, count: 2 }, server, []));
  for (const malformed of ['bad\ud800job', 'bad\u0085job', 'bad\u202ejob', 'bad\u2066job']) {
    for (const row of [
      { ...build, buildTypeId: malformed, buildType: { id: malformed, projectId: 'AxiContract' } },
      { ...build, buildType: { id: build.buildTypeId, projectId: malformed } },
    ])
      assert.throws(
        () => normalizeQueuePage({ count: 1, build: [row] }, query, server, []),
        (e) => e.code === 'UPSTREAM_SCHEMA_MISMATCH',
      );
    for (const field of ['jobId', 'projectId'])
      assert.throws(
        () => queueRequest({ ...query, [field]: malformed }),
        (e) => e.code === 'USAGE_ERROR' && e.exitCode === 2,
      );
  }
  for (const patch of [
    { projectId: undefined },
    { projectId: 'x'.repeat(257) },
    { jobId: 'bad\njob' },
    { count: 101 },
    { start: 5000 },
    { scanLimit: 5001 },
  ])
    assert.throws(
      () => queueRequest({ ...query, ...patch }),
      (e) => e.exitCode === 2,
    );
});
test('queue boundaries retain useful rows, never invent empty exhaustion, and bound hundred-row diagnostics', () => {
  const original = body('queue-project-positive');
  for (const nextHref of [
    'https://attacker.invalid/app/rest/buildQueue',
    original.nextHref.replace('5000', '10000'),
    original.nextHref.replace('QXhpQ29udHJhY3Q', 'Rm9yYmlkZGVu'),
    original.nextHref.replace('start:1', 'start:0'),
  ]) {
    const page = normalizeQueuePage({ ...original, nextHref }, query, server, []);
    assert.equal(page.items.length, 1);
    assert.equal(page.position, null);
    assert.equal(page.hasMore, null);
    assert.ok(page.limitations.some((l) => l.code === 'UNSAFE_CONTINUATION'));
  }
  const empty = normalizeQueuePage(
    body('queue-project-empty'),
    { ...query, start: 4999 },
    server,
    [],
  );
  assert.deepEqual(empty.items, []);
  assert.equal(empty.hasMore, null);
  const many = normalizeQueuePage(
    {
      count: 100,
      build: Array.from({ length: 100 }, (_, i) => ({
        ...original.build[0],
        id: i + 1,
        state: 'future-state',
        queuedDate: 'invalid',
      })),
    },
    { ...query, count: 100 },
    server,
    [],
  );
  assert.equal(many.items.length, 100);
  assert.equal(many.limitations.filter((l) => l.code === 'UNKNOWN_QUEUE_STATE').length, 1);
  assert.equal(many.limitations.filter((l) => l.code === 'INVALID_TIMESTAMP').length, 1);
});
test('native queue reader preserves recorded GET input and independent permission errors', async () => {
  let operation;
  const reader = new NativeTeamCityReader(
    {
      execute: async (value) => {
        operation = value;
        return raw('queue-project-positive');
      },
    },
    server,
  );
  const result = await reader.listQueue(query, { deadline: Date.now() + 1000 });
  assert.equal(result.state, 'available');
  assert.equal(result.provenance.projectId, 'AxiContract');
  assert.equal(operation.path, queueRequest(query).path);
  const denied = new NativeTeamCityReader(
    { execute: async () => raw('queue-project-denied') },
    server,
  );
  const unavailable = await denied.listQueue(query, { deadline: Date.now() + 1000 });
  assert.equal(unavailable.state, 'unavailable');
  assert.equal(unavailable.error.code, 'PERMISSION_DENIED');
});
