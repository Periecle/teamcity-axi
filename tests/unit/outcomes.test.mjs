import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { normalizeRun } from '../../dist/adapter/run.js';
import { runFilters, normalizeRunPage } from '../../dist/adapter/run-page.js';
import { parseRaw } from '../../dist/adapter/raw.js';
import { run } from '../fixtures/mock-server.mjs';

const server = 'http://127.0.0.1:32768';
const contract = JSON.parse(
  readFileSync('tests/fixtures/teamcity-2026.2-native-1.5.0/contract.json', 'utf8'),
);
const body = (name) => {
  const record = contract.records[name];

  return parseRaw({
    stdout: Buffer.from(record.stdout),
    stderr: Buffer.from(record.stderr),
    exitCode: record.code,
    signal: record.signal,
  }).body;
};

test('actual native outcome captures distinguish exceptional results and composite from lifecycle', () => {
  for (const [name, id, result] of [
    ['outcome-normal-failed', '1', 'failure'],
    ['outcome-normal-green', '2', 'success'],
    ['outcome-failed-to-start', '5', 'failed_to_start'],
    ['outcome-canceled-queued', '13', 'canceled'],
    ['outcome-canceled-running', '14', 'canceled'],
    ['outcome-composite', '16', 'success'],
  ]) {
    const normalized = normalizeRun(body(name), server);

    assert.equal(normalized.run.id, id);
    assert.equal(normalized.run.state, 'finished');
    assert.equal(normalized.run.result, result);
    assert.equal(normalized.run.composite, name === 'outcome-composite');
    assert.ok(!JSON.stringify(normalized.run).includes('canceledInfo'));
    assert.ok(!normalized.limitations.some((value) => value.code === 'UNKNOWN_RESULT'));
  }
});

test('missing, malformed and conflicting explicit outcome metadata cannot validate green', () => {
  for (const patch of [
    { failedToStart: undefined },
    { failedToStart: null },
    { failedToStart: true },
    { canceledInfo: { timestamp: '20261002T180555+0000' } },
    { canceledInfo: {} },
    { canceledInfo: { timestamp: 'invalid' } },
    { failedToStart: true, canceledInfo: { timestamp: '20261002T180555+0000' } },
  ]) {
    const normalized = normalizeRun({ ...run, status: 'SUCCESS', ...patch }, server);

    assert.equal(normalized.run.result, 'unknown');
    assert.ok(
      normalized.limitations.some((value) =>
        ['OUTCOME_METADATA_UNAVAILABLE', 'CONFLICTING_OUTCOME_METADATA'].includes(value.code),
      ),
    );
  }

  for (const patch of [
    { failedToStart: 'false' },
    { canceledInfo: true },
    { canceledInfo: { timestamp: 123 } },
  ]) {
    assert.throws(
      () => normalizeRun({ ...run, ...patch }, server),
      (error) => error.code === 'UPSTREAM_SCHEMA_MISMATCH',
    );
  }

  const metadata = {
    timestamp: '20261002T180555+0000',
    text: 'Private cancellation comment',
    user: { username: 'Private actor' },
  };
  const normalized = normalizeRun({ ...run, status: 'UNKNOWN', canceledInfo: metadata }, server);

  assert.equal(normalized.run.result, 'canceled');
  assert.ok(!JSON.stringify(normalized).includes('Private'));
});

test('ordinary list result filters exclude exceptional runs; exceptional filters use explicit dimensions', () => {
  const base = { jobId: 'Payments_Build', state: 'finished', count: 20, start: 0, scanLimit: 5000 };

  for (const result of ['success', 'failure', 'error']) {
    const filters = runFilters({ ...base, result });

    assert.ok(filters.includes('canceled:false'));
    assert.ok(filters.includes('failedToStart:false'));
  }

  assert.ok(runFilters({ ...base, result: 'canceled' }).includes('canceled:true'));
  assert.ok(runFilters({ ...base, result: 'failed_to_start' }).includes('failedToStart:true'));

  const query = { ...base, result: 'failure' };
  const request = {
    serverUrl: server,
    resource: 'builds',
    filters: runFilters(query),
    fields: 'count,nextHref,build(id)',
    count: 20,
    start: 0,
    scanLimit: 5000,
  };

  assert.throws(
    () =>
      normalizeRunPage({ count: 1, build: [{ ...run, failedToStart: true }] }, query, request, []),
    (error) => error.code === 'CONTEXT_MISMATCH',
  );
});

test('actual restricted native running capture preserves lifecycle independently of its nominal status', () => {
  const observed = normalizeRun(body('outcome-running'), server);
  assert.equal(observed.run.id, contract.fixture.lifecycle.runningObservedRunId);
  assert.equal(observed.run.jobId, contract.fixture.lifecycle.jobs.slow);
  assert.equal(observed.projectId, contract.fixture.lifecycle.projectId);
  assert.equal(observed.run.state, 'running');
  assert.equal(observed.run.result, 'success');
  assert.equal(observed.run.composite, false);
  const terminal = contract.fixture.lifecycle.runningWatchEvidence;
  assert.equal(terminal.runId, observed.run.id);
  assert.equal(terminal.initialState, 'running');
  assert.equal(terminal.terminalResult, 'canceled');
  assert.ok(terminal.polls >= 2);
  assert.equal(terminal.checkPassed, false);
});
