import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, rm, readFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createHash } from 'node:crypto';
import { decode } from '@toon-format/toon';
import { mockServer } from '../fixtures/mock-server.mjs';
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
test('context is local until verify; verified scope and identity agree in JSON and TOON', async () => {
  const f = await fixture();
  try {
    const local = await f.call(['context', 'show', '--job', 'Payments_Build', '--json']);
    assert.equal(local.code, 0);
    validateResponse(local.value);
    assert.equal(f.server.requests.length, 0);
    assert.equal(local.value.data.verification.requested, false);
    const longBranch = await f.call([
      'context',
      'show',
      '--literal-branch',
      'x'.repeat(300),
      '--json',
    ]);
    assert.equal(longBranch.code, 0);
    validateResponse(longBranch.value);
    assert.equal(longBranch.value.data.checkout.branch.length, 300);
    for (const format of ['json', 'toon']) {
      const r = await f.call([
        'context',
        'show',
        '--verify',
        '--job',
        'Payments_Build',
        ...(format === 'json' ? ['--json'] : []),
      ]);
      assert.equal(r.code, 0);
      assert.equal(r.stderr, '');
      validateResponse(r.value);
      assert.equal(r.value.data.verification.authentication, 'authenticated');
      assert.equal(r.value.data.verification.policy, 'verified');
      assert.equal(r.value.data.verification.jobs[0].projectId, 'Payments');
      assert.match(r.value.data.verification.identityFingerprint, /^sha256:/);
      assert.ok(!r.stdout.includes('fixture-only-token'));
      assert.ok(!r.stdout.includes('fixture-reader'));
    }
    const mismatch = await f.call([
      'context',
      'show',
      '--verify',
      '--job',
      'Payments_Build',
      '--project',
      'Other',
      '--json',
    ]);
    assert.equal(mismatch.code, 1);
    assert.equal(mismatch.value.error.code, 'CONTEXT_MISMATCH');
    assert.ok(
      f.server.requests.every(
        (r) => r.method === 'GET' && r.path !== '/teamcity/app/rest/projects',
      ),
    );
  } finally {
    await f.close();
  }
});
test('offline doctor has no HTTP; online probes are bounded and unsupported optional logs stay explicit', async () => {
  const f = await fixture();
  try {
    const offline = await f.call(['doctor', '--offline', '--json']);
    assert.equal(offline.code, 0);
    validateResponse(offline.value);
    assert.equal(f.server.requests.length, 0);
    assert.equal(offline.value.data.executable.version, '1.5.0');
    assert.equal(offline.value.data.authentication.state, 'not_checked');
    assert.equal(offline.value.meta.counts.childProcesses, 1);
    assert.equal(offline.value.data.liveCertified, false);
    const unboundOffline = await f.call(['doctor', '--offline', '--json'], {
      TEAMCITY_URL: undefined,
      TEAMCITY_TOKEN: 'offline-canary',
    });
    assert.equal(unboundOffline.code, 0);
    assert.equal(unboundOffline.value.data.executable.version, '1.5.0');
    assert.equal(f.server.requests.length, 0);
    assert.ok(!unboundOffline.stdout.includes('offline-canary'));
    const online = await f.call(['doctor', '--job', 'Payments_Build', '--json']);
    assert.equal(online.code, 0);
    validateResponse(online.value);
    assert.equal(online.value.status, 'partial');
    assert.equal(online.value.meta.counts.childProcesses, 8);
    assert.equal(online.value.data.limits.maxChildProcesses, 8);
    const capabilities = online.value.data.capabilities;
    for (const name of ['structuredRunDetail', 'boundedRunPages', 'structuredLogTail'])
      assert.equal(capabilities.find((c) => c.name === name).state, 'available');
    assert.equal(capabilities.find((c) => c.name === 'safeAgentRead').state, 'not_probed');
    f.server.setMode('logs-unsupported');
    const optional = await f.call(['doctor', '--job', 'Payments_Build', '--json']);
    assert.equal(optional.code, 0);
    assert.equal(
      optional.value.data.capabilities.find((c) => c.name === 'structuredLogTail').state,
      'unavailable',
    );
    assert.ok(optional.value.meta.limitations.some((l) => l.code === 'OPTIONAL_LOG_UNAVAILABLE'));
    const strict = await f.call([
      'doctor',
      '--job',
      'Payments_Build',
      '--require-complete',
      '--json',
    ]);
    assert.equal(strict.code, 1);
    assert.equal(strict.value.status, 'partial');
    f.server.setMode('logs-hang');
    const timed = await f.call(['doctor', '--job', 'Payments_Build', '--timeout', '2s', '--json']);
    assert.equal(timed.code, 0);
    assert.equal(timed.value.status, 'partial');
    assert.equal(timed.value.data.server.buildNumber, 'not-a-TeamCity-server');
    assert.equal(timed.value.data.authentication.state, 'authenticated');
    assert.equal(
      timed.value.data.capabilities.find((c) => c.name === 'structuredRunDetail').state,
      'available',
    );
    assert.ok(timed.value.meta.limitations.some((l) => l.code === 'DEADLINE_EXCEEDED'));
    for (const [mode, code] of [
      ['denied', 'PERMISSION_DENIED'],
      ['expired', 'AUTH_REQUIRED'],
      ['project-wrong-id', 'CONTEXT_MISMATCH'],
      ['project-cycle', 'UPSTREAM_SCHEMA_MISMATCH'],
    ]) {
      f.server.setMode(mode);
      const r = await f.call(['context', 'show', '--verify', '--project', 'Payments', '--json']);
      assert.equal(r.code, 1);
      assert.equal(r.value.error.code, code);
      assert.equal(r.value.data, undefined);
    }
  } finally {
    await f.close();
  }
});
