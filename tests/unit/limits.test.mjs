import { test } from 'node:test';
import assert from 'node:assert/strict';

import { readLimits, readProfile, publicLimits } from '../../dist/transport/limits.js';

test('effective read ceilings follow actual command profiles and tighter trusted settings', () => {
  const context = { deadline: Date.now() + 10000 };
  for (const [command, children, concurrency, stdoutBytes] of [
    ['run.view', 8, 3, 2097152],
    ['run.list', 8, 3, 2097152],
    ['status', 6, 2, 1048576],
    ['run.watch', 32, 1, 1048576],
    ['run.tree', 24, 3, 2097152],
    ['run.failure', 24, 3, 2097152],
  ]) {
    const limits = readLimits(context, readProfile(command));
    assert.deepEqual(limits, {
      deadline: context.deadline,
      maxChildren: children,
      concurrency,
      stdoutBytes,
      stderrBytes: 65536,
    });
    const tighter = readLimits(
      { ...context, config: { limits: { maxChildProcesses: 2, concurrency: 1 } } },
      readProfile(command),
    );
    assert.equal(tighter.maxChildren, 2);
    assert.equal(tighter.concurrency, 1);
    const broader = readLimits(
      { ...context, config: { limits: { maxChildProcesses: 256, concurrency: 8 } } },
      readProfile(command),
    );
    assert.deepEqual(broader, limits);
    assert.deepEqual(publicLimits(tighter, 4096), {
      maxBytes: 4096,
      maxChildProcesses: 2,
      concurrency: 1,
      deadline: context.deadline,
      stdoutCaptureBytes: stdoutBytes,
      stderrCaptureBytes: 65536,
    });
  }
});
