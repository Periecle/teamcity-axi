import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { NativeTeamCityReader } from '../../dist/adapter/reader.js';
import { parseRaw } from '../../dist/adapter/raw.js';
import { DomainError } from '../../dist/domain/errors.js';

const contract = JSON.parse(
  readFileSync('tests/fixtures/teamcity-2026.2-native-1.5.0/contract.json', 'utf8'),
);
const captured = (record) => ({
  stdout: Buffer.from(record.stdout),
  stderr: Buffer.from(record.stderr),
  exitCode: record.code,
  signal: record.signal,
});
const query = {
  projectId: contract.fixture.projectId,
  state: 'finished',
  count: 20,
  start: 0,
  scanLimit: 5000,
};
const budget = { deadline: Date.now() + 30000 };

function reader(record, server = captured(contract.records.server)) {
  const calls = [];
  const transport = {
    async execute(operation) {
      calls.push(operation);
      if (operation.path.startsWith('/app/rest/server')) {
        if (server instanceof DomainError) throw server;
        return server;
      }
      return captured(record);
    },
  };
  return { value: new NativeTeamCityReader(transport, 'http://127.0.0.1:32768'), calls };
}

test('recorded exhausted empty page certifies only the verified server and exact production request', async () => {
  const p = contract.fixture.exhaustion;
  const record = contract.records['exhaustion-empty-finished-job'];
  const q = { ...query, jobId: p.emptyFinishedJobId, window: { since: p.since, until: p.until } };
  const f = reader(record);
  const read = await f.value.listRuns(q, budget);
  assert.equal(f.calls[0].path, record.args[1]);
  assert.equal(f.calls.length, 2);
  assert.equal(read.state, 'available');
  assert.deepEqual(read.value.runs, []);
  assert.equal(read.value.hasMore, false);
  assert.deepEqual(read.value.limitations, []);
  for (const server of [
    {
      stdout: Buffer.from(
        'HTTP/1.1 200 OK\nContent-Type: application/json\n\n{"version":"2026.2 (build 238925)","buildNumber":"238925"}',
      ),
      stderr: Buffer.alloc(0),
      exitCode: 0,
      signal: null,
    },
    new DomainError('PERMISSION_DENIED', 'Probe denied'),
    new DomainError('PROCESS_BUDGET_EXCEEDED', 'No reserved process'),
    new DomainError('CAPTURE_LIMIT_EXCEEDED', 'Probe too large'),
    new DomainError('DEADLINE_EXCEEDED', 'Probe deadline'),
  ]) {
    const uncertain = await reader(record, server).value.listRuns(q, budget);
    assert.equal(uncertain.state, 'available');
    assert.equal(uncertain.value.hasMore, null);
    assert.deepEqual(uncertain.value.runs, []);
    assert.ok(uncertain.value.limitations.some((note) => note.code === 'SCAN_COVERAGE_UNKNOWN'));
  }
  const interrupted = await reader(
    record,
    new DomainError('INTERRUPTED', 'Stopped'),
  ).value.listRuns(q, budget);
  assert.equal(interrupted.state, 'unavailable');
  assert.equal(interrupted.error.code, 'INTERRUPTED');
});

test('actual capped empty and undersized positive pages cannot certify exhaustion or increase scan budgets', async () => {
  for (const [name, q] of [
    ['exhaustion-cap-empty', { ...query, scanLimit: 1, result: 'failure' }],
    ['exhaustion-cap-positive', { ...query, scanLimit: 1 }],
  ]) {
    const record = contract.records[name];
    const f = reader(record);
    const read = await f.value.listRuns(q, budget);
    assert.equal(f.calls[0].path, record.args[1]);
    assert.equal(f.calls.length, 1);
    assert.equal(read.state, 'available');
    assert.equal(read.value.hasMore, null);
    assert.equal(read.value.position, null);
    assert.ok(read.value.limitations.some((note) => note.code === 'UNSAFE_CONTINUATION'));
    assert.equal(read.value.runs.length, name.endsWith('empty') ? 0 : 1);
  }
});

test('malformed continuation and a full page missing its required continuation retain uncertainty', async () => {
  const original = parseRaw(captured(contract.records['exhaustion-cap-positive'])).body;
  for (const [body, q] of [
    [{ ...original, nextHref: null }, query],
    [
      Object.fromEntries(Object.entries(original).filter(([key]) => key !== 'nextHref')),
      { ...query, count: 1 },
    ],
  ]) {
    const f = reader({
      stdout: 'HTTP/1.1 200 OK\nContent-Type: application/json\n\n' + JSON.stringify(body),
      stderr: '',
      code: 0,
      signal: null,
    });
    const read = await f.value.listRuns(q, budget);
    assert.equal(read.state, 'available');
    assert.equal(read.value.hasMore, null);
    assert.equal(read.value.runs.length, 1);
    assert.equal(f.calls.length, 1);
  }
});
