import { createHash } from 'node:crypto';

import { assertCursor, decodeCursor, encodeCursor } from '../adapter/cursor.js';
import type { CursorBinding } from '../adapter/cursor.js';
import { jobRequest } from '../adapter/jobs.js';
import { literal } from '../adapter/locator.js';
import type { Parsed } from '../cli/parser.js';
import { ProjectPolicy } from '../context/project-policy.js';
import type { ExecutionContext } from '../context/resolve.js';
import { DomainError } from '../domain/errors.js';
import { response } from '../domain/response.js';
import type { Response } from '../domain/response.js';
import type { JobQuery } from '../domain/teamcity.js';
import { openReadSession } from './read-session.js';

function finish(
  output: Response,
  parsed: Parsed,
  context: ExecutionContext,
  session: Awaited<ReturnType<typeof openReadSession>>,
) {
  if (session.nativeVersion !== '1.5.0')
    (output.meta.limitations ??= []).push({
      code: 'UNVERIFIED_VERSION',
      message: 'Native version has not been release-certified',
      source: 'context',
    });

  output.meta.counts = { childProcesses: session.transport.childProcesses };
  output.meta.limits = {
    maxBytes: Math.min(
      Number(parsed.flags['max-bytes'] ?? 16384),
      context.config?.limits?.maxBytes ?? 262144,
    ),
    maxChildProcesses: session.maxChildProcesses,
    concurrency: Math.min(3, context.config?.limits?.concurrency ?? 3),
  };

  if (
    output.meta.limitations?.some(
      (l) => !['BEST_EFFORT_PAGINATION', 'UNVERIFIED_VERSION'].includes(l.code),
    )
  ) {
    output.status = 'partial';
    output.meta.complete = false;
  }

  if (parsed.flags['no-hints']) delete output.next;

  if (parsed.flags['require-complete'] && !output.meta.complete) process.exitCode = 1;

  return output;
}

export async function viewJob(
  parsed: Parsed,
  context: ExecutionContext,
  signal: AbortSignal,
): Promise<Response> {
  const id = parsed.positional!;

  if (!id || id.length > 256)
    throw new DomainError('USAGE_ERROR', 'Job ID exceeds its identity bound', 2);

  literal(id);
  const session = await openReadSession(context, signal);

  try {
    const budget = { deadline: context.deadline };
    const read = await session.reader.getJob({ id }, budget);

    if (read.state === 'unavailable') throw read.error;

    if (parsed.flags.project !== undefined && read.value.projectId !== parsed.flags.project)
      throw new DomainError('CONTEXT_MISMATCH', 'Requested job belongs to another project');

    const policy = new ProjectPolicy(
      session.reader,
      context.config?.servers[context.server!]?.allowedProjects,
      budget,
    );

    await policy.assert(read.value.projectId);
    const output = response('job.view', { job: read.value });

    output.context = { server: context.server!, job: read.value.id, project: read.value.projectId };
    output.meta.observedAt = read.provenance.observedAt;
    output.meta.limitations = [...read.provenance.limitations];
    output.next = [
      {
        reason: 'Read executions for this exact job',
        argv: [
          'teamcity-axi',
          'run',
          'list',
          '--server',
          context.server!,
          `--project=${read.value.projectId}`,
          `--job=${read.value.id}`,
          '--all-branches',
        ],
      },
    ];

    return finish(output, parsed, context, session);
  } finally {
    await session.transport.dispose();
  }
}

export async function listJobs(
  parsed: Parsed,
  context: ExecutionContext,
  signal: AbortSignal,
): Promise<Response> {
  if (!context.project)
    throw new DomainError('CONTEXT_REQUIRED', 'Select an exact project for job list', 2);

  const now = Date.now();
  const cursor = parsed.flags.cursor ? decodeCursor(String(parsed.flags.cursor), now) : undefined;
  const query: JobQuery = {
    projectId: context.project,
    count: Number(parsed.flags.limit ?? 20),
    start: cursor?.position ?? 0,
    scanLimit: 5000,
  };
  const request = jobRequest(query);
  const server = context.config?.servers[context.server!];
  const binding: CursorBinding = {
    command: 'job.list',
    server: context.server!,
    count: query.count,
    filterHash: createHash('sha256')
      .update(
        JSON.stringify({
          serverUrl: context.serverUrl,
          filters: request.filters,
          allowedProjects: server?.allowedProjects?.toSorted() ?? null,
        }),
      )
      .digest('hex'),
  };

  if (cursor) assertCursor(cursor, binding);

  const session = await openReadSession(context, signal);

  try {
    const budget = { deadline: context.deadline };
    const policy = new ProjectPolicy(session.reader, server?.allowedProjects, budget);

    await policy.project(query.projectId);
    await policy.assert(query.projectId);
    const read = await session.reader.listJobs(query, budget);

    if (read.state === 'unavailable') throw read.error;

    const page = read.value;
    const limitations = [
      ...page.limitations,
      {
        code: 'BEST_EFFORT_PAGINATION',
        message: 'Offset pages can change when jobs are created, moved or removed',
        source: 'job',
      },
    ];
    const expiresAt = cursor?.expiresAt ?? now + 1800000;
    const continuationNow = Date.now();

    if (page.position !== null && expiresAt <= continuationNow)
      limitations.push({
        code: 'CURSOR_EXPIRED',
        message: 'The query cursor expired during acquisition; continuation is unavailable',
        source: 'job',
      });

    const token =
      page.position === null || expiresAt <= continuationNow
        ? null
        : encodeCursor(
            { ...binding, version: 1, position: page.position, expiresAt },
            continuationNow,
          );
    const output = response('job.list', {
      jobs: page.items,
      page: {
        returned: page.items.length,
        total: null,
        totalKind: 'unknown',
        hasMore: page.hasMore,
        cursor: token,
      },
      selection: {
        projectId: query.projectId,
        membership: 'direct',
        pageSize: query.count,
        providerReturned: page.providerReturned,
        position: query.start,
        scanLimit: query.scanLimit,
        consistency: 'best_effort_offset',
      },
      ...(!page.items.length
        ? {
            emptyReason: page.hasMore
              ? 'No jobs returned in this page; scoped continuation remains'
              : 'No jobs returned; bounded collection exhaustion is unverified',
          }
        : {}),
    });

    output.context = { server: context.server!, project: query.projectId };
    output.meta.observedAt = read.provenance.observedAt;
    output.meta.limitations = limitations;

    if (token)
      output.next = [
        {
          reason: 'Read the next bounded page for this exact project',
          argv: [
            'teamcity-axi',
            'job',
            'list',
            '--server',
            context.server!,
            `--project=${query.projectId}`,
            '--limit',
            String(query.count),
            '--cursor',
            token,
          ],
        },
      ];
    else if (page.items.length)
      output.next = page.items.slice(0, 2).map((job) => ({
        reason: 'Inspect safe metadata for this job',
        argv: [
          'teamcity-axi',
          'job',
          'view',
          '--server',
          context.server!,
          `--project=${query.projectId}`,
          '--',
          job.id,
        ],
      }));

    return finish(output, parsed, context, session);
  } finally {
    await session.transport.dispose();
  }
}
