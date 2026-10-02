import { test } from 'node:test';
import assert from 'node:assert/strict';
import { decode } from '@toon-format/toon';

import { liveFixture } from '../../scripts/live-harness.mjs';
import { parseRaw } from '../../dist/adapter/raw.js';
import { validateResponse } from '../../dist/output/schema.js';

const body = (value) =>
  parseRaw({
    stdout: Buffer.from(value.stdout),
    stderr: Buffer.from(value.stderr),
    exitCode: value.code,
    signal: value.signal,
  }).body;

test('restricted live outcome reads retain cancellation, failed start and composite across command services', async () => {
  const f = await liveFixture();
  try {
    const fixture = f.contract.fixture.lifecycle;
    const cases = [
      [fixture.queuedCanceledRunId, fixture.jobs.blocked, 'canceled'],
      [fixture.runningCanceledRunId, fixture.jobs.slow, 'canceled'],
      [fixture.failedToStartRunId, 'AxiContract_Vcs', 'failed_to_start'],
      [fixture.compositeRunId, fixture.jobs.composite, 'success'],
    ];
    for (const [id, jobId, result] of cases) {
      for (const flags of [['--json'], []]) {
        const wire = await f.wrapper(['run', 'view', id, ...flags]);
        assert.equal(wire.code, 0);
        const value = flags.length ? JSON.parse(wire.stdout) : decode(wire.stdout);
        validateResponse(value);
        assert.equal(value.data.run.id, id);
        assert.equal(value.data.run.result, result);
        assert.equal(value.data.run.state, 'finished');
        assert.equal(value.data.run.composite, id === fixture.compositeRunId);
        assert.ok(!wire.stdout.includes('canceledInfo'));
      }
      const list = JSON.parse(
        (
          await f.wrapper([
            'run',
            'list',
            '--job',
            jobId,
            '--all-branches',
            '--result',
            result,
            '--json',
          ])
        ).stdout,
      );
      validateResponse(list);
      assert.ok(list.data.runs.some((run) => run.id === id && run.result === result));
      assert.equal(list.data.aggregates[result], list.data.runs.length);
      const watchWire = await f.wrapper(['run', 'watch', id, '--check', '--json']);
      const watch = JSON.parse(watchWire.stdout);
      validateResponse(watch);
      assert.equal(watch.data.run.result, result);
      assert.equal(watch.data.check.passed, result === 'success');
      assert.equal(watchWire.code, result === 'success' ? 0 : 1);
      const failure = JSON.parse((await f.wrapper(['run', 'failure', id, '--json'])).stdout);
      validateResponse(failure);
      assert.equal(failure.data.run.result, result);
      assert.equal(
        failure.data.assessment,
        result === 'success' ? 'not_failed' : 'failure_observed',
      );
    }
    const ordinary = JSON.parse(
      (
        await f.wrapper([
          'run',
          'list',
          '--job',
          'AxiContract_Vcs',
          '--all-branches',
          '--result',
          'failure',
          '--json',
        ])
      ).stdout,
    );
    validateResponse(ordinary);
    assert.deepEqual(ordinary.data.runs, []);
    const permissions = body(await f.native(f.contract.records['outcome-permissions'].args));
    assert.deepEqual(permissions.permissionAssignment.map((row) => row.permission.id).sort(), [
      'change_own_profile',
      'view_project',
      'view_project',
      'view_project',
    ]);
    assert.deepEqual(
      permissions.permissionAssignment
        .filter((row) => row.permission.id === 'view_project')
        .map((row) => row.project.id)
        .sort(),
      [f.contract.fixture.projectId, fixture.projectId, '_Root'].sort(),
    );
    assert.ok(
      permissions.permissionAssignment
        .filter((row) => row.permission.id === 'view_project')
        .every((row) => !row.isGlobalScope),
    );
  } finally {
    await f.close();
  }
});
