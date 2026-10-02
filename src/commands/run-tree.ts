import { DomainError } from '../domain/errors.js';
import { response } from '../domain/response.js';
import type { Parsed } from '../cli/parser.js';
import type { ExecutionContext } from '../context/resolve.js';
import { ProjectPolicy } from '../context/project-policy.js';
import { graphRun, sameGraphObservation } from '../domain/graph.js';
import { traverseSnapshotGraph } from '../planner/graph.js';
import { openReadSession } from './read-session.js';

export async function readTree(parsed: Parsed, context: ExecutionContext, signal: AbortSignal) {
  const runId = parsed.positional!,
    depth = Number(parsed.flags.depth ?? 4),
    maxNodes = Number(parsed.flags['max-nodes'] ?? 30);
  const session = await openReadSession(context, signal, 'graph');

  try {
    const primaryBudget = { deadline: context.deadline },
      read = await session.reader.getRun({ id: runId }, primaryBudget);

    if (read.state === 'unavailable') throw read.error;
    if (parsed.flags.job !== undefined && read.value.jobId !== parsed.flags.job)
      throw new DomainError('CONTEXT_MISMATCH', 'Requested run belongs to another job');
    if (parsed.flags.project !== undefined && read.provenance.projectId !== parsed.flags.project)
      throw new DomainError('CONTEXT_MISMATCH', 'Requested run belongs to another project');
    const reserve = read.value.state !== 'finished' ? 1 : 0;
    const budget = {
      deadline: context.deadline,
      maxChildProcesses: Math.max(0, session.maxChildProcesses - reserve),
    };
    const policy = new ProjectPolicy(
      session.reader,
      context.config?.servers[context.server!]?.allowedProjects,
      budget,
    );

    await policy.assert(read.provenance.projectId);
    const maxGraphReads = Math.max(
      0,
      session.maxChildProcesses - session.transport.childProcesses - reserve,
    );
    const traversal = await traverseSnapshotGraph({
      root: read.value,
      rootProjectId: read.provenance.projectId,
      reader: session.reader,
      policy,
      budget,
      depth,
      maxNodes,
      maxGraphReads,
      canRead: () => session.transport.childProcesses < budget.maxChildProcesses,
    });
    const output = response('run.tree', {});

    output.context = {
      server: context.server!,
      job: read.value.jobId,
      branch: read.value.branch ?? null,
      ...(read.provenance.projectId ? { project: read.provenance.projectId } : {}),
    };
    output.meta.limitations = [...read.provenance.limitations, ...traversal.limitations];
    let primary = read.value;

    if (reserve) {
      const final = await session.reader.getRun({ id: runId }, primaryBudget);

      if (final.state === 'unavailable') {
        if (final.error.code === 'INTERRUPTED') throw final.error;
        const root = traversal.graph.nodes.find((n) => n.run.id === runId)!;

        root.expansion = 'unavailable';
        traversal.graph.complete = false;
        traversal.graph.unexpanded = traversal.graph.nodes.filter(
          (n) => n.expansion !== 'complete',
        ).length;
        output.meta.limitations.push({
          code: final.error.code,
          message: 'Reserved final-state observation is unavailable',
          source: 'run',
          runId,
        });
      } else {
        if (
          final.value.jobId !== primary.jobId ||
          final.provenance.projectId !== read.provenance.projectId
        )
          throw new DomainError(
            'CONTEXT_MISMATCH',
            'Final observation changed the frozen execution scope',
          );
        primary = final.value;
        output.context.branch = primary.branch ?? null;
        output.meta.limitations.push(...final.provenance.limitations);

        if (!sameGraphObservation(primary, read.value)) {
          const root = traversal.graph.nodes.find((n) => n.run.id === runId)!;

          root.run = graphRun(primary);
          root.expansion = 'unavailable';
          traversal.graph.complete = false;
          traversal.graph.unexpanded = traversal.graph.nodes.filter(
            (n) => n.expansion !== 'complete',
          ).length;
          output.meta.limitations.push({
            code: 'ROOT_STATE_CHANGED',
            message:
              'Root lifecycle, result or scoped metadata changed while graph evidence was acquired',
            source: 'run',
            runId,
          });
        }

        if (primary.state !== 'finished')
          output.meta.limitations.push({
            code: 'PROVISIONAL_GRAPH',
            message: 'The execution remains non-terminal; observed topology can change',
            source: 'dependencies',
            runId,
          });
      }
    }

    if (session.nativeVersion !== '1.5.0')
      output.meta.limitations.push({
        code: 'UNVERIFIED_VERSION',
        message: 'Native version has not been release-certified',
        source: 'context',
      });
    output.data = {
      run: graphRun(primary),
      graph: traversal.graph,
      selection: {
        depth,
        maxNodes,
        maxGraphReads,
        graphReadAttempts: traversal.graphReadAttempts,
        omittedTargets: traversal.omittedTargets,
        consistency: 'best_effort_counts_and_pages',
      },
    };
    output.meta.counts = { childProcesses: session.transport.childProcesses };
    output.meta.limits = {
      maxBytes: Math.min(
        Number(parsed.flags['max-bytes'] ?? 24576),
        context.config?.limits?.maxBytes ?? 262144,
      ),
      maxChildProcesses: session.maxChildProcesses,
      concurrency: Math.min(3, context.config?.limits?.concurrency ?? 3),
    };

    if (
      !traversal.graph.complete ||
      output.meta.limitations.some(
        (l) =>
          ![
            'UNVERIFIED_VERSION',
            'UNKNOWN_LIFECYCLE',
            'UNKNOWN_RESULT',
            'GRAPH_CYCLE_DETECTED',
          ].includes(l.code),
      )
    ) {
      output.status = 'partial';
      output.meta.complete = false;
    }

    if (!parsed.flags['no-hints']) {
      const target = traversal.graph.nodes.find(
        (n) => n.run.id !== runId && n.expansion !== 'complete',
      );

      if (target)
        output.next = [
          {
            reason: 'Inspect a retained unexpanded execution',
            argv: [
              'teamcity-axi',
              'run',
              'tree',
              target.run.id,
              '--server',
              context.server!,
              '--job',
              target.run.jobId,
              ...(traversal.projects.get(target.run.id)
                ? ['--project', traversal.projects.get(target.run.id)!]
                : []),
              '--depth',
              String(depth),
              '--max-nodes',
              String(maxNodes),
            ],
          },
        ];
    }

    if (parsed.flags['require-complete'] && output.status === 'partial') process.exitCode = 1;

    return output;
  } finally {
    await session.transport.dispose();
  }
}
