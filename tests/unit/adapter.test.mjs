import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { parseRaw } from '../../dist/adapter/raw.js';
import { normalizeRun } from '../../dist/adapter/run.js';
import { NativeTeamCityReader } from '../../dist/adapter/reader.js';
import { run } from '../fixtures/mock-server.mjs';
const recorded = JSON.parse(readFileSync('tests/fixtures/native-v1.5.0/contract.json', 'utf8'));
const captured = (r) => ({
  stdout: Buffer.from(r.stdout),
  stderr: Buffer.from(r.stderr),
  exitCode: r.code,
  signal: r.signal,
});
test('raw parser handles the actual released CLI envelope and classifies status rather than English', () => {
  assert.equal(parseRaw(captured(recorded.records['run-view'])).body.id, 482193);
  for (const [mode, code] of [
    ['denied', 'PERMISSION_DENIED'],
    ['missing', 'NOT_FOUND'],
    ['expired', 'AUTH_REQUIRED'],
    ['malformed', 'UPSTREAM_SCHEMA_MISMATCH'],
    ['html', 'AUTH_REQUIRED'],
  ])
    assert.throws(
      () => parseRaw(captured(recorded.records['error-' + mode])),
      (e) => e.code === code,
    );
  for (const stdout of [
    '{"id":1}',
    'HTTP/1.1 200 OK\nInvalid\n\n{}',
    'HTTP/1.1 200 OK\nContent-Type: application/json\nContent-Type: text/html\n\n{}',
  ])
    assert.throws(() =>
      parseRaw({ stdout: Buffer.from(stdout), stderr: Buffer.alloc(0), exitCode: 0, signal: null }),
    );
  const nonzero = captured(recorded.records['run-view']);
  nonzero.exitCode = 17;
  assert.throws(
    () => parseRaw(nonzero),
    (e) => e.code === 'UPSTREAM_FAILURE',
  );
  const utf8 = captured(recorded.records['run-view']);
  utf8.stdout = Buffer.from([255]);
  assert.throws(
    () => parseRaw(utf8),
    (e) => e.code === 'UPSTREAM_SCHEMA_MISMATCH',
  );
});
test('DTO identity validation tolerates additive fields while refusing unsafe IDs and malformed revisions', () => {
  const normalized = normalizeRun(
    { ...run, unexpected: { token: 'not-propagated' } },
    'https://teamcity.example.test/teamcity',
  );
  assert.equal(normalized.run.id, '482193');
  assert.equal(normalized.run.result, 'failure');
  assert.equal(normalized.run.startedAt, '2026-10-01T14:00:00.000Z');
  assert.equal(normalized.run.durationMs, 60000);
  assert.equal(normalized.projectId, 'Payments');
  assert.equal(normalized.run.unexpected, undefined);
  for (const patch of [
    { id: 9007199254740992 },
    { id: '9007199254740993' },
    { id: null },
    { state: null },
    { status: null },
    { buildTypeId: '' },
    { buildType: { id: 'Other' } },
    { revisions: { revision: null } },
    { revisions: { revision: [{ version: 'sha', 'vcs-root-instance': { id: '17' } }] } },
  ])
    assert.throws(
      () => normalizeRun({ ...run, ...patch }, 'https://teamcity.example.test'),
      (e) => e.code === 'UPSTREAM_SCHEMA_MISMATCH',
    );
  for (const status of ['NEW_RESULT', 'toString', '__proto__'])
    assert.equal(
      normalizeRun({ ...run, status }, 'https://teamcity.example.test').run.result,
      'unknown',
    );
  const invalidTime = normalizeRun(
    { ...run, finishDate: '20260230T140000+0000' },
    'https://teamcity.example.test',
  );
  assert.equal(invalidTime.run.finishedAt, null);
  assert.ok(invalidTime.limitations.some((l) => l.code === 'INVALID_TIMESTAMP'));
});
test('requested exact ID is never replaced by an unrelated green run', async () => {
  const detail = recorded.records['run-view'];
  const transport = {
    async execute() {
      const value = captured(detail);
      value.stdout = Buffer.from(
        detail.stdout
          .replace('"id":482193', '"id":482100')
          .replace('"status":"FAILURE"', '"status":"SUCCESS"'),
      );
      return value;
    },
  };
  const reader = new NativeTeamCityReader(transport, 'https://teamcity.example.test/teamcity');
  const result = await reader.getRun({ id: '482193' }, { deadline: Date.now() + 1000 });
  assert.equal(result.state, 'unavailable');
  assert.equal(result.error.code, 'CONTEXT_MISMATCH');
});
test('redaction precedes diagnostic and summary truncation; omitted revisions stay unknown', () => {
  const canary = 'canary-' + 'q'.repeat(1600);
  const value = normalizeRun(
    { ...run, status: canary, statusText: canary },
    'https://teamcity.example.test',
    [canary],
  );
  assert.equal(value.run.rawStatus, '[REDACTED]');
  assert.equal(value.run.statusText, '[REDACTED]');
  const decorated = canary.slice(0, 600) + '\x1b[31m' + canary.slice(600);
  const controls = normalizeRun(
    { ...run, status: decorated, statusText: decorated },
    'https://teamcity.example.test',
    [canary],
  );
  assert.equal(controls.run.rawStatus, '[REDACTED]');
  assert.equal(controls.run.statusText, '[REDACTED]');
  const { revisions, ...withoutRevisions } = run;
  const missing = normalizeRun(withoutRevisions, 'https://teamcity.example.test');
  assert.equal(missing.run.revisions, undefined);
  assert.ok(missing.limitations.some((l) => l.code === 'MISSING_REVISION_METADATA'));
  const early = normalizeRun(
    { ...run, startDate: '00991001T140000+0000' },
    'https://teamcity.example.test',
  );
  assert.equal(early.run.startedAt, null);
  assert.ok(early.limitations.some((l) => l.code === 'INVALID_TIMESTAMP'));
});
test('HTTP failures retain internal pacing metadata without serializing response headers', () => {
  assert.throws(
    () =>
      parseRaw({
        stdout: Buffer.from('HTTP/1.1 429 Too Many Requests\nRetry-After: 15\n\n'),
        stderr: Buffer.alloc(0),
        exitCode: 1,
        signal: null,
      }),
    (e) => {
      assert.equal(e.httpStatus, 429);
      assert.equal(e.retryAfter, '15');
      assert.equal(e.retryable, true);
      assert.equal(e.publicValue().retryAfter, undefined);
      return true;
    },
  );
});
