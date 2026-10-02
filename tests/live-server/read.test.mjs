import { test } from 'node:test';
import assert from 'node:assert/strict';
import { decode } from '@toon-format/toon';
import { liveFixture } from '../../scripts/live-harness.mjs';
import { parseRaw } from '../../dist/adapter/raw.js';
import { validateResponse } from '../../dist/output/schema.js';
import { parse } from '../../dist/cli/parser.js';
const apiBody = (r) =>
  parseRaw({
    stdout: Buffer.from(r.stdout),
    stderr: Buffer.from(r.stderr),
    exitCode: r.code,
    signal: r.signal,
  }).body;
test('pinned CLI confirms the live server and restricted permission inventory with bounded reads', async () => {
  const f = await liveFixture();
  try {
    const version = await f.native(['--version']);
    assert.equal(version.stdout, 'teamcity version 1.5.0\n');
    assert.equal(version.code, 0);
    const server = apiBody(await f.native(f.contract.records.server.args));
    assert.equal(server.buildNumber, f.contract.server.buildNumber);
    const permissions = apiBody(
      await f.native(f.contract.records.permissions.args),
    ).permissionAssignment;
    assert.deepEqual(permissions.map((p) => p.permission.id).sort(), [
      'change_own_profile',
      'view_project',
      'view_project',
    ]);
    assert.ok(
      permissions
        .filter((p) => p.permission.id === 'view_project')
        .every(
          (p) => !p.isGlobalScope && [f.contract.fixture.projectId, '_Root'].includes(p.project.id),
        ),
    );
    for (const name of [
      'problems',
      'tests',
      'dependencies',
      'changes',
      'queue',
      'jobs',
      'agents',
      'pages',
      'encoded-page',
      'empty-page',
    ])
      apiBody(await f.native(f.contract.records[name].args));
    const unsupported = await f.native(f.contract.records['dependencies-unsupported'].args);
    assert.equal(unsupported.code, 1);
    assert.throws(
      () => apiBody(unsupported),
      (e) => e.code === 'UPSTREAM_FAILURE',
    );
    const log = await f.native(f.contract.records.log.args);
    assert.equal(log.code, 0);
    assert.equal(JSON.parse(log.stdout).run_id, f.contract.fixture.failedRunId);
  } finally {
    await f.close();
  }
});
test('actual bounded run list returns a page then preserves exhaustion uncertainty on continuation', async () => {
  const f = await liveFixture();
  try {
    const args = [
      'run',
      'list',
      '--job',
      f.contract.fixture.jobId,
      '--all-branches',
      '--limit',
      '1',
      '--since',
      '2026-10-01T00:00:00Z',
      '--until',
      '2026-10-03T00:00:00Z',
      '--json',
    ];
    const r = await f.wrapper(args);
    assert.equal(r.code, 0);
    assert.equal(r.stderr, '');
    const first = JSON.parse(r.stdout);
    validateResponse(first);
    assert.equal(first.status, 'ok');
    assert.equal(first.data.runs[0].id, '1');
    assert.equal(first.data.runs[0].result, 'failure');
    assert.equal(first.data.page.returned, 1);
    assert.equal(first.data.page.hasMore, true);
    assert.equal(first.data.page.total, null);
    assert.equal(first.data.page.totalKind, 'unknown');
    assert.equal(first.meta.counts.childProcesses, 3);
    for (const action of first.next) parse(action.argv.slice(1));
    const second = JSON.parse((await f.wrapper([...first.next[0].argv.slice(1), '--json'])).stdout);
    validateResponse(second);
    assert.deepEqual(second.data.runs, []);
    assert.equal(second.status, 'partial');
    assert.equal(second.data.page.hasMore, null);
    assert.equal(second.data.page.total, null);
    const changed = await f.wrapper([
      ...args,
      '--cursor',
      first.data.page.cursor,
      '--result',
      'success',
    ]);
    assert.equal(changed.code, 2);
    const project = JSON.parse(
      (
        await f.wrapper([
          'run',
          'list',
          '--project',
          f.contract.fixture.projectId,
          '--all-branches',
          '--fields',
          'number',
          '--json',
        ])
      ).stdout,
    );
    validateResponse(project);
    assert.deepEqual(
      project.data.runs.map((v) => v.id),
      f.contract.fixture.projectRunIds,
    );
    assert.equal(project.data.aggregates.scope, 'returnedPage');
    assert.equal(project.data.aggregates.failure, f.contract.fixture.projectFailedRunIds.length);
    assert.ok(project.data.runs.every((r) => Object.hasOwn(r, 'branch')));
    const precision = await f.wrapper([
      'run',
      'list',
      '--job',
      f.contract.fixture.jobId,
      '--all-branches',
      '--since',
      '2026-10-01T00:00:00.0001Z',
      '--json',
    ]);
    assert.equal(precision.code, 2);
    assert.equal(JSON.parse(precision.stdout).error.code, 'USAGE_ERROR');
  } finally {
    await f.close();
  }
});
test('actual wrapper returns exact failed observation and preserves denied, missing and mismatch errors', async () => {
  const f = await liveFixture();
  try {
    for (const format of ['json', 'toon']) {
      const r = await f.wrapper([
        'run',
        'view',
        f.contract.fixture.failedRunId,
        '--format',
        format,
      ]);
      assert.equal(r.code, 0);
      assert.equal(r.stderr, '');
      const v = format === 'json' ? JSON.parse(r.stdout) : decode(r.stdout);
      validateResponse(v);
      assert.equal(v.status, 'ok');
      assert.equal(v.data.run.id, f.contract.fixture.failedRunId);
      assert.equal(v.data.run.result, 'failure');
      assert.equal(v.context.server, 'sandbox');
    }
    for (const [id, flags, code] of [
      [f.contract.fixture.deniedRunId, [], 'PERMISSION_DENIED'],
      [f.contract.fixture.missingRunId, [], 'NOT_FOUND'],
      [f.contract.fixture.failedRunId, ['--job', 'AnotherJob'], 'CONTEXT_MISMATCH'],
    ]) {
      const r = await f.wrapper(['run', 'view', id, '--json', ...flags]);
      const v = JSON.parse(r.stdout);
      assert.equal(r.code, 1);
      assert.equal(v.error.code, code);
      assert.equal(v.data, undefined);
      validateResponse(v);
    }
    const expired = await f.native(f.contract.records['invalid-auth'].args, true);
    assert.throws(
      () => apiBody(expired),
      (e) => e.code === 'AUTH_REQUIRED',
    );
  } finally {
    await f.close();
  }
});

test('live context verification and doctor preserve restricted scope and optional log limits', async () => {
  const f = await liveFixture();
  try {
    const local = JSON.parse(
      (await f.wrapper(['context', 'show', '--job', f.contract.fixture.jobId, '--json'])).stdout,
    );
    validateResponse(local);
    assert.equal(local.data.verification.requested, false);
    const verified = await f.wrapper([
      'context',
      'show',
      '--verify',
      '--job',
      f.contract.fixture.jobId,
      '--json',
    ]);
    assert.equal(verified.code, 0);
    const context = JSON.parse(verified.stdout);
    validateResponse(context);
    assert.equal(context.data.verification.authentication, 'authenticated');
    assert.equal(context.data.verification.project.id, f.contract.fixture.projectId);
    assert.equal(context.data.verification.policy, 'verified');
    assert.match(context.data.verification.identityFingerprint, /^sha256:[a-f0-9]{32}$/);
    const offline = JSON.parse((await f.wrapper(['doctor', '--offline', '--json'])).stdout);
    validateResponse(offline);
    assert.equal(offline.meta.counts.childProcesses, 1);
    assert.equal(offline.data.authentication.state, 'not_checked');
    const diagnosed = await f.wrapper(['doctor', '--job', f.contract.fixture.jobId, '--json']);
    assert.equal(diagnosed.code, 0);
    const doctor = JSON.parse(diagnosed.stdout);
    validateResponse(doctor);
    assert.equal(doctor.status, 'partial');
    assert.equal(doctor.data.server.buildNumber, '238924');
    assert.equal(doctor.data.liveCertified, false);
    for (const name of ['structuredRunDetail', 'boundedRunPages', 'structuredLogTail'])
      assert.equal(doctor.data.capabilities.find((c) => c.name === name).state, 'available');
    // Broaden only the local client policy, while keeping the server identity restricted.
    // The observed AxiContract parent is _Root: no ID-prefix inference is involved.
    const { readFile, writeFile } = await import('node:fs/promises');
    const { join } = await import('node:path');
    const path = join(f.dir, 'teamcity-axi', 'config.json');
    const config = JSON.parse(await readFile(path, 'utf8'));
    config.servers.sandbox.allowedProjects = ['_Root'];
    await writeFile(path, JSON.stringify(config), { mode: 0o600 });
    const subtree = await f.wrapper(['run', 'view', f.contract.fixture.failedRunId, '--json']);
    assert.equal(subtree.code, 0);
    assert.equal(JSON.parse(subtree.stdout).data.run.id, f.contract.fixture.failedRunId);
    const policyContext = await f.wrapper([
      'context',
      'show',
      '--verify',
      '--project',
      f.contract.fixture.projectId,
      '--json',
    ]);
    assert.equal(policyContext.code, 0);
    assert.equal(JSON.parse(policyContext.stdout).data.verification.policy, 'verified');
  } finally {
    await f.close();
  }
});

test('live independent occurrences retain exact run, selected occurrence, filter and paging identities', async () => {
  const f = await liveFixture();
  try {
    const id = f.contract.fixture.failedRunId;
    for (const format of ['json', 'toon']) {
      const r = await f.wrapper(['run', 'tests', id, '--format', format]);
      assert.equal(r.code, 0);
      const value = format === 'json' ? JSON.parse(r.stdout) : decode(r.stdout);
      validateResponse(value);
      assert.equal(value.data.tests[0].runId, id);
      assert.equal(value.data.tests[0].testId, '517450581327024597');
      assert.equal(value.data.tests[0].result, 'failure');
      assert.equal(value.data.page.total, null);
    }
    const selected = await f.wrapper([
      'run',
      'tests',
      id,
      '--test',
      'build:(id:1),id:2000000000',
      '--full',
      '--json',
    ]);
    assert.equal(selected.code, 0);
    const detail = JSON.parse(selected.stdout);
    validateResponse(detail);
    assert.equal(detail.status, 'ok');
    assert.equal(detail.data.tests[0].durationMs, 25);
    assert.equal(detail.data.page.total, 1);
    const first = JSON.parse(
      (await f.wrapper(['run', 'problems', id, '--limit', '1', '--json'])).stdout,
    );
    validateResponse(first);
    assert.equal(first.status, 'ok');
    assert.equal(first.data.page.hasMore, true);
    first.next.forEach((a) => parse(a.argv.slice(1)));
    const second = JSON.parse((await f.wrapper([...first.next[0].argv.slice(1), '--json'])).stdout);
    assert.notEqual(second.data.problems[0].id, first.data.problems[0].id);
    const problem = JSON.parse(
      (await f.wrapper(['run', 'problems', id, '--problem', first.data.problems[0].id, '--json']))
        .stdout,
    );
    assert.equal(problem.data.problems[0].id, first.data.problems[0].id);
    assert.equal(problem.data.page.totalKind, 'exact');
    const failed = JSON.parse((await f.wrapper(['run', 'tests', id, '--failed', '--json'])).stdout);
    assert.equal(failed.data.tests.length, 1);
    assert.equal(failed.data.tests[0].muted, false);
    const muted = JSON.parse((await f.wrapper(['run', 'tests', id, '--muted', '--json'])).stdout);
    assert.deepEqual(muted.data.tests, []);
    assert.equal(muted.status, 'partial');
    assert.equal(muted.data.page.total, null);
    const foreign = await f.wrapper(['run', 'tests', f.contract.fixture.deniedRunId, '--json']);
    assert.equal(foreign.code, 1);
    assert.equal(JSON.parse(foreign.stdout).error.code, 'PERMISSION_DENIED');
  } finally {
    await f.close();
  }
});

test('live bounded logs cap native overdelivery and failure view accounts independent sources', async () => {
  const f = await liveFixture();
  try {
    const id = f.contract.fixture.failedRunId;
    const r = await f.wrapper(['run', 'log', id, '--tail', '1', '--json']);
    assert.equal(r.code, 0);
    const value = JSON.parse(r.stdout);
    validateResponse(value);
    assert.equal(value.data.messages.length, 1);
    assert.equal(value.data.window.providerReturned, 2);
    assert.equal(value.data.window.omittedProviderMessages, 1);
    assert.equal(value.data.messages[0].runId, id);
    assert.match(value.data.messages[0].timestamp, /Z$/);
    const noMatches = JSON.parse(
      (
        await f.wrapper([
          'run',
          'log',
          id,
          '--tail',
          '1',
          '--contains',
          'synthetic-no-match-canary',
          '--json',
        ])
      ).stdout,
    );
    assert.equal(noMatches.data.messages.length, 0);
    assert.equal(noMatches.data.window.retained, 1);
    const failure = await f.wrapper(['run', 'log', id, '--failed', '--json']);
    assert.equal(failure.code, 0);
    const view = JSON.parse(failure.stdout);
    validateResponse(view);
    assert.equal(view.status, 'partial');
    assert.equal(view.data.problems.length, 3);
    assert.equal(view.data.tests.length, 1);
    assert.equal(view.data.sources.tests.coverage, 'bounded_page');
    assert.equal(view.data.sources.log.coverage, 'tail_window');
    assert.equal(view.meta.counts.childProcesses, 5);
  } finally {
    await f.close();
  }
});

test('live changes preserve root identity, message expansion, files and cursor scope', async () => {
  const f = await liveFixture();
  try {
    const id = f.contract.fixture.vcsRunId;
    const first = JSON.parse(
      (await f.wrapper(['run', 'changes', id, '--limit', '1', '--json'])).stdout,
    );
    validateResponse(first);
    assert.equal(first.status, 'ok');
    assert.equal(first.data.changes[0].vcsRootId, f.contract.fixture.vcsRootId);
    assert.equal(first.data.changes[0].message, 'Synthetic change 3');
    assert.equal(first.data.changes[0].files, undefined);
    assert.equal(first.data.page.hasMore, true);
    first.next.forEach((a) => parse(a.argv.slice(1)));
    const second = JSON.parse(
      (
        await f.wrapper([
          ...first.next.find((a) => a.argv.includes('--cursor')).argv.slice(1),
          '--json',
        ])
      ).stdout,
    );
    assert.notEqual(second.data.changes[0].id, first.data.changes[0].id);
    const full = JSON.parse(
      (
        await f.wrapper([
          ...first.next.find((a) => a.argv.includes('--full')).argv.slice(1),
          '--json',
        ])
      ).stdout,
    );
    assert.ok(full.data.changes[0].message.includes('Contextual fixture evidence only'));
    const files = decode((await f.wrapper(['run', 'changes', id, '--files'])).stdout);
    validateResponse(files);
    assert.equal(files.data.changes.length, 3);
    assert.deepEqual(files.data.changes[0].files, ['fixture.txt']);
    assert.equal(files.status, 'partial');
    const changed = await f.wrapper([
      'run',
      'changes',
      id,
      '--limit',
      '1',
      '--files',
      '--cursor',
      first.data.page.cursor,
      '--json',
    ]);
    assert.equal(changed.code, 2);
    const foreign = await f.wrapper(['run', 'changes', f.contract.fixture.deniedRunId, '--json']);
    assert.equal(foreign.code, 1);
    assert.equal(JSON.parse(foreign.stdout).error.code, 'PERMISSION_DENIED');
  } finally {
    await f.close();
  }
});

test('actual restricted run tree proves immediate direction and scoped leaves with bounded partial expansion', async () => {
  const f = await liveFixture();
  try {
    const rootId = f.contract.fixture.vcsRunId;
    const childId = f.contract.fixture.dependencyRunId;
    const args = ['run', 'tree', rootId];
    const wire = await f.wrapper([...args, '--json']);
    assert.equal(wire.code, 0);
    assert.equal(wire.stderr, '');
    const result = JSON.parse(wire.stdout);
    validateResponse(result);
    assert.equal(result.status, 'ok');
    assert.equal(result.meta.complete, true);
    assert.equal(result.meta.counts.childProcesses, 5);
    assert.equal(result.data.graph.complete, true);
    assert.deepEqual(
      result.data.graph.nodes.map((n) => n.run.id),
      [rootId, childId],
    );
    assert.deepEqual(result.data.graph.edges, [
      { fromRunId: rootId, toRunId: childId, kind: 'snapshot' },
    ]);
    assert.deepEqual(result.data.graph.cycles, []);
    assert.deepEqual(
      result.data.graph.nodes.map((n) => [n.dependencyCount, n.observedDependencies]),
      [
        [1, 1],
        [0, 0],
      ],
    );
    assert.ok(result.data.graph.nodes.every((n) => n.expansion === 'complete'));
    const toon = decode((await f.wrapper(args)).stdout);
    validateResponse(toon);
    assert.deepEqual(toon.data, result.data);
    const zero = JSON.parse((await f.wrapper([...args, '--depth', '0', '--json'])).stdout);
    validateResponse(zero);
    assert.equal(zero.status, 'partial');
    assert.equal(zero.data.graph.nodes[0].expansion, 'depth_limit');
    assert.equal(zero.data.graph.nodes[0].dependencyCount, null);
    assert.equal(zero.meta.counts.childProcesses, 2);
    const capWire = await f.wrapper([
      ...args,
      '--max-nodes',
      '1',
      '--require-complete',
      '--no-hints',
      '--json',
    ]);
    assert.equal(capWire.code, 1);
    const cap = JSON.parse(capWire.stdout);
    validateResponse(cap);
    assert.equal(cap.status, 'partial');
    assert.equal(cap.data.graph.nodes.length, 1);
    assert.equal(cap.data.graph.edges.length, 0);
    assert.equal(cap.data.graph.nodes[0].expansion, 'node_limit');
    assert.equal(cap.data.selection.omittedTargets, 1);
    assert.equal(cap.next, undefined);
    const denied = await f.wrapper(['run', 'tree', f.contract.fixture.deniedRunId, '--json']);
    assert.equal(denied.code, 1);
    assert.equal(JSON.parse(denied.stdout).error.code, 'PERMISSION_DENIED');
  } finally {
    await f.close();
  }
});

test('actual restricted failure reports preserve run-bound source evidence and success short circuit', async () => {
  const f = await liveFixture();
  try {
    const wire = await f.wrapper(['run', 'failure', f.contract.fixture.failedRunId, '--json']);
    assert.equal(wire.code, 0);
    assert.equal(wire.stderr, '');
    const result = JSON.parse(wire.stdout);
    validateResponse(result);
    assert.equal(result.status, 'partial');
    assert.equal(result.meta.complete, false);
    assert.equal(result.data.assessment, 'failure_observed');
    assert.equal(result.data.run.id, f.contract.fixture.failedRunId);
    assert.ok(result.data.graph.complete);
    assert.deepEqual(result.data.selection.diagnosedRunIds, [f.contract.fixture.failedRunId]);
    assert.ok(result.meta.counts.childProcesses <= 24);
    const test = result.data.findings.find((finding) => finding.kind === 'failed_test');
    assert.ok(test);
    assert.equal(test.claim, 'observation');
    assert.equal(test.evidence[0].itemId, 'build:(id:1),id:2000000000');
    assert.equal(test.evidence[0].sourceRef, 'tests:1');
    assert.ok(result.data.sources.find((source) => source.id === 'tests:1').total === null);
    const muted = result.data.sources.find((source) => source.id === 'tests:1:muted');
    assert.ok(muted);
    assert.equal(muted.returned, 0);
    for (const finding of result.data.findings)
      for (const evidence of finding.evidence) {
        parse(evidence.retrieve.argv.slice(1));
        assert.ok(
          result.data.sources.some(
            (source) => source.id === evidence.sourceRef && source.runId === evidence.runId,
          ),
        );
        assert.ok(evidence.retrieve.argv.includes('--project'));
      }
    result.next.forEach((action) => parse(action.argv.slice(1)));
    const toon = decode(
      (await f.wrapper(['run', 'failure', f.contract.fixture.failedRunId])).stdout,
    );
    validateResponse(toon);
    assert.deepEqual(
      toon.data.findings.map((finding) => finding.id),
      result.data.findings.map((finding) => finding.id),
    );
    const successWire = await f.wrapper([
      'run',
      'failure',
      f.contract.fixture.greenRunId,
      '--json',
    ]);
    assert.equal(successWire.code, 0);
    const success = JSON.parse(successWire.stdout);
    validateResponse(success);
    assert.equal(success.status, 'ok');
    assert.equal(success.meta.counts.childProcesses, 2);
    assert.equal(success.data.assessment, 'not_failed');
    assert.deepEqual(success.data.findings, []);
    assert.equal(success.data.graph.nodes[0].expansion, 'not_requested');
    assert.equal(success.data.graph.complete, false);
    const strict = await f.wrapper([
      'run',
      'failure',
      f.contract.fixture.failedRunId,
      '--require-complete',
      '--no-hints',
      '--json',
    ]);
    assert.equal(strict.code, 1);
    assert.equal(JSON.parse(strict.stdout).status, 'partial');
    assert.equal(JSON.parse(strict.stdout).next, undefined);
    const denied = await f.wrapper(['run', 'failure', f.contract.fixture.deniedRunId, '--json']);
    assert.equal(denied.code, 1);
    assert.equal(JSON.parse(denied.stdout).error.code, 'PERMISSION_DENIED');
  } finally {
    await f.close();
  }
});
test('actual restricted job reads retain direct project scope, safe metadata and unknown bounded exhaustion', async () => {
  const f = await liveFixture();
  try {
    const args = ['job', 'list', '--project', f.contract.fixture.projectId, '--limit', '1'];
    const first = await f.wrapper([...args, '--json']);
    assert.equal(first.code, 0);
    const data = JSON.parse(first.stdout);
    validateResponse(data);
    assert.equal(data.data.jobs[0].id, f.contract.fixture.jobId);
    assert.equal(data.data.jobs[0].paused, false);
    assert.equal(data.data.page.hasMore, true);
    assert.equal(data.data.page.total, null);
    assert.equal(data.data.selection.membership, 'direct');
    assert.equal(data.meta.counts.childProcesses, 3);
    for (const hint of data.next) parse(hint.argv.slice(1));
    const next = JSON.parse((await f.wrapper([...data.next[0].argv.slice(1), '--json'])).stdout);
    assert.equal(next.data.jobs[0].id, f.contract.fixture.greenJobId);
    assert.equal(next.data.selection.position, 1);
    const toon = decode((await f.wrapper(args)).stdout);
    assert.deepEqual(toon.data.jobs, data.data.jobs);
    const viewArgs = [
      'job',
      'view',
      f.contract.fixture.jobId,
      '--project',
      f.contract.fixture.projectId,
    ];
    for (const flags of [['--json'], []]) {
      const exact = await f.wrapper([...viewArgs, ...flags]);
      assert.equal(exact.code, 0);
      const value = flags.length ? JSON.parse(exact.stdout) : decode(exact.stdout);
      validateResponse(value);
      assert.deepEqual(value.data.job, data.data.jobs[0]);
      assert.equal(value.meta.counts.childProcesses, 2);
      assert.deepEqual(Object.keys(value.data.job).sort(), ['id', 'name', 'paused', 'projectId']);
    }
    const all = JSON.parse(
      (await f.wrapper(['job', 'list', '--project', f.contract.fixture.projectId, '--json']))
        .stdout,
    );
    assert.deepEqual(
      all.data.jobs.map((j) => j.id),
      [
        'AxiContract_Fail',
        'AxiContract_Green',
        ...f.contract.fixture.queueJobIds,
        'AxiContract_Vcs',
      ],
    );
    assert.equal(all.status, 'partial');
    assert.equal(all.data.page.hasMore, null);
    assert.equal(all.data.page.total, null);
    const strict = await f.wrapper([
      'job',
      'list',
      '--project',
      f.contract.fixture.projectId,
      '--require-complete',
      '--no-hints',
      '--json',
    ]);
    assert.equal(strict.code, 1);
    assert.equal(JSON.parse(strict.stdout).next, undefined);
    const denied = await f.wrapper(['job', 'list', '--project', 'AxiDenied', '--json']);
    assert.equal(denied.code, 1);
    assert.equal(JSON.parse(denied.stdout).error.code, 'PERMISSION_DENIED');
    const mismatch = await f.wrapper([
      'job',
      'view',
      f.contract.fixture.jobId,
      '--project',
      'AxiDenied',
      '--json',
    ]);
    assert.equal(mismatch.code, 1);
    assert.equal(JSON.parse(mismatch.stdout).error.code, 'CONTEXT_MISMATCH');
  } finally {
    await f.close();
  }
});
test('actual restricted queue reads preserve positive queued IDs, scoped continuation and provider wait reason', async () => {
  const f = await liveFixture();
  try {
    const args = ['queue', 'list', '--project', f.contract.fixture.projectId, '--limit', '1'];
    const json = await f.wrapper([...args, '--json']);
    assert.equal(json.code, 0);
    const first = JSON.parse(json.stdout);
    validateResponse(first);
    assert.equal(first.data.items[0].id, f.contract.fixture.queuedRunIds[0]);
    assert.equal(first.data.items[0].jobId, f.contract.fixture.queueJobIds[0]);
    assert.equal(first.data.items[0].state, 'queued');
    assert.equal(first.data.page.hasMore, true);
    assert.equal(first.data.page.total, null);
    assert.equal(
      first.data.items[0].waitReason,
      apiBody(await f.native(f.contract.records['queue-project-positive'].args)).build[0]
        .waitReason,
    );
    assert.match(first.data.items[0].queuedAt, /Z$/);
    assert.equal(first.data.items[0].branch, null);
    const toon = decode((await f.wrapper(args)).stdout);
    assert.deepEqual(toon.data.items, first.data.items);
    for (const hint of first.next) parse(hint.argv.slice(1));
    const next = JSON.parse((await f.wrapper([...first.next[0].argv.slice(1), '--json'])).stdout);
    assert.equal(next.data.items[0].id, f.contract.fixture.queuedRunIds[1]);
    assert.equal(next.data.selection.position, 1);
    const one = await f.wrapper([
      'queue',
      'list',
      '--job',
      f.contract.fixture.queueJobIds[0],
      '--json',
    ]);
    assert.equal(one.code, 0);
    const scoped = JSON.parse(one.stdout);
    validateResponse(scoped);
    assert.deepEqual(
      scoped.data.items.map((i) => i.id),
      [f.contract.fixture.queuedRunIds[0]],
    );
    assert.equal(scoped.context.project, f.contract.fixture.projectId);
    assert.equal(scoped.status, 'partial');
    assert.equal(scoped.data.page.hasMore, null);
    for (const hint of scoped.next) parse(hint.argv.slice(1));
    const detail = await f.wrapper([...scoped.next[0].argv.slice(1), '--json']);
    assert.equal(detail.code, 0);
    assert.equal(JSON.parse(detail.stdout).data.run.state, 'queued');
    const empty = await f.wrapper([
      'queue',
      'list',
      '--job',
      f.contract.fixture.jobId,
      '--require-complete',
      '--no-hints',
      '--json',
    ]);
    assert.equal(empty.code, 1);
    const value = JSON.parse(empty.stdout);
    assert.deepEqual(value.data.items, []);
    assert.equal(value.data.page.total, null);
    assert.equal(value.data.page.hasMore, null);
    assert.equal(value.next, undefined);
    const denied = await f.wrapper(['queue', 'list', '--project', 'AxiDenied', '--json']);
    assert.equal(denied.code, 1);
    assert.equal(JSON.parse(denied.stdout).error.code, 'PERMISSION_DENIED');
  } finally {
    await f.close();
  }
});
