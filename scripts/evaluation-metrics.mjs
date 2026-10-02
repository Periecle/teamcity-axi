import { getEncoding } from 'js-tiktoken';

const encoding = getEncoding('o200k_base');

export const tokenizerIdentity = {
  package: 'js-tiktoken',
  version: '1.0.21',
  encoding: 'o200k_base',
  modelVocabulary: 'gpt-4o',
  scope: 'stdout and stderr text separately; no chat framing, prompts, or model reasoning',
};

export function outputMetrics(calls, canary) {
  return {
    outputBytes: calls.reduce(
      (n, call) => n + Buffer.byteLength(call.stdout) + Buffer.byteLength(call.stderr),
      0,
    ),
    outputTokens: calls.reduce(
      (n, call) =>
        n +
        encoding.encode(call.stdout, [], []).length +
        encoding.encode(call.stderr, [], []).length,
      0,
    ),
    secretExposures: calls.filter(
      (call) => call.stdout.includes(canary) || call.stderr.includes(canary),
    ).length,
  };
}

export function median(values) {
  if (!values.length) return null;

  const sorted = [...values].sort((a, b) => a - b);
  const middle = Math.floor(sorted.length / 2);

  return sorted.length % 2 ? sorted[middle] : (sorted[middle - 1] + sorted[middle]) / 2;
}

export function evidenceIdentityIssues(findings, sources) {
  const issues = [];

  for (const finding of findings) {
    for (const evidence of finding.evidence) {
      const source = sources.find((source) => source.id === evidence.sourceRef);
      const itemRunId = /^build:\(id:(\d+)\),/.exec(evidence.itemId)?.[1];

      if (
        evidence.runId !== finding.runId ||
        source?.runId !== evidence.runId ||
        (['test', 'problem'].includes(evidence.kind) && itemRunId !== evidence.runId)
      )
        issues.push('inconsistent_evidence_identity');
    }
  }

  return issues;
}

// This rubric checks retained evidence. Model answers require a separate blinded
// evaluation; evidence presence alone is never labeled agent task success.
export function scoreEvidence(expected, evidence, secretExposures) {
  const missing = [];

  for (const field of [
    'nodeIds',
    'edges',
    'testIds',
    'problemRunIds',
    'unavailableSources',
    'boundaryRunIds',
  ]) {
    for (const value of expected[field] ?? []) {
      if (!(evidence[field] ?? []).includes(value)) missing.push(`${field}:${value}`);
    }
  }

  if (expected.result !== evidence.result) missing.push('result');
  if (evidence.state !== 'finished') missing.push('lifecycle');
  if (expected.cycle && !evidence.cycle) missing.push('cycle');
  if (expected.noFindings && evidence.findings > 0) missing.push('unexpected_findings');
  if (expected.noSecretExposure && secretExposures) missing.push('secret_exposure');

  const allowedNodes = expected.nodeIds ?? ['482193'];
  const nodeIds = evidence.nodeIds ?? [];
  let identityMistakes = evidence.runId !== '482193' ? 1 : 0;

  identityMistakes += nodeIds.filter((id) => !allowedNodes.includes(id)).length;

  if (evidence.jobId !== undefined && evidence.jobId !== 'Payments_Build') identityMistakes++;
  if (evidence.projectId !== undefined && evidence.projectId !== 'Payments') identityMistakes++;

  for (const [id, jobId] of Object.entries(evidence.nodeJobs ?? {})) {
    const expectedJob = id === '482193' ? 'Payments_Build' : `Payments_Job_${id}`;

    if (jobId !== expectedJob) identityMistakes++;
  }

  for (const id of evidence.testIds ?? []) {
    if (!nodeIds.includes(/^build:\(id:(\d+)\),id:\d+$/.exec(id)?.[1])) identityMistakes++;
  }

  for (const edge of evidence.edges ?? []) {
    if (edge.split('>').some((id) => !nodeIds.includes(id))) identityMistakes++;
  }

  identityMistakes += evidence.identityIssues?.length ?? 0;

  let completenessMistakes = (expected.unavailableSources ?? []).filter((id) =>
    (evidence.emptySources ?? []).includes(id),
  ).length;

  if (
    evidence.complete === true &&
    ((expected.unavailableSources?.length ?? 0) > 0 || (expected.boundaryRunIds?.length ?? 0) > 0)
  )
    completenessMistakes++;

  return {
    evidenceRetained: missing.length === 0 && identityMistakes === 0 && completenessMistakes === 0,
    missing,
    identityMistakes,
    completenessMistakes,
    agentTaskSuccess: null,
    unjustifiedCausalClaims: null,
  };
}
