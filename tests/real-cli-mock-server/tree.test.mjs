import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, rm, readFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createHash } from 'node:crypto';

import { decode } from '@toon-format/toon';

import { treeServer } from '../fixtures/tree-server.mjs';
import { parse } from '../../dist/cli/parser.js';
import { validateResponse } from '../../dist/output/schema.js';

async function fixture() {
  const binary = process.env.TEAMCITY_AXI_TEST_BINARY;

  if (!binary) throw Error('A checksum-verified native binary is required; no skips');
  const manifest = JSON.parse(await readFile('docs/compatibility.json', 'utf8'));
  const sha = createHash('sha256')
    .update(await readFile(binary))
    .digest('hex');

  assert.ok(manifest.artifacts.some((a) => a.binarySha256 === sha));
  const server = await treeServer();
  const dir = await mkdtemp(join(tmpdir(), 'axi-tree-'));
  const config = {
    schemaVersion: '1.0',
    readOnly: true,
    defaultServer: 'work',
    binaryPath: resolve(binary),
    servers: { work: { url: server.base, allowHttpLoopback: true, allowedProjects: ['Payments'] } },
  };
  const configure = async (limits) => {
    if (limits) config.limits = limits;
    else delete config.limits;
    await writeFile(join(dir, 'teamcity-axi', 'config.json'), JSON.stringify(config), {
      mode: 0o600,
    });
  };

  await mkdir(join(dir, 'teamcity-axi'));
  await configure();

  return {
    server,
    configure,
    call: (flags = []) =>
      new Promise((res, rej) => {
        const args = ['run', 'tree', '482193', ...flags];
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
        });
        const out = [],
          err = [];
        child.stdout.on('data', (b) => out.push(b));
        child.stderr.on('data', (b) => err.push(b));
        child.on('error', rej);
        child.on('close', (code) => {
          const stdout = Buffer.concat(out).toString();
          res({
            code,
            stdout,
            stderr: Buffer.concat(err).toString(),
            value: flags.includes('--json') ? JSON.parse(stdout) : decode(stdout),
          });
        });
      }),
    close: async () => {
      await server.close();
      await rm(dir, { recursive: true, force: true });
    },
  };
}

const ids = (response) => response.data.graph.nodes.map((n) => n.run.id);
const expansion = (response, id) =>
  response.data.graph.nodes.find((n) => n.run.id === id).expansion;

test('released CLI tree retains a shared DAG and cycle, equivalent renderers and exact scope', async () => {
  const f = await fixture();
  try {
    const r = await f.call(['--json']);
    assert.equal(r.code, 0);
    assert.equal(r.stderr, '');
    validateResponse(r.value);
    assert.equal(r.value.status, 'ok');
    assert.deepEqual(ids(r.value), ['482193', '482190', '482191', '482188']);
    assert.equal(r.value.data.graph.edges.length, 4);
    assert.equal(r.value.data.graph.complete, true);
    assert.deepEqual(r.value.data.graph.cycles, []);
    assert.equal(r.value.meta.counts.childProcesses, 9);
    assert.equal(r.value.data.selection.graphReadAttempts, 7);
    assert.equal(r.value.data.graph.nodes.at(-1).dependencyCount, 0);
    const toon = await f.call();
    validateResponse(toon.value);
    assert.deepEqual(toon.value.data, r.value.data);
    assert.ok(Buffer.byteLength(toon.stdout) <= 24576);
    const mismatch = await f.call(['--job', 'Other', '--json']);
    assert.equal(mismatch.code, 1);
    assert.equal(mismatch.value.error.code, 'CONTEXT_MISMATCH');
    assert.equal(mismatch.value.data, undefined);
    f.server.setMode('cycle');
    const cycle = await f.call(['--json']);
    assert.equal(cycle.code, 0, JSON.stringify(cycle.value));
    validateResponse(cycle.value);
    assert.equal(cycle.value.data.graph.nodes.length, 3);
    assert.equal(cycle.value.data.graph.edges.length, 3);
    assert.equal(cycle.value.data.graph.cycles.length, 1);
    assert.equal(cycle.value.data.graph.complete, true);
    assert.equal(cycle.value.status, 'ok');
    assert.ok(f.server.requests.every((r) => r.method === 'GET'));
    assert.ok(
      f.server.requests
        .filter((r) => r.path === '/app/rest/builds')
        .every((r) => r.query.locator.includes('recursive:false')),
    );
  } finally {
    await f.close();
  }
});

test('tree boundaries and process ceilings remain explicit without dangling edges or false leaves', async () => {
  const f = await fixture();
  try {
    const zero = await f.call(['--depth', '0', '--json']);
    validateResponse(zero.value);
    assert.equal(zero.value.status, 'partial', JSON.stringify(zero.value));
    assert.equal(zero.value.data.graph.nodes.length, 1);
    assert.equal(expansion(zero.value, '482193'), 'depth_limit');
    assert.equal(zero.value.data.graph.nodes[0].dependencyCount, null);
    assert.equal(zero.value.meta.counts.childProcesses, 2);
    const cap = await f.call(['--max-nodes', '2', '--json']);
    assert.equal(cap.code, 0, JSON.stringify(cap.value));
    assert.equal(cap.value.data.graph.nodes.length, 2);
    assert.equal(cap.value.data.selection.omittedTargets, 2);
    assert.ok(cap.value.data.graph.edges.every((edge) => ids(cap.value).includes(edge.toRunId)));
    assert.equal(expansion(cap.value, '482193'), 'node_limit');
    cap.value.next.forEach((a) => parse(a.argv.slice(1)));
    assert.ok(cap.value.next[0].argv.includes('--project'));
    const strict = await f.call(['--depth', '1', '--require-complete', '--no-hints', '--json']);
    assert.equal(strict.code, 1);
    assert.equal(strict.value.next, undefined);
    assert.equal(strict.value.status, 'partial');
    f.server.setMode('wide');
    const wide = await f.call(['--max-nodes', '200', '--json']);
    validateResponse(wide.value);
    assert.equal(wide.value.meta.counts.childProcesses, 24);
    assert.equal(wide.value.data.graph.complete, false);
    assert.ok(wide.value.data.graph.nodes.some((n) => n.expansion === 'call_limit'));
    const small = await f.call(['--max-bytes', '2048', '--json']);
    assert.equal(small.code, 1);
    assert.equal(small.value.error.code, 'INPUT_LIMIT_EXCEEDED');
    assert.ok(Buffer.byteLength(small.stdout) <= 2048);
  } finally {
    await f.close();
  }
});

test('independent graph failures preserve siblings and exclude foreign identities and diagnostics', async () => {
  const f = await fixture();
  try {
    for (const mode of ['denied', 'mismatch', 'unsafe', 'foreign', 'missing-metadata']) {
      f.server.setMode(mode);
      const r = await f.call(['--json']);
      assert.equal(r.code, 0, JSON.stringify(r.value));
      validateResponse(r.value);
      assert.equal(r.value.status, 'partial', mode);
      if (mode === 'denied') {
        assert.equal(expansion(r.value, '482190'), 'permission_denied');
        assert.equal(expansion(r.value, '482188'), 'complete');
      }
      if (mode === 'foreign') {
        assert.ok(!ids(r.value).includes('482191'));
        assert.ok(!r.stdout.includes('Foreign'));
        assert.ok(r.value.meta.limitations.every((l) => l.runId !== '482191'));
      }
      if (['mismatch', 'unsafe'].includes(mode))
        assert.equal(expansion(r.value, '482193'), 'unavailable');
      assert.ok(r.value.data.graph.nodes.length > 1);
    }
    f.server.setMode('invalid-timestamp');
    const timestamps = await f.call(['--json']);
    assert.ok(
      timestamps.value.meta.limitations
        .filter((l) => l.code === 'INVALID_TIMESTAMP')
        .every((l) => l.runId && l.runId !== '482193'),
    );
    f.server.setMode('empty');
    const empty = await f.call(['--json']);
    assert.equal(empty.value.status, 'ok');
    assert.equal(empty.value.data.graph.complete, true);
    assert.ok(f.server.requests.some((r) => r.query.locator?.includes('start:100')));
    f.server.setMode('root-denied');
    const denied = await f.call(['--json']);
    assert.equal(denied.code, 1);
    assert.equal(denied.value.error.code, 'PERMISSION_DENIED');
    assert.equal(denied.value.data, undefined);
  } finally {
    await f.close();
  }
});

test('tree reserves a final non-terminal root observation even when graph discovery has no capacity', async () => {
  const f = await fixture();
  try {
    await f.configure({ maxChildProcesses: 3 });
    for (const mode of ['changed', 'changed-metadata', 'provisional', 'final-unavailable']) {
      f.server.setMode(mode);
      const r = await f.call(['--json']);
      assert.equal(r.code, 0, JSON.stringify(r.value));
      validateResponse(r.value);
      assert.equal(r.value.status, 'partial', mode);
      assert.equal(r.value.meta.counts.childProcesses, 3);
      assert.equal(r.value.data.selection.maxGraphReads, 0);
      assert.equal(
        f.server.requests.filter((r) => r.path === '/app/rest/builds/id:482193').length,
        2,
      );
      if (mode === 'changed') {
        assert.equal(r.value.data.run.state, 'finished');
        assert.equal(r.value.data.run.result, 'success');
        assert.deepEqual(r.value.data.graph.nodes[0].run, r.value.data.run);
        assert.ok(r.value.meta.limitations.some((l) => l.code === 'ROOT_STATE_CHANGED'));
      }
      if (mode === 'changed-metadata') {
        assert.equal(r.value.data.run.branch, 'changed-branch');
        assert.ok(r.value.meta.limitations.some((l) => l.code === 'ROOT_STATE_CHANGED'));
      }
      assert.equal(r.value.data.graph.complete, false);
    }
    await f.configure();
    f.server.setMode('final-unavailable');
    const unavailable = await f.call(['--json']);
    assert.equal(unavailable.value.data.graph.complete, false);
    assert.equal(expansion(unavailable.value, '482193'), 'unavailable');
    assert.equal(unavailable.value.data.graph.unexpanded, 1);
  } finally {
    await f.close();
  }
});
