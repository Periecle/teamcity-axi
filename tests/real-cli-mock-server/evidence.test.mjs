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
  if (!binary) throw Error('A checksum-verified native binary is required; no skips');
  const manifest = JSON.parse(await readFile('docs/compatibility.json', 'utf8'));
  const sha = createHash('sha256')
    .update(await readFile(binary))
    .digest('hex');
  assert.ok(manifest.artifacts.some((a) => a.binarySha256 === sha));
  const server = await mockServer(),
    dir = await mkdtemp(join(tmpdir(), 'axi-doctor-'));
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
  return {
    server,
    call: (args, env = {}) =>
      new Promise((res, rej) => {
        const child = spawn(process.execPath, [resolve('bin/teamcity-axi.mjs'), ...args], {
          cwd: dir,
          env: {
            PATH: process.env.PATH,
            HOME: dir,
            XDG_CONFIG_HOME: dir,
            TEAMCITY_URL: server.base,
            TEAMCITY_TOKEN: 'fixture-only-token',
            ...env,
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
            value: args.includes('--json') ? JSON.parse(stdout) : decode(stdout),
          });
        });
      }),
    close: async () => {
      await server.close();
      await rm(dir, { recursive: true, force: true });
    },
  };
}
test('independent pages preserve duplicate names, muted/ignored categories, safe cursors and exact expansion', async () => {
  const f = await fixture();
  try {
    const all = await f.call(['run', 'tests', '482193', '--json']);
    validateResponse(all.value);
    assert.equal(all.code, 0);
    assert.equal(all.value.status, 'partial');
    assert.equal(all.value.data.tests.length, 3);
    assert.equal(all.value.data.tests[0].name, all.value.data.tests[1].name);
    assert.notEqual(all.value.data.tests[0].id, all.value.data.tests[1].id);
    assert.equal(all.value.data.tests[2].result, 'ignored');
    assert.equal(all.value.data.tests[0].testId, '517450581327024597');
    for (const [flags, expected] of [
      [['--failed'], 1],
      [['--failed', '--include-muted'], 2],
      [['--muted'], 1],
    ]) {
      const r = await f.call(['run', 'tests', '482193', ...flags, '--json']);
      assert.equal(r.code, 0);
      assert.equal(r.value.data.tests.length, expected);
      assert.equal(r.value.data.page.total, null);
    }
    const first = await f.call(['run', 'problems', '482193', '--limit', '1', '--json']);
    assert.equal(first.code, 0);
    assert.equal(first.value.status, 'ok');
    assert.equal(first.value.data.page.hasMore, true);
    first.value.next.forEach((a) => parse(a.argv.slice(1)));
    const second = await f.call([...first.value.next[0].argv.slice(1), '--json']);
    assert.equal(second.code, 0);
    assert.notEqual(second.value.data.problems[0].id, first.value.data.problems[0].id);
    const selected = await f.call([
      'run',
      'tests',
      '482193',
      '--test',
      'build:(id:482193),id:2000000000',
      '--json',
    ]);
    assert.equal(selected.code, 0);
    assert.equal(selected.value.data.page.total, 1);
    assert.equal(selected.value.data.tests[0].id, 'build:(id:482193),id:2000000000');
    const changed = await f.call([
      'run',
      'tests',
      '482193',
      '--limit',
      '1',
      '--cursor',
      first.value.data.page.cursor,
      '--json',
    ]);
    assert.equal(changed.code, 2);
    const mistaken = await f.call([
      'run',
      'tests',
      '482193',
      '--test',
      '517450581327024597',
      '--json',
    ]);
    assert.equal(mistaken.code, 2);
    f.server.setMode('evidence-preview');
    const preview = await f.call(['run', 'problems', '482193', '--json']);
    assert.equal(preview.code, 0);
    assert.equal(Array.from(preview.value.data.problems[1].description).length, 1200);
    assert.equal(
      preview.value.next.find((a) => a.argv.includes('--problem')).argv.at(-2),
      'build:(id:482193),problem:(id:2)',
    );
    for (const action of preview.value.next) parse(action.argv.slice(1));
    const oversized = await f.call([
      'run',
      'problems',
      '482193',
      '--problem',
      'build:(id:482193),problem:(id:2)',
      '--full',
      '--max-bytes',
      '2048',
      '--json',
    ]);
    assert.equal(oversized.code, 1);
    assert.equal(oversized.value.error.code, 'INPUT_LIMIT_EXCEEDED');
    assert.ok(Buffer.byteLength(oversized.stdout) <= 2048);
    f.server.setMode('evidence-secret');
    const secret = await f.call(['run', 'problems', '482193', '--json'], { APP_TOKEN: longCanary });
    assert.equal(secret.code, 0);
    assert.equal(secret.value.data.problems[0].description, '[REDACTED]');
    assert.ok(!secret.stdout.includes(longCanary.slice(0, 80)));
  } finally {
    await f.close();
  }
});
test('independent source errors, unsafe continuation, empty pages and malformed identities cannot become false success', async () => {
  const f = await fixture();
  try {
    for (const [mode, command, code] of [
      ['problems-denied', 'problems', 'PERMISSION_DENIED'],
      ['tests-denied', 'tests', 'PERMISSION_DENIED'],
      ['evidence-wrong-run', 'tests', 'CONTEXT_MISMATCH'],
      ['evidence-duplicate-id', 'tests', 'UPSTREAM_SCHEMA_MISMATCH'],
      ['evidence-huge', 'problems', 'INPUT_LIMIT_EXCEEDED'],
    ]) {
      f.server.setMode(mode);
      const r = await f.call(['run', command, '482193', '--json']);
      assert.equal(r.code, 1, mode);
      assert.equal(r.value.error.code, code, mode);
      assert.equal(r.value.data, undefined);
    }
    f.server.setMode('evidence-unsafe');
    const unsafe = await f.call(['run', 'tests', '482193', '--json']);
    assert.equal(unsafe.code, 0);
    assert.equal(unsafe.value.status, 'partial');
    assert.equal(unsafe.value.data.tests.length, 3);
    assert.equal(unsafe.value.data.page.cursor, null);
    f.server.setMode('evidence-empty');
    const empty = await f.call(['run', 'tests', '482193', '--json']);
    assert.equal(empty.value.data.tests.length, 0);
    assert.equal(empty.value.data.page.hasMore, true);
    assert.equal(empty.value.data.page.total, null);
    f.server.setMode('evidence-unknown-test');
    const unknown = await f.call(['run', 'tests', '482193', '--fields', 'durationMs', '--json']);
    assert.equal(unknown.value.data.tests[0].result, 'unknown');
    assert.equal(unknown.value.data.tests[0].rawStatus, 'FUTURE_RESULT');
    f.server.setMode('evidence-no-flags');
    const uncertain = await f.call(['run', 'tests', '482193', '--failed', '--json']);
    assert.equal(uncertain.value.data.tests.length, 0);
    assert.equal(uncertain.value.status, 'partial');
    const strict = await f.call([
      'run',
      'tests',
      '482193',
      '--failed',
      '--require-complete',
      '--json',
    ]);
    assert.equal(strict.code, 1);
    assert.equal(strict.value.status, 'partial');
  } finally {
    await f.close();
  }
});
test('log filtering searches declared full messages, tail bounds hold, and failure mode retains sibling evidence', async () => {
  const f = await fixture();
  try {
    f.server.setMode('log-window');
    const log = await f.call([
      'run',
      'log',
      '482193',
      '--tail',
      '80',
      '--contains',
      'literal[needle]',
      '--json',
    ]);
    assert.equal(log.code, 0);
    validateResponse(log.value);
    assert.equal(log.value.data.messages.length, 1);
    assert.equal(log.value.data.window.matched, 1);
    assert.equal(log.value.data.messages[0].text.length, 2000);
    for (const action of log.value.next) parse(action.argv.slice(1));
    const full = await f.call([...log.value.next[0].argv.slice(1), '--json']);
    assert.equal(full.code, 0);
    assert.ok(full.value.data.messages[0].text.includes('literal[needle]'));
    const none = await f.call(['run', 'log', '482193', '--contains', 'absent', '--json']);
    assert.equal(none.value.data.messages.length, 0);
    assert.equal(none.value.data.window.retained, 2);
    assert.ok(none.value.data.emptyReason.includes('window'));
    const capped = await f.call(['run', 'log', '482193', '--tail', '1', '--json']);
    assert.equal(capped.code, 0);
    assert.equal(capped.value.data.window.omittedProviderMessages, 1);
    assert.equal(capped.value.meta.truncated, true);
    assert.equal(capped.value.next, undefined);
    f.server.setMode('log-overdelivery');
    const failedCapped = await f.call(['run', 'log', '482193', '--failed', '--json']);
    assert.equal(failedCapped.code, 0);
    assert.equal(failedCapped.value.data.window.omittedProviderMessages, 1);
    assert.equal(failedCapped.value.meta.truncated, true);
    assert.equal(failedCapped.value.next, undefined);
    f.server.setMode('logs-unsupported');
    const unsupported = await f.call(['run', 'log', '482193', '--json']);
    assert.equal(unsupported.code, 1);
    assert.equal(unsupported.value.error.code, 'CAPABILITY_UNAVAILABLE');
    const partial = await f.call(['run', 'log', '482193', '--failed', '--json']);
    assert.equal(partial.code, 0);
    validateResponse(partial.value);
    assert.equal(partial.value.status, 'partial');
    assert.equal(partial.value.data.sources.log.availability, 'unavailable');
    assert.equal(partial.value.data.sources.tests.availability, 'available');
    assert.equal(partial.value.data.messages, undefined);
    f.server.setMode('tests-denied');
    const denied = await f.call(['run', 'log', '482193', '--failed', '--json']);
    assert.equal(denied.code, 0);
    assert.equal(denied.value.data.sources.tests.errorCode, 'PERMISSION_DENIED');
    assert.equal(denied.value.data.tests, undefined);
    assert.ok(denied.value.data.problems.length);
    assert.ok(
      f.server.requests.every((r) => r.method === 'GET' && !r.path.includes('downloadBuildLog')),
    );
  } finally {
    await f.close();
  }
});
