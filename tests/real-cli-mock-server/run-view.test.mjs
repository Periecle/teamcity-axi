import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, rm, readFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createHash } from 'node:crypto';
import { decode } from '@toon-format/toon';
import { mockServer, longCanary } from '../fixtures/mock-server.mjs';
import { parse } from '../../dist/cli/parser.js';
import { validateResponse } from '../../dist/output/schema.js';
async function fixture() {
  const binary = process.env.TEAMCITY_AXI_TEST_BINARY;
  if (!binary) throw Error('TEAMCITY_AXI_TEST_BINARY is required; this suite never skips');
  const manifest = JSON.parse(await readFile('docs/compatibility.json', 'utf8'));
  const sha = createHash('sha256')
    .update(await readFile(binary))
    .digest('hex');
  assert.ok(
    manifest.artifacts.some((a) => a.binarySha256 === sha),
    'The native binary must be checksum verified',
  );
  const server = await mockServer();
  const dir = await mkdtemp(join(tmpdir(), 'axi-view-'));
  await mkdir(join(dir, 'teamcity-axi'));
  const config = {
    schemaVersion: '1.0',
    readOnly: true,
    defaultServer: 'work',
    binaryPath: resolve(binary),
    servers: { work: { url: server.base, allowHttpLoopback: true, allowedProjects: ['Payments'] } },
  };
  await writeFile(join(dir, 'teamcity-axi', 'config.json'), JSON.stringify(config), {
    mode: 0o600,
  });
  const call = (args = [], signal = null, command = ['run', 'view', '482193']) =>
    new Promise((resolve, reject) => {
      const child = spawn(process.execPath, [resolveBin, ...command, ...args], {
        env: {
          HOME: dir,
          XDG_CONFIG_HOME: dir,
          PATH: process.env.PATH,
          TEAMCITY_URL: server.base,
          TEAMCITY_TOKEN: 'fixture-only-token',
          APP_TOKEN: longCanary,
        },
        cwd: dir,
        stdio: ['ignore', 'pipe', 'pipe'],
      });
      const out = [],
        err = [];
      child.stdout.on('data', (v) => out.push(v));
      child.stderr.on('data', (v) => err.push(v));
      child.on('error', reject);
      if (signal) setTimeout(() => child.kill(signal), 500);
      child.on('close', (code) => {
        const stdout = Buffer.concat(out).toString(),
          stderr = Buffer.concat(err).toString();
        resolve({
          code,
          stdout,
          stderr,
          value: args.includes('--json') ? JSON.parse(stdout) : decode(stdout),
        });
      });
    });
  return {
    server,
    config,
    dir,
    call,
    close: async () => {
      await server.close();
      await rm(dir, { recursive: true, force: true });
    },
  };
}
const resolveBin = resolve('bin/teamcity-axi.mjs');
test('actual wrapper and released CLI observe an exact failed run successfully in both formats', async () => {
  const f = await fixture();
  try {
    for (const args of [['--json'], []]) {
      const r = await f.call(args);
      assert.equal(r.code, 0);
      assert.equal(r.stderr, '');
      validateResponse(r.value);
      assert.equal(r.value.status, 'ok');
      assert.equal(r.value.data.run.id, '482193');
      assert.equal(r.value.data.run.result, 'failure');
      assert.equal(r.value.context.server, 'work');
      assert.equal(r.value.context.job, 'Payments_Build');
      assert.equal(r.value.meta.counts.childProcesses, 2);
      assert.ok(!r.stdout.includes('fixture-only-token'));
      assert.ok(f.server.requests.every((q) => q.method === 'GET' && q.authenticated));
    }
    const projected = await f.call(['--fields', 'number', '--json']);
    assert.deepEqual(Object.keys(projected.value.data.run).sort(), [
      'id',
      'jobId',
      'number',
      'result',
      'state',
    ]);
    const mismatch = await f.call(['--job', 'Other', '--json']);
    assert.equal(mismatch.code, 1);
    assert.equal(mismatch.value.error.code, 'CONTEXT_MISMATCH');
    assert.equal(mismatch.value.context.server, 'work');
  } finally {
    await f.close();
  }
});
test('permission, authentication, wrong identity, malformed and oversized reads never become empty success', async () => {
  const f = await fixture();
  try {
    for (const [mode, code] of [
      ['denied', 'PERMISSION_DENIED'],
      ['expired', 'AUTH_REQUIRED'],
      ['missing', 'NOT_FOUND'],
      ['malformed', 'UPSTREAM_SCHEMA_MISMATCH'],
      ['html', 'AUTH_REQUIRED'],
      ['wrong-id', 'CONTEXT_MISMATCH'],
      ['invalid-identity', 'UPSTREAM_SCHEMA_MISMATCH'],
      ['huge', 'INPUT_LIMIT_EXCEEDED'],
    ]) {
      f.server.setMode(mode);
      const r = await f.call(['--json']);
      assert.equal(r.code, 1, mode);
      assert.equal(r.stderr, '', mode);
      assert.equal(r.value.status, 'error', mode);
      assert.equal(r.value.error.code, code, mode);
      assert.equal(r.value.data, undefined, mode);
      assert.equal(r.value.context.server, 'work', mode);
      validateResponse(r.value);
    }
    f.server.setMode('unknown-enum');
    const unknown = await f.call(['--json']);
    assert.equal(unknown.code, 0);
    assert.equal(unknown.value.data.run.result, 'unknown');
    assert.equal(unknown.value.data.run.state, 'unknown');
  } finally {
    await f.close();
  }
});
test('preview expansion is bounded and next actions parse; SIGINT/deadline remain read-only', async () => {
  const f = await fixture();
  try {
    f.server.setMode('huge-text');
    const preview = await f.call(['--json', '--job', 'Payments_Build', '--project', 'Payments']);
    assert.equal(preview.code, 0);
    assert.equal(preview.value.meta.truncated, true);
    assert.equal(Array.from(preview.value.data.run.statusText).length, 1200);
    for (const action of preview.value.next) parse(action.argv.slice(1));
    assert.ok(preview.value.next[0].argv.includes('--job'));
    assert.ok(preview.value.next[0].argv.includes('--project'));
    const full = await f.call(['--full', '--max-bytes', '2048', '--json']);
    assert.equal(full.code, 1);
    assert.ok(Buffer.byteLength(full.stdout) <= 2048);
    assert.equal(full.value.context.server, 'work');
    assert.equal(full.value.error.details.ceiling, 2048);
    assert.equal(full.value.meta.limits.maxBytes, 2048);
    assert.equal(full.value.meta.limits.maxChildProcesses, 8);
    assert.equal(full.value.meta.limits.stdoutCaptureBytes, 2097152);
    f.server.setMode('hang');
    const timeout = await f.call(['--timeout', '200ms', '--json']);
    assert.equal(timeout.code, 1);
    assert.equal(timeout.value.error.code, 'DEADLINE_EXCEEDED');
    const interrupted = await f.call(['--json'], 'SIGINT');
    assert.equal(interrupted.code, 130);
    assert.equal(interrupted.value.error.code, 'INTERRUPTED');
    assert.equal(interrupted.stderr, '');
    assert.ok(f.server.requests.every((q) => q.method === 'GET'));
  } finally {
    await f.close();
  }
});
test('released CLI output redacts long credentials before previews and marks omitted revisions partial', async () => {
  const f = await fixture();
  try {
    f.server.setMode('long-secret');
    const secret = await f.call(['--json']);
    assert.equal(secret.code, 0);
    assert.equal(secret.value.data.run.rawStatus, '[REDACTED]');
    assert.equal(secret.value.data.run.statusText, '[REDACTED]');
    assert.ok(!secret.stdout.includes(longCanary.slice(0, 80)));
    f.server.setMode('decorated-secret');
    const decorated = await f.call(['--json']);
    assert.equal(decorated.value.data.run.rawStatus, '[REDACTED]');
    assert.equal(decorated.value.data.run.statusText, '[REDACTED]');
    assert.ok(!decorated.stdout.includes(longCanary.slice(0, 80)));
    f.server.setMode('missing-revisions');
    const missing = await f.call(['--json']);
    assert.equal(missing.code, 0);
    assert.equal(missing.value.status, 'partial');
    assert.equal(missing.value.meta.complete, false);
    assert.equal(missing.value.data.run.revisions, undefined);
    const strict = await f.call(['--json', '--require-complete']);
    assert.equal(strict.code, 1);
    assert.equal(strict.value.status, 'partial');
  } finally {
    await f.close();
  }
});

test('released native outcome projections remain explicit across view, list, watch and investigation', async () => {
  const f = await fixture();
  try {
    for (const [mode, result] of [
      ['outcome-canceled', 'canceled'],
      ['outcome-failed-to-start', 'failed_to_start'],
      ['outcome-composite', 'success'],
    ]) {
      f.server.setMode(mode);
      for (const flags of [['--json'], []]) {
        const view = await f.call(flags);
        validateResponse(view.value);
        assert.equal(view.code, 0);
        assert.equal(view.value.data.run.result, result);
        assert.equal(view.value.data.run.state, 'finished');
        assert.equal(view.value.data.run.composite, mode === 'outcome-composite');
        assert.ok(!view.stdout.includes('Private'));
      }
      const list = await f.call(
        [
          '--job',
          'Payments_Build',
          '--all-branches',
          '--result',
          result,
          '--since',
          '2026-10-01T00:00:00Z',
          '--until',
          '2026-10-02T00:00:00Z',
          '--json',
        ],
        null,
        ['run', 'list'],
      );
      validateResponse(list.value);
      assert.equal(list.code, 0);
      assert.equal(list.value.data.runs[0].result, result);
      assert.equal(list.value.data.aggregates[result], 1);
      const watch = await f.call(['--check', '--json'], null, ['run', 'watch', '482193']);
      validateResponse(watch.value);
      assert.equal(watch.value.data.run.result, result);
      assert.equal(watch.value.data.check.passed, result === 'success');
      assert.equal(watch.code, result === 'success' ? 0 : 1);
      const failure = await f.call(['--json'], null, ['run', 'failure', '482193']);
      validateResponse(failure.value);
      assert.equal(failure.value.data.run.result, result);
      assert.equal(
        failure.value.data.assessment,
        result === 'success' ? 'not_failed' : 'failure_observed',
      );
    }
    f.server.setMode('outcome-missing');
    const watch = await f.call(['--check', '--json'], null, ['run', 'watch', '482193']);
    assert.equal(watch.code, 1);
    assert.equal(watch.value.data.run.result, 'unknown');
    assert.equal(watch.value.data.check.passed, false);
    const projected = f.server.requests.filter((request) =>
      request.query.fields?.includes('canceledInfo'),
    );
    assert.ok(projected.length > 0);
    assert.ok(projected.every((request) => !request.query.fields.includes('canceledInfo(user')));
    assert.ok(f.server.requests.every((request) => request.method === 'GET'));
  } finally {
    await f.close();
  }
});

test('a full unknown-outcome page retains all rows within the public diagnostic ceiling', async () => {
  const f = await fixture();
  try {
    f.server.setMode('outcome-many-missing');
    for (const flags of [['--json'], []]) {
      const result = await f.call(
        [
          '--job',
          'Payments_Build',
          '--all-branches',
          '--limit',
          '100',
          '--max-bytes',
          '65536',
          '--since',
          '2026-10-01T00:00:00Z',
          '--until',
          '2026-10-02T00:00:00Z',
          ...flags,
        ],
        null,
        ['run', 'list'],
      );
      validateResponse(result.value);
      assert.equal(result.code, 0);
      assert.equal(result.value.status, 'partial');
      assert.equal(result.value.meta.complete, false);
      assert.equal(result.value.data.runs.length, 100);
      assert.equal(result.value.data.aggregates.unknown, 100);
      assert.ok(
        result.value.meta.limitations.some((note) =>
          note.message.includes('100 distinct executions affected'),
        ),
      );
    }
  } finally {
    await f.close();
  }
});

test('debug and limit-hit responses expose actual tighter ceilings without credentials in both serializers', async () => {
  const f = await fixture();
  try {
    f.config.limits = { maxBytes: 4096, maxChildProcesses: 4, concurrency: 1 };
    const configPath = join(f.dir, 'teamcity-axi', 'config.json');
    await writeFile(configPath, JSON.stringify(f.config), { mode: 0o600 });
    for (const flags of [['--json'], []]) {
      const result = await f.call([
        ...flags,
        '--debug',
        '--max-bytes',
        '32768',
        '--timeout',
        '1000ms',
      ]);
      assert.equal(result.code, 0);
      validateResponse(result.value);
      const debug = JSON.parse(result.stderr);
      const limits = result.value.meta.limits;
      assert.deepEqual(debug.limits, limits);
      assert.equal(debug.command, 'run.view');
      assert.equal(debug.childProcesses, 2);
      assert.equal(limits.maxBytes, 4096);
      assert.equal(limits.maxChildProcesses, 4);
      assert.equal(limits.concurrency, 1);
      assert.equal(limits.stdoutCaptureBytes, 2097152);
      assert.equal(limits.stderrCaptureBytes, 65536);
      assert.ok(limits.deadline >= Date.now() && limits.deadline <= Date.now() + 1000);
      assert.ok(!result.stderr.includes('fixture-only-token'));
      assert.ok(!result.stderr.includes(longCanary));
      f.config.limits.maxChildProcesses = 1;
      await writeFile(configPath, JSON.stringify(f.config), { mode: 0o600 });
      const blocked = await f.call([...flags, '--debug']);
      assert.equal(blocked.code, 1);
      validateResponse(blocked.value);
      assert.equal(blocked.value.error.code, 'INPUT_LIMIT_EXCEEDED');
      assert.deepEqual(blocked.value.error.details, {
        limit: 'maxChildProcesses',
        ceiling: 1,
        observed: 1,
      });
      assert.equal(blocked.value.meta.limits.maxChildProcesses, 1);
      assert.deepEqual(JSON.parse(blocked.stderr).limits, blocked.value.meta.limits);
      f.config.limits.maxChildProcesses = 4;
      await writeFile(configPath, JSON.stringify(f.config), { mode: 0o600 });
      f.server.setMode('huge');
      const capture = await f.call(flags);
      assert.equal(capture.code, 1);
      validateResponse(capture.value);
      assert.equal(capture.stderr, '');
      assert.equal(capture.value.error.details.limit, 'stdoutCaptureBytes');
      assert.equal(capture.value.error.details.ceiling, 2097152);
      assert.ok(capture.value.error.details.observed > 2097152);
      assert.equal(capture.value.meta.limits.stdoutCaptureBytes, 2097152);
      assert.ok(Buffer.byteLength(capture.stdout) <= 4096);
      f.server.setMode('ok');
    }
  } finally {
    await f.close();
  }
});
