import { setTimeout } from 'node:timers/promises';

import type { Parsed } from '../cli/parser.js';
import type { ExecutionContext } from '../context/resolve.js';
import { ProjectPolicy } from '../context/project-policy.js';
import { DomainError } from '../domain/errors.js';
import { response } from '../domain/response.js';
import type { Limitation, Response } from '../domain/response.js';
import type { Run } from '../domain/teamcity.js';
import { openReadSession } from './read-session.js';

export async function watchRun(
  parsed: Parsed,
  context: ExecutionContext,
  signal: AbortSignal,
): Promise<Response> {
  const id = parsed.positional!;
  const interval = Number(parsed.flags.interval ?? 10000);
  const session = await openReadSession(context, signal, 'watch');

  try {
    const budget = { deadline: context.deadline };
    const server = context.config?.servers[context.server!];
    const policy = new ProjectPolicy(session.reader, server?.allowedProjects, budget);
    const startedAt = Date.now();
    let latest: Run | null = null;
    let projectId: string | null = null;
    let observedAt = new Date().toISOString();
    let polls = 0;
    let outcome: 'finished' | 'deadline' | 'vanished' | 'inaccessible' | 'unavailable' = 'deadline';
    const limitations: Limitation[] = [];
    let latestLimitations: Limitation[] = [];

    while (true) {
      if (signal.aborted) throw new DomainError('INTERRUPTED', 'Watcher interrupted');

      if (Date.now() >= context.deadline) {
        limitations.push({
          code: 'DEADLINE_EXCEEDED',
          message: 'Watch deadline reached; the latest available observation is retained',
          source: 'run',
          runId: id,
        });
        break;
      }

      const read = await session.reader.getRun({ id }, budget);

      polls++;

      if (read.state === 'unavailable') {
        if (read.error.code === 'INTERRUPTED') throw read.error;

        outcome =
          read.error.code === 'NOT_FOUND'
            ? 'vanished'
            : ['AUTH_REQUIRED', 'PERMISSION_DENIED'].includes(read.error.code)
              ? 'inaccessible'
              : read.error.code === 'DEADLINE_EXCEEDED'
                ? 'deadline'
                : 'unavailable';
        limitations.push({
          code: read.error.code,
          message:
            'The watched execution could not be observed; retained state is the last verified observation',
          source: 'run',
          runId: id,
        });
        break;
      }

      const run = read.value;

      if (
        (context.job && run.jobId !== context.job) ||
        (context.project && read.provenance.projectId !== context.project)
      )
        throw new DomainError(
          'CONTEXT_MISMATCH',
          'Watched execution belongs to another selected scope',
        );

      if (latest && (latest.jobId !== run.jobId || projectId !== read.provenance.projectId))
        throw new DomainError(
          'CONTEXT_MISMATCH',
          'Watched execution identity changed during polling',
        );

      await policy.assert(read.provenance.projectId);
      latest = run;
      projectId = read.provenance.projectId;
      observedAt = read.provenance.observedAt;
      latestLimitations = read.provenance.limitations;

      if (run.state === 'finished') {
        outcome = 'finished';
        break;
      }

      const delay = Math.min(interval, context.deadline - Date.now());

      if (delay > 0) {
        try {
          await setTimeout(delay, undefined, { signal });
        } catch {
          throw new DomainError('INTERRUPTED', 'Watcher interrupted');
        }
      }
    }

    if (session.nativeVersion !== '1.5.0')
      limitations.push({
        code: 'UNVERIFIED_VERSION',
        message: 'Native version has not been release-certified',
        source: 'context',
      });

    const passed = outcome === 'finished' && latest?.result === 'success';
    const output = response('run.watch', {
      runId: id,
      run: latest
        ? Object.fromEntries(
            Object.entries(latest).filter(([key]) =>
              [
                'id',
                'jobId',
                'state',
                'result',
                'branch',
                'personal',
                'composite',
                'revisions',
                'rawStatus',
                'startedAt',
                'finishedAt',
                'queuedAt',
              ].includes(key),
            ),
          )
        : null,
      outcome,
      check: { requested: !!parsed.flags.check, passed },
      polling: {
        count: polls,
        intervalMs: interval,
        elapsedMs: Date.now() - startedAt,
        consistency: 'best_effort',
      },
    });

    output.context = latest
      ? {
          server: context.server!,
          job: latest.jobId,
          ...(projectId ? { project: projectId } : {}),
          branch: latest.branch ?? null,
        }
      : {
          server: context.server!,
          ...(context.job ? { job: context.job } : {}),
          ...(context.project ? { project: context.project } : {}),
        };
    output.meta.observedAt = observedAt;
    output.meta.counts = { childProcesses: session.transport.childProcesses };
    output.meta.limits = {
      maxBytes: Math.min(
        Number(parsed.flags['max-bytes'] ?? 8192),
        context.config?.limits?.maxBytes ?? 262144,
      ),
      maxChildProcesses: session.maxChildProcesses,
      concurrency: 1,
    };

    limitations.push(...latestLimitations);

    if (limitations.length) {
      output.status = 'partial';
      output.meta.complete = false;
      output.meta.limitations = limitations;
    }

    if (!parsed.flags['no-hints'] && latest)
      output.next = [
        {
          reason: 'Inspect this exact observed execution',
          argv: [
            'teamcity-axi',
            'run',
            latest.state === 'finished' &&
            ['failure', 'error', 'canceled', 'failed_to_start'].includes(latest.result)
              ? 'failure'
              : 'view',
            id,
            `--server=${context.server!}`,
            `--job=${latest.jobId}`,
            ...(projectId ? [`--project=${projectId}`] : []),
          ],
        },
      ];

    return output;
  } finally {
    await session.transport.dispose();
  }
}
