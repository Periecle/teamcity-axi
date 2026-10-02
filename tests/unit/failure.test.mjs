import { test } from 'node:test';
import assert from 'node:assert/strict';

import { investigateFailure, selectDiagnosedRuns } from '../../dist/planner/failure.js';
import { DomainError } from '../../dist/domain/errors.js';
import { ProjectPolicy } from '../../dist/context/project-policy.js';
import { parse } from '../../dist/cli/parser.js';
import { response } from '../../dist/domain/response.js';
import { validateResponse } from '../../dist/output/schema.js';
import { render } from '../../dist/output/render.js';
import { decode } from '@toon-format/toon';

const run = (id, overrides = {}) => ({
  id: String(id),
  jobId: `Job_${id}`,
  branch: 'main',
  revisions: [],
  state: 'finished',
  result: 'failure',
  ...overrides,
});
const available = (value) => ({
  state: 'available',
  value,
  provenance: {
    projectId: 'Allowed',
    observedAt: '2026-10-02T00:00:00Z',
    limitations: [],
    operation: 'fixture',
  },
});
const page = (items = [], overrides = {}) =>
  available({
    items,
    providerReturned: items.length,
    hasMore: false,
    position: null,
    limitations: [],
    ...overrides,
  });
const problem = (id) => ({
  runId: String(id),
  id: `build:(id:${id}),problem:(id:1)`,
  type: 'SYNTHETIC',
  description: 'Explicit synthetic build problem',
});
const occurrence = (id, number = 1, overrides = {}) => ({
  runId: String(id),
  id: `build:(id:${id}),id:${number}`,
  testId: '517450581327024597',
  name: 'same name',
  result: 'failure',
  muted: false,
  ignored: false,
  durationMs: 25,
  details: 'Connection refused',
  ...overrides,
});

async function fixture(options = {}) {
  let launches = 2;
  const calls = [];
  const primary = available(run(1, options.primary));
  const adjacency = options.adjacency ?? { 1: [] };
  const read = async (name, id, budget, value) => {
    if (budget.maxChildProcesses !== undefined && launches >= budget.maxChildProcesses)
      return {
        state: 'unavailable',
        error: new DomainError('CALL_LIMIT_EXCEEDED', 'Reserved fixture read'),
        provenance: primary.provenance,
      };
    launches++;
    calls.push(`${name}:${id}`);
    if (options.errors?.[`${name}:${id}`])
      return {
        state: 'unavailable',
        error: new DomainError(options.errors[`${name}:${id}`], 'Synthetic independent failure'),
        provenance: primary.provenance,
      };
    return value;
  };
  const reader = {
    getSnapshotDependencyCount: ({ id }, budget) =>
      read('count', id, budget, available((adjacency[id] ?? []).length)),
    listSnapshotDependencies: ({ runId: id }, budget) =>
      read(
        'dependencies',
        id,
        budget,
        page(
          (adjacency[id] ?? []).map((child) => ({
            run: run(child, options.runs?.[child]),
            projectId: options.foreign?.includes(child)
              ? 'Foreign'
              : (options.childProject ?? 'Allowed'),
          })),
        ),
      ),
    listProblems: ({ runId: id }, budget) =>
      read(
        'problems',
        id,
        budget,
        page(
          options.problems?.[id] ?? [problem(id)],
          options.partialPages
            ? {
                hasMore: null,
                limitations: [{ code: 'SCAN_COVERAGE_UNKNOWN', message: 'Unknown total' }],
              }
            : {},
        ),
      ),
    listTests: ({ runId: id, muted }, budget) =>
      read(
        muted ? 'muted' : 'tests',
        id,
        budget,
        page(
          muted ? (options.muted?.[id] ?? []) : (options.tests?.[id] ?? [occurrence(id)]),
          options.partialPages
            ? {
                hasMore: null,
                limitations: [{ code: 'SCAN_COVERAGE_UNKNOWN', message: 'Unknown total' }],
              }
            : {},
        ),
      ),
    getLogTail: ({ id }, tail, budget) =>
      read(
        'log',
        id,
        budget,
        available({
          runId: id,
          messages: options.logs?.[id] ?? [
            { id: '12', text: 'Connection refused', level: 0, status: 4 },
          ],
          providerReturned: 1,
          truncated: false,
          limitations: [],
        }),
      ),
    listChanges: ({ runId: id }, budget) =>
      read('changes', id, budget, page(options.changes ?? [])),
    getRun: ({ id }, budget) =>
      read('final', id, budget, available(run(id, options.final ?? options.primary))),
  };
  reader.getProject = ({ id }, budget) =>
    read(
      'project',
      id,
      budget,
      available({
        id,
        name: id,
        archived: false,
        parentProjectId: options.projects?.[id] ?? 'Allowed',
      }),
    );
  const policy = options.childProject
    ? new ProjectPolicy(reader, ['Allowed'], { deadline: Date.now() + 5000 })
    : {
        assert: async (id) => {
          if (id === 'Foreign') throw new DomainError('POLICY_DENIED', 'Foreign');
        },
      };
  const result = await investigateFailure({
    primary,
    reader,
    policy,
    budget: { deadline: Date.now() + 5000 },
    maxChildProcesses: options.maxChildren ?? 24,
    childProcesses: () => launches,
    server: 'work',
    depth: options.depth ?? 4,
    maxNodes: 30,
    maxDiagnosedRuns: options.maxDiagnosed ?? 3,
    full: options.full ?? false,
    secrets: options.secrets ?? [],
  });
  const envelope = response('run.failure', { ...result.data });
  envelope.context = { server: 'work', job: primary.value.jobId, branch: primary.value.branch };
  envelope.status = result.complete ? 'ok' : 'partial';
  envelope.meta.complete = result.complete;
  envelope.meta.limitations = result.limitations;
  validateResponse(envelope);
  for (const finding of result.data.findings)
    for (const evidence of finding.evidence) {
      parse(evidence.retrieve.argv.slice(1));
      assert.ok(
        result.data.sources.some(
          (s) => s.id === evidence.sourceRef && ['complete', 'partial'].includes(s.state),
        ),
      );
    }
  return { result, calls, launches };
}

test('finished success short-circuits without any evidence or graph reads', async () => {
  const { result, calls, launches } = await fixture({ primary: { result: 'success' } });
  assert.equal(result.data.assessment, 'not_failed');
  assert.equal(result.complete, true);
  assert.deepEqual(calls, []);
  assert.equal(launches, 2);
  assert.equal(result.data.graph.complete, false);
  assert.equal(result.data.graph.nodes[0].expansion, 'not_requested');
  assert.ok(
    result.data.sources.filter((s) => s.kind !== 'run').every((s) => s.state === 'not_requested'),
  );
});

test('complete direct evidence yields observations, stable IDs and separately accounted muted occurrences', async () => {
  const options = {
    tests: { 1: [occurrence(1), occurrence(1, 2)] },
    muted: { 1: [occurrence(1, 3, { muted: true })] },
  };
  const first = await fixture(options);
  const second = await fixture(options);
  assert.equal(first.result.complete, true);
  assert.equal(first.result.data.assessment, 'failure_observed');
  assert.deepEqual(first.result.data.findings, second.result.data.findings);
  assert.equal(first.result.data.findings.filter((f) => f.kind === 'failed_test').length, 3);
  assert.ok(first.result.data.findings.every((f) => f.claim === 'observation'));
  assert.ok(first.result.data.findings.some((f) => f.summary.startsWith('Muted')));
  assert.equal(first.result.data.sources.find((s) => s.id === 'tests:1:muted').returned, 1);
  assert.equal(first.result.data.sources.find((s) => s.id === 'tests:1').returned, 2);
  assert.ok(!first.calls.includes('log:1'));
  assert.equal(first.result.data.sources.find((s) => s.id === 'tests:1').total, null);
});

test('source denial preserves successful siblings and requires log fallback without creating false zero', async () => {
  const { result, calls } = await fixture({
    errors: { 'problems:1': 'PERMISSION_DENIED', 'tests:1': 'NOT_FOUND' },
  });
  assert.equal(result.complete, false);
  const denied = result.data.sources.find((s) => s.id === 'problems:1');
  assert.equal(denied.state, 'unavailable');
  assert.equal(denied.returned, null);
  assert.equal(denied.total, null);
  assert.ok(calls.includes('log:1'));
  assert.ok(result.data.findings.some((f) => f.kind === 'log_signal'));
  assert.ok(!JSON.stringify(result.data).includes('rootCause'));
  const partial = await fixture({ partialPages: true });
  assert.equal(partial.result.complete, false);
  assert.equal(partial.result.data.sources.find((s) => s.id === 'tests:1').state, 'partial');
});

test('DAG candidate selection is bounded, deterministic and does not equate depth with cause', async () => {
  const { result } = await fixture({ adjacency: { 1: [2, 3], 2: [4], 3: [4], 4: [] } });
  assert.equal(result.data.graph.nodes.length, 4);
  assert.equal(result.data.graph.edges.length, 4);
  assert.deepEqual(result.data.selection.diagnosedRunIds, ['1', '2', '3']);
  assert.equal(result.data.selection.omittedDiagnosedRuns, 1);
  assert.equal(result.complete, false);
  assert.equal(result.data.sources.find((s) => s.id === 'tests:4').state, 'not_requested');
  const selected = selectDiagnosedRuns(
    result.data.graph,
    new Map([
      ['1', 0],
      ['2', 1],
      ['3', 1],
      ['4', 2],
    ]),
    2,
    new Set(['4']),
  );
  assert.deepEqual(
    selected.selected.map((s) => s.run.id),
    ['1', '4'],
  );
  assert.ok(result.data.findings.every((f) => !f.summary.includes('root cause')));
});

test('cycles, foreign observations and depth caps never create complete leaf assertions', async () => {
  const cycle = await fixture({ adjacency: { 1: [2], 2: [1] } });
  assert.equal(cycle.result.data.graph.cycles.length, 1);
  assert.equal(cycle.result.data.graph.nodes.length, 2);
  const denied = await fixture({ adjacency: { 1: [2] }, foreign: [2] });
  assert.equal(denied.result.data.graph.nodes.length, 1);
  assert.equal(denied.result.complete, false);
  assert.ok(!JSON.stringify(denied.result.data).includes('Job_2'));
  const capped = await fixture({ depth: 0 });
  assert.equal(capped.result.data.graph.nodes[0].expansion, 'depth_limit');
  assert.equal(capped.result.complete, false);
});

test('primary evidence and final state survive discovery exhaustion and root changes', async () => {
  const { result, calls, launches } = await fixture({
    primary: { state: 'running' },
    final: { state: 'finished', result: 'success' },
    maxChildren: 5,
    adjacency: { 1: [2] },
  });
  assert.equal(launches, 5);
  assert.ok(calls.includes('tests:1'));
  assert.ok(calls.includes('final:1'));
  assert.equal(result.data.assessment, 'inconclusive');
  assert.equal(result.data.run.result, 'success');
  assert.equal(result.complete, false);
  assert.ok(result.limitations.some((l) => l.code === 'ROOT_STATE_CHANGED'));
  const finalDenied = await fixture({
    primary: { state: 'running' },
    errors: { 'final:1': 'PERMISSION_DENIED' },
  });
  assert.equal(finalDenied.result.data.graph.complete, false);
  assert.equal(finalDenied.result.data.graph.nodes[0].expansion, 'unavailable');
  assert.equal(
    finalDenied.result.data.sources.find((s) => s.id === 'run:1:final').state,
    'unavailable',
  );
});

test('unknown, canceled and failed-to-start outcomes stay explicit and interruption propagates', async () => {
  for (const result of ['canceled', 'failed_to_start', 'error']) {
    const f = await fixture({ primary: { result, composite: true } });
    assert.equal(f.result.data.assessment, 'failure_observed');
    assert.equal(f.result.data.run.result, result);
  }
  const unknown = await fixture({ primary: { result: 'unknown', rawStatus: 'FUTURE' } });
  assert.equal(unknown.result.data.assessment, 'inconclusive');
  assert.equal(unknown.result.data.run.rawStatus, 'FUTURE');
  await assert.rejects(
    fixture({ errors: { 'tests:1': 'INTERRUPTED' } }),
    (e) => e.code === 'INTERRUPTED',
  );
});

test('redaction precedes fingerprints and previews; full evidence remains bounded', async () => {
  const secret = 'fixture-only-secret';
  const options = {
    secrets: [secret],
    tests: { 1: [occurrence(1, 1, { details: secret + 'x'.repeat(10000) })] },
  };
  const preview = await fixture(options);
  const full = await fixture({ ...options, full: true });
  const a = preview.result.data.findings.find((f) => f.kind === 'failed_test');
  const b = full.result.data.findings.find((f) => f.kind === 'failed_test');
  assert.equal(a.id, b.id);
  assert.equal(a.evidence[0].excerpt.length, 2000);
  assert.equal(b.evidence[0].excerpt.length, 8192);
  assert.equal(full.result.complete, false);
  assert.equal(full.result.truncated, true);
  assert.ok(!JSON.stringify(full.result).includes(secret));
});

test('graph ancestry respects primary evidence reservations and wholly denied dependencies remain unavailable', async () => {
  const f = await fixture({
    maxChildren: 8,
    adjacency: { 1: [2] },
    childProject: 'NestedA',
    projects: { NestedA: 'NestedB', NestedB: 'Allowed' },
    problems: { 1: [] },
    tests: { 1: [] },
  });
  assert.equal(f.launches, 8);
  assert.ok(f.calls.includes('tests:1'));
  assert.ok(f.calls.includes('log:1'));
  assert.ok(!f.calls.includes('project:NestedB'));
  assert.equal(f.result.data.sources.find((s) => s.id === 'tests:1').state, 'complete');
  assert.equal(f.result.data.sources.find((s) => s.id === 'log:1').state, 'complete');
  const denied = await fixture({
    adjacency: { 1: [2] },
    errors: { 'count:2': 'PERMISSION_DENIED', 'dependencies:2': 'PERMISSION_DENIED' },
  });
  const source = denied.result.data.sources.find((s) => s.id === 'dependencies:2');
  assert.equal(source.state, 'unavailable');
  assert.equal(source.reasonCode, 'PERMISSION_DENIED');
  assert.equal(source.returned, null);
  assert.equal(source.total, null);
  assert.ok(denied.result.data.findings.some((finding) => finding.runId === '1'));
});

test('sensitive identity components are redacted before fingerprints are generated', async () => {
  const options = { primary: { jobId: 'secret-job' }, secrets: ['secret-job'] };
  const secret = await fixture(options);
  const redacted = await fixture({ primary: { jobId: '[REDACTED]' } });
  assert.deepEqual(
    secret.result.data.findings.map((f) => f.id),
    redacted.result.data.findings.map((f) => f.id),
  );
});

test('nonempty optional changes preserve timestamp and verified root scope without causal findings', async () => {
  const change = {
    id: '101',
    version: 'a'.repeat(40),
    vcsRootId: 'Root',
    message: 'Context only',
    timestamp: '2026-10-02T00:00:00Z',
  };
  const good = await fixture({
    primary: { revisions: [{ vcsRootId: 'Root', revision: 'a'.repeat(40) }] },
    changes: [change],
  });
  assert.deepEqual(good.result.data.changes, [change]);
  assert.ok(good.result.data.findings.every((finding) => finding.kind !== 'revision_difference'));
  const foreign = await fixture({ changes: [change] });
  assert.equal(foreign.result.data.changes, undefined);
  assert.equal(
    foreign.result.data.sources.find((source) => source.kind === 'changes').state,
    'unavailable',
  );
  assert.equal(
    foreign.result.data.sources.find((source) => source.kind === 'changes').reasonCode,
    'CONTEXT_MISMATCH',
  );
  assert.equal(foreign.result.complete, true);
});

test('previewed summaries and multiline changes declare truncation', async () => {
  const name = await fixture({
    tests: { 1: [occurrence(1, 1, { name: 'x'.repeat(1500), details: 'short' })] },
  });
  assert.equal(name.result.truncated, true);
  const change = {
    id: '101',
    version: 'a'.repeat(40),
    vcsRootId: 'Root',
    message: 'Short line\nAdditional context',
    timestamp: null,
  };
  const f = await fixture({
    primary: { revisions: [{ vcsRootId: 'Root', revision: 'a'.repeat(40) }] },
    changes: [change],
  });
  assert.equal(f.result.truncated, true);
  assert.equal(f.result.data.changes[0].message, 'Short line');
});

test('optional change reductions preserve required findings and reconcile actual JSON/TOON bytes and counts', async () => {
  const changes = Array.from({ length: 10 }, (_, i) => ({
    id: String(i + 101),
    version: 'a'.repeat(40),
    vcsRootId: 'Root',
    message: '🦊'.repeat(4000),
    timestamp: null,
  }));
  const f = await fixture({
    full: true,
    primary: { revisions: [{ vcsRootId: 'Root', revision: 'a'.repeat(40) }] },
    changes,
  });
  const input = response('run.failure', { ...f.result.data });
  input.context = { server: 'work', job: 'Job_1', branch: 'main' };
  input.meta.truncated = f.result.truncated;
  input.meta.limitations = f.result.limitations;
  for (const format of ['json', 'toon']) {
    const rendered = render(input, format, 10000);
    assert.ok(Buffer.byteLength(rendered.document) <= 10000);
    assert.equal(rendered.response.status, 'ok');
    assert.deepEqual(rendered.response.data.findings, input.data.findings);
    assert.deepEqual(rendered.response.data.graph, input.data.graph);
    assert.equal(rendered.response.meta.truncated, true);
    assert.equal(
      rendered.response.data.selection.omittedChanges,
      10 - rendered.response.data.changes.length,
    );
    const source = rendered.response.data.sources.find((source) => source.kind === 'changes');
    assert.equal(source.returned, rendered.response.data.changes.length);
    assert.equal(source.providerReturned, 10);
    assert.equal(source.state, 'partial');
    validateResponse(format === 'json' ? JSON.parse(rendered.document) : decode(rendered.document));
  }
});

test('log findings declare their inspected window and safely encode hostile literal text', async () => {
  const f = await fixture({
    problems: { 1: [] },
    tests: { 1: [] },
    logs: {
      1: [
        {
          id: '12',
          text: '--error=Connection refused ' + 'x'.repeat(52) + '🦊',
          level: 0,
          status: 4,
        },
      ],
    },
  });
  const source = f.result.data.sources.find((source) => source.id === 'log:1');
  assert.deepEqual(source.window, {
    requested: 80,
    firstMessageId: '12',
    lastMessageId: '12',
    omittedProviderMessages: 0,
  });
  assert.equal(source.providerReturned, 1);
  const finding = f.result.data.findings.find((finding) => finding.kind === 'log_signal');
  assert.equal(
    parse(finding.evidence[0].retrieve.argv.slice(1)).flags.contains,
    '--error=Connection refused ' + 'x'.repeat(52) + '🦊',
  );
});

test('dependency evidence never JSON-escapes sensitive metadata before redaction', async () => {
  const secret = 'fixture-quote"and\\backslash';
  const f = await fixture({
    adjacency: { 1: [2] },
    runs: { 2: { branch: secret } },
    secrets: [secret],
  });
  const finding = f.result.data.findings.find((finding) => finding.kind === 'dependency_failure');
  assert.ok(finding);
  const escaped = JSON.stringify(secret).slice(1, -1);
  assert.ok(!JSON.stringify(finding).includes(escaped));
  const input = response('run.failure', { ...f.result.data });
  input.context = { server: 'work', job: 'Job_1', branch: 'main' };
  input.meta.complete = f.result.complete;
  input.status = f.result.complete ? 'ok' : 'partial';
  for (const format of ['json', 'toon']) {
    const rendered = render(input, format, 24576, [secret]);
    assert.ok(!rendered.document.includes(secret));
    assert.ok(!rendered.document.includes(escaped));
    assert.ok(rendered.document.includes('[REDACTED]'));
  }
});
