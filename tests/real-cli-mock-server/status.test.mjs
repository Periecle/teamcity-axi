import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, readFile, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { createHash } from 'node:crypto';
import { decode } from '@toon-format/toon';
import { mockServer } from '../fixtures/mock-server.mjs';
import { validateResponse } from '../../dist/output/schema.js';
import { parse } from '../../dist/cli/parser.js';

async function fixture(jobCount = 1) {
  const binary = process.env.TEAMCITY_AXI_TEST_BINARY;
  if (!binary) throw Error('Checksum-verified released CLI required; no skips');
  const manifest = JSON.parse(await readFile('docs/compatibility.json', 'utf8'));
  const sha = createHash('sha256')
    .update(await readFile(binary))
    .digest('hex');
  assert.ok(manifest.artifacts.some((artifact) => artifact.binarySha256 === sha));
  const server = await mockServer();
  const dir = await mkdtemp(join(tmpdir(), 'axi-status-'));
  const repo = join(dir, 'repo');
  await mkdir(repo);
  await mkdir(join(dir, 'teamcity-axi'));
  const configPath = join(dir, 'teamcity-axi', 'config.json');
  const config = {
    schemaVersion: '1.0',
    readOnly: true,
    defaultServer: 'work',
    binaryPath: resolve(binary),
    servers: { work: { url: server.base, allowHttpLoopback: true, allowedProjects: ['Payments'] } },
  };
  await writeFile(configPath, JSON.stringify(config), { mode: 0o600 });
  const env = {
    HOME: dir,
    XDG_CONFIG_HOME: dir,
    PATH: process.env.PATH,
    GIT_CONFIG_GLOBAL: '/dev/null',
    GIT_CONFIG_NOSYSTEM: '1',
    TEAMCITY_URL: server.base,
    TEAMCITY_TOKEN: 'fixture-only-token',
  };
  const git = (...args) =>
    execFileSync('git', args, {
      cwd: repo,
      env,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
    }).trim();
  git('init', '-b', 'feature/refund');
  git('remote', 'add', 'origin', 'https://example.invalid/repository.git');
  const jobs = Array.from({ length: jobCount }, (_, index) =>
    index ? `Payments_Job${index}` : 'Payments_Build',
  );
  await writeFile(
    join(repo, 'teamcity.toml'),
    `[[server]]\nurl = "${server.base}"\nproject = "Payments"\njobs = ${JSON.stringify(jobs)}\n`,
  );
  await writeFile(
    join(repo, '.teamcity-axi.json'),
    JSON.stringify({
      schemaVersion: '1.0',
      vcsRoots: [{ server: 'work', remote: 'origin', rootId: 'Payments_Git' }],
    }),
  );
  git('add', '.');
  git(
    '-c',
    'user.name=Fixture',
    '-c',
    'user.email=fixture@example.invalid',
    '-c',
    'commit.gpgsign=false',
    '-c',
    'core.hooksPath=/dev/null',
    'commit',
    '-m',
    'Owned status test fixture',
  );
  const head = git('rev-parse', 'HEAD');
  server.setStatusRevision(head);
  const call = (args, cwd = repo) =>
    new Promise((done, reject) => {
      const child = spawn(process.execPath, [resolve('bin/teamcity-axi.mjs'), ...args], {
        cwd,
        env,
        stdio: ['ignore', 'pipe', 'pipe'],
        timeout: 15000,
      });
      const out = [],
        err = [];
      child.stdout.on('data', (chunk) => out.push(chunk));
      child.stderr.on('data', (chunk) => err.push(chunk));
      child.on('error', reject);
      child.on('close', (code) => {
        const stdout = Buffer.concat(out).toString();
        const value = args.includes('--json') ? JSON.parse(stdout) : decode(stdout);
        done({ code, stdout, value, stderr: Buffer.concat(err).toString() });
      });
    });
  return {
    server,
    dir,
    repo,
    head,
    git,
    jobs,
    config,
    configPath,
    call,
    close: async () => {
      await server.close();
      await rm(dir, { recursive: true, force: true });
    },
  };
}

test('exact clean status checks five required jobs within home budgets and both serializers', async () => {
  const f = await fixture(5);
  try {
    const json = await f.call(['status', '--check', '--json']);
    const toon = await f.call(['status', '--check']);
    for (const result of [json, toon]) {
      assert.equal(result.code, 0);
      assert.equal(result.stderr, '');
      validateResponse(result.value);
      assert.equal(result.value.status, 'ok');
      assert.equal(result.value.data.assessment, 'passed');
      assert.equal(result.value.data.check.passed, true);
      assert.equal(result.value.data.jobs.length, 5);
      assert.equal(result.value.data.checkout.head, f.head);
      assert.equal(result.value.meta.counts.childProcesses, 2);
      assert.equal(result.value.meta.limits.maxChildProcesses, 6);
      assert.equal(result.value.meta.limits.concurrency, 2);
      assert.ok(Buffer.byteLength(result.stdout) <= 6144);
      for (const hint of result.value.next) parse(hint.argv.slice(1));
    }
    assert.deepEqual(json.value.data, toon.value.data);
    assert.equal(f.server.requests.length, 2);
    for (const request of f.server.requests) {
      assert.equal(request.method, 'GET');
      assert.equal(request.path, '/teamcity/app/rest/buildTypes');
      assert.equal((request.query.locator.match(/item:/g) ?? []).length, 5);
      assert.ok(request.query.fields.includes('count:20,start:0,lookupLimit:5000'));
      assert.ok(request.query.fields.includes('state:any'));
      assert.ok(!request.query.fields.includes('parameters'));
    }
  } finally {
    await f.close();
  }
});

test('red is exit zero for observation and one for assertion; active exact executions never fall back to green', async () => {
  const f = await fixture();
  try {
    for (const mode of ['status-red', 'status-running', 'status-queued']) {
      f.server.setMode(mode);
      const observed = await f.call(['status', '--json']);
      const checked = await f.call(['status', '--check', '--json']);
      assert.equal(observed.code, 0);
      assert.equal(checked.code, 1);
      validateResponse(checked.value);
      assert.equal(checked.value.data.check.passed, false);
      assert.equal(checked.value.data.jobs[0].match, 'exact');
      assert.equal(
        checked.value.data.jobs[0].assessment,
        mode === 'status-red' ? 'failed' : 'in_progress',
      );
      if (mode === 'status-red') assert.deepEqual(checked.value.data.failedRunIds, ['482193']);
    }
  } finally {
    await f.close();
  }
});

test('stale, missing, newer unknown, personal and multi-root checkouts never assert green', async () => {
  const f = await fixture();
  try {
    for (const mode of [
      'status-stale',
      'status-unknown',
      'status-newer-unknown',
      'status-personal',
      'status-multi-root',
      'status-missing-job',
      'status-unsafe-continuation',
    ]) {
      f.server.setMode(mode);
      const result = await f.call(['status', '--check', '--json']);
      assert.equal(result.code, 1, mode);
      validateResponse(result.value);
      assert.equal(result.value.status, 'partial', mode);
      assert.equal(result.value.data.check.passed, false, mode);
      assert.equal(result.value.data.assessment, 'unverified', mode);
      if (mode === 'status-multi-root')
        assert.equal(result.value.data.jobs[0].run.revisions.length, 2);
      if (mode === 'status-missing-job')
        assert.equal(result.value.data.jobs[0].availability, 'unavailable');
    }
  } finally {
    await f.close();
  }
});

test('dirty, detached, explicit foreign revision and omitted sixth required job have distinct checks', async () => {
  const f = await fixture(6);
  try {
    const truncated = await f.call(['status', '--check', '--json']);
    assert.equal(truncated.code, 1);
    assert.equal(truncated.value.data.coverage.requiredJobs, 6);
    assert.equal(truncated.value.data.coverage.displayedJobs, 5);
    assert.equal(truncated.value.context.jobs.length, 5);
    assert.ok(
      truncated.value.meta.limitations.some((note) => note.code === 'TRACKED_JOBS_TRUNCATED'),
    );
    const explicit = ['status', '--job', 'Payments_Build', '--check', '--json'];
    assert.equal((await f.call(explicit)).code, 0);
    await writeFile(join(f.repo, 'uncommitted.txt'), 'Owned dirty worktree fixture');
    const dirty = await f.call(explicit);
    assert.equal(dirty.code, 1);
    assert.equal(dirty.value.data.checkout.dirty, true);
    await rm(join(f.repo, 'uncommitted.txt'));
    const different = await f.call([...explicit, '--revision', 'b'.repeat(40)]);
    assert.equal(different.code, 1);
    assert.ok(
      different.value.meta.limitations.some((note) => note.code === 'CHECKOUT_IDENTITY_UNVERIFIED'),
    );
    f.git('checkout', '--detach', f.head);
    const detached = await f.call(explicit);
    assert.equal(detached.code, 0);
    assert.equal(detached.value.data.checkout.branch, null);
  } finally {
    await f.close();
  }
});

test('status preserves local unconfigured home, scoped errors, permission denial and output limits', async () => {
  const f = await fixture();
  try {
    const home = await f.call(['--json'], f.dir);
    assert.equal(home.code, 0);
    assert.equal(home.value.data.mode, 'unconfigured');
    validateResponse(home.value);
    assert.equal(f.server.requests.length, 0);
    const noJobs = await f.call(['status', '--json'], f.dir);
    assert.equal(noJobs.code, 2);
    assert.equal(noJobs.value.error.code, 'CONTEXT_REQUIRED');
    assert.equal(f.server.requests.length, 0);
    for (const mode of ['denied', 'malformed', 'expired', 'status-foreign', 'status-wrong-run']) {
      f.server.setMode(mode);
      const result = await f.call(['status', '--check', '--json']);
      assert.equal(result.code, 1);
      assert.equal(result.value.status, 'error');
      assert.equal(result.value.data, undefined);
    }
    f.server.setMode('status-huge');
    const huge = await f.call(['status', '--json']);
    assert.equal(huge.code, 1);
    assert.equal(huge.value.error.code, 'INPUT_LIMIT_EXCEEDED');
    assert.ok(Buffer.byteLength(huge.stdout) <= 6144);
    f.config.limits = { maxChildProcesses: 1 };
    await writeFile(f.configPath, JSON.stringify(f.config), { mode: 0o600 });
    f.server.requests.splice(0);
    const limited = await f.call(['status', '--check', '--json']);
    assert.equal(limited.code, 1);
    assert.equal(limited.value.error.code, 'INPUT_LIMIT_EXCEEDED');
    assert.equal(f.server.requests.length, 0);
  } finally {
    await f.close();
  }
});

test('strict status and dash-leading identity hints preserve public flag contracts', async () => {
  const f = await fixture();
  try {
    const selected = await f.call(['status', '--job=-leading-job', '--check', '--json']);
    assert.equal(selected.code, 0);
    for (const hint of selected.value.next) {
      parse(hint.argv.slice(1));
      assert.ok(hint.argv.includes('--job=-leading-job'));
      assert.ok(hint.argv.includes('--project=Payments'));
    }
    f.server.setMode('status-unknown');
    const strict = await f.call(['status', '--require-complete', '--json']);
    assert.equal(strict.code, 1);
    assert.equal(strict.value.status, 'partial');
    const unchecked = await f.call(['--check', '--json'], f.dir);
    assert.equal(unchecked.code, 1);
    assert.equal(unchecked.value.data.mode, 'unconfigured');
  } finally {
    await f.close();
  }
});

test('concurrent separate checkouts preserve their own revisions with invocation-local status state', async () => {
  const a = await fixture(),
    b = await fixture();
  try {
    await writeFile(join(b.repo, 'another.txt'), 'Different committed worktree');
    b.git('add', '.');
    b.git(
      '-c',
      'user.name=Fixture',
      '-c',
      'user.email=fixture@example.invalid',
      '-c',
      'commit.gpgsign=false',
      '-c',
      'core.hooksPath=/dev/null',
      'commit',
      '-m',
      'Different checkout',
    );
    const bHead = b.git('rev-parse', 'HEAD');
    b.server.setStatusRevision(bHead);
    const results = await Promise.all([
      a.call(['status', '--check', '--json']),
      b.call(['status', '--check', '--json']),
    ]);
    assert.ok(results.every((result) => result.code === 0));
    assert.equal(results[0].value.data.checkout.head, a.head);
    assert.equal(results[1].value.data.checkout.head, bHead);
    assert.notEqual(a.head, bHead);
    assert.equal(results[0].value.data.jobs[0].run.revisions[0].revision, a.head);
    assert.equal(results[1].value.data.jobs[0].run.revisions[0].revision, bHead);
  } finally {
    await a.close();
    await b.close();
  }
});

test('checkout checks distinguish exceptional and composite outcomes without false green', async () => {
  const f = await fixture();
  try {
    for (const [mode, result, passed] of [
      ['status-canceled', 'canceled', false],
      ['status-failed-to-start', 'failed_to_start', false],
      ['status-missing-outcome', 'unknown', false],
      ['status-composite', 'success', true],
    ]) {
      f.server.setMode(mode);
      const response = await f.call(['status', '--check', '--json']);
      validateResponse(response.value);
      assert.equal(response.value.data.jobs[0].run.result, result);
      assert.equal(response.value.data.check.passed, passed);
      assert.equal(response.code, passed ? 0 : 1);
    }
    assert.ok(f.server.requests.every((request) => request.method === 'GET'));
  } finally {
    await f.close();
  }
});
