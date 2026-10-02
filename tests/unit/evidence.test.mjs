import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { parseRaw } from '../../dist/adapter/raw.js';
import {
  normalizeEvidencePage,
  normalizeProblem,
  normalizeTest,
  occurrenceLocator,
} from '../../dist/adapter/evidence.js';
import { normalizeLogTail } from '../../dist/adapter/metadata.js';
import { encode, decode } from '@toon-format/toon';
const contract = JSON.parse(
  readFileSync('tests/fixtures/teamcity-2026.2-native-1.5.0/contract.json', 'utf8'),
);
const body = (name) => {
  const r = contract.records[name];
  return parseRaw({
    stdout: Buffer.from(r.stdout),
    stderr: Buffer.from(r.stderr),
    exitCode: r.code,
    signal: r.signal,
  }).body;
};
const query = { runId: '1', count: 1, start: 0, scanLimit: 5000 };
test('live bounded occurrence pages preserve identity and reconstruct continuation', () => {
  for (const [kind, name] of [
    ['problems', 'problems-page'],
    ['tests', 'tests-page'],
  ]) {
    const page = normalizeEvidencePage(kind, body(name), query, 'http://127.0.0.1:32768', []);
    assert.equal(page.items.length, 1);
    assert.equal(page.items[0].runId, '1');
    assert.equal(page.position, 1);
    assert.equal(page.hasMore, true);
    for (const nextHref of [
      'https://attacker.invalid/app/rest/testOccurrences',
      42,
      null,
      '',
      {},
    ]) {
      const retained = normalizeEvidencePage(
        kind,
        { ...body(name), nextHref },
        query,
        'http://127.0.0.1:32768',
        [],
      );
      assert.equal(retained.items.length, 1);
      assert.equal(retained.position, null);
      assert.ok(retained.limitations.some((l) => l.code === 'UNSAFE_CONTINUATION'));
    }
  }
  const muted = normalizeEvidencePage(
    'tests',
    body('tests-muted'),
    { ...query, count: 20, failed: true, muted: true },
    'http://127.0.0.1:32768',
    [],
  );
  assert.deepEqual(muted.items, []);
  assert.equal(muted.hasMore, null);
  assert.ok(muted.limitations.some((l) => l.code === 'SCAN_COVERAGE_UNKNOWN'));
});
test('exact occurrences are run-bound; definitions, raw selectors and mismatched rows fail closed', () => {
  assert.equal(
    occurrenceLocator('tests', 'build:(id:1),id:2000000000', '1'),
    'build:(id:1),id:2000000000',
  );
  assert.throws(
    () => occurrenceLocator('tests', '517450581327024597', '1'),
    (e) => e.code === 'USAGE_ERROR',
  );
  assert.throws(
    () => occurrenceLocator('problems', 'build:(id:2),problem:(id:1)', '1'),
    (e) => e.code === 'CONTEXT_MISMATCH',
  );
  assert.throws(
    () => occurrenceLocator('tests', 'build:(id:1),id:1),item:(build:(id:9))', '1'),
    (e) => e.code === 'USAGE_ERROR',
  );
  assert.throws(
    () => normalizeProblem({ ...body('problem-selected'), build: { id: 2 } }, '1', []),
    (e) => e.code === 'CONTEXT_MISMATCH',
  );
  assert.throws(
    () => normalizeTest({ ...body('test-selected'), build: { id: 2 } }, '1', [], []),
    (e) => e.code === 'CONTEXT_MISMATCH',
  );
  const test = normalizeTest(body('test-selected'), '1', [], []);
  assert.equal(test.testId, '517450581327024597');
  assert.equal(typeof test.testId, 'string');
});
test('known secrets are removed before previews and unknown test flags/outcomes remain explicit', () => {
  const secret = 'secret-' + 'x'.repeat(2200),
    limitations = [];
  const test = normalizeTest(
    {
      ...body('test-selected'),
      status: secret,
      details: secret,
      muted: undefined,
      ignored: undefined,
    },
    '1',
    [secret],
    limitations,
  );
  assert.equal(test.rawStatus, '[REDACTED]');
  assert.equal(test.details, '[REDACTED]');
  assert.equal(test.result, 'unknown');
  assert.equal(test.muted, null);
  assert.ok(limitations.some((l) => l.code === 'TEST_FLAGS_UNAVAILABLE'));
  const dto = {
    count: 1,
    testOccurrence: [{ ...body('test-selected'), muted: undefined, ignored: undefined }],
  };
  const page = normalizeEvidencePage(
    'tests',
    dto,
    { ...query, failed: true, muted: false },
    'http://127.0.0.1:32768',
    [],
  );
  assert.equal(page.providerReturned, 1);
  assert.equal(page.items.length, 0);
  assert.ok(page.limitations.length);
});
test('log timestamp offsets normalize to UTC and invalid dates stay unavailable', () => {
  const dto = JSON.parse(contract.records['log-probe'].stdout),
    message = dto.messages[0];
  const good = normalizeLogTail(
    { ...dto, messages: [{ ...message, timestamp: '2026-10-02T12:13:30.123+0200' }] },
    '1',
    1,
    [],
  );
  assert.equal(good.messages[0].timestamp, '2026-10-02T10:13:30.123Z');
  const bad = normalizeLogTail(
    { ...dto, messages: [{ ...message, timestamp: '2026-02-30T12:13:30+0000' }] },
    '1',
    1,
    [],
  );
  assert.equal(bad.messages[0].timestamp, null);
  assert.ok(bad.limitations.length);
  assert.throws(
    () => normalizeLogTail(dto, '1', 0, []),
    (e) => e.code === 'USAGE_ERROR',
  );
});

test('malformed Unicode is visibly escaped and evidence remains equivalent in JSON and TOON', () => {
  const malformed = '\ud800broken\udfff valid \ud83d\ude00';
  const expected = '\\ud800broken\\udfff valid \ud83d\ude00';
  const problem = normalizeProblem({ ...body('problem-selected'), details: malformed }, '1', []);
  const occurrence = normalizeTest(
    { ...body('test-selected'), name: malformed, details: malformed },
    '1',
    [],
    [],
  );
  const dto = JSON.parse(contract.records['log-probe'].stdout);
  const log = normalizeLogTail(
    { ...dto, messages: [{ ...dto.messages[0], text: malformed }] },
    '1',
    1,
    [],
  );
  assert.equal(problem.description, expected);
  assert.equal(occurrence.name, expected);
  assert.equal(occurrence.details, expected);
  assert.equal(log.messages[0].text, expected);
  for (const value of [problem, occurrence, log]) {
    assert.deepEqual(decode(encode(value)), JSON.parse(JSON.stringify(value)));
  }
});
