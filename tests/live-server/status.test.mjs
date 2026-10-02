import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { writeFile, rm } from 'node:fs/promises';
import { join, isAbsolute } from 'node:path';
import { decode } from '@toon-format/toon';
import { liveFixture } from '../../scripts/live-harness.mjs';
import { validateResponse } from '../../dist/output/schema.js';
import { parseRaw } from '../../dist/adapter/raw.js';

test('restricted live status certifies the exact clean checkout and fails dirty, stale, missing and unmatched-root assertions', async () => {
  const source = process.env.TEAMCITY_AXI_LIVE_CHECKOUT;
  if (!source || !isAbsolute(source))
    throw Error(
      'Live exact-checkout tests require an absolute TEAMCITY_AXI_LIVE_CHECKOUT fixture path; no skips',
    );
  const f = await liveFixture();
  try {
    const checkout = join(f.dir, 'checkout');
    const env = {
      PATH: process.env.PATH,
      HOME: f.dir,
      GIT_CONFIG_GLOBAL: '/dev/null',
      GIT_CONFIG_NOSYSTEM: '1',
    };
    execFileSync(
      'git',
      [
        '-c',
        'core.hooksPath=/dev/null',
        'clone',
        '--no-hardlinks',
        '--no-local',
        '--',
        source,
        checkout,
      ],
      { env, stdio: ['ignore', 'pipe', 'pipe'], timeout: 10000 },
    );
    const head = execFileSync('git', ['rev-parse', 'HEAD'], {
      cwd: checkout,
      env,
      encoding: 'utf8',
    }).trim();
    const args = [
      'status',
      '--job',
      f.contract.fixture.vcsJobId,
      '--vcs-root',
      f.contract.fixture.vcsRootId,
      '--all-branches',
      '--cwd',
      checkout,
      '--check',
    ];
    const json = await f.wrapper([...args, '--json']);
    const toon = await f.wrapper(args);
    for (const result of [json, toon]) {
      assert.equal(result.code, 0);
      const value = result === json ? JSON.parse(result.stdout) : decode(result.stdout);
      validateResponse(value);
      assert.equal(value.data.check.passed, true);
      assert.equal(value.data.checkout.head, head);
      assert.equal(value.data.jobs[0].run.id, f.contract.fixture.vcsRunId);
      assert.equal(value.data.jobs[0].run.revisions[0].revision, head);
      assert.equal(value.meta.counts.childProcesses, 2);
      assert.ok(Buffer.byteLength(result.stdout) <= 6144);
    }
    await writeFile(join(checkout, 'owned-dirty-test.txt'), 'Owned dirty checkout fixture');
    const dirty = await f.wrapper([...args, '--json']);
    assert.equal(dirty.code, 1);
    assert.equal(JSON.parse(dirty.stdout).data.checkout.dirty, true);
    await rm(join(checkout, 'owned-dirty-test.txt'));
    for (const extra of [
      ['--revision', '0'.repeat(40)],
      ['--vcs-root', 'Unmatched_Git'],
    ]) {
      const filtered = args.filter(
        (_, index) => !extra.includes('--vcs-root') || ![3, 4].includes(index),
      );
      const result = await f.wrapper([...filtered, ...extra, '--json']);
      assert.equal(result.code, 1);
      const value = JSON.parse(result.stdout);
      validateResponse(value);
      assert.equal(value.data.check.passed, false);
      assert.notEqual(value.data.assessment, 'passed');
    }
    for (const name of [
      'status-one',
      'status-five',
      'status-branch',
      'status-missing-job',
      'status-denied',
    ]) {
      const capture = await f.native(f.contract.records[name].args);
      const body = parseRaw({
        stdout: Buffer.from(capture.stdout),
        stderr: Buffer.from(capture.stderr),
        exitCode: capture.code,
        signal: capture.signal,
      }).body;
      if (['status-missing-job', 'status-denied'].includes(name)) assert.equal(body.count, 0);
      if (name === 'status-five') assert.equal(body.count, 5);
    }
    const missing = await f.wrapper([
      'status',
      '--job',
      'AxiContract_Missing',
      '--all-branches',
      '--cwd',
      checkout,
      '--check',
      '--json',
    ]);
    assert.equal(missing.code, 1);
    assert.equal(JSON.parse(missing.stdout).data.jobs[0].availability, 'unavailable');
  } finally {
    await f.close();
  }
});
