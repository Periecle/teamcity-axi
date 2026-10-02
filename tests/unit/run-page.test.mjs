import { test } from 'node:test';
import assert from 'node:assert/strict';
import { run } from '../fixtures/mock-server.mjs';
import { normalizeRunPage, runFilters } from '../../dist/adapter/run-page.js';
const query = {
  jobId: 'Payments_Build',
  branch: 'feature/refund',
  state: 'finished',
  count: 20,
  start: 0,
  scanLimit: 5000,
  allowedProjects: ['Payments'],
};
const request = {
  serverUrl: 'https://fixture.test/teamcity',
  resource: 'builds',
  filters: runFilters(query),
  fields: 'count,nextHref,build(id)',
  count: 20,
  start: 0,
  scanLimit: 5000,
};
const href = (start, filters = request.filters) =>
  '/teamcity/app/rest/builds?' +
  new URLSearchParams({
    locator: [...filters, 'count:20', `start:${start}`, 'lookupLimit:5000'].join(','),
    fields: request.fields,
  });
test('run page preserves bounded continuation, including empty pages; counts are not global totals', () => {
  const page = normalizeRunPage({ build: [run], count: 1, nextHref: href(20) }, query, request, []);
  assert.equal(page.runs[0].id, '482193');
  assert.equal(page.position, 20);
  assert.equal(page.hasMore, true);
  assert.equal(page.providerReturned, 1);
  const empty = normalizeRunPage({ build: [], count: 0, nextHref: href(40) }, query, request, []);
  assert.deepEqual(empty.runs, []);
  assert.equal(empty.position, 40);
  assert.equal(empty.hasMore, true);
  const last = normalizeRunPage({ build: [], count: 0 }, query, request, []);
  assert.equal(last.hasMore, null);
  assert.ok(last.limitations.some((l) => l.code === 'SCAN_COVERAGE_UNKNOWN'));
});
test('run page fails on wrong scope, unsafe counts and duplicate IDs instead of fabricating empty success', () => {
  for (const dto of [
    { build: null, count: 0 },
    { build: [], count: 1 },
    { build: [run, run], count: 2 },
    {
      build: [{ ...run, buildTypeId: 'Other', buildType: { id: 'Other', projectId: 'Payments' } }],
      count: 1,
    },
    { build: [{ ...run, buildType: { id: run.buildTypeId, projectId: 'Forbidden' } }], count: 1 },
    { build: [{ ...run, branchName: 'Other' }], count: 1 },
    { build: [{ ...run, state: 'running' }], count: 1 },
  ])
    assert.throws(() => normalizeRunPage(dto, query, request, []));
});
test('unsafe or escalating provider continuation keeps useful rows and reports partial acquisition', () => {
  for (const nextHref of [
    'https://attacker.test/app/rest/builds?locator=count:20,start:20',
    href(0),
    href(20).replace('lookupLimit%3A5000', 'lookupLimit%3A10000'),
  ]) {
    const p = normalizeRunPage({ build: [run], count: 1, nextHref }, query, request, []);
    assert.equal(p.runs.length, 1);
    assert.equal(p.hasMore, null);
    assert.equal(p.position, null);
    assert.ok(p.limitations.some((l) => l.code === 'UNSAFE_CONTINUATION'));
  }
});
test('revision candidates require the requested root identity and exact revision; missing metadata stays partial', () => {
  const q = { ...query, revision: 'a'.repeat(40), vcsRootId: 'Payments_Git' };
  const p = normalizeRunPage({ build: [run], count: 1, nextHref: href(20) }, q, request, []);
  assert.equal(p.runs.length, 1);
  const missing = normalizeRunPage(
    { build: [{ ...run, revisions: undefined }], count: 1, nextHref: href(20) },
    q,
    request,
    [],
  );
  assert.deepEqual(missing.runs, []);
  assert.ok(missing.limitations.some((l) => l.code === 'REVISION_UNVERIFIED'));
  const mismatch = normalizeRunPage(
    {
      build: [
        {
          ...run,
          revisions: {
            revision: [
              { version: 'a'.repeat(40), 'vcs-root-instance': { 'vcs-root-id': 'OtherRoot' } },
            ],
          },
        },
      ],
      count: 1,
      nextHref: href(20),
    },
    q,
    request,
    [],
  );
  assert.deepEqual(mismatch.runs, []);
});
