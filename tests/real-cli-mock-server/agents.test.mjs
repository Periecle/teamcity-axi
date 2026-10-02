import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, readFile, rm } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { tmpdir } from 'node:os';
import { createHash } from 'node:crypto';
import { decode } from '@toon-format/toon';
import { mockServer } from '../fixtures/mock-server.mjs';
import { parse } from '../../dist/cli/parser.js';
import { validateResponse } from '../../dist/output/schema.js';
import { decodeCursor, encodeCursor } from '../../dist/adapter/cursor.js';
async function fixture() {
  const binary = process.env.TEAMCITY_AXI_TEST_BINARY;
  if (!binary) throw Error('Verified binary required; no skips');
  const manifest = JSON.parse(await readFile('docs/compatibility.json', 'utf8'));
  const sha = createHash('sha256')
    .update(await readFile(binary))
    .digest('hex');
  assert.ok(manifest.artifacts.some((a) => a.binarySha256 === sha));
  const server = await mockServer();
  const dir = await mkdtemp(join(tmpdir(), 'axi-agents-'));
  await mkdir(join(dir, 'teamcity-axi'));
  await writeFile(
    join(dir, 'teamcity-axi', 'config.json'),
    JSON.stringify({
      schemaVersion: '1.0',
      readOnly: true,
      defaultServer: 'work',
      binaryPath: resolve(binary),
      servers: {
        work: { url: server.base, allowHttpLoopback: true, allowedProjects: ['Payments'] },
      },
    }),
    { mode: 0o600 },
  );
  const call = (args, onSpawn) =>
    new Promise((done, reject) => {
      const child = spawn(process.execPath, [resolve('bin/teamcity-axi.mjs'), ...args], {
        cwd: dir,
        env: {
          HOME: dir,
          XDG_CONFIG_HOME: dir,
          PATH: process.env.PATH,
          TEAMCITY_URL: server.base,
          TEAMCITY_TOKEN: 'fixture-only-token',
        },
        stdio: ['ignore', 'pipe', 'pipe'],
        timeout: 10000,
      });
      onSpawn?.(child);
      const out = [],
        err = [];
      child.stdout.on('data', (b) => out.push(b));
      child.stderr.on('data', (b) => err.push(b));
      child.on('error', reject);
      child.on('close', (code) => {
        const stdout = Buffer.concat(out).toString(),
          stderr = Buffer.concat(err).toString();
        done({
          code,
          stdout,
          stderr,
          value: args.includes('--json') ? JSON.parse(stdout) : decode(stdout),
        });
      });
    });
  return {
    server,
    call,
    close: async () => {
      await server.close();
      await rm(dir, { recursive: true, force: true });
    },
  };
}
const project = ['agent', 'list', '--project', 'Payments'];
const job = ['agent', 'list', '--job', 'Payments_Build'];
const pool = ['agent', 'list', '--pool', '1'];
const view = ['agent', 'view', '7'];
test('released agent reads preserve separate states, scoped pages, pool zero and exact retrieval in both serializers', async () => {
  const f = await fixture();
  try {
    f.server.setMode('agent-page');
    const json = await f.call([...project, '--json']),
      toon = await f.call(project);
    for (const r of [json, toon]) {
      assert.equal(r.code, 0);
      validateResponse(r.value);
      assert.equal(r.value.data.agents[0].id, '7');
      assert.equal(r.value.data.agents[0].connected, true);
      assert.equal(r.value.data.agents[0].enabled, true);
      assert.equal(r.value.data.agents[0].authorized, true);
      assert.equal(r.value.data.agents[0].activeRunState, 'not_reported');
      assert.equal(r.value.status, 'partial');
      assert.equal(r.value.data.page.total, null);
      assert.equal(r.value.data.page.hasMore, true);
      assert.equal(r.value.meta.counts.childProcesses, 3);
      for (const hint of r.value.next) parse(hint.argv.slice(1));
    }
    assert.deepEqual(json.value.data.agents, toon.value.data.agents);
    const next = await f.call([...json.value.next[0].argv.slice(1), '--json']);
    assert.equal(next.value.data.selection.position, 20);
    const scoped = await f.call([...job, '--json']);
    assert.equal(scoped.value.context.project, 'Payments');
    assert.equal(scoped.value.data.selection.meaning, 'compatible_agents');
    const pooled = await f.call([...pool, '--json']);
    assert.equal(pooled.code, 0);
    assert.equal(pooled.value.data.selection.projectId, null);
    assert.equal(pooled.value.data.selection.meaning, 'pool_agents');
    f.server.setMode('agent-idle');
    const exact = await f.call([...view, '--json']);
    assert.equal(exact.code, 0);
    validateResponse(exact.value);
    assert.equal(exact.value.data.agent.activeRunState, 'idle');
    assert.equal(exact.value.status, 'ok');
    assert.equal(exact.value.meta.counts.childProcesses, 2);
    f.server.setMode('agent-state-mix');
    const mixed = await f.call([...view, '--json']);
    assert.equal(mixed.value.data.agent.connected, true);
    assert.equal(mixed.value.data.agent.enabled, false);
    assert.equal(mixed.value.data.agent.authorized, false);
    assert.equal(mixed.value.status, 'ok');
    f.server.setMode('agent-idle');
    const listing = await f.call([...pool, '--json']);
    const hinted = await f.call([...listing.value.next[0].argv.slice(1), '--json']);
    assert.equal(hinted.value.data.agent.id, '7');
    assert.equal(hinted.value.data.selection.poolId, '1');
    f.server.setMode('agent-zero-pool');
    const zero = await f.call(['agent', 'list', '--pool', '0', '--json']);
    assert.equal(zero.code, 0);
    assert.equal(zero.value.data.agents[0].pool.id, '0');
    assert.ok(f.server.requests.every((r) => r.method === 'GET' && r.authenticated));
    assert.ok(
      f.server.requests.every(
        (r) => !r.query.fields?.includes('properties') && !r.query.fields?.includes('environment'),
      ),
    );
  } finally {
    await f.close();
  }
});
test('agent denial, missing capability, wrong exact ID, pool mismatch and malformed DTOs cannot become empty success', async () => {
  const f = await fixture();
  try {
    for (const [mode, code, args] of [
      ['agent-denied', 'PERMISSION_DENIED', project],
      ['agent-unsupported', 'NOT_FOUND', project],
      ['agent-missing', 'NOT_FOUND', view],
      ['agent-wrong-id', 'CONTEXT_MISMATCH', view],
      ['agent-wrong-pool', 'CONTEXT_MISMATCH', pool],
      ['agent-malformed-id', 'UPSTREAM_SCHEMA_MISMATCH', project],
      ['agent-bad-state', 'UPSTREAM_SCHEMA_MISMATCH', project],
      ['agent-malformed-page', 'UPSTREAM_SCHEMA_MISMATCH', project],
      ['agent-duplicate', 'UPSTREAM_SCHEMA_MISMATCH', project],
      ['agent-conflict-active', 'UPSTREAM_SCHEMA_MISMATCH', view],
    ]) {
      f.server.setMode(mode);
      const r = await f.call([...args, '--json']);
      assert.equal(r.code, 1, mode);
      assert.equal(r.value.error.code, code, mode);
      assert.equal(r.value.data, undefined);
      assert.ok(!r.stdout.includes('Forbidden'));
    }
    f.server.setMode('agent-page');
    f.server.requests.splice(0);
    for (const args of [
      ['agent', 'list'],
      ['agent', 'list', '--pool', '-1'],
      ['agent', 'list', '--pool', '01'],
      ['agent', 'list', '--project', 'bad\u202eid'],
      ['agent', 'view', '0'],
      ['agent', 'view', '9007199254740993'],
    ]) {
      const r = await f.call([...args, '--json']);
      assert.equal(r.code, 2);
      assert.equal(f.server.requests.length, 0);
    }
    const mismatch = await f.call([...job, '--project', 'Other', '--json']);
    assert.equal(mismatch.value.error.code, 'CONTEXT_MISMATCH');
  } finally {
    await f.close();
  }
});
test('agent continuation binds policy/scope/filter/page size and retains useful rows after unsafe or expired continuation', async () => {
  const f = await fixture();
  try {
    f.server.setMode('agent-page');
    const first = await f.call([...project, '--json']);
    const cursor = decodeCursor(first.value.data.page.cursor);
    for (const args of [
      [...project, '--pool', '1', '--cursor', first.value.data.page.cursor],
      [...project, '--limit', '1', '--cursor', first.value.data.page.cursor],
      [...project, '--cursor', encodeCursor({ ...cursor, command: 'queue.list' })],
      [...project, '--cursor', encodeCursor({ ...cursor, filterHash: '0'.repeat(64) })],
    ]) {
      f.server.requests.splice(0);
      const r = await f.call([...args, '--json']);
      assert.equal(r.code, 2);
      assert.equal(f.server.requests.length, 0);
    }
    for (const mode of ['agent-unsafe', 'agent-unsafe-scope', 'agent-unsafe-filter']) {
      f.server.setMode(mode);
      const r = await f.call([...project, '--require-complete', '--json']);
      assert.equal(r.code, 1);
      assert.equal(r.value.status, 'partial');
      assert.equal(r.value.data.agents.length, 1);
      assert.equal(r.value.data.page.cursor, null);
      assert.ok(r.value.meta.limitations.some((l) => l.code === 'UNSAFE_CONTINUATION'));
    }
    f.server.setMode('agent-slow-page');
    const expiring = encodeCursor({ ...cursor, expiresAt: Date.now() + 700 });
    const expired = await f.call([...project, '--cursor', expiring, '--json']);
    assert.equal(expired.code, 0);
    assert.equal(expired.value.data.agents.length, 1);
    assert.equal(expired.value.data.page.cursor, null);
    assert.ok(expired.value.meta.limitations.some((l) => l.code === 'CURSOR_EXPIRED'));
  } finally {
    await f.close();
  }
});
test('agent active pointers obey current project policy, preserve exact retrieval and respect strict/no-hints and byte ceilings', async () => {
  const f = await fixture();
  try {
    f.server.setMode('agent-active');
    const active = await f.call([...view, '--json']);
    assert.equal(active.code, 0);
    assert.equal(active.value.data.agent.activeRun.id, '482193');
    for (const hint of active.value.next) parse(hint.argv.slice(1));
    const detail = await f.call([...active.value.next[0].argv.slice(1), '--json']);
    assert.equal(detail.value.data.run.id, '482193');
    f.server.setMode('agent-foreign-active');
    for (const args of [view, [...view, '--project', 'Payments'], project]) {
      const r = await f.call([...args, '--json']);
      assert.equal(r.code, 0);
      const agent = r.value.data.agent ?? r.value.data.agents[0];
      assert.equal(agent.activeRun, null);
      assert.equal(agent.activeRunState, 'unavailable');
      if (args === view || args[1] === 'view') assert.equal(r.value.next, undefined);
      else for (const hint of r.value.next) parse(hint.argv.slice(1));
      assert.ok(!r.stdout.includes('Forbidden'));
    }
    f.server.setMode('agent-unknown');
    const unknown = await f.call([...project, '--json', '--require-complete', '--no-hints']);
    assert.equal(unknown.code, 1);
    assert.equal(unknown.value.next, undefined);
    assert.equal(unknown.value.data.agents[0].connected, null);
    f.server.setMode('agent-secret');
    for (const flags of [['--json'], []]) {
      const r = await f.call([...view, ...flags]);
      assert.equal(r.code, 0);
      assert.equal(r.value.data.agent.name, '[REDACTED]');
      assert.ok(!r.stdout.includes('fixture-only-token'));
    }
    f.server.setMode('agent-huge');
    const huge = await f.call([...view, '--json']);
    assert.equal(huge.code, 1);
    assert.equal(huge.value.error.code, 'INPUT_LIMIT_EXCEEDED');
    f.server.setMode('agent-oversized');
    for (const flags of [['--json'], []]) {
      const r = await f.call([...view, '--max-bytes', '2048', ...flags]);
      assert.equal(r.code, 1);
      assert.equal(r.value.error.code, 'INPUT_LIMIT_EXCEEDED');
      assert.ok(Buffer.byteLength(r.stdout) <= 2048);
    }
    f.server.setMode('agent-many-unknown');
    const many = await f.call([...project, '--limit', '100', '--max-bytes', '65536', '--json']);
    assert.equal(many.code, 0);
    assert.equal(many.value.data.agents.length, 100);
    assert.ok(many.value.meta.limitations.length <= 5);
    const schema = await f.call(['schema', 'agent.view', '--json']);
    assert.equal(schema.value.data.payload.$id, 'urn:teamcity-axi:agent-view:1.0');
  } finally {
    await f.close();
  }
});
test('interruption during an active-pointer policy read preserves exit 130 and emits one error document', async () => {
  const f = await fixture();
  try {
    f.server.setMode('agent-policy-hang');
    let child;
    const pending = f.call([...view, '--json'], (value) => {
      child = value;
    });
    const deadline = Date.now() + 5000;
    while (
      !f.server.requests.some((r) => r.path.includes('UGF5bWVudHNfQ2hpbGQ')) &&
      Date.now() < deadline
    )
      await new Promise((r) => setTimeout(r, 20));
    assert.ok(
      f.server.requests.some((r) => r.path.includes('UGF5bWVudHNfQ2hpbGQ')),
      'Active-pointer policy request must be running before interruption',
    );
    child.kill('SIGINT');
    const r = await pending;
    assert.equal(r.code, 130);
    assert.equal(r.value.status, 'error');
    assert.equal(r.value.error.code, 'INTERRUPTED');
    assert.equal(r.value.data, undefined);
  } finally {
    await f.close();
  }
});
