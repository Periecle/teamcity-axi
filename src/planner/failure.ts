import { createHash } from 'node:crypto';

import { asDomainError, DomainError } from '../domain/errors.js';
import type {
  Evidence,
  Finding,
  Investigation,
  SourceCoverage,
  SourceKind,
} from '../domain/failure.js';
import { graphRun, sameGraphObservation } from '../domain/graph.js';
import type { Graph, GraphNode } from '../domain/graph.js';
import type {
  Budget,
  Change,
  EvidencePage,
  Problem,
  ReadResult,
  Run,
  TeamCityReader,
} from '../domain/teamcity.js';
import { sanitizeText } from '../output/sanitize.js';
import { traverseSnapshotGraph } from './graph.js';

interface Options {
  primary: Extract<ReadResult<Run>, { state: 'available' }>;
  reader: TeamCityReader;
  policy: { assert(projectId: string | null, budget?: Budget): Promise<void> };
  budget: Budget;
  maxChildProcesses: number;
  childProcesses: () => number;
  server: string;
  depth: number;
  maxNodes: number;
  maxDiagnosedRuns: number;
  full: boolean;
  secrets: readonly string[];
}

const failed = (run: Run) =>
  ['failure', 'error', 'canceled', 'failed_to_start'].includes(run.result);

export function selectDiagnosedRuns(
  graph: Graph,
  depths: ReadonlyMap<string, number>,
  cap: number,
  referenced = new Set<string>(),
) {
  const root = graph.nodes.find((node) => node.run.id === graph.rootRunId)!;
  const candidates = graph.nodes.filter(
    (node) =>
      node !== root &&
      (failed(node.run) || node.run.state === 'unknown' || node.run.result === 'unknown'),
  );
  const priority = (node: GraphNode) =>
    referenced.has(node.run.id) ? 0 : failed(node.run) ? 1 : 2;

  candidates.sort(
    (a, b) =>
      priority(a) - priority(b) ||
      (depths.get(a.run.id) ?? 0) - (depths.get(b.run.id) ?? 0) ||
      Number(a.run.id) - Number(b.run.id),
  );

  return {
    selected: [root, ...candidates.slice(0, cap - 1)],
    omitted: Math.max(0, candidates.length - cap + 1),
  };
}

export async function investigateFailure(options: Options): Promise<Investigation> {
  if (
    !Number.isInteger(options.maxDiagnosedRuns) ||
    options.maxDiagnosedRuns < 1 ||
    options.maxDiagnosedRuns > 10
  )
    throw new DomainError('USAGE_ERROR', 'Invalid diagnosis bound', 2);
  const initial = options.primary.value;
  const rootId = initial.id;
  const reserve = initial.state !== 'finished' ? 1 : 0;
  const budget: Budget = {
    ...options.budget,
    maxChildProcesses: Math.max(0, options.maxChildProcesses - reserve),
  };
  const sources: SourceCoverage[] = [];
  const findings: Finding[] = [];
  const limitations = [...options.primary.provenance.limitations];
  let truncated = false;
  let hardTextLimit = false;
  let finalFailed = false;
  let changed = false;
  let primary = initial;
  let optionalChanges: Change[] | undefined;

  function coverage(
    runId: string,
    kind: SourceKind,
    required: boolean,
    suffix = '',
  ): SourceCoverage {
    const source: SourceCoverage = {
      id: `${kind}:${runId}${suffix}`,
      runId,
      kind,
      required,
      scope:
        kind === 'log'
          ? 'tail_window'
          : kind === 'run'
            ? 'execution'
            : kind === 'dependencies'
              ? 'immediate_dependencies'
              : 'bounded_page',
      state: 'not_requested',
      returned: null,
      total: null,
      reasonCode: 'NOT_REQUESTED',
    };

    sources.push(source);

    return source;
  }

  const primarySource = coverage(rootId, 'run', true);

  Object.assign(primarySource, {
    state: 'complete',
    returned: 1,
    total: 1,
    observedAt: options.primary.provenance.observedAt,
  });
  delete primarySource.reasonCode;

  function scopedArgs(run: Run, project: string | null | undefined) {
    return [
      '--server',
      options.server,
      '--job',
      run.jobId,
      ...(project ? ['--project', project] : []),
    ];
  }

  function excerpt(value: string, kind: SourceKind, runId: string) {
    const text = Array.from(sanitizeText(value, options.secrets));
    const limit = options.full
      ? 8192
      : kind === 'problems'
        ? 1200
        : kind === 'changes'
          ? 200
          : 2000;

    if (text.length > limit) {
      truncated = true;

      if (options.full) {
        hardTextLimit = true;
        limitations.push({
          code: 'TEXT_HARD_LIMIT',
          message: 'Evidence excerpt reached its bounded full-text ceiling',
          source: kind,
          runId,
        });
      }
    }

    return text.slice(0, limit).join('');
  }

  function finding(
    run: Run,
    source: SourceCoverage,
    kind: Finding['kind'],
    itemId: string,
    summary: string,
    text: string,
    argv: string[],
  ) {
    const safeText = sanitizeText(text, options.secrets);
    const hash = createHash('sha256')
      .update(
        JSON.stringify(
          [run.id, run.jobId, kind, source.id, itemId, safeText.replace(/\s+/g, ' ').trim()].map(
            (value) => sanitizeText(value, options.secrets),
          ),
        ),
      )
      .digest('hex');
    const reference: Evidence = {
      id: `ev:${run.id}:${hash}`,
      sourceRef: source.id,
      runId: run.id,
      kind:
        kind === 'build_problem'
          ? 'problem'
          : kind === 'failed_test'
            ? 'test'
            : kind === 'log_signal'
              ? 'log'
              : 'run',
      itemId,
      excerpt: excerpt(safeText, source.kind, run.id),
      observedAt: source.observedAt!,
      retrieve: {
        reason:
          kind === 'log_signal'
            ? 'Reinspect the declared source tail window'
            : 'Read the exact supporting evidence',
        argv,
      },
    };

    const safeSummary = Array.from(sanitizeText(summary, options.secrets));

    if (safeSummary.length > 1200) truncated = true;
    findings.push({
      id: `finding:${run.id}:${hash}`,
      runId: run.id,
      kind,
      claim: 'observation',
      summary: safeSummary.slice(0, 1200).join(''),
      evidence: [reference],
    });
  }

  async function collect<T>(
    source: SourceCoverage,
    operation: () => Promise<ReadResult<T>>,
  ): Promise<Extract<ReadResult<T>, { state: 'available' }> | undefined> {
    try {
      if (Date.now() >= budget.deadline)
        throw new DomainError('DEADLINE_EXCEEDED', 'Shared investigation deadline exhausted');
      if (options.childProcesses() >= budget.maxChildProcesses!)
        throw new DomainError('CALL_LIMIT_EXCEEDED', 'Reserved evidence capacity exhausted');
      const read = await operation();

      if (read.state === 'unavailable') throw read.error;
      source.observedAt = read.provenance.observedAt;
      source.state = 'complete';
      delete source.reasonCode;

      return read;
    } catch (error) {
      const domain = asDomainError(error);

      if (domain.code === 'INTERRUPTED') throw domain;
      source.state = ['CALL_LIMIT_EXCEEDED', 'DEADLINE_EXCEEDED'].includes(domain.code)
        ? 'budget_exhausted'
        : 'unavailable';
      source.reasonCode = domain.code;
      source.observedAt = new Date().toISOString();
      if (source.required)
        limitations.push({
          code: domain.code,
          message: 'A planned independent evidence source is unavailable',
          source: source.kind,
          runId: source.runId,
        });

      return undefined;
    }
  }

  function page<T>(
    source: SourceCoverage,
    read: Extract<ReadResult<EvidencePage<T>>, { state: 'available' }> | undefined,
  ) {
    if (!read) return;
    source.returned = read.value.items.length;
    source.providerReturned = read.value.providerReturned;
    source.total = null;
    source.state =
      read.value.hasMore === false && read.value.limitations.length === 0 ? 'complete' : 'partial';
    if (source.state === 'partial')
      source.reasonCode = read.value.limitations[0]?.code ?? 'BOUNDED_PAGE';
    limitations.push(
      ...read.value.limitations
        .filter((l) => source.required || l.code !== 'SCAN_COVERAGE_UNKNOWN')
        .map((l) => ({ ...l, source: source.kind, runId: source.runId })),
    );
  }

  const notFailed = initial.state === 'finished' && initial.result === 'success';
  const rootProblemsSource = coverage(rootId, 'problems', !notFailed);
  const rootTestsSource = coverage(rootId, 'tests', !notFailed);
  const rootLogSource = coverage(rootId, 'log', false);
  const changesSource = coverage(rootId, 'changes', false);
  let graph: Graph;
  let graphReadAttempts = 0;
  let maxGraphReads = 0;
  let omittedGraphTargets = 0;
  let omittedDiagnosedRuns = 0;
  let diagnosedRunIds: string[] = [];
  let rootProblems: Extract<ReadResult<EvidencePage<Problem>>, { state: 'available' }> | undefined;
  const projects = new Map<string, string | null>([[rootId, options.primary.provenance.projectId]]);

  if (notFailed) {
    graph = {
      rootRunId: rootId,
      complete: false,
      nodes: [
        {
          run: graphRun(initial),
          expansion: 'not_requested',
          dependencyCount: null,
          observedDependencies: null,
        },
      ],
      edges: [],
      cycles: [],
      unexpanded: 1,
    };
    coverage(rootId, 'dependencies', false);
  } else {
    rootProblems = await collect(rootProblemsSource, () =>
      options.reader.listProblems({ runId: rootId, count: 20, start: 0, scanLimit: 5000 }, budget),
    );
    page(rootProblemsSource, rootProblems);
    const graphCeiling = Math.max(0, budget.maxChildProcesses! - 2);

    maxGraphReads = Math.min(10, Math.max(0, graphCeiling - options.childProcesses()));
    const traversal = await traverseSnapshotGraph({
      root: initial,
      rootProjectId: options.primary.provenance.projectId,
      reader: options.reader,
      policy: options.policy,
      budget: { ...budget, maxChildProcesses: graphCeiling },
      depth: options.depth,
      maxNodes: options.maxNodes,
      maxGraphReads,
      canRead: () => options.childProcesses() < graphCeiling,
    });

    graph = traversal.graph;
    graphReadAttempts = traversal.graphReadAttempts;
    omittedGraphTargets = traversal.omittedTargets;
    limitations.push(...traversal.limitations);
    for (const [id, project] of traversal.projects) projects.set(id, project);
    const selection = selectDiagnosedRuns(graph, traversal.depths, options.maxDiagnosedRuns);

    omittedDiagnosedRuns = selection.omitted;
    diagnosedRunIds = selection.selected.map((node) => node.run.id);
    if (omittedDiagnosedRuns)
      limitations.push({
        code: 'DIAGNOSIS_LIMIT',
        message: 'Relevant retained executions remain outside the diagnosis bound',
        source: 'dependencies',
      });

    for (const node of graph.nodes) {
      const source = coverage(node.run.id, 'dependencies', true);

      source.returned = node.observedDependencies;
      source.total = node.dependencyCount;
      source.state =
        node.expansion === 'complete'
          ? 'complete'
          : ['call_limit'].includes(node.expansion)
            ? 'budget_exhausted'
            : ['permission_denied', 'unavailable'].includes(node.expansion) &&
                node.observedDependencies === null &&
                node.dependencyCount === null
              ? 'unavailable'
              : 'partial';
      source.observedAt = new Date().toISOString();
      if (source.state === 'complete') delete source.reasonCode;
      else
        source.reasonCode =
          traversal.limitations.find((limitation) => limitation.runId === node.run.id)?.code ??
          `GRAPH_${node.expansion.toUpperCase()}`;

      if (!diagnosedRunIds.includes(node.run.id)) {
        for (const kind of ['problems', 'tests', 'log'] as const) {
          const skipped = coverage(node.run.id, kind, false);

          skipped.reasonCode =
            failed(node.run) || node.run.result === 'unknown'
              ? 'DIAGNOSIS_LIMIT'
              : 'NOT_A_FAILURE_CANDIDATE';
        }
      }
    }

    const diagnosed: Run[] = [];

    for (const node of selection.selected) {
      const run = node.run;

      if (run.id !== rootId) {
        const source = coverage(run.id, 'run', true);

        source.state = 'complete';
        source.returned = 1;
        source.total = 1;
        source.observedAt = traversal.observations.get(run.id)!;
        delete source.reasonCode;
        if (failed(run))
          finding(
            run,
            source,
            'dependency_failure',
            run.id,
            `Snapshot prerequisite execution reports ${run.result}; the edge does not establish causation`,
            `Observed prerequisite execution ${run.id}: lifecycle=${run.state}, result=${run.result}`,
            ['teamcity-axi', 'run', 'view', run.id, ...scopedArgs(run, projects.get(run.id))],
          );
      }

      const problemsSource =
        run.id === rootId ? rootProblemsSource : coverage(run.id, 'problems', true);
      const testsSource = run.id === rootId ? rootTestsSource : coverage(run.id, 'tests', true);
      const logSource = run.id === rootId ? rootLogSource : coverage(run.id, 'log', false);
      const [problems, tests] = await Promise.all([
        run.id === rootId
          ? Promise.resolve(rootProblems)
          : collect(problemsSource, () =>
              options.reader.listProblems(
                { runId: run.id, count: 20, start: 0, scanLimit: 5000 },
                budget,
              ),
            ),
        collect(testsSource, () =>
          options.reader.listTests(
            { runId: run.id, count: 20, start: 0, scanLimit: 5000, failed: true, muted: false },
            budget,
          ),
        ),
      ]);

      if (run.id !== rootId) page(problemsSource, problems);
      page(testsSource, tests);
      const scope = scopedArgs(run, projects.get(run.id));

      for (const problem of problems?.value.items ?? [])
        finding(
          run,
          problemsSource,
          'build_problem',
          problem.id,
          `Server reports build problem ${problem.type}`,
          problem.description || problem.identity || problem.type,
          ['teamcity-axi', 'run', 'problems', run.id, ...scope, '--problem', problem.id, '--full'],
        );

      for (const test of tests?.value.items ?? []) {
        if (test.result === 'failure' && test.muted === false && test.ignored === false)
          finding(
            run,
            testsSource,
            'failed_test',
            test.id,
            `Unmuted test occurrence failed: ${test.name}`,
            test.details || test.name,
            ['teamcity-axi', 'run', 'tests', run.id, ...scope, '--test', test.id, '--full'],
          );
      }

      const hasDirectEvidence =
        (problems?.value.items.some(
          (p) => p.description.trim() && !['TC_TESTS_FAILED', 'TC_EXIT_CODE'].includes(p.type),
        ) ??
          false) ||
        (tests?.value.items.some(
          (t) =>
            t.result === 'failure' &&
            t.muted === false &&
            t.ignored === false &&
            Boolean(t.details?.trim()),
        ) ??
          false);

      if (!hasDirectEvidence) {
        logSource.required = true;
        const log = await collect(logSource, () =>
          options.reader.getLogTail({ id: run.id }, 80, budget),
        );

        if (log) {
          logSource.returned = log.value.messages.length;
          logSource.providerReturned = log.value.providerReturned;
          logSource.window = {
            requested: 80,
            firstMessageId: log.value.messages[0]?.id ?? null,
            lastMessageId: log.value.messages.at(-1)?.id ?? null,
            omittedProviderMessages: log.value.providerReturned - log.value.messages.length,
          };
          logSource.total = null;

          if (log.value.limitations.length) {
            logSource.state = 'partial';
            logSource.reasonCode = log.value.limitations[0]!.code;
          }

          limitations.push(
            ...log.value.limitations.map((l) => ({ ...l, source: 'log', runId: run.id })),
          );
          for (const message of log.value.messages.filter((m) =>
            /connection refused|timeout|exception|\berror\b/i.test(m.text),
          ))
            finding(
              run,
              logSource,
              'log_signal',
              message.id,
              'A failure-related text signal appears in the inspected tail window',
              message.text,
              [
                'teamcity-axi',
                'run',
                'log',
                run.id,
                ...scope,
                '--tail',
                '80',
                `--contains=${Array.from(message.text).slice(0, 80).join('')}`,
                '--full',
              ],
            );
          if (log.value.truncated) truncated = true;
        }
      } else logSource.reasonCode = 'DIRECT_EVIDENCE_AVAILABLE';
      diagnosed.push(run);
    }

    for (const run of diagnosed) {
      const source = coverage(run.id, 'tests', false, ':muted');
      const read = await collect(source, () =>
        options.reader.listTests(
          { runId: run.id, count: 20, start: 0, scanLimit: 5000, failed: true, muted: true },
          budget,
        ),
      );

      page(source, read);

      for (const test of read?.value.items ?? []) {
        if (test.result === 'failure' && test.muted === true && test.ignored === false)
          finding(
            run,
            source,
            'failed_test',
            test.id,
            `Muted test occurrence failed: ${test.name}`,
            test.details || test.name,
            [
              'teamcity-axi',
              'run',
              'tests',
              run.id,
              ...scopedArgs(run, projects.get(run.id)),
              '--test',
              test.id,
              '--full',
            ],
          );
      }
    }

    const changes = await collect(changesSource, () =>
      options.reader.listChanges({ runId: rootId, count: 10, start: 0, scanLimit: 5000 }, budget),
    );

    page(changesSource, changes);

    if (changes) {
      const roots = initial.revisions && new Set(initial.revisions.map((r) => r.vcsRootId));

      if (roots && changes.value.items.some((item) => !roots.has(item.vcsRootId))) {
        changesSource.state = 'unavailable';
        changesSource.returned = null;
        changesSource.reasonCode = 'CONTEXT_MISMATCH';
      } else {
        if (
          !options.full &&
          changes.value.items.some((item) => item.message.split(/\r?\n/, 1)[0] !== item.message)
        )
          truncated = true;
        // Optional context is retained only after all required diagnosed sources.
        optionalChanges = changes.value.items.map((item) => ({
          id: item.id,
          version: item.version,
          vcsRootId: item.vcsRootId,
          timestamp: item.timestamp,
          message: excerpt(
            options.full ? item.message : item.message.split(/\r?\n/, 1)[0]!,
            'changes',
            rootId,
          ),
        }));
      }
    }
  }

  if (reserve) {
    const source = coverage(rootId, 'run', true, ':final');
    let final: ReadResult<Run>;

    try {
      final = await options.reader.getRun({ id: rootId }, options.budget);
    } catch (error) {
      const domain = asDomainError(error);

      if (domain.code === 'INTERRUPTED') throw domain;
      final = { state: 'unavailable', error: domain, provenance: options.primary.provenance };
    }

    source.observedAt = final.provenance.observedAt;

    if (final.state === 'unavailable') {
      if (final.error.code === 'INTERRUPTED') throw final.error;
      source.state = 'unavailable';
      source.reasonCode = final.error.code;
      finalFailed = true;
      limitations.push({
        code: final.error.code,
        message: 'Reserved final root observation is unavailable',
        source: 'run',
        runId: rootId,
      });
    } else {
      if (
        final.value.id !== rootId ||
        final.value.jobId !== initial.jobId ||
        final.provenance.projectId !== options.primary.provenance.projectId
      )
        throw new DomainError(
          'CONTEXT_MISMATCH',
          'Final observation changed the frozen execution scope',
        );
      source.state = 'complete';
      source.returned = 1;
      source.total = 1;
      delete source.reasonCode;
      primary = final.value;
      changed = !sameGraphObservation(initial, primary);
      limitations.push(...final.provenance.limitations);
      if (changed)
        limitations.push({
          code: 'ROOT_STATE_CHANGED',
          message: 'Root observation changed while evidence was acquired',
          source: 'run',
          runId: rootId,
        });
    }

    if (changed || finalFailed) {
      const root = graph.nodes.find((node) => node.run.id === rootId)!;

      root.run = graphRun(primary);
      root.expansion = 'unavailable';
      graph.complete = false;
      graph.unexpanded = graph.nodes.filter((node) => node.expansion !== 'complete').length;
      const graphSource = sources.find((s) => s.id === `dependencies:${rootId}`)!;

      graphSource.state = 'partial';
      graphSource.reasonCode = changed ? 'ROOT_STATE_CHANGED' : 'FINAL_READ_UNAVAILABLE';
    }
  }

  if (!notFailed && primary.state !== 'finished')
    limitations.push({
      code: 'PROVISIONAL_EVIDENCE',
      message: 'Execution remains non-terminal; observations are provisional',
      source: 'run',
      runId: rootId,
    });
  let assessment: Investigation['data']['assessment'] = notFailed
    ? 'not_failed'
    : primary.state === 'queued' || primary.state === 'running'
      ? 'in_progress'
      : primary.state === 'finished' && failed(primary)
        ? 'failure_observed'
        : 'inconclusive';

  if (changed && primary.state === 'finished' && primary.result === 'success')
    assessment = 'inconclusive';
  const complete =
    (notFailed || graph.complete) &&
    !omittedDiagnosedRuns &&
    !finalFailed &&
    !changed &&
    !hardTextLimit &&
    (notFailed || primary.state === 'finished') &&
    sources.every((source) => !source.required || source.state === 'complete') &&
    !limitations.some((l) =>
      ['MISSING_REVISION_METADATA', 'UNKNOWN_LIFECYCLE', 'UNKNOWN_RESULT'].includes(l.code),
    );

  findings.sort(
    (a, b) =>
      diagnosedRunIds.indexOf(a.runId) - diagnosedRunIds.indexOf(b.runId) ||
      a.kind.localeCompare(b.kind) ||
      a.id.localeCompare(b.id),
  );
  if (findings.length > 100)
    limitations.push({
      code: 'FINDING_LIMIT',
      message: 'Additional observed findings exceed the hundred-finding ceiling',
      source: 'output',
    });

  return {
    data: {
      run: graphRun(primary),
      assessment,
      findings: findings.slice(0, 100),
      sources,
      graph,
      ...(optionalChanges ? { changes: optionalChanges } : {}),
      selection: {
        maxDiagnosedRuns: options.maxDiagnosedRuns,
        diagnosedRunIds,
        omittedDiagnosedRuns,
        graphReadAttempts,
        maxGraphReads,
        omittedGraphTargets,
        omittedFindings: Math.max(0, findings.length - 100),
        omittedChanges: 0,
        consistency: 'best_effort',
        dependencyReferences: 'unavailable',
      },
    },
    limitations,
    complete: complete && findings.length <= 100,
    truncated: truncated || findings.length > 100,
    projects,
  };
}
