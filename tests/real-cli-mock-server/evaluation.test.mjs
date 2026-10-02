import { test } from 'node:test';
import assert from 'node:assert/strict';

import { evaluate } from '../../scripts/evaluate.mjs';

test('released native evaluation retains critical wrapper evidence and publishes honest baseline regressions', async () => {
  const report = await evaluate({ repetitions: 1 });

  assert.equal(report.observations.length, 32);
  assert.equal(report.agentEvaluation, 'not-performed');

  for (const row of report.observations.filter((row) => row.condition.startsWith('wrapper'))) {
    assert.equal(
      row.score.evidenceRetained,
      true,
      `${row.condition}/${row.taskId}: ${row.score.missing.join(', ')}`,
    );
    assert.equal(row.secretExposures, 0);
    assert.equal(row.withinByteBudget, true);
    assert.equal(row.agentFacingToolTurns, null);
    assert.ok(row.nativeSubprocessCount <= 24);
  }

  for (const row of report.observations.filter(
    (row) => row.condition === 'native-selected-json' && row.taskId !== 'secret-in-tests',
  )) {
    assert.equal(
      row.score.evidenceRetained,
      true,
      `${row.taskId}: ${row.score.missing.join(', ')}`,
    );

    for (const call of row.calls.filter((value) => value.kind === 'run')) {
      const dto = JSON.parse(call.stdout);

      assert.equal(dto.number, undefined);
      assert.equal(dto.statusText, undefined);
      assert.equal(dto.startDate, undefined);
      assert.equal(dto.buildType.name, undefined);
    }
  }

  const diagnostics = report.observations.find(
    (row) => row.condition === 'native-failure-diagnostics' && row.taskId === 'denied-problems',
  );

  assert.equal(diagnostics.score.evidenceRetained, false);
  assert.ok(diagnostics.score.missing.includes('unavailableSources:problems:482193'));
  assert.ok(
    report.observations.some(
      (row) => row.condition === 'native-selected-json' && row.secretExposures > 0,
    ),
  );
  assert.ok(JSON.stringify(report).includes('<secret-canary>'));
  assert.ok(!JSON.stringify(report).includes('evaluation-secret-'));
});
