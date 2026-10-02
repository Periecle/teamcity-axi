import { DomainError } from '../domain/errors.js';
import { response } from '../domain/response.js';
import type { Parsed } from '../cli/parser.js';
import type { ExecutionContext } from '../context/resolve.js';
import { ProjectPolicy } from '../context/project-policy.js';
import { investigateFailure } from '../planner/failure.js';
import { knownSecrets } from '../output/sanitize.js';
import { openReadSession } from './read-session.js';

export async function readFailure(parsed: Parsed, context: ExecutionContext, signal: AbortSignal) {
  const runId = parsed.positional!;
  const session = await openReadSession(context, signal, 'graph');

  try {
    const budget = { deadline: context.deadline };
    const primary = await session.reader.getRun({ id: runId }, budget);

    if (primary.state === 'unavailable') throw primary.error;

    if (parsed.flags.job !== undefined && primary.value.jobId !== parsed.flags.job)
      throw new DomainError('CONTEXT_MISMATCH', 'Requested run belongs to another job');

    if (parsed.flags.project !== undefined && primary.provenance.projectId !== parsed.flags.project)
      throw new DomainError('CONTEXT_MISMATCH', 'Requested run belongs to another project');

    const policy = new ProjectPolicy(
      session.reader,
      context.config?.servers[context.server!]?.allowedProjects,
      {
        ...budget,
        maxChildProcesses: Math.max(
          0,
          session.maxChildProcesses - (primary.value.state !== 'finished' ? 1 : 0),
        ),
      },
    );

    await policy.assert(primary.provenance.projectId);
    const investigation = await investigateFailure({
      primary,
      reader: session.reader,
      policy,
      budget,
      maxChildProcesses: session.maxChildProcesses,
      childProcesses: () => session.transport.childProcesses,
      server: context.server!,
      depth: Number(parsed.flags.depth ?? 4),
      maxNodes: Number(parsed.flags['max-nodes'] ?? 30),
      maxDiagnosedRuns: Number(parsed.flags['max-diagnosed-runs'] ?? 3),
      full: Boolean(parsed.flags.full),
      secrets: knownSecrets(
        process.env,
        context.config?.secretNamePatterns,
        context.config?.servers[context.server!]?.forwardHeaderEnvNames,
      ),
    });
    const output = response('run.failure', { ...investigation.data });

    output.context = {
      server: context.server!,
      job: primary.value.jobId,
      branch: investigation.data.run.branch ?? null,
      ...(primary.provenance.projectId ? { project: primary.provenance.projectId } : {}),
    };
    output.meta.complete = investigation.complete;
    output.meta.truncated = investigation.truncated;
    output.meta.limitations = investigation.limitations;
    output.meta.counts = { childProcesses: session.transport.childProcesses };
    output.meta.limits = {
      maxBytes: Math.min(
        Number(parsed.flags['max-bytes'] ?? 24576),
        context.config?.limits?.maxBytes ?? 262144,
      ),
      maxChildProcesses: session.maxChildProcesses,
      concurrency: Math.min(3, context.config?.limits?.concurrency ?? 3),
    };

    if (!investigation.complete) output.status = 'partial';

    if (session.nativeVersion !== '1.5.0')
      output.meta.limitations.push({
        code: 'UNVERIFIED_VERSION',
        message: 'Native version has not been release-certified',
        source: 'context',
      });

    if (!parsed.flags['no-hints']) {
      const evidence = investigation.data.findings[0]?.evidence[0];
      const unexpanded = investigation.data.graph.nodes.find(
        (node) => node.expansion !== 'complete' && node.run.id !== runId,
      );

      output.next = [
        ...(evidence ? [evidence.retrieve] : []),
        ...(unexpanded
          ? [
              {
                reason: 'Inspect a retained incomplete dependency execution',
                argv: [
                  'teamcity-axi',
                  'run',
                  'failure',
                  unexpanded.run.id,
                  '--server',
                  context.server!,
                  '--job',
                  unexpanded.run.jobId,
                  ...(investigation.projects.get(unexpanded.run.id)
                    ? ['--project', investigation.projects.get(unexpanded.run.id)!]
                    : []),
                  '--depth',
                  String(parsed.flags.depth ?? 4),
                  '--max-nodes',
                  String(parsed.flags['max-nodes'] ?? 30),
                  '--max-diagnosed-runs',
                  String(parsed.flags['max-diagnosed-runs'] ?? 3),
                  ...(parsed.flags.full ? ['--full'] : []),
                ],
              },
            ]
          : []),
      ];

      if (!output.next.length) delete output.next;
    }

    if (parsed.flags['require-complete'] && !investigation.complete) process.exitCode = 1;

    return output;
  } finally {
    await session.transport.dispose();
  }
}
