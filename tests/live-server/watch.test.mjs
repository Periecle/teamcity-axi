import { test } from 'node:test';
import assert from 'node:assert/strict';
import { decode } from '@toon-format/toon';
import { liveFixture } from '../../scripts/live-harness.mjs';
import { validateResponse } from '../../dist/output/schema.js';

test('live watch observes fixed green/red/queued executions read-only with assertion and deadline semantics', async () => {
  const f = await liveFixture();
  try {
    const greenArgs = ['run', 'watch', f.contract.fixture.vcsRunId, '--check'];
    for (const args of [[...greenArgs, '--json'], greenArgs]) {
      const result = await f.wrapper(args);
      assert.equal(result.code, 0);
      const value = args.includes('--json') ? JSON.parse(result.stdout) : decode(result.stdout);
      validateResponse(value);
      assert.equal(value.data.outcome, 'finished');
      assert.equal(value.data.check.passed, true);
      assert.equal(value.data.run.id, f.contract.fixture.vcsRunId);
      assert.equal(value.meta.counts.childProcesses, 2);
    }
    const redArgs = ['run', 'watch', f.contract.fixture.failedRunId, '--json'];
    assert.equal((await f.wrapper(redArgs)).code, 0);
    assert.equal((await f.wrapper([...redArgs, '--check'])).code, 1);
    const queued = await f.wrapper([
      'run',
      'watch',
      f.contract.fixture.queuedRunIds[0],
      '--timeout',
      '700ms',
      '--check',
      '--json',
    ]);
    assert.equal(queued.code, 1);
    const value = JSON.parse(queued.stdout);
    validateResponse(value);
    assert.equal(value.data.outcome, 'deadline');
    assert.equal(value.data.run.state, 'queued');
    assert.equal(value.meta.complete, false);
    assert.ok(value.meta.limitations.some((note) => note.code === 'DEADLINE_EXCEEDED'));
  } finally {
    await f.close();
  }
});
