import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  agentFields,
  agentFilters,
  agentRequest,
  normalizeAgent,
  normalizeAgentPage,
} from '../../dist/adapter/agents.js';
import { NativeTeamCityReader } from '../../dist/adapter/reader.js';
import { parseRaw } from '../../dist/adapter/raw.js';
const contract = JSON.parse(
  readFileSync('tests/fixtures/teamcity-2026.2-native-1.5.0/contract.json', 'utf8'),
);
const raw = (name) => {
  const r = contract.records[name];
  return {
    stdout: Buffer.from(r.stdout),
    stderr: Buffer.from(r.stderr),
    exitCode: r.code,
    signal: r.signal,
  };
};
const body = (name) => parseRaw(raw(name)).body;
const server = 'http://127.0.0.1:32768';
const query = { projectId: 'AxiContract', count: 1, start: 0, scanLimit: 5000 };
test('recorded agent scopes preserve separate availability, safe pool metadata and unreported activity', () => {
  const page = normalizeAgentPage(body('agents-project'), query, server, []);
  assert.deepEqual(page.items, [
    {
      id: '1',
      name: 'axi-contract-agent',
      connected: true,
      enabled: true,
      authorized: true,
      pool: { id: '1', name: 'axi-contract-pool-20261002' },
      activeRun: null,
      activeRunState: 'not_reported',
    },
  ]);
  assert.equal(page.hasMore, true);
  assert.equal(page.position, 1);
  assert.ok(page.limitations.some((l) => l.code === 'ACTIVE_RUN_UNREPORTED'));
  const empty = normalizeAgentPage(body('agents-project-next'), { ...query, start: 1 }, server, []);
  assert.equal(empty.items.length, 0);
  assert.equal(empty.hasMore, null);
  assert.ok(empty.limitations.some((l) => l.code === 'SCAN_COVERAGE_UNKNOWN'));
  assert.equal(
    agentFilters({ ...query, jobId: 'AxiContract_Fail' }).filter((s) => s.startsWith('compatible:'))
      .length,
    1,
  );
  assert.ok(!agentFields.includes('properties'));
});
test('agent unknown states and omitted build never imply ready or idle; positive active pointers require exact identity', () => {
  const dto = body('agent-detail');
  const mixed = normalizeAgent(
    { ...dto, connected: true, enabled: false, authorized: false, build: null },
    query,
    [],
  );
  assert.equal(mixed.agent.connected, true);
  assert.equal(mixed.agent.enabled, false);
  assert.equal(mixed.agent.authorized, false);
  const partial = normalizeAgent({ id: 1, name: 'Synthetic' }, query, []);
  assert.equal(partial.agent.connected, null);
  assert.equal(partial.agent.enabled, null);
  assert.equal(partial.agent.authorized, null);
  assert.equal(partial.agent.pool, null);
  assert.equal(partial.agent.activeRunState, 'not_reported');
  assert.equal(partial.limitations.filter((l) => l.code === 'AGENT_STATUS_UNAVAILABLE').length, 1);
  assert.equal(
    normalizeAgent({ ...dto, pool: { id: 0, name: 'Default' }, build: null }, {}, []).agent
      .activeRunState,
    'idle',
  );
  assert.equal(
    normalizeAgent({ ...dto, pool: { id: 0, name: 'Default' }, build: null }, { poolId: '0' }, [])
      .agent.pool.id,
    '0',
  );
  const build = {
    id: 12,
    buildTypeId: 'AxiContract_Fail',
    buildType: { id: 'AxiContract_Fail', projectId: 'AxiContract' },
  };
  const active = normalizeAgent({ ...dto, build }, query, []);
  assert.deepEqual(active.agent.activeRun, {
    id: '12',
    jobId: 'AxiContract_Fail',
    projectId: 'AxiContract',
  });
  const foreign = normalizeAgent(
    { ...dto, build: { ...build, buildType: { ...build.buildType, projectId: 'Forbidden' } } },
    query,
    [],
  );
  assert.equal(foreign.agent.activeRun, null);
  assert.equal(foreign.agent.activeRunState, 'unavailable');
  assert.ok(!JSON.stringify(foreign).includes('Forbidden'));
  const secret = normalizeAgent(
    { ...dto, name: 'canary\x1b[31m', pool: { id: 1, name: 'canary' } },
    query,
    ['canary'],
  );
  assert.equal(secret.agent.name, '[REDACTED]');
  assert.equal(secret.agent.pool.name, '[REDACTED]');
});
test('unsafe agent identities, state types, active scope and duplicate/malformed pages fail closed', () => {
  const dto = body('agent-detail');
  for (const patch of [
    { id: 0 },
    { id: 9007199254740992 },
    { id: '01' },
    { name: null },
    { enabled: 'true' },
    { pool: { id: -1 } },
    { pool: { id: '01' } },
    { pool: { id: 'bad\ud800' } },
    { build: { id: 12, buildTypeId: 'x', buildType: { id: 'other', projectId: 'AxiContract' } } },
    {
      build: {
        id: 12,
        buildTypeId: 'bad\u0085job',
        buildType: { id: 'bad\u0085job', projectId: 'AxiContract' },
      },
    },
  ])
    assert.throws(
      () => normalizeAgent({ ...dto, ...patch }, query, []),
      (e) => e.code === 'UPSTREAM_SCHEMA_MISMATCH',
    );
  assert.throws(
    () => normalizeAgent(dto, { poolId: '2' }, []),
    (e) => e.code === 'CONTEXT_MISMATCH',
  );
  for (const page of [
    { count: 0, agent: [dto] },
    { count: 1, agent: null },
    { count: 2, agent: [dto, dto] },
  ])
    assert.throws(() => normalizeAgentPage(page, { ...query, count: 2 }, server, []));
  for (const patch of [
    { projectId: undefined },
    { poolId: '01' },
    { poolId: '-1' },
    { poolId: '9007199254740993' },
    { projectId: 'bad\u202eid' },
    { count: 101 },
    { start: 5000 },
    { scanLimit: 5001 },
  ])
    assert.throws(
      () => agentRequest({ ...query, ...patch }),
      (e) => e.exitCode === 2,
    );
});
test('agent continuation preserves rows after unsafe filtering and groups hundred-row missing metadata', () => {
  const original = body('agents-project');
  for (const nextHref of [
    'https://attacker.invalid/app/rest/agents',
    original.nextHref.replace('defaultFilter:false', 'defaultFilter:true'),
    original.nextHref.replace('5000', '10000'),
    original.nextHref.replace('start:1', 'start:0'),
  ]) {
    const page = normalizeAgentPage({ ...original, nextHref }, query, server, []);
    assert.equal(page.items.length, 1);
    assert.equal(page.hasMore, null);
    assert.ok(page.limitations.some((l) => l.code === 'UNSAFE_CONTINUATION'));
  }
  const many = normalizeAgentPage(
    {
      count: 100,
      agent: Array.from({ length: 100 }, (_, i) => ({ id: i + 1, name: 'Synthetic' })),
    },
    { ...query, count: 100 },
    server,
    [],
  );
  assert.equal(many.items.length, 100);
  assert.equal(many.limitations.length, 4);
});
test('recorded exact agent reader checks requested ID and preserves unavailable pool and scope reads', async () => {
  let operation;
  const reader = new NativeTeamCityReader(
    {
      execute: async (value) => {
        operation = value;
        return raw('agent-detail-job');
      },
    },
    server,
  );
  const result = await reader.getAgent(
    { id: '1', projectId: 'AxiContract', jobId: 'AxiContract_Fail' },
    { deadline: Date.now() + 1000 },
  );
  assert.equal(result.state, 'available');
  assert.equal(result.value.id, '1');
  assert.equal(operation.path, contract.records['agent-detail-job'].args[1]);
  const wrong = await reader.getAgent({ id: '2' }, { deadline: Date.now() + 1000 });
  assert.equal(wrong.state, 'unavailable');
  assert.equal(wrong.error.code, 'CONTEXT_MISMATCH');
  for (const [name, code] of [
    ['agents-project-denied', 'PERMISSION_DENIED'],
    ['agents-pool', 'NOT_FOUND'],
  ]) {
    const unavailable = new NativeTeamCityReader({ execute: async () => raw(name) }, server);
    const result = await unavailable.listAgents(
      { ...query, poolId: name === 'agents-pool' ? '1' : undefined },
      { deadline: Date.now() + 1000 },
    );
    assert.equal(result.state, 'unavailable');
    assert.equal(result.error.code, code);
  }
});
