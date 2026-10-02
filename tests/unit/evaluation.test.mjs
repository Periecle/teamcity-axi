import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';

import {
  evidenceIdentityIssues,
  median,
  outputMetrics,
  scoreEvidence,
} from '../../scripts/evaluation-metrics.mjs';
import { selectFields } from '../fixtures/field-projection.mjs';

test('published evaluation binds the current corpus and distinguishes measured evidence from agent claims', async () => {
  const corpus = await readFile(new URL('../../evaluations/corpus.json', import.meta.url));
  const report = JSON.parse(
    await readFile(
      new URL('../../evaluations/results/linux-x64-node24-native1.5.0.json', import.meta.url),
      'utf8',
    ),
  );

  assert.equal(report.corpusSha256, createHash('sha256').update(corpus).digest('hex'));
  assert.equal(report.observations.length, 96);
  assert.equal(report.agentEvaluation, 'not-performed');
  assert.equal(report.startup.httpRequestCount, 0);

  for (const condition of report.conditions.filter((value) =>
    value.condition.startsWith('wrapper'),
  )) {
    assert.equal(condition.evidenceRetained, 24);
    assert.equal(condition.secretExposures, 0);
    assert.equal(condition.medianAgentFacingToolTurns, null);
    assert.equal(condition.agentTaskSuccess, null);
  }

  assert.ok(!JSON.stringify(report).includes('evaluation-secret-'));
});

test('evaluation fixture applies nested REST projections before native output is captured', () => {
  const input = {
    count: 1,
    build: [
      {
        id: 7,
        statusText: 'Excluded',
        buildType: { id: 'Job', name: 'Excluded', projectId: 'Project' },
      },
    ],
    unknown: true,
  };

  assert.deepEqual(selectFields(input, 'count,build(id,buildType(id,projectId))'), {
    count: 1,
    build: [{ id: 7, buildType: { id: 'Job', projectId: 'Project' } }],
  });
  assert.throws(() => selectFields(input, 'build(id'), /Unbalanced/);
});

test('evaluation counts tokenizer tokens per channel, including literal special-token text', () => {
  const result = outputMetrics(
    [{ stdout: 'hello world', stderr: '<|endoftext|>🦊secret-canary' }],
    'secret-canary',
  );

  assert.ok(result.outputTokens > 2);
  assert.equal(result.outputBytes, Buffer.byteLength('hello world<|endoftext|>🦊secret-canary'));
  assert.equal(result.secretExposures, 1);
  assert.equal(outputMetrics([{ stdout: 'hello world', stderr: '' }], 'absent').outputTokens, 2);
  assert.equal(median([9, 1, 5, 3]), 4);
  assert.equal(median([]), null);
});

test('evaluation rejects wrong identities, false empty sources, omissions, and exposed canaries without inventing agent accuracy', () => {
  const expected = {
    result: 'failure',
    nodeIds: ['482193', '482190'],
    unavailableSources: ['tests:482193'],
    noSecretExposure: true,
  };
  const result = scoreEvidence(
    expected,
    { runId: '482999', result: 'failure', nodeIds: ['482193'], emptySources: ['tests:482193'] },
    1,
  );

  assert.equal(result.evidenceRetained, false);
  assert.equal(result.identityMistakes, 1);
  assert.equal(result.completenessMistakes, 1);
  assert.ok(result.missing.includes('nodeIds:482190'));
  assert.ok(result.missing.includes('secret_exposure'));
  assert.equal(result.agentTaskSuccess, null);
  assert.equal(result.unjustifiedCausalClaims, null);
});

test('evaluation rejects foreign sibling identities and unsupported complete-coverage claims', () => {
  const result = scoreEvidence(
    { result: 'failure', unavailableSources: ['problems:482193'] },
    {
      runId: '482193',
      jobId: 'Foreign_Job',
      projectId: 'Foreign',
      result: 'failure',
      nodeIds: ['482193', '999'],
      nodeJobs: { 482193: 'Foreign_Job' },
      unavailableSources: ['problems:482193'],
      complete: true,
    },
    0,
  );

  assert.equal(result.evidenceRetained, false);
  assert.equal(result.identityMistakes, 4);
  assert.equal(result.completenessMistakes, 1);
});

test('evaluation binds compound occurrence identity to its exact finding and source execution', () => {
  const sources = [{ id: 'tests:482193', runId: '482193' }];
  const findings = [
    {
      runId: '482193',
      evidence: [
        {
          kind: 'test',
          runId: '482193',
          sourceRef: 'tests:482193',
          itemId: 'build:(id:482190),id:1',
        },
      ],
    },
  ];

  assert.deepEqual(evidenceIdentityIssues(findings, sources), ['inconsistent_evidence_identity']);
  findings[0].evidence[0].itemId = 'build:(id:482193),id:1';
  assert.deepEqual(evidenceIdentityIssues(findings, sources), []);
});
