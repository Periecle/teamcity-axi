import type { Parsed } from '../cli/parser.js';
import { publicContext } from '../context/resolve.js';
import type { ExecutionContext } from '../context/resolve.js';
import { ProjectPolicy } from '../context/project-policy.js';
import { asDomainError, DomainError } from '../domain/errors.js';
import { response } from '../domain/response.js';
import type { Limitation, Response } from '../domain/response.js';
import type { JobSnapshot, Run, StatusQuery } from '../domain/teamcity.js';
import { statusRequest } from '../adapter/status.js';
import { assessStatusJob } from '../planner/status.js';
import { openReadSession } from './read-session.js';

function compactRun(run: Run | null): Record<string, unknown> | null {
  if (!run) return null;

  return {
    id: run.id,
    jobId: run.jobId,
    state: run.state,
    result: run.result,
    branch: run.branch ?? null,
    ...(run.revisions !== undefined ? { revisions: run.revisions } : {}),
    ...(run.result === 'unknown' ? { rawStatus: run.rawStatus ?? null } : {}),
  };
}

export async function readStatus(
  parsed: Parsed,
  context: ExecutionContext,
  signal: AbortSignal,
): Promise<Response> {
  if (!context.jobs.length)
    throw new DomainError(
      'CONTEXT_REQUIRED',
      'Select a job or repository tracked jobs for status',
      2,
    );

  const selectedIds = context.jobs.slice(0, 5);
  const query: StatusQuery = {
    jobIds: selectedIds,
    ...(context.branch !== undefined ? { branch: context.branch } : {}),
  };

  statusRequest(query);
  const session = await openReadSession(context, signal, 'status');

  try {
    const budget = { deadline: context.deadline };
    const server = context.config?.servers[context.server!];
    const policy = new ProjectPolicy(session.reader, server?.allowedProjects, budget);
    const read = await session.reader.readStatus(query, budget);

    if (read.state === 'unavailable') throw read.error;

    const snapshots = new Map<string, JobSnapshot>();
    const limitations: Limitation[] = [];

    for (const snapshot of read.value) {
      if (context.project && snapshot.job.projectId !== context.project)
        throw new DomainError(
          'CONTEXT_MISMATCH',
          'A tracked status job belongs to another project',
        );

      try {
        await policy.assert(snapshot.job.projectId);
        snapshots.set(snapshot.job.id, snapshot);
      } catch (error) {
        const domain = asDomainError(error);

        if (domain.code === 'INTERRUPTED') throw domain;

        limitations.push({
          code: domain.code,
          message: 'A required job could not be admitted under the trusted project policy',
          source: 'context',
        });
      }
    }

    const jobs = selectedIds.map((id) =>
      assessStatusJob(id, snapshots.get(id), context.revision, context.vcsRootId),
    );

    if (context.jobs.length > selectedIds.length)
      limitations.push({
        code: 'TRACKED_JOBS_TRUNCATED',
        message: 'The bounded home view omits additional required jobs',
        source: 'context',
      });

    if (context.dirty !== false)
      limitations.push({
        code: context.dirty ? 'DIRTY_WORKTREE' : 'WORKTREE_UNVERIFIED',
        message: context.dirty
          ? 'Remote builds do not cover uncommitted local changes'
          : 'A clean local worktree could not be verified',
        source: 'context',
      });

    if (!context.head || context.revision !== context.head)
      limitations.push({
        code: 'CHECKOUT_IDENTITY_UNVERIFIED',
        message: 'The selected revision is not the verified local Git HEAD',
        source: 'context',
      });

    if (!context.vcsRootId)
      limitations.push({
        code: 'VCS_ROOT_UNVERIFIED',
        message: 'Select an exact VCS root or configure an unambiguous repository mapping',
        source: 'context',
      });

    if (session.nativeVersion !== '1.5.0')
      limitations.push({
        code: 'UNVERIFIED_VERSION',
        message: 'Native version has not been release-certified',
        source: 'context',
      });

    const scopeComplete =
      snapshots.size === selectedIds.length && context.jobs.length === selectedIds.length;
    const checkoutVerified =
      context.dirty === false && !!context.head && context.revision === context.head;
    const passed =
      scopeComplete && checkoutVerified && jobs.every((job) => job.assessment === 'passed');
    const assessment = passed
      ? 'passed'
      : jobs.some((job) => job.assessment === 'failed')
        ? 'failed'
        : scopeComplete &&
            checkoutVerified &&
            jobs.every((job) => ['passed', 'in_progress'].includes(job.assessment))
          ? 'in_progress'
          : 'unverified';
    const output = response('status', {
      mode: 'configured',
      checkout: {
        head: context.head ?? null,
        branch: context.branch ?? null,
        dirty: context.dirty ?? null,
        revision: context.revision ?? null,
        vcsRootId: context.vcsRootId ?? null,
      },
      assessment,
      check: { requested: !!parsed.flags.check, passed },
      jobs: jobs.map(({ limitations: notes, run, ...job }) => ({
        ...job,
        run: compactRun(run),
        limitationCodes: [...new Set(notes.map((note) => note.code))],
      })),
      coverage: {
        requiredJobs: context.jobs.length,
        displayedJobs: jobs.length,
        observedJobs: snapshots.size,
        complete: scopeComplete,
        candidateLimitPerJob: 20,
        scanLimit: 5000,
        ordering: 'newest_run_id',
        historyComplete: false,
        consistency: 'best_effort',
      },
      failedRunIds: jobs.filter((job) => job.assessment === 'failed').map((job) => job.run!.id),
    });

    const { jobs: trackedJobs, job: contextJob, ...safeContext } = publicContext(context)!;

    output.context = {
      ...safeContext,
      jobs: selectedIds,
      ...(context.job && selectedIds.includes(context.job) ? { job: context.job } : {}),
    };
    const notes = [...limitations, ...jobs.flatMap((job) => job.limitations)];

    if (notes.length) {
      output.meta.limitations = [...new Map(notes.map((note) => [note.code, note])).values()];
      output.meta.complete = false;
      output.status = 'partial';
    }

    output.meta.counts = { childProcesses: session.transport.childProcesses };
    output.meta.observedAt = read.provenance.observedAt;
    output.meta.limits = {
      maxBytes: Math.min(
        Number(parsed.flags['max-bytes'] ?? 6144),
        context.config?.limits?.maxBytes ?? 262144,
      ),
      maxChildProcesses: session.maxChildProcesses,
      concurrency: Math.min(2, context.config?.limits?.concurrency ?? 2),
    };

    if (!parsed.flags['no-hints']) {
      output.next = jobs
        .filter((job) => job.run)
        .slice(0, 3)
        .map((job) => ({
          reason:
            job.assessment === 'failed'
              ? 'Inspect this exact failed execution'
              : 'Inspect this exact status execution',
          argv: [
            'teamcity-axi',
            'run',
            job.assessment === 'failed' ? 'failure' : 'view',
            job.run!.id,
            `--server=${context.server!}`,
            `--job=${job.jobId}`,
            `--project=${snapshots.get(job.jobId)!.job.projectId}`,
          ],
        }));
    }

    return output;
  } finally {
    await session.transport.dispose();
  }
}
