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
  const dir = await mkdtemp(join(tmpdir(), 'axi-jobs-'));
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
const list = ['job', 'list', '--project', 'Payments'];
const view = ['job', 'view', 'Payments_Build'];
test('released CLI job views and scoped pages expose only safe metadata with typed continuation', async () => {
  const f = await fixture();
  try {
    f.server.setMode('jobs-page');
    const json = await f.call([...list, '--json']);
    const toon = await f.call(list);
    for (const r of [json, toon]) {
      assert.equal(r.code, 0);
      validateResponse(r.value);
      assert.equal(r.value.data.page.hasMore, true);
      assert.equal(r.value.data.page.total, null);
      assert.deepEqual(r.value.data.jobs, [
        { id: 'Payments_Build', name: 'Build', projectId: 'Payments', paused: false },
      ]);
      assert.equal(r.value.data.selection.membership, 'direct');
      assert.equal(r.value.meta.counts.childProcesses, 3);
      for (const action of r.value.next) parse(action.argv.slice(1));
    }
    assert.deepEqual(json.value.data.jobs, toon.value.data.jobs);
    const exact = await f.call([...view, '--project', 'Payments', '--json']);
    assert.equal(exact.code, 0);
    validateResponse(exact.value);
    assert.equal(exact.value.meta.counts.childProcesses, 2);
    assert.deepEqual(exact.value.data.job, json.value.data.jobs[0]);
    for (const action of exact.value.next) parse(action.argv.slice(1));
    const hinted = await f.call(json.value.next[0].argv.slice(1).concat('--json'));
    assert.equal(hinted.value.data.selection.position, 20);
    f.server.setMode('jobs-leading-id');
    const leading = await f.call([...list, '--json']);
    assert.equal(leading.value.data.jobs[0].id, '--job');
    for (const hint of leading.value.next) parse(hint.argv.slice(1));
    const expanded = await f.call(['--json', ...leading.value.next[0].argv.slice(1)]);
    assert.equal(expanded.value.data.job.id, '--job');
    for (const hint of expanded.value.next) parse(hint.argv.slice(1));
    assert.ok(f.server.requests.every((r) => r.method === 'GET' && r.authenticated));
    assert.ok(f.server.requests.every((r) => !r.query.fields?.includes('parameters')));
    f.server.setMode('jobs-empty-next');
    const empty = await f.call([...list, '--json', '--cursor', json.value.data.page.cursor]);
    assert.equal(empty.value.data.page.returned, 0);
    assert.equal(empty.value.data.page.hasMore, true);
    assert.ok(empty.value.data.page.cursor);
    f.server.setMode('jobs-empty');
    const unknown = await f.call([...list, '--json', '--require-complete']);
    assert.equal(unknown.code, 1);
    assert.equal(unknown.value.status, 'partial');
    assert.equal(unknown.value.data.page.hasMore, null);
    assert.equal(unknown.value.data.page.total, null);
  } finally {
    await f.close();
  }
});
test('job permission, identity, policy, capability and schema errors never become empty success', async () => {
  const f = await fixture();
  try {
    for (const [mode, args, error] of [
      ['jobs-denied', list, 'PERMISSION_DENIED'],
      ['jobs-unsupported', list, 'NOT_FOUND'],
      ['jobs-foreign', list, 'CONTEXT_MISMATCH'],
      ['jobs-duplicate', list, 'UPSTREAM_SCHEMA_MISMATCH'],
      ['jobs-malformed', list, 'UPSTREAM_SCHEMA_MISMATCH'],
      ['jobs-detail-denied', view, 'PERMISSION_DENIED'],
      ['jobs-detail-missing', view, 'NOT_FOUND'],
      ['jobs-wrong-id', view, 'CONTEXT_MISMATCH'],
      ['jobs-foreign', view, 'POLICY_DENIED'],
    ]) {
      f.server.setMode(mode);
      const r = await f.call([...args, '--json']);
      assert.equal(r.code, 1, mode);
      assert.equal(r.value.error.code, error, mode);
      assert.equal(r.value.data, undefined);
      assert.ok(!r.stdout.includes('Forbidden'));
      validateResponse(r.value);
    }
    f.server.setMode('jobs-page');
    const mismatch = await f.call([...view, '--project', 'Other', '--json']);
    assert.equal(mismatch.value.error.code, 'CONTEXT_MISMATCH');
    f.server.requests.splice(0);
    const missing = await f.call(['job', 'list', '--json']);
    assert.equal(missing.code, 2);
    assert.equal(f.server.requests.length, 0);
    for (const args of [
      ['job', 'view', 'x'.repeat(257), '--json'],
      ['job', 'list', '--project', 'Payments', '--limit', '101', '--json'],
      ['job', 'list', '--project', 'x'.repeat(257), '--json'],
      ['job', 'list', '--project', 'invalid\nproject', '--json'],
    ]) {
      const r = await f.call(args);
      assert.equal(r.code, 2);
      assert.equal(f.server.requests.length, 0);
    }
  } finally {
    await f.close();
  }
});
test('job cursor reconstruction rejects scope changes and preserves rows after unsafe continuation', async () => {
  const f = await fixture();
  try {
    for (const mode of ['jobs-unsafe', 'jobs-escalating', 'jobs-scope-change']) {
      f.server.setMode(mode);
      const r = await f.call([...list, '--json', '--require-complete']);
      assert.equal(r.code, 1);
      assert.equal(r.value.status, 'partial');
      assert.equal(r.value.data.jobs.length, 1);
      assert.equal(r.value.data.page.cursor, null);
      assert.ok(r.value.meta.limitations.some((l) => l.code === 'UNSAFE_CONTINUATION'));
    }
    f.server.setMode('jobs-page');
    const initial = await f.call([...list, '--json']);
    const cursor = decodeCursor(initial.value.data.page.cursor);
    for (const forged of [
      encodeCursor({ ...cursor, filterHash: '0'.repeat(64) }),
      encodeCursor({ ...cursor, command: 'run.list' }),
      'invalid-cursor',
    ]) {
      f.server.requests.splice(0);
      const r = await f.call([...list, '--cursor', forged, '--json']);
      assert.equal(r.code, 2);
      assert.equal(r.value.error.code, 'USAGE_ERROR');
      assert.equal(f.server.requests.length, 0);
    }
    const changedSize = await f.call([
      ...list,
      '--cursor',
      initial.value.data.page.cursor,
      '--limit',
      '1',
      '--json',
    ]);
    assert.equal(changedSize.code, 2);
    const noHints = await f.call([...list, '--no-hints', '--json']);
    assert.equal(noHints.value.next, undefined);
  } finally {
    await f.close();
  }
});
test('unknown paused state, redacted names and oversized job output remain explicit in both serializers', async () => {
  const f = await fixture();
  try {
    for (const flags of [['--json'], []]) {
      f.server.setMode('jobs-unknown-paused');
      const r = await f.call([...view, '--require-complete', ...flags]);
      assert.equal(r.code, 1);
      assert.equal(r.value.status, 'partial');
      assert.equal(r.value.data.job.paused, null);
      assert.ok(r.value.meta.limitations.some((l) => l.code === 'JOB_PAUSED_UNAVAILABLE'));
      f.server.setMode('jobs-secret');
      for (const args of [list, view]) {
        const redacted = await f.call([...args, ...flags]);
        assert.equal(redacted.code, 0);
        assert.ok(!redacted.stdout.includes('fixture-only-token'));
        assert.ok(!redacted.stdout.includes('server-private-parameter'));
        const job = args === list ? redacted.value.data.jobs[0] : redacted.value.data.job;
        assert.equal(job.name, '[REDACTED]');
      }
      f.server.setMode('jobs-oversized');
      const large = await f.call([...list, '--limit', '100', '--max-bytes', '2048', ...flags]);
      assert.equal(large.code, 1);
      assert.equal(large.value.error.code, 'INPUT_LIMIT_EXCEEDED');
      assert.ok(
        large.value.meta.limitations.some(
          (l) => l.code === 'OUTPUT_LIMIT_EXCEEDED' && l.source === 'output',
        ),
      );
      assert.ok(Buffer.byteLength(large.stdout) <= 2048);
    }
    f.server.setMode('jobs-all-unknown');
    const unknownPage = await f.call([...list, '--limit', '100', '--json', '--require-complete']);
    assert.equal(unknownPage.code, 1);
    assert.equal(unknownPage.value.status, 'partial');
    assert.equal(unknownPage.value.data.jobs.length, 100);
    assert.ok(unknownPage.value.data.jobs.every((j) => j.paused === null));
    assert.equal(
      unknownPage.value.meta.limitations.filter((l) => l.code === 'JOB_PAUSED_UNAVAILABLE').length,
      1,
    );
    f.server.setMode('jobs-huge');
    const huge = await f.call([...list, '--json']);
    assert.equal(huge.code, 1);
    assert.equal(huge.value.error.code, 'INPUT_LIMIT_EXCEEDED');
    for (const name of ['job.list', 'job.view']) {
      f.server.requests.splice(0);
      const schema = await f.call(['schema', name, '--json']);
      assert.equal(schema.code, 0);
      assert.equal(schema.value.data.payload.$id, `urn:teamcity-axi:${name.replace('.', '-')}:1.0`);
      assert.equal(f.server.requests.length, 0);
    }
  } finally {
    await f.close();
  }
});
