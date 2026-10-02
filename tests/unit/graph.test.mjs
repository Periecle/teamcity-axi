import { test } from 'node:test';
import assert from 'node:assert/strict';
import { traverseSnapshotGraph } from '../../dist/planner/graph.js';
import { DomainError } from '../../dist/domain/errors.js';
const run = (id) => ({
  id: String(id),
  jobId: `Job_${id}`,
  state: 'finished',
  result: 'failure',
  branch: 'main',
  revisions: [],
});
const available = (value) => ({
  state: 'available',
  value,
  provenance: {
    observedAt: '2026-10-02T00:00:00Z',
    operation: 'fixture',
    projectId: 'Allowed',
    limitations: [],
  },
});
const unavailable = (code) => ({
  state: 'unavailable',
  error: new DomainError(code, 'Synthetic source failure'),
  provenance: { limitations: [] },
});
function fixture(adjacency, options = {}) {
  const calls = [];
  const reader = {
    async getSnapshotDependencyCount({ id }) {
      calls.push(`count:${id}`);
      return options.countError?.[id]
        ? unavailable(options.countError[id])
        : available(options.counts?.[id] ?? (adjacency[id] ?? []).length);
    },
    async listSnapshotDependencies(query) {
      calls.push(`page:${query.runId}:${query.start}`);
      if (options.pageError?.[query.runId]) return unavailable(options.pageError[query.runId]);
      if (options.pages?.[query.runId]) return available(options.pages[query.runId][query.start]);
      const ids = (adjacency[query.runId] ?? []).slice(query.start, query.start + query.count);
      return available({
        items: ids.map((id) => ({
          run: run(id),
          projectId: options.foreign?.includes(String(id)) ? 'Foreign' : 'Allowed',
        })),
        providerReturned: ids.length,
        position: null,
        hasMore: null,
        limitations: [
          { code: 'SCAN_COVERAGE_UNKNOWN', message: 'No continuation', source: 'dependencies' },
        ],
      });
    },
  };
  const policy = {
    async assert(project) {
      if (project === 'Foreign')
        throw new DomainError('POLICY_DENIED', 'Synthetic foreign project');
    },
  };
  return { reader, policy, calls };
}
async function traverse(f, options = {}) {
  return traverseSnapshotGraph({
    root: run(1),
    reader: f.reader,
    policy: f.policy,
    budget: { deadline: Date.now() + 5000 },
    depth: 4,
    maxNodes: 30,
    maxGraphReads: 20,
    ...options,
  });
}
test('shared DAG retains every parent edge and one node per execution without false cycles', async () => {
  const f = fixture({ 1: [2, 3], 2: [4], 3: [4], 4: [] }),
    r = await traverse(f);
  assert.equal(r.graph.complete, true);
  assert.equal(r.graph.nodes.length, 4);
  assert.equal(r.graph.edges.length, 4);
  assert.deepEqual(r.graph.cycles, []);
  assert.equal(r.graph.unexpanded, 0);
  assert.equal(f.calls.filter((c) => c === 'count:4').length, 1);
  assert.equal(r.graph.nodes.find((n) => n.run.id === '4').dependencyCount, 0);
  assert.equal(r.graph.nodes.find((n) => n.run.id === '4').expansion, 'complete');
  assert.equal(r.graphReadAttempts, 7);
  assert.equal(
    r.limitations.some((l) => l.code === 'SCAN_COVERAGE_UNKNOWN'),
    false,
  );
});
test('cycles are reported separately from shared descendants and cannot recurse forever', async () => {
  const r = await traverse(fixture({ 1: [2], 2: [3], 3: [1] }));
  assert.equal(r.graph.nodes.length, 3);
  assert.equal(r.graph.edges.length, 3);
  assert.deepEqual(r.graph.cycles, [{ fromRunId: '3', toRunId: '1', kind: 'snapshot' }]);
  assert.equal(r.graphReadAttempts, 6);
  assert.equal(r.graph.complete, true);
});
test('zero depth performs no dependency reads and boundaries are never declared leaves', async () => {
  const f = fixture({ 1: [2] }),
    r = await traverse(f, { depth: 0 });
  assert.deepEqual(f.calls, []);
  assert.equal(r.graph.nodes[0].expansion, 'depth_limit');
  assert.equal(r.graph.nodes[0].dependencyCount, null);
  assert.equal(r.graph.nodes[0].observedDependencies, null);
  assert.equal(r.graph.complete, false);
  assert.equal(r.graph.unexpanded, 1);
  const one = await traverse(fixture({ 1: [2, 3] }), { depth: 1 });
  assert.equal(one.graph.nodes[0].expansion, 'complete');
  assert.ok(one.graph.nodes.slice(1).every((n) => n.expansion === 'depth_limit'));
});
test('node caps retain known edges only, account omitted unique targets and preserve sibling expansion', async () => {
  const r = await traverse(fixture({ 1: [2, 3], 2: [], 3: [] }), { maxNodes: 2 });
  assert.equal(r.graph.nodes.length, 2);
  assert.deepEqual(r.graph.edges, [{ fromRunId: '1', toRunId: '2', kind: 'snapshot' }]);
  assert.equal(r.graph.nodes[0].expansion, 'node_limit');
  assert.equal(r.graph.nodes[1].expansion, 'complete');
  assert.equal(r.omittedTargets, 1);
  assert.equal(r.graph.complete, false);
});
test('count/page denial, malformed continuation and conflicting counts preserve useful siblings', async () => {
  const f = fixture(
    { 1: [2, 3], 2: [], 3: [4], 4: [] },
    { countError: { 2: 'PERMISSION_DENIED' }, pageError: { 2: 'PERMISSION_DENIED' } },
  );
  const r = await traverse(f);
  assert.equal(r.graph.nodes.find((n) => n.run.id === '2').expansion, 'permission_denied');
  assert.equal(r.graph.nodes.find((n) => n.run.id === '4').expansion, 'complete');
  assert.equal(r.graph.complete, false);
  const mismatch = await traverse(fixture({ 1: [2], 2: [] }, { counts: { 1: 2 } }));
  assert.equal(mismatch.graph.nodes.length, 2);
  assert.equal(mismatch.graph.nodes[0].expansion, 'unavailable');
  assert.ok(mismatch.limitations.some((l) => l.code === 'DEPENDENCY_COUNT_MISMATCH'));
  const unsafe = await traverse(
    fixture(
      {},
      {
        counts: { 1: 1 },
        pages: {
          1: {
            0: {
              items: [{ run: run(2), projectId: 'Allowed' }],
              providerReturned: 1,
              position: null,
              hasMore: null,
              limitations: [{ code: 'UNSAFE_CONTINUATION', message: 'Synthetic unsafe cursor' }],
            },
          },
        },
      },
    ),
  );
  assert.equal(unsafe.graph.nodes.length, 2);
  assert.equal(unsafe.graph.nodes[0].expansion, 'unavailable');
});
test('policy-denied children never enter retained nodes or edges and call caps remain explicit', async () => {
  const r = await traverse(fixture({ 1: [2, 3], 2: [] }, { foreign: ['3'] }));
  assert.deepEqual(
    r.graph.nodes.map((n) => n.run.id),
    ['1', '2'],
  );
  assert.ok(r.graph.edges.every((e) => e.toRunId !== '3'));
  assert.equal(r.graph.nodes[0].expansion, 'permission_denied');
  const f = fixture({ 1: [2] });
  const capped = await traverse(f, { maxGraphReads: 1 });
  assert.deepEqual(f.calls, ['count:1']);
  assert.equal(capped.graph.nodes[0].expansion, 'call_limit');
  assert.equal(capped.graph.nodes[0].observedDependencies, null);
});
test('empty continued pages are followed; duplicate pages cannot create exact exhaustion', async () => {
  const item = { run: run(2), projectId: 'Allowed' };
  const f = fixture(
    { 2: [] },
    {
      counts: { 1: 1 },
      pages: {
        1: {
          0: { items: [], providerReturned: 0, position: 100, hasMore: true, limitations: [] },
          100: {
            items: [item],
            providerReturned: 1,
            position: null,
            hasMore: null,
            limitations: [{ code: 'SCAN_COVERAGE_UNKNOWN', message: 'No cursor' }],
          },
        },
      },
    },
  );
  const r = await traverse(f);
  assert.equal(r.graph.complete, true);
  assert.ok(f.calls.includes('page:1:100'));
  const duplicate = await traverse(
    fixture(
      { 2: [] },
      {
        counts: { 1: 3 },
        pages: {
          1: {
            0: {
              items: [item],
              providerReturned: 1,
              position: 100,
              hasMore: true,
              limitations: [],
            },
            100: {
              items: [item],
              providerReturned: 1,
              position: null,
              hasMore: null,
              limitations: [],
            },
          },
        },
      },
    ),
  );
  assert.equal(duplicate.graph.nodes[0].expansion, 'unavailable');
  assert.ok(duplicate.limitations.some((l) => l.code === 'DUPLICATE_DEPENDENCY'));
});

test('conflicting or denied observations of retained IDs cannot publish rejected metadata', async () => {
  const child = { run: run(2), projectId: 'Allowed' };
  const f = fixture(
    {},
    {
      counts: { 1: 1, 2: 1 },
      pages: {
        1: {
          0: {
            items: [child],
            providerReturned: 1,
            position: null,
            hasMore: null,
            limitations: [],
          },
        },
        2: {
          0: {
            items: [{ run: { ...run(1), result: 'unknown' }, projectId: 'Foreign' }],
            providerReturned: 1,
            position: null,
            hasMore: null,
            limitations: [
              { code: 'UNKNOWN_RESULT', message: 'Rejected metadata', source: 'run', runId: '1' },
            ],
          },
        },
      },
    },
  );
  const denied = await traverse(f, { rootProjectId: 'Allowed' });
  assert.equal(denied.graph.edges.length, 1);
  assert.equal(denied.graph.nodes[1].expansion, 'permission_denied');
  assert.ok(!denied.limitations.some((l) => l.code === 'UNKNOWN_RESULT'));
  f.reader.listSnapshotDependencies = async (query) =>
    available({
      items:
        query.runId === '1'
          ? [child]
          : [{ run: { ...run(1), jobId: 'Substituted' }, projectId: 'Allowed' }],
      providerReturned: 1,
      position: null,
      hasMore: null,
      limitations:
        query.runId === '1'
          ? []
          : [
              {
                code: 'MISSING_REVISION_METADATA',
                message: 'Rejected metadata',
                source: 'run',
                runId: '1',
              },
            ],
    });
  const conflict = await traverse(f, { rootProjectId: 'Allowed' });
  assert.equal(conflict.graph.edges.length, 1);
  assert.ok(conflict.limitations.some((l) => l.code === 'CONTEXT_MISMATCH'));
  assert.ok(!conflict.limitations.some((l) => l.code === 'MISSING_REVISION_METADATA'));
});

test('shared metadata changes stay explicit and hard node ceilings bound repeated budget diagnostics', async () => {
  const f = fixture({ 1: [2, 3], 2: [4], 3: [4], 4: [] });
  const original = f.reader.listSnapshotDependencies;
  f.reader.listSnapshotDependencies = async (query) => {
    const page = await original(query);
    if (query.runId === '3') page.value.items[0].run.branch = 'different';
    return page;
  };
  const changed = await traverse(f);
  assert.equal(changed.graph.complete, false);
  assert.ok(changed.limitations.some((l) => l.code === 'RUN_STATE_CHANGED'));
  assert.equal(changed.graph.nodes.find((n) => n.run.id === '4').run.branch, 'main');
  const wide = await traverse(fixture({ 1: Array.from({ length: 199 }, (_, i) => i + 2) }), {
    maxNodes: 200,
    maxGraphReads: 24,
  });
  assert.equal(wide.graph.nodes.length, 101);
  assert.equal(wide.graph.complete, false);
  assert.ok(wide.graph.nodes.some((n) => n.expansion === 'call_limit'));
  assert.equal(wide.limitations.filter((l) => l.code === 'CALL_LIMIT_EXCEEDED').length, 1);
});
