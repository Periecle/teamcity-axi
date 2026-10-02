import { spawn } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { decode } from '@toon-format/toon';
import { validateResponse } from '../dist/output/schema.js';

import { treeServer } from '../tests/fixtures/tree-server.mjs';
import {
  evidenceIdentityIssues,
  median,
  outputMetrics,
  scoreEvidence,
  tokenizerIdentity,
} from './evaluation-metrics.mjs';

const repository = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const detailFields =
  'id,buildTypeId,state,status,failedToStart,canceledInfo(timestamp),branchName,personal,composite,buildType(id,projectId),revisions(revision(version,vcs-root-instance(id,vcs-root-id)))';
const problemFields = 'id,type,identity,details,build(id)';
const testFields = 'id,name,status,muted,ignored,details,build(id),test(id)';

function execute(executable, argv, env, cwd) {
  return new Promise((resolveCall, reject) => {
    const start = performance.now();
    const child = spawn(executable, argv, {
      env,
      cwd,
      detached: true,
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    const stdout = [],
      stderr = [];
    let bytes = 0,
      failure;

    const stop = (reason) => {
      failure ??= reason;

      try {
        process.kill(-child.pid, 'SIGKILL');
      } catch {
        /* The child already exited. */
      }
    };
    const timer = setTimeout(() => stop('Evaluation process deadline exceeded'), 20000);

    for (const [stream, chunks] of [
      [child.stdout, stdout],
      [child.stderr, stderr],
    ]) {
      stream.on('data', (chunk) => {
        bytes += chunk.length;

        if (bytes > 2097152) stop('Evaluation capture limit exceeded');
        else chunks.push(chunk);
      });
    }

    child.on('error', (error) => {
      clearTimeout(timer);
      reject(error);
    });
    child.on('close', (code, signal) => {
      clearTimeout(timer);

      if (failure) return reject(new Error(failure));

      resolveCall({
        argv,
        code,
        signal,
        stdout: Buffer.concat(stdout).toString(),
        stderr: Buffer.concat(stderr).toString(),
        wallTimeMs: performance.now() - start,
      });
    });
  });
}

const api = (path) => [
  'api',
  path,
  '-X',
  'GET',
  '--raw',
  '-H',
  'Accept: application/json',
  '--no-input',
];
const pagePath = (resource, locator, fields) =>
  `/app/rest/${resource}?${new URLSearchParams({ locator, fields })}`;

function hasCycle(edges) {
  const active = new Set(),
    complete = new Set();
  const visit = (id) => {
    if (active.has(id)) return true;
    if (complete.has(id)) return false;

    active.add(id);

    for (const edge of edges.filter((value) => value.startsWith(`${id}>`))) {
      if (visit(edge.split('>')[1])) return true;
    }

    active.delete(id);
    complete.add(id);

    return false;
  };

  return edges.some((edge) => visit(edge.split('>')[0]));
}

function wrapperEvidence(document) {
  const data = document.data;
  const findings = data?.findings ?? [];
  const sources = data?.sources ?? [];
  const nodes = data?.graph?.nodes ?? [];
  const identityIssues = evidenceIdentityIssues(findings, sources);

  return {
    runId: data?.run?.id,
    state: data?.run?.state,
    jobId: data?.run?.jobId,
    projectId: document.context?.project,
    nodeJobs: Object.fromEntries(nodes.map((node) => [node.run.id, node.run.jobId])),
    identityIssues,
    complete: document.meta.complete,
    result: data?.run?.result,
    nodeIds: data?.graph?.nodes.map((node) => node.run.id) ?? [],
    edges: data?.graph?.edges.map((edge) => `${edge.fromRunId}>${edge.toRunId}`) ?? [],
    cycle: (data?.graph?.cycles.length ?? 0) > 0,
    testIds: findings.flatMap((finding) =>
      finding.evidence.filter((e) => e.kind === 'test').map((e) => e.itemId),
    ),
    problemRunIds: findings
      .filter((finding) => finding.kind === 'build_problem')
      .map((finding) => finding.runId),
    unavailableSources: sources
      .filter((source) => source.state === 'unavailable')
      .map((source) => `${source.kind}:${source.runId}`),
    emptySources: sources
      .filter((source) => source.state === 'complete' && source.returned === 0)
      .map((source) => `${source.kind}:${source.runId}`),
    boundaryRunIds:
      data?.graph?.nodes
        .filter((node) => node.expansion === 'depth_limit')
        .map((node) => node.run.id) ?? [],
    findings: findings.length,
  };
}

async function selectedNative(task, call) {
  const root = await call(
    'run',
    '482193',
    api(`/app/rest/builds/id:482193?fields=${detailFields}`),
  );
  const nodes = new Map([['482193', root?.body]]),
    edges = [],
    boundaryRunIds = [];
  let frontier = ['482193'];
  const result =
    root?.body?.status === 'SUCCESS'
      ? 'success'
      : root?.body?.status === 'FAILURE'
        ? 'failure'
        : 'unknown';

  // This is an optimized scripted protocol: independent reads are batched three
  // at a time. Protocol rounds are not claimed to be observed agent tool turns.
  const batch = async (requests) => {
    for (let offset = 0; offset < requests.length; offset += 3) {
      await Promise.all(requests.slice(offset, offset + 3).map((request) => request()));
    }
  };

  if (result === 'failure') {
    for (let depth = 0; frontier.length && depth <= task.depth; depth++) {
      if (depth === task.depth) {
        boundaryRunIds.push(...frontier);
        break;
      }

      const next = [];

      await batch(
        frontier.map((id) => async () => {
          const count = await call(
            'dependency-count',
            id,
            api(`/app/rest/builds/id:${id}?fields=id,snapshot-dependencies(count)`),
          );

          if (!count?.body?.['snapshot-dependencies']?.count) return;

          const page = await call(
            'dependencies',
            id,
            api(
              pagePath(
                'builds',
                `snapshotDependency:(to:(id:${id}),recursive:false),defaultFilter:false,count:100,start:0,lookupLimit:5000`,
                `count,nextHref,build(${detailFields})`,
              ),
            ),
          );

          for (const node of page?.body?.build ?? []) {
            const childId = String(node.id);

            edges.push(`${id}>${childId}`);

            if (!nodes.has(childId)) {
              nodes.set(childId, node);
              next.push(childId);
            }
          }
        }),
      );
      frontier = next.sort((a, b) => Number(b) - Number(a));
    }

    const targets = [...nodes.keys()].slice(0, task.maxDiagnosedRuns);

    await batch(
      targets.flatMap((id) => [
        () =>
          call(
            'problems',
            id,
            api(
              pagePath(
                'problemOccurrences',
                `build:(id:${id}),count:20,start:0,lookupLimit:5000`,
                `count,nextHref,problemOccurrence(${problemFields})`,
              ),
            ),
          ),
        ...[false, true].map(
          (muted) => () =>
            call(
              'tests',
              id,
              api(
                pagePath(
                  'testOccurrences',
                  `build:(id:${id}),status:FAILURE,muted:${muted},count:20,start:0,lookupLimit:5000`,
                  `count,nextHref,testOccurrence(${testFields})`,
                ),
              ),
            ),
        ),
      ]),
    );
    await call(
      'changes',
      '482193',
      api(
        pagePath(
          'changes',
          'build:(id:482193),count:10,start:0,lookupLimit:5000',
          'count,nextHref,change(id,version,comment,date,vcsRootInstance(vcs-root-id))',
        ),
      ),
    );

    // The missing-source task requires a log check; normal tasks have direct
    // problem/test evidence and need no redundant full-log or tail read.
    if (task.mode === 'logs-needed')
      await call('log', '482193', ['run', 'log', '482193', '--tail', '80', '--json', '--no-input']);
  }

  return { nodes, edges, boundaryRunIds, result };
}

function nativeEvidence(calls, graph) {
  const unavailable = calls.filter((call) => call.code !== 0);
  const tests = calls
    .filter((call) => call.kind === 'tests')
    .flatMap((call) => call.body?.testOccurrence ?? []);
  const problems = calls.filter(
    (call) => call.kind === 'problems' && (call.body?.problemOccurrence?.length ?? 0) > 0,
  );

  return {
    runId: String(graph.nodes.get('482193')?.id),
    state: graph.nodes.get('482193')?.state,
    jobId: graph.nodes.get('482193')?.buildTypeId,
    projectId: graph.nodes.get('482193')?.buildType?.projectId,
    nodeJobs: Object.fromEntries([...graph.nodes].map(([id, node]) => [id, node?.buildTypeId])),
    identityIssues: calls.flatMap((call) => {
      const rows = call.body?.testOccurrence ?? call.body?.problemOccurrence ?? [];

      return rows
        .filter(
          (row) =>
            String(row.build?.id) !== call.runId || !row.id.startsWith(`build:(id:${call.runId}),`),
        )
        .map(() => 'inconsistent_occurrence_identity');
    }),
    result: graph.result,
    nodeIds: [...graph.nodes.keys()],
    edges: graph.edges,
    cycle: hasCycle(graph.edges),
    testIds: tests.map((test) => test.id),
    problemRunIds: problems.map((call) => call.runId),
    unavailableSources: unavailable.map((call) => `${call.kind}:${call.runId}`),
    emptySources: calls
      .filter((call) => call.code === 0 && call.body?.count === 0)
      .map((call) => `${call.kind}:${call.runId}`),
    boundaryRunIds: graph.boundaryRunIds,
    findings: tests.length + problems.length,
  };
}

export async function evaluate({
  binary = process.env.TEAMCITY_AXI_TEST_BINARY,
  repetitions = 3,
} = {}) {
  if (!binary)
    throw new Error('TEAMCITY_AXI_TEST_BINARY must name the pinned native executable; no skips');
  if (!Number.isSafeInteger(repetitions) || repetitions < 1 || repetitions > 10)
    throw new Error('Repetitions must be an integer from 1 to 10');

  const manifest = JSON.parse(await readFile(join(repository, 'docs/compatibility.json'), 'utf8'));
  const sha256 = createHash('sha256')
    .update(await readFile(binary))
    .digest('hex');
  const artifact = manifest.artifacts.find(
    (value) => value.binarySha256 === sha256 && value.executionTested,
  );

  if (!artifact)
    throw new Error('Evaluation requires the checksum-verified, execution-tested native artifact');

  const corpusBytes = await readFile(join(repository, 'evaluations/corpus.json'));
  const corpus = JSON.parse(corpusBytes);
  const canary = `evaluation-secret-${randomBytes(24).toString('hex')}`;
  const directory = await mkdtemp(join(tmpdir(), 'axi-evaluation-'));
  const server = await treeServer({ secretCanary: canary, respectFields: true });
  const env = {
    PATH: process.env.PATH,
    HOME: directory,
    XDG_CONFIG_HOME: directory,
    TEAMCITY_URL: server.base,
    TEAMCITY_TOKEN: canary,
    TEAMCITY_RO: '1',
    TEAMCITY_NO_UPDATE: '1',
    DO_NOT_TRACK: '1',
    NO_COLOR: '1',
    TERM: 'dumb',
  };
  const observations = [];

  try {
    await mkdir(join(directory, 'teamcity-axi'));
    await writeFile(
      join(directory, 'teamcity-axi/config.json'),
      JSON.stringify({
        schemaVersion: '1.0',
        readOnly: true,
        defaultServer: 'work',
        binaryPath: resolve(binary),
        servers: {
          work: { url: server.base, allowHttpLoopback: true, allowedProjects: ['Payments'] },
        },
      }),
      { mode: 0o600 },
    );

    const version = await execute(resolve(binary), ['--version'], env, directory);

    if (version.code !== 0 || version.stdout !== `teamcity version ${manifest.nativeVersion}\n`)
      throw new Error('Native version mismatch');

    const startupSamples = [];

    for (let sample = 0; sample < 9; sample++) {
      const commands = sample % 2 ? ['wrapper', 'bare'] : ['bare', 'wrapper'];
      const durations = {};

      for (const kind of commands) {
        const result = await execute(
          process.execPath,
          kind === 'bare'
            ? ['--eval', '']
            : [join(repository, 'bin/teamcity-axi.mjs'), '--version'],
          env,
          directory,
        );

        if (result.code !== 0 || result.signal || result.stderr)
          throw new Error('Startup probe failed');

        durations[kind] = result.wallTimeMs;
      }

      startupSamples.push({
        bareNodeMs: durations.bare,
        wrapperVersionMs: durations.wrapper,
        addedMs: durations.wrapper - durations.bare,
      });
    }

    if (server.requests.length !== 0)
      throw new Error('Version probes performed unexpected HTTP requests');

    for (let repetition = 0; repetition < repetitions; repetition++) {
      for (const task of corpus.tasks) {
        // Alternate condition order to reduce systematic warm-cache advantage.
        const conditions = [
          'wrapper-toon',
          'wrapper-json',
          'native-selected-json',
          'native-failure-diagnostics',
        ];

        if (repetition % 2) conditions.reverse();

        for (const condition of conditions) {
          server.setMode(task.mode);
          const calls = [],
            start = performance.now();
          let evidence, nativeSubprocessCount, byteBudget;
          const call = async (kind, runId, argv) => {
            const result = {
              ...(await execute(resolve(binary), argv, env, directory)),
              kind,
              runId,
            };

            try {
              result.body = JSON.parse(result.stdout);
            } catch {
              /* Native failures may have no JSON body. */
            }
            calls.push(result);

            return result;
          };

          if (condition.startsWith('wrapper')) {
            const argv = [
              'run',
              'failure',
              '482193',
              '--server=work',
              '--job=Payments_Build',
              '--project=Payments',
              '--depth',
              String(task.depth),
              '--max-diagnosed-runs',
              String(task.maxDiagnosedRuns),
            ];

            if (condition === 'wrapper-json') argv.push('--json');

            const result = await execute(
              process.execPath,
              [join(repository, 'bin/teamcity-axi.mjs'), ...argv],
              env,
              directory,
            );
            const document =
              condition === 'wrapper-json' ? JSON.parse(result.stdout) : decode(result.stdout);

            validateResponse(document);

            if (result.code !== 0 || result.signal)
              throw new Error(
                `Wrapper observation failed for ${task.id}: ${document.error?.code ?? 'unexpected process exit'}`,
              );

            calls.push(result);
            evidence = wrapperEvidence(document);
            nativeSubprocessCount = document.meta.counts?.childProcesses ?? null;
            byteBudget = document.meta.limits?.maxBytes ?? 24576;
          } else if (condition === 'native-selected-json') {
            evidence = nativeEvidence(calls, await selectedNative(task, call));
            nativeSubprocessCount = calls.length;
          } else {
            const result = await call('diagnostics', '482193', [
              'run',
              'log',
              '482193',
              '--failed',
              '--json',
              '--no-input',
            ]);
            const body = result.body;

            evidence = {
              runId: body?.run_id,
              result:
                body?.status === 'SUCCESS'
                  ? 'success'
                  : body?.status === 'FAILURE'
                    ? 'failure'
                    : 'unknown',
              nodeIds: [body?.run_id],
              testIds: body?.failed_tests?.testOccurrence?.map((test) => test.id) ?? [],
              problemRunIds: body?.problems?.length ? ['482193'] : [],
              // An omitted summary source has unknown availability; do not
              // translate that omission into either denied or exact zero.
              unavailableSources: [],
              emptySources: [],
              findings:
                (body?.problems?.length ?? 0) + (body?.failed_tests?.testOccurrence?.length ?? 0),
            };
            nativeSubprocessCount = calls.length;
          }

          const wallTimeMs = performance.now() - start;
          const metrics = outputMetrics(calls, canary);

          observations.push({
            taskId: task.id,
            repetition,
            condition,
            wallTimeMs,
            ...metrics,
            nativeSubprocessCount,
            httpRequestCount: server.requests.length,
            agentFacingToolTurns: null,
            byteBudget: byteBudget ?? null,
            withinByteBudget: byteBudget ? Buffer.byteLength(calls[0].stdout) <= byteBudget : null,
            score: scoreEvidence(task.expected, evidence, metrics.secretExposures),
            evidence,
            calls: calls.map(({ body, ...result }) => result),
          });

          if (server.requests.some((request) => request.method !== 'GET'))
            throw new Error('Evaluation performed an unexpected non-read request');
        }
      }
    }

    const report = {
      schemaVersion: '1.0',
      corpusId: corpus.id,
      corpusSha256: createHash('sha256').update(corpusBytes).digest('hex'),
      kind: 'scripted-evidence-benchmark',
      agentEvaluation: 'not-performed',
      environment: {
        platform: process.platform,
        architecture: process.arch,
        nodeVersion: process.versions.node,
        nativeVersion: manifest.nativeVersion,
        nativeSha256: sha256,
      },
      tokenizer: tokenizerIdentity,
      startup: {
        samples: startupSamples,
        medianBareNodeMs: median(startupSamples.map((sample) => sample.bareNodeMs)),
        medianWrapperVersionMs: median(startupSamples.map((sample) => sample.wrapperVersionMs)),
        medianAddedMs: median(startupSamples.map((sample) => sample.addedMs)),
        httpRequestCount: 0,
      },
      repetitions,
      conditions: [
        'wrapper-toon',
        'wrapper-json',
        'native-selected-json',
        'native-failure-diagnostics',
      ].map((condition) => {
        const rows = observations.filter((row) => row.condition === condition);

        return {
          condition,
          observations: rows.length,
          evidenceRetained: rows.filter((row) => row.score.evidenceRetained).length,
          medianOutputTokens: median(rows.map((row) => row.outputTokens)),
          medianWallTimeMs: median(rows.map((row) => row.wallTimeMs)),
          medianNativeSubprocessCount: median(rows.map((row) => row.nativeSubprocessCount)),
          secretExposures: rows.reduce((n, row) => n + row.secretExposures, 0),
          identityMistakes: rows.reduce((n, row) => n + row.score.identityMistakes, 0),
          completenessMistakes: rows.reduce((n, row) => n + row.score.completenessMistakes, 0),
          agentTaskSuccess: null,
          medianAgentFacingToolTurns: null,
          unjustifiedCausalClaims: null,
        };
      }),
      observations,
    };
    const sanitized = JSON.stringify(report)
      .replaceAll(canary, '<secret-canary>')
      .replaceAll(server.base, 'http://127.0.0.1:PORT/teamcity')
      .replaceAll(directory, '<isolated-config>');
    const portable = sanitized.replaceAll(repository, '<repository>');

    return JSON.parse(portable);
  } finally {
    await server.close();
    await rm(directory, { recursive: true, force: true });
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const report = await evaluate({
    repetitions: Number(process.env.TEAMCITY_AXI_EVAL_REPETITIONS ?? 3),
  });
  const destination = resolve(
    process.env.TEAMCITY_AXI_EVAL_OUTPUT ?? join(repository, 'test-results/evaluation.json'),
  );

  await mkdir(dirname(destination), { recursive: true });
  await writeFile(destination, JSON.stringify(report, null, 2) + '\n');
  console.log(
    JSON.stringify(
      { report: destination, kind: report.kind, conditions: report.conditions },
      null,
      2,
    ),
  );

  if (
    report.observations.some(
      (row) =>
        row.condition.startsWith('wrapper') &&
        (!row.score.evidenceRetained || row.secretExposures || !row.withinByteBudget),
    )
  )
    process.exitCode = 1;
}
