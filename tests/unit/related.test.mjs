import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { parseRaw } from '../../dist/adapter/raw.js';
import {
  normalizeChangePage,
  normalizeDependencyPage,
  normalizeDependencyCount,
  relatedRequest,
} from '../../dist/adapter/related.js';
import { runDetailFields } from '../../dist/adapter/reader.js';
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
const query = { runId: '9', count: 1, start: 0, scanLimit: 5000 };
test('actual positive changes retain commit/root identity and reconstruct bounded continuation', () => {
  const page = normalizeChangePage(body('changes-positive'), query, 'http://127.0.0.1:32768', []);
  assert.equal(page.items[0].id, '3');
  assert.equal(page.items[0].vcsRootId, 'AxiContract_Git');
  assert.equal(page.position, 1);
  assert.equal(page.hasMore, true);
  assert.ok(page.items[0].message.includes('\n'));
  assert.match(page.items[0].timestamp, /Z$/);
  assert.equal(page.items[0].files, undefined);
  const files = normalizeChangePage(
    body('changes-files-positive'),
    { ...query, count: 10, files: true },
    'http://127.0.0.1:32768',
    [],
  );
  assert.equal(files.items.length, 3);
  assert.deepEqual(files.items[0].files, ['fixture.txt']);
  assert.deepEqual(files.items[0].fileCoverage, { returned: 1, providerReturned: 1, omitted: 0 });
  assert.equal(files.hasMore, null);
  assert.ok(files.limitations.some((l) => l.code === 'SCAN_COVERAGE_UNKNOWN'));
});
test('unsafe paging retains changes; missing roots/files remain explicit; file names are bounded and redacted', () => {
  const dto = body('changes-positive');
  for (const nextHref of [
    42,
    null,
    'https://attacker.invalid/app/rest/changes',
    '/app/rest/changes?locator=build:(id:1),count:1,start:1',
  ]) {
    const page = normalizeChangePage({ ...dto, nextHref }, query, 'http://127.0.0.1:32768', []);
    assert.equal(page.items.length, 1);
    assert.equal(page.position, null);
    assert.ok(page.limitations.some((l) => l.code === 'UNSAFE_CONTINUATION'));
  }
  const missing = normalizeChangePage(
    { ...dto, change: [{ ...dto.change[0], vcsRootInstance: null }] },
    query,
    'http://127.0.0.1:32768',
    [],
  );
  assert.equal(missing.items.length, 0);
  assert.equal(missing.providerReturned, 1);
  assert.ok(missing.limitations.some((l) => l.code === 'CHANGE_ROOT_UNAVAILABLE'));
  const secret = 'synthetic-secret-' + 'x'.repeat(2200);
  const large = normalizeChangePage(
    {
      ...dto,
      change: [
        {
          ...dto.change[0],
          comment: secret,
          files: { count: 101, file: Array.from({ length: 101 }, () => ({ file: secret })) },
        },
      ],
    },
    { ...query, files: true },
    'http://127.0.0.1:32768',
    [secret],
  );
  assert.equal(large.items[0].message, '[REDACTED]');
  assert.equal(large.items[0].files[0], '[REDACTED]');
  assert.equal(large.items[0].fileCoverage.omitted, 1);
  assert.equal(large.items[0].files.length, 100);
  assert.ok(large.limitations.some((l) => l.code === 'CHANGE_FILE_LIMIT'));
  assert.throws(
    () => normalizeChangePage({ ...dto, count: 2 }, query, 'http://a.invalid', []),
    (e) => e.code === 'UPSTREAM_SCHEMA_MISMATCH',
  );
});
test('actual snapshot direction and scoped dependency counts preserve unknown exhaustion and proven leaves', () => {
  const q = { ...query, count: 20 };
  const page = normalizeDependencyPage(
    body('dependencies-positive'),
    q,
    'http://127.0.0.1:32768',
    runDetailFields,
    [],
  );
  assert.equal(page.items[0].run.id, '8');
  assert.equal(page.items[0].projectId, 'AxiContract');
  assert.equal(page.hasMore, null);
  assert.equal(normalizeDependencyCount(body('dependency-count-positive'), '9'), 1);
  assert.equal(normalizeDependencyCount(body('dependency-count-leaf'), '8'), 0);
  assert.throws(
    () => normalizeDependencyCount(body('dependency-count-leaf'), '9'),
    (e) => e.code === 'CONTEXT_MISMATCH',
  );
  assert.throws(
    () => normalizeDependencyCount({ id: 9, 'snapshot-dependencies': {} }, '9'),
    (e) => e.code === 'UPSTREAM_SCHEMA_MISMATCH',
  );
  const dto = body('dependencies-positive');
  assert.throws(
    () =>
      normalizeDependencyPage(
        { ...dto, count: 2, build: [...dto.build, ...dto.build] },
        q,
        'http://a.invalid',
        runDetailFields,
        [],
      ),
    (e) => e.code === 'UPSTREAM_SCHEMA_MISMATCH',
  );
  assert.ok(
    relatedRequest('dependencies', q, runDetailFields).filters.includes(
      'snapshotDependency:(to:(id:9),recursive:false)',
    ),
  );
});
