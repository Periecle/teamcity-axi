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
  const manifest = JSON.parse(await readFile('docs/compatibility.json', 'utf8')),
    sha = createHash('sha256')
      .update(await readFile(binary))
      .digest('hex');
  assert.ok(manifest.artifacts.some((a) => a.binarySha256 === sha));
  const server = await mockServer(),
    dir = await mkdtemp(join(tmpdir(), 'axi-list-'));
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
  const call = (
    flags = [],
    window = { since: '2026-10-01T00:00:00Z', until: '2026-10-02T00:00:00Z' },
  ) =>
    new Promise((done, reject) => {
      const args = [
        'run',
        'list',
        '--job',
        'Payments_Build',
        '--branch',
        'feature/refund',
        ...(window.since ? ['--since', window.since] : []),
        ...(window.until ? ['--until', window.until] : []),
        ...flags,
      ];
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
          value: flags.includes('--json') ? JSON.parse(stdout) : decode(stdout),
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
test('released CLI list preserves one page, mandatory identities, typed continuation and empty continuation', async () => {
  const f = await fixture();
  try {
    f.server.setMode('list-page');
    for (const flags of [['--json'], []]) {
      const r = await f.call(flags);
      assert.equal(r.code, 0);
      validateResponse(r.value);
      assert.equal(r.value.data.page.hasMore, true);
      assert.equal(r.value.data.page.total, null);
      assert.equal(r.value.data.runs[0].id, '482193');
      assert.equal(r.value.data.runs[0].result, 'failure');
      for (const action of r.value.next) parse(action.argv.slice(1));
    }
    const first = await f.call(['--json', '--fields', 'number']);
    assert.deepEqual(Object.keys(first.value.data.runs[0]).sort(), [
      'id',
      'jobId',
      'number',
      'result',
      'state',
    ]);
    f.server.setMode('list-empty');
    const empty = await f.call(['--json', '--cursor', first.value.data.page.cursor]);
    assert.equal(empty.code, 0);
    assert.equal(empty.value.data.page.returned, 0);
    assert.equal(empty.value.data.page.hasMore, true);
    assert.ok(empty.value.data.page.cursor);
    assert.ok(f.server.requests.every((r) => r.method === 'GET' && r.authenticated));
  } finally {
    await f.close();
  }
});
test('unsafe continuations preserve rows while scope, malformed and denied responses fail closed', async () => {
  const f = await fixture();
  try {
    for (const mode of ['list-unsafe', 'list-escalating']) {
      f.server.setMode(mode);
      const r = await f.call(['--json', '--require-complete']);
      assert.equal(r.code, 1);
      assert.equal(r.value.status, 'partial');
      assert.equal(r.value.data.runs.length, 1);
      assert.equal(r.value.data.page.cursor, null);
      assert.ok(r.value.meta.limitations.some((l) => l.code === 'UNSAFE_CONTINUATION'));
      validateResponse(r.value);
    }
    for (const [mode, code] of [
      ['denied', 'PERMISSION_DENIED'],
      ['malformed', 'UPSTREAM_SCHEMA_MISMATCH'],
      ['list-wrong-branch', 'CONTEXT_MISMATCH'],
      ['list-duplicate', 'UPSTREAM_SCHEMA_MISMATCH'],
    ]) {
      f.server.setMode(mode);
      const r = await f.call(['--json']);
      assert.equal(r.code, 1);
      assert.equal(r.value.error.code, code);
      assert.equal(r.value.data, undefined);
    }
    f.server.setMode('list-page');
    const first = await f.call(['--json']);
    const cursor = decodeCursor(first.value.data.page.cursor);
    const forged = encodeCursor({ ...cursor, filterHash: '0'.repeat(64) });
    const rejected = await f.call(['--json', '--cursor', forged]);
    assert.equal(rejected.code, 2);
    assert.equal(rejected.value.error.code, 'USAGE_ERROR');
    const malformed = await f.call(['--json', '--cursor', 'not-a-cursor']);
    assert.equal(malformed.code, 2);
    assert.equal(malformed.stderr, '');
    f.server.setMode('list-unknown-result');
    const unknown = await f.call(['--json', '--fields', 'number']);
    assert.equal(unknown.value.data.runs[0].result, 'unknown');
    assert.equal(unknown.value.data.runs[0].rawStatus, 'FUTURE_RESULT');
  } finally {
    await f.close();
  }
});

test('unknown-result pages retain candidate coverage, empty continuation and independent cursor identity', async () => {
  const f = await fixture();
  try {
    f.server.setMode('list-unknown-candidates');
    for (const flags of [['--json'], []]) {
      const first = await f.call(['--result', 'unknown', ...flags]);
      assert.equal(first.code, 0);
      validateResponse(first.value);
      assert.equal(first.value.data.selection.result, 'unknown');
      assert.equal(first.value.data.selection.resultBasis, 'normalized_candidates');
      assert.equal(first.value.data.selection.providerReturned, 2);
      assert.deepEqual(
        first.value.data.runs.map((run) => run.id),
        ['482194'],
      );
      assert.equal(first.value.data.runs[0].rawStatus, 'FUTURE_RESULT');
      assert.equal(first.value.data.page.hasMore, true);
      const token = first.value.data.page.cursor;
      const next = await f.call(['--result', 'unknown', '--cursor', token, ...flags]);
      assert.equal(next.code, 0);
      validateResponse(next.value);
      assert.equal(next.value.data.selection.providerReturned, 2);
      assert.deepEqual(next.value.data.runs, []);
      assert.equal(next.value.data.page.hasMore, true);
      assert.ok(next.value.data.page.cursor);
      assert.ok(next.value.next[0].argv.includes('unknown'));
      for (const action of next.value.next) parse(action.argv.slice(1));
      const crossed = await f.call(['--cursor', token, '--json']);
      assert.equal(crossed.code, 2);
      assert.equal(crossed.value.error.code, 'USAGE_ERROR');
      const ordinary = await f.call(['--json']);
      const reverse = await f.call([
        '--result',
        'unknown',
        '--cursor',
        ordinary.value.data.page.cursor,
        '--json',
      ]);
      assert.equal(reverse.code, 2);
    }
    const locators = f.server.requests
      .filter((request) => request.path.endsWith('/app/rest/builds'))
      .map((request) => request.query.locator);
    assert.ok(locators.length > 0);
    assert.ok(locators.every((locator) => !locator.includes('status:UNKNOWN')));
    assert.ok(f.server.requests.every((request) => request.method === 'GET'));
  } finally {
    await f.close();
  }
});

test('fractional windows survive provider bounds, exact verification, default lookback and cursor continuation', async () => {
  const f = await fixture();
  try {
    f.server.setMode('list-fractional');
    const window = {
      since: '2026-10-01T13:00:00.999999999+02:00',
      until: '2026-10-01T11:00:01.000000001Z',
    };
    const first = await f.call(['--json'], window);
    assert.equal(first.code, 0);
    validateResponse(first.value);
    assert.deepEqual(
      first.value.data.runs.map((run) => run.id),
      ['482194'],
    );
    assert.equal(first.value.data.selection.window.since, '2026-10-01T11:00:00.999999999Z');
    assert.equal(first.value.data.selection.window.until, window.until);
    const cursor = decodeCursor(first.value.data.page.cursor);
    assert.equal(cursor.window.until, window.until);
    const next = await f.call(['--cursor', first.value.data.page.cursor, '--json'], {});
    assert.equal(next.code, 0);
    assert.deepEqual(next.value.data.selection.window, first.value.data.selection.window);
    const changed = await f.call(['--cursor', first.value.data.page.cursor, '--json'], {
      ...window,
      until: '2026-10-01T11:00:01.000000002Z',
    });
    assert.equal(changed.code, 2);
    const defaultWindow = await f.call(['--json'], { until: window.until });
    assert.equal(defaultWindow.value.data.selection.window.since, '2026-09-24T11:00:01.000000001Z');
    const locator = f.server.requests.find((request) => request.path.endsWith('/app/rest/builds'))
      .query.locator;
    assert.ok(locator.includes('finishDate:(date:20261001T110001.001+0000,condition:before)'));
    assert.ok(locator.includes('finishDate:(date:20261001T110000.999+0000,condition:after)'));
  } finally {
    await f.close();
  }
});

test('long exact bounds preserve useful rows when continuation exceeds its cursor budget', async () => {
  const f = await fixture();
  try {
    f.server.setMode('list-page');
    const until = '2026-10-02T00:00:00.' + '1'.repeat(2000) + 'Z';
    for (const flags of [['--json'], [], ['--require-complete', '--json']]) {
      const result = await f.call([...flags, '--max-bytes', '65536'], { until });
      validateResponse(result.value);
      assert.equal(result.code, flags.includes('--require-complete') ? 1 : 0);
      assert.equal(result.value.status, 'partial');
      assert.equal(result.value.meta.complete, false);
      assert.equal(result.value.data.runs.length, 1);
      assert.equal(result.value.data.selection.window.until, until);
      assert.equal(
        result.value.data.selection.window.since,
        until.replace('2026-10-02', '2026-09-25'),
      );
      assert.equal(result.value.data.page.cursor, null);
      assert.equal(result.value.data.page.hasMore, true);
      assert.ok(
        result.value.meta.limitations.some((note) => note.code === 'CURSOR_LIMIT_EXCEEDED'),
      );
      assert.equal(result.value.next, undefined);
    }
  } finally {
    await f.close();
  }
});
