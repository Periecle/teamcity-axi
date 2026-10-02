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
  const dir = await mkdtemp(join(tmpdir(), 'axi-queue-'));
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
  const call = (args) =>
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
const job = ['queue', 'list', '--job', 'Payments_Build'];
const project = ['queue', 'list', '--project', 'Payments'];
test('released CLI scoped queue pages preserve observed state/reason, nullable totals and query-bound hints', async () => {
  const f = await fixture();
  try {
    f.server.setMode('queue-page');
    const json = await f.call([...job, '--json']),
      toon = await f.call(job);
    for (const r of [json, toon]) {
      assert.equal(r.code, 0);
      validateResponse(r.value);
      assert.deepEqual(r.value.data.items, [
        {
          id: '482194',
          jobId: 'Payments_Build',
          state: 'queued',
          branch: 'feature/refund',
          queuedAt: '2026-10-01T14:00:00.000Z',
          waitReason: 'Waiting for compatible agent',
        },
      ]);
      assert.equal(r.value.context.project, 'Payments');
      assert.equal(r.value.data.page.total, null);
      assert.equal(r.value.data.page.hasMore, true);
      assert.equal(r.value.meta.counts.childProcesses, 3);
      for (const action of r.value.next) parse(action.argv.slice(1));
      const next = await f.call([...r.value.next[0].argv.slice(1), '--json']);
      assert.equal(next.value.data.selection.position, 20);
    }
    assert.deepEqual(json.value.data.items, toon.value.data.items);
    const scoped = await f.call([...project, '--json']);
    assert.equal(scoped.value.data.selection.jobId, null);
    f.server.setMode('queue-empty-next');
    const empty = await f.call([...job, '--cursor', json.value.data.page.cursor, '--json']);
    assert.equal(empty.value.data.items.length, 0);
    assert.equal(empty.value.data.page.hasMore, true);
    f.server.setMode('queue-empty');
    const bounded = await f.call([...job, '--require-complete', '--json']);
    assert.equal(bounded.code, 1);
    assert.equal(bounded.value.status, 'partial');
    assert.equal(bounded.value.data.page.hasMore, null);
    assert.equal(bounded.value.data.page.total, null);
    f.server.setMode('queue-no-reason');
    const observed = await f.call([...job, '--json']);
    assert.equal(observed.value.data.items[0].waitReason, null);
    const expanded = await f.call([...observed.value.next[0].argv.slice(1), '--json']);
    assert.equal(expanded.value.data.run.id, '482194');
    assert.equal(expanded.value.data.run.state, 'queued');
    assert.equal(expanded.code, 0);
    assert.equal(expanded.value.data.run.result, 'unknown');
    assert.equal(expanded.value.data.run.rawStatus, null);
    assert.ok(expanded.value.meta.limitations.some((l) => l.code === 'RESULT_UNAVAILABLE'));
    assert.ok(f.server.requests.every((r) => r.method === 'GET' && r.authenticated));
    assert.ok(f.server.requests.every((r) => !r.query.fields?.includes('parameters')));
  } finally {
    await f.close();
  }
});
test('queue permission/capability/scope/schema failures remain errors; unsafe continuation keeps useful rows', async () => {
  const f = await fixture();
  try {
    for (const [mode, code] of [
      ['queue-denied', 'PERMISSION_DENIED'],
      ['queue-unsupported', 'NOT_FOUND'],
      ['queue-foreign', 'CONTEXT_MISMATCH'],
      ['queue-wrong-job', 'CONTEXT_MISMATCH'],
      ['queue-conflict', 'UPSTREAM_SCHEMA_MISMATCH'],
      ['queue-duplicate', 'UPSTREAM_SCHEMA_MISMATCH'],
      ['queue-malformed', 'UPSTREAM_SCHEMA_MISMATCH'],
    ]) {
      f.server.setMode(mode);
      const r = await f.call([...job, '--json']);
      assert.equal(r.code, 1, mode);
      assert.equal(r.value.error.code, code, mode);
      assert.equal(r.value.data, undefined);
      assert.ok(!r.stdout.includes('Forbidden'));
    }
    for (const mode of ['queue-surrogate-id', 'queue-control-id', 'queue-bidi-id']) {
      f.server.setMode(mode);
      const r = await f.call([...project, '--json']);
      assert.equal(r.code, 1, mode);
      assert.equal(r.value.error.code, 'UPSTREAM_SCHEMA_MISMATCH', mode);
      assert.equal(r.value.data, undefined);
      assert.ok(!r.stdout.includes('bad'));
    }
    for (const mode of ['queue-unsafe', 'queue-escalating', 'queue-unsafe-scope']) {
      f.server.setMode(mode);
      const r = await f.call([...job, '--json', '--require-complete']);
      assert.equal(r.code, 1);
      assert.equal(r.value.status, 'partial');
      assert.equal(r.value.data.items.length, 1);
      assert.equal(r.value.data.page.cursor, null);
      assert.ok(r.value.meta.limitations.some((l) => l.code === 'UNSAFE_CONTINUATION'));
    }
    f.server.setMode('queue-page');
    f.server.requests.splice(0);
    const missing = await f.call(['queue', 'list', '--json']);
    assert.equal(missing.code, 2);
    assert.equal(f.server.requests.length, 0);
    for (const scope of ['x'.repeat(257), 'bad\nproject', 'bad\u0085project', 'bad\u202eproject']) {
      const invalid = await f.call(['queue', 'list', '--project', scope, '--json']);
      assert.equal(invalid.code, 2);
      assert.equal(f.server.requests.length, 0);
    }
    const mismatch = await f.call([...job, '--project', 'Other', '--json']);
    assert.equal(mismatch.value.error.code, 'CONTEXT_MISMATCH');
  } finally {
    await f.close();
  }
});
test('queue cursors bind page size, command, project, resolved job scope and reject malformed tokens locally', async () => {
  const f = await fixture();
  try {
    f.server.setMode('queue-page');
    const first = await f.call([...project, '--json']);
    const cursor = decodeCursor(first.value.data.page.cursor);
    for (const token of [
      encodeCursor({ ...cursor, filterHash: '0'.repeat(64) }),
      encodeCursor({ ...cursor, command: 'job.list' }),
      'invalid-token',
    ]) {
      f.server.requests.splice(0);
      const rejected = await f.call([...project, '--cursor', token, '--json']);
      assert.equal(rejected.code, 2);
      assert.equal(rejected.value.error.code, 'USAGE_ERROR');
      assert.equal(f.server.requests.length, 0);
    }
    f.server.requests.splice(0);
    const resized = await f.call([
      ...project,
      '--limit',
      '1',
      '--cursor',
      first.value.data.page.cursor,
      '--json',
    ]);
    assert.equal(resized.code, 2);
    assert.equal(f.server.requests.length, 0);
    const wrongJob = await f.call([...job, '--cursor', first.value.data.page.cursor, '--json']);
    assert.equal(wrongJob.code, 2);
    assert.equal(wrongJob.value.error.code, 'USAGE_ERROR');
    const noHints = await f.call([...job, '--no-hints', '--json']);
    assert.equal(noHints.value.next, undefined);
    const schema = await f.call(['schema', 'queue.list', '--json']);
    assert.equal(schema.code, 0);
    assert.equal(schema.value.data.payload.$id, 'urn:teamcity-axi:queue-list:1.0');
  } finally {
    await f.close();
  }
});
test('queue unknown/changed state, wait-text redaction, output/capture ceilings and grouped diagnostics stay explicit', async () => {
  const f = await fixture();
  try {
    for (const flags of [['--json'], []]) {
      for (const [mode, state, code] of [
        ['queue-unknown', 'unknown', 'UNKNOWN_QUEUE_STATE'],
        ['queue-running', 'running', 'QUEUE_STATE_CHANGED'],
        ['queue-finished', 'finished', 'QUEUE_STATE_CHANGED'],
      ]) {
        f.server.setMode(mode);
        const r = await f.call([...job, '--require-complete', ...flags]);
        assert.equal(r.code, 1);
        assert.equal(r.value.status, 'partial');
        assert.equal(r.value.data.items[0].state, state);
        assert.ok(r.value.meta.limitations.some((l) => l.code === code));
      }
      f.server.setMode('queue-secret');
      const clean = await f.call([...job, ...flags]);
      assert.equal(clean.value.data.items[0].waitReason, '[REDACTED]');
      assert.equal(clean.value.data.items[0].branch, '[REDACTED]');
      assert.ok(!clean.stdout.includes('fixture-only-token'));
      f.server.setMode('queue-oversized');
      const large = await f.call([...job, '--max-bytes', '2048', ...flags]);
      assert.equal(large.code, 1);
      assert.equal(large.value.error.code, 'INPUT_LIMIT_EXCEEDED');
      assert.ok(large.value.meta.limitations.some((l) => l.code === 'OUTPUT_LIMIT_EXCEEDED'));
      assert.ok(Buffer.byteLength(large.stdout) <= 2048);
    }
    f.server.setMode('queue-many-unknown');
    const many = await f.call([...job, '--limit', '100', '--max-bytes', '65536', '--json']);
    assert.equal(many.code, 0);
    assert.equal(many.value.status, 'partial');
    assert.equal(many.value.data.items.length, 100);
    assert.equal(
      many.value.meta.limitations.filter((l) => l.code === 'UNKNOWN_QUEUE_STATE').length,
      1,
    );
    assert.equal(
      many.value.meta.limitations.filter((l) => l.code === 'INVALID_TIMESTAMP').length,
      1,
    );
    f.server.setMode('queue-huge');
    const huge = await f.call([...job, '--json']);
    assert.equal(huge.code, 1);
    assert.equal(huge.value.error.code, 'INPUT_LIMIT_EXCEEDED');
  } finally {
    await f.close();
  }
});
