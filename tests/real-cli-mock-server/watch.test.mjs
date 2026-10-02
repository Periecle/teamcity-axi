import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, readFile, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { createHash } from 'node:crypto';
import { setTimeout } from 'node:timers/promises';
import { decode } from '@toon-format/toon';
import { mockServer } from '../fixtures/mock-server.mjs';
import { validateResponse } from '../../dist/output/schema.js';
import { parse } from '../../dist/cli/parser.js';

async function fixture() {
  const binary = process.env.TEAMCITY_AXI_TEST_BINARY;
  if (!binary) throw Error('Verified released CLI required; no skips');
  const manifest = JSON.parse(await readFile('docs/compatibility.json', 'utf8'));
  const sha = createHash('sha256')
    .update(await readFile(binary))
    .digest('hex');
  assert.ok(manifest.artifacts.some((artifact) => artifact.binarySha256 === sha));
  const server = await mockServer();
  const dir = await mkdtemp(join(tmpdir(), 'axi-watch-'));
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
          PATH: process.env.PATH,
          HOME: dir,
          XDG_CONFIG_HOME: dir,
          TEAMCITY_URL: server.base,
          TEAMCITY_TOKEN: 'fixture-only-token',
        },
        stdio: ['ignore', 'pipe', 'pipe'],
        timeout: 15000,
      });
      onSpawn?.(child);
      const out = [],
        err = [];
      child.stdout.on('data', (chunk) => out.push(chunk));
      child.stderr.on('data', (chunk) => err.push(chunk));
      child.on('error', reject);
      child.on('close', (code) => {
        const stdout = Buffer.concat(out).toString();
        done({
          code,
          stdout,
          stderr: Buffer.concat(err).toString(),
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
const watch = ['run', 'watch', '482193'];

test('watch emits one document for completed failure and success, with explicit assertion exits', async () => {
  const f = await fixture();
  try {
    const json = await f.call([...watch, '--json']);
    const toon = await f.call(watch);
    for (const result of [json, toon]) {
      assert.equal(result.code, 0);
      validateResponse(result.value);
      assert.equal(result.stderr, '');
      assert.equal(result.value.data.outcome, 'finished');
      assert.equal(result.value.data.run.id, '482193');
      assert.equal(result.value.data.run.result, 'failure');
      assert.equal(result.value.meta.counts.childProcesses, 2);
      assert.equal(result.value.meta.limits.maxChildProcesses, 32);
      assert.equal(result.value.meta.limits.concurrency, 1);
      assert.ok(Buffer.byteLength(result.stdout) <= 8192);
      for (const hint of result.value.next) parse(hint.argv.slice(1));
    }
    assert.deepEqual(json.value.data.run, toon.value.data.run);
    assert.equal((await f.call([...watch, '--check', '--json'])).code, 1);
    f.server.setMode('watch-success');
    const success = await f.call([...watch, '--check', '--json']);
    assert.equal(success.code, 0);
    assert.equal(success.value.data.check.passed, true);
    const contextual = await f.call([...watch, '--cwd', resolve('.'), '--json']);
    assert.equal(contextual.value.context.branch, 'feature/refund');
    assert.equal(contextual.value.context.jobs, undefined);
    f.server.setMode('huge-text');
    const concise = await f.call([...watch, '--json']);
    assert.equal(concise.code, 0);
    assert.equal(concise.value.data.run.statusText, undefined);
    assert.ok(Buffer.byteLength(concise.stdout) <= 8192);
    assert.ok(
      f.server.requests.every(
        (request) =>
          request.method === 'GET' && request.path === '/teamcity/app/rest/builds/id:482193',
      ),
    );
  } finally {
    await f.close();
  }
});

test('bounded polls distinguish terminal transition, vanished and inaccessible executions and retain prior identity', async () => {
  await Promise.all(
    ['watch-transition', 'watch-vanish', 'watch-inaccessible'].map(async (mode) => {
      const f = await fixture();
      try {
        f.server.setMode(mode);
        const result = await f.call([...watch, '--interval', '5s', '--timeout', '9s', '--json']);
        assert.equal(result.code, 0);
        validateResponse(result.value);
        assert.equal(
          result.value.data.outcome,
          mode === 'watch-transition'
            ? 'finished'
            : mode === 'watch-vanish'
              ? 'vanished'
              : 'inaccessible',
        );
        assert.equal(result.value.data.run.id, '482193');
        assert.equal(result.value.data.polling.count, 2);
        assert.equal(result.value.meta.counts.childProcesses, 3);
        assert.equal(
          result.value.data.run.state,
          mode === 'watch-transition' ? 'finished' : 'running',
        );
        assert.equal(result.value.status, mode === 'watch-transition' ? 'ok' : 'partial');
        assert.ok(
          f.server.requests.every(
            (request) => request.method === 'GET' && request.path.endsWith('/builds/id:482193'),
          ),
        );
      } finally {
        await f.close();
      }
    }),
  );
});

test('watch deadline retains latest queued/running evidence, remains partial and fails check', async () => {
  const f = await fixture();
  try {
    for (const mode of ['watch-running', 'watch-queued']) {
      f.server.setMode(mode);
      const observed = await f.call([...watch, '--timeout', '700ms', '--json']);
      assert.equal(observed.code, 0);
      validateResponse(observed.value);
      assert.equal(observed.value.status, 'partial');
      assert.equal(observed.value.data.outcome, 'deadline');
      assert.equal(observed.value.data.run.state, mode.slice(6));
      assert.equal(observed.value.data.check.passed, false);
      assert.ok(observed.value.meta.limitations.some((note) => note.code === 'DEADLINE_EXCEEDED'));
      const checked = await f.call([...watch, '--timeout', '700ms', '--check', '--json']);
      assert.equal(checked.code, 1);
      const strict = await f.call([...watch, '--timeout', '700ms', '--require-complete', '--json']);
      assert.equal(strict.code, 1);
    }
    f.server.setMode('watch-missing-revisions');
    const incomplete = await f.call([...watch, '--timeout', '700ms', '--json']);
    assert.equal(incomplete.value.data.run.revisions, undefined);
    assert.ok(
      incomplete.value.meta.limitations.some((note) => note.code === 'MISSING_REVISION_METADATA'),
    );
  } finally {
    await f.close();
  }
});

test('watch interruption during polling sleep preserves signal exit and never mutates the run', async () => {
  for (const [signal, exit] of [
    ['SIGINT', 130],
    ['SIGTERM', 143],
  ]) {
    const f = await fixture();
    try {
      f.server.setMode('watch-running');
      let child;
      const pending = f.call([...watch, '--json'], (spawned) => {
        child = spawned;
      });
      for (let attempt = 0; attempt < 200 && f.server.requests.length === 0; attempt++)
        await setTimeout(10);
      assert.equal(f.server.requests.length, 1);
      await setTimeout(50);
      child.kill(signal);
      const result = await pending;
      assert.equal(result.code, exit);
      assert.equal(result.value.error.code, 'INTERRUPTED');
      assert.equal(result.stderr, '');
      assert.ok(f.server.requests.every((request) => request.method === 'GET'));
    } finally {
      await f.close();
    }
  }
});

test('watch cannot replace the frozen ID or selected scope and surfaces inaccessible initial reads', async () => {
  const f = await fixture();
  try {
    f.server.setMode('wrong-id');
    const wrong = await f.call([...watch, '--check', '--json']);
    assert.equal(wrong.code, 1);
    assert.equal(wrong.value.data.run, null);
    assert.equal(wrong.value.data.outcome, 'unavailable');
    f.server.setMode('ok');
    const mismatch = await f.call([...watch, '--job=Other', '--json']);
    assert.equal(mismatch.code, 1);
    assert.equal(mismatch.value.error.code, 'CONTEXT_MISMATCH');
    for (const mode of ['missing', 'denied', 'expired']) {
      f.server.setMode(mode);
      const result = await f.call([...watch, '--check', '--json']);
      assert.equal(result.code, 1);
      validateResponse(result.value);
      assert.equal(result.value.data.run, null);
      assert.equal(result.value.data.outcome, mode === 'missing' ? 'vanished' : 'inaccessible');
    }
  } finally {
    await f.close();
  }
});
