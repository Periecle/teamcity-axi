import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  canonicalTimestamp,
  compareTimestamps,
  shiftTimestamp,
  providerDate,
} from '../../dist/domain/time.js';
import { parse } from '../../dist/cli/parser.js';

test('RFC3339 windows preserve fractional precision through UTC conversion and lookback', () => {
  assert.equal(
    canonicalTimestamp('2026-10-02T02:00:00.000000001+02:00'),
    '2026-10-02T00:00:00.000000001Z',
  );
  assert.equal(canonicalTimestamp('2026-10-02T00:00:00.123400Z'), '2026-10-02T00:00:00.1234Z');
  assert.equal(
    shiftTimestamp('2026-10-02T00:00:00.000000001Z', -7 * 86400),
    '2026-09-25T00:00:00.000000001Z',
  );
  assert.equal(compareTimestamps('2026-10-02T00:00:00.000000001Z', '2026-10-02T00:00:00Z'), 1);
  assert.equal(compareTimestamps('2026-10-02T00:00:00.123000001Z', '2026-10-02T00:00:00.123Z'), 1);
  assert.equal(compareTimestamps('2026-10-02T02:00:00.1230+02:00', '2026-10-02T00:00:00.123Z'), 0);
  assert.equal(providerDate('2026-10-02T00:00:00.000000001Z', 'after'), '20261002T000000+0000');
  assert.equal(
    providerDate('2026-10-02T00:00:00.000000001Z', 'before'),
    '20261002T000000.001+0000',
  );
  assert.equal(providerDate('2026-10-02T00:00:00.000Z', 'before'), '20261002T000000+0000');
  assert.equal(providerDate('9999-12-31T23:59:59.999999Z', 'before'), null);
  assert.equal(providerDate('2026-10-02T00:00:00.123999999Z', 'after'), '20261002T000000.123+0000');
  assert.equal(
    providerDate('2026-10-02T00:00:00.123999999Z', 'before'),
    '20261002T000000.124+0000',
  );
  assert.equal(providerDate('2026-10-02T00:00:00.999999999Z', 'before'), '20261002T000001+0000');
});

test('invalid dates and reversed sub-millisecond windows fail before acquisition', () => {
  for (const value of [
    '2026-02-30T00:00:00Z',
    '2026-10-02T24:00:00Z',
    '2026-10-02T00:00:60Z',
    '2026-10-02T00:00:00+24:00',
    '2026-10-02T00:00:00Zjunk',
  ]) {
    assert.throws(
      () => canonicalTimestamp(value),
      (error) => error.code === 'USAGE_ERROR',
    );
  }
  const args = [
    'run',
    'list',
    '--job',
    'Payments_Build',
    '--since',
    '2026-10-02T00:00:00.000000002Z',
    '--until',
    '2026-10-02T00:00:00.000000001Z',
  ];
  assert.throws(
    () => parse(args),
    (error) => error.code === 'USAGE_ERROR',
  );
  assert.doesNotThrow(() =>
    parse(['run', 'list', '--job', 'Payments_Build', '--until', '2026-10-02T00:00:00.000000001Z']),
  );
});

test('recorded restricted native predicates prove precise membership without fabricating DTO milliseconds', async () => {
  const { readFile } = await import('node:fs/promises');
  const { parseRaw } = await import('../../dist/adapter/raw.js');
  const { normalizeRunPage, runFilters } = await import('../../dist/adapter/run-page.js');
  const { runDetailFields } = await import('../../dist/adapter/reader.js');
  const contract = JSON.parse(
    await readFile('tests/fixtures/teamcity-2026.2-native-1.5.0/contract.json', 'utf8'),
  );
  const body = (record) =>
    parseRaw({
      stdout: Buffer.from(record.stdout),
      stderr: Buffer.from(record.stderr),
      exitCode: record.code,
      signal: record.signal,
    }).body;
  for (const name of [
    'date-millisecond-exact-before',
    'date-millisecond-exact-after',
    'date-build-exact-before',
    'date-build-exact-after',
  ])
    assert.deepEqual(body(contract.records[name]).build, []);
  for (const name of ['date-millisecond-next-before', 'date-millisecond-previous-after'])
    assert.equal(
      String(body(contract.records[name]).build[0].id),
      contract.fixture.listPrecision.runId,
    );
  const precision = contract.fixture.listPrecision;
  const query = {
    jobId: precision.jobId,
    projectId: contract.fixture.projectId,
    state: 'finished',
    count: 20,
    start: 0,
    scanLimit: 5000,
    window: { since: precision.lowerExclusive, until: precision.upperExclusive },
  };
  const fields = `count,nextHref,build(${runDetailFields})`;
  const filters = runFilters(query);
  const record = contract.records['fractional-millisecond-page-positive'];
  const path = new URL(record.args[1], contract.server.url ?? 'http://127.0.0.1:32768');
  assert.equal(path.searchParams.get('fields'), fields);
  assert.equal(
    path.searchParams.get('locator'),
    [...filters, 'count:20', 'start:0', 'lookupLimit:5000'].join(','),
  );
  const page = normalizeRunPage(
    body(record),
    query,
    {
      serverUrl: 'http://127.0.0.1:32768',
      resource: 'builds',
      filters,
      fields,
      count: 20,
      start: 0,
      scanLimit: 5000,
    },
    [],
  );
  assert.equal(page.runs[0].id, precision.runId);
  assert.equal(page.runs[0].finishedAt, precision.finish);
  assert.notEqual(page.runs[0].finishedAt, precision.exactFinish);
  for (const name of [
    'fractional-millisecond-page-lower-excluded',
    'fractional-millisecond-page-upper-excluded',
  ])
    assert.deepEqual(body(contract.records[name]).build, []);
});
