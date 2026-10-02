import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { normalizeJob, normalizeJobPage, jobRequest } from '../../dist/adapter/jobs.js';
import { NativeTeamCityReader } from '../../dist/adapter/reader.js';
import { parseRaw } from '../../dist/adapter/raw.js';
const contract = JSON.parse(
  readFileSync('tests/fixtures/teamcity-2026.2-native-1.5.0/contract.json', 'utf8'),
);
const captured = (name) => {
  const r = contract.records[name];
  return {
    stdout: Buffer.from(r.stdout),
    stderr: Buffer.from(r.stderr),
    exitCode: r.code,
    signal: r.signal,
  };
};
const body = (name) => parseRaw(captured(name)).body;
const query = { projectId: 'AxiContract', count: 1, start: 0, scanLimit: 5000 };
const server = 'http://127.0.0.1:32768';
test('actual scoped job pages retain exact metadata, continuation and unknown empty-page exhaustion', () => {
  const request = jobRequest(query);
  assert.equal(
    new URL(request.path, server).searchParams.get('locator'),
    'project:(id:($base64:QXhpQ29udHJhY3Q)),count:1,start:0,lookupLimit:5000',
  );
  const first = normalizeJobPage(body('bounded-jobs'), query, server, []);
  assert.deepEqual(first.items, [
    { id: 'AxiContract_Fail', name: 'AxiContract_Fail', projectId: 'AxiContract', paused: false },
  ]);
  assert.equal(first.position, 1);
  assert.equal(first.hasMore, true);
  assert.deepEqual(first.limitations, []);
  const empty = normalizeJobPage(body('bounded-jobs-empty'), { ...query, start: 4999 }, server, []);
  assert.deepEqual(empty.items, []);
  assert.equal(empty.hasMore, null);
  assert.equal(empty.position, null);
  assert.ok(empty.limitations.some((l) => l.code === 'SCAN_COVERAGE_UNKNOWN'));
});
test('job adapters reject foreign/duplicate/malformed rows and unsafe page bounds before exposing metadata', () => {
  const page = body('bounded-jobs');
  for (const patch of [
    { count: 2 },
    { count: 0 },
    { buildType: null },
    { buildType: [{ ...page.buildType[0], projectId: 'Forbidden' }] },
  ])
    assert.throws(() => normalizeJobPage({ ...page, ...patch }, query, server, []));
  assert.throws(() =>
    normalizeJobPage(
      { count: 2, buildType: [page.buildType[0], page.buildType[0]] },
      { ...query, count: 2 },
      server,
      [],
    ),
  );
  for (const patch of [
    { count: 101 },
    { start: 5000 },
    { scanLimit: 5001 },
    { projectId: '' },
    { projectId: 'x'.repeat(257) },
  ])
    assert.throws(
      () => jobRequest({ ...query, ...patch }),
      (e) => e.code === 'USAGE_ERROR',
    );
  for (const patch of [
    { id: null },
    { name: 42 },
    { paused: 'false' },
    { paused: null },
    { projectId: '' },
  ])
    assert.throws(() => normalizeJob({ ...page.buildType[0], ...patch }, []));
});
test('unsafe job continuations retain useful scoped rows and unavailable paused metadata stays unknown', () => {
  const page = body('bounded-jobs');
  for (const nextHref of [
    'https://attacker.invalid/app/rest/buildTypes',
    page.nextHref.replace('5000', '10000'),
    page.nextHref.replace('QXhpQ29udHJhY3Q', 'Rm9yYmlkZGVu'),
    page.nextHref.replace('start:1', 'start:0'),
  ]) {
    const normalized = normalizeJobPage({ ...page, nextHref }, query, server, []);
    assert.equal(normalized.items.length, 1);
    assert.equal(normalized.hasMore, null);
    assert.equal(normalized.position, null);
    assert.ok(normalized.limitations.some((l) => l.code === 'UNSAFE_CONTINUATION'));
  }
  const { paused, ...job } = page.buildType[0];
  const normalized = normalizeJobPage({ count: 1, buildType: [job] }, query, server, []);
  assert.equal(normalized.items[0].paused, null);
  assert.ok(normalized.limitations.some((l) => l.code === 'JOB_PAUSED_UNAVAILABLE'));
  const many = normalizeJobPage(
    { count: 100, buildType: Array.from({ length: 100 }, (_, i) => ({ ...job, id: `Job${i}` })) },
    { ...query, count: 100 },
    server,
    [],
  );
  assert.equal(many.items.length, 100);
  assert.equal(many.limitations.filter((l) => l.code === 'JOB_PAUSED_UNAVAILABLE').length, 1);
  const secret = 'credential-canary';
  assert.equal(normalizeJob({ ...job, name: secret + '\x1b[31m' }, [secret]).name, '[REDACTED]');
  assert.equal(normalizeJob({ ...job, parameters: { secret } }, []).parameters, undefined);
});
test('native job methods preserve actual restricted DTOs and errors without manufacturing empty success', async () => {
  let operation;
  const reader = new NativeTeamCityReader(
    {
      execute: async (value) => {
        operation = value;
        return captured('bounded-jobs');
      },
    },
    server,
  );
  const read = await reader.listJobs(query, { deadline: Date.now() + 1000 });
  assert.equal(read.state, 'available');
  assert.equal(read.provenance.projectId, 'AxiContract');
  assert.equal(operation.path, jobRequest(query).path);
  const denied = new NativeTeamCityReader(
    { execute: async () => captured('bounded-jobs-denied') },
    server,
  );
  const failure = await denied.listJobs(query, { deadline: Date.now() + 1000 });
  assert.equal(failure.state, 'unavailable');
  assert.equal(failure.error.code, 'PERMISSION_DENIED');
  const exact = new NativeTeamCityReader({ execute: async () => captured('encoded-job') }, server);
  assert.equal(
    (await exact.getJob({ id: 'AxiContract_Fail' }, { deadline: Date.now() + 1000 })).value.paused,
    false,
  );
  assert.equal(
    (await exact.getJob({ id: 'Other' }, { deadline: Date.now() + 1000 })).error.code,
    'CONTEXT_MISMATCH',
  );
});
