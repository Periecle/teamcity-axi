import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, mkdtempSync, rmSync } from 'node:fs';
import { spawnSync, execFileSync } from 'node:child_process';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { sanitizeLiveStdout, verifyLiveCapture } from '../../scripts/live-harness.mjs';
const contract = JSON.parse(
  readFileSync('tests/fixtures/teamcity-2026.2-native-1.5.0/contract.json', 'utf8'),
);
test('live recorder redacts decoded credentials including JSON and Go escapes before public capture', () => {
  const secret = 'fixture-canary"slash\\end<&';
  const wire = JSON.stringify({ statusText: secret });
  for (const json of [wire, wire.replaceAll('<', '\\u003c').replaceAll('&', '\\u0026')]) {
    const clean = sanitizeLiveStdout(
      'HTTP/1.1 200 OK\nContent-Type: application/json\nSet-Cookie: another-private-session\n\n' +
        json,
      [secret],
    );
    assert.equal(JSON.parse(clean.split('\n\n')[1]).statusText, '[REDACTED]');
    assert.ok(!clean.includes('another-private-session'));
  }
  const controls = JSON.stringify({ text: secret.slice(0, 10) + '\x1b[31m' + secret.slice(10) });
  assert.equal(JSON.parse(sanitizeLiveStdout(controls, [secret])).text, '[REDACTED]');
  assert.ok(
    !sanitizeLiveStdout('<html>credential-encoded</html>', []).includes('credential-encoded'),
  );
});
test('live session redaction preserves JSON delimiters and continuation identity', () => {
  const input = JSON.stringify({
    nextHref: '/app/rest/builds;TCSESSIONID=fixture-session',
    name: 'original',
  });
  assert.deepEqual(JSON.parse(sanitizeLiveStdout(input, [])), {
    nextHref: '/app/rest/builds;TCSESSIONID=[REDACTED]',
    name: 'original',
  });
});
test('live recorder refuses reattributing reused numeric IDs to another job, project or result', () => {
  verifyLiveCapture(contract.records, contract);
  for (const [name, patch] of [
    ['run-detail', { buildTypeId: 'OtherJob' }],
    ['green', { buildType: { id: 'OtherJob', projectId: 'AxiContract' } }],
    ['run-detail', { buildType: { id: 'AxiContract_Fail', projectId: 'OtherProject' } }],
    ['green', { status: 'FAILURE' }],
    ['run-detail', { state: 'running' }],
  ]) {
    const records = structuredClone(contract.records),
      r = records[name],
      at = r.stdout.indexOf('\n\n');
    const body = JSON.parse(r.stdout.slice(at + 2));
    r.stdout = r.stdout.slice(0, at + 2) + JSON.stringify({ ...body, ...patch });
    assert.throws(() => verifyLiveCapture(records, contract), /identity or outcome/);
  }
});
test('FIFO credential input fails promptly before reading or launching the native executable', () => {
  const dir = mkdtempSync(join(tmpdir(), 'axi-live-fifo-'));
  try {
    const fifo = join(dir, 'credentials');
    execFileSync('mkfifo', [fifo]);
    const source = `import {liveFixture} from ${JSON.stringify(new URL('../../scripts/live-harness.mjs', import.meta.url).href)}; try{await liveFixture();process.exitCode=1;}catch(error){if(error.message!=='Live credential configuration must be an owned private regular file')process.exitCode=1;}`;
    const result = spawnSync(process.execPath, ['--input-type=module', '-e', source], {
      env: {
        ...process.env,
        TEAMCITY_AXI_LIVE_CREDENTIALS: fifo,
        TEAMCITY_AXI_TEST_BINARY: join(dir, 'not-an-executable'),
      },
      timeout: 2000,
      encoding: 'utf8',
    });
    assert.equal(result.error, undefined);
    assert.equal(result.status, 0);
    assert.equal(result.stderr, '');
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
test('live recorder rejects reattributing bounded job pages to another identity or project', () => {
  for (const patch of [{ id: 'OtherJob' }, { projectId: 'OtherProject' }, { paused: true }]) {
    const records = structuredClone(contract.records),
      r = records['bounded-jobs'];
    const at = r.stdout.indexOf('\n\n'),
      body = JSON.parse(r.stdout.slice(at + 2));
    body.buildType[0] = { ...body.buildType[0], ...patch };
    r.stdout = r.stdout.slice(0, at + 2) + JSON.stringify(body);
    assert.throws(() => verifyLiveCapture(records, contract), /job page identity or scope/);
  }
});
test('live recorder refuses queue ID reuse, scope changes and finished-state substitution', () => {
  for (const patch of [
    { id: 999 },
    { buildTypeId: 'OtherJob' },
    { state: 'finished' },
    { buildType: { id: 'AxiContract_QueueA', projectId: 'Forbidden' } },
  ]) {
    const records = structuredClone(contract.records),
      r = records['queue-project-positive'];
    const at = r.stdout.indexOf('\n\n'),
      body = JSON.parse(r.stdout.slice(at + 2));
    body.build[0] = { ...body.build[0], ...patch };
    r.stdout = r.stdout.slice(0, at + 2) + JSON.stringify(body);
    assert.throws(() => verifyLiveCapture(records, contract), /queue identity, scope or lifecycle/);
  }
});
