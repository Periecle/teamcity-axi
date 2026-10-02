import { createHash } from 'node:crypto';

import { assertCursor, decodeCursor, encodeCursor } from '../adapter/cursor.js';
import type { CursorBinding } from '../adapter/cursor.js';
import { queueRequest } from '../adapter/queue.js';
import type { Parsed } from '../cli/parser.js';
import { ProjectPolicy } from '../context/project-policy.js';
import type { ExecutionContext } from '../context/resolve.js';
import { DomainError } from '../domain/errors.js';
import { response } from '../domain/response.js';
import type { Response } from '../domain/response.js';
import type { QueueQuery } from '../domain/teamcity.js';
import { openReadSession } from './read-session.js';

export async function listQueue(
  parsed: Parsed,
  context: ExecutionContext,
  signal: AbortSignal,
): Promise<Response> {
  if (!context.job && !context.project)
    throw new DomainError('CONTEXT_REQUIRED', 'Select a job or project for queue list', 2);
  const now = Date.now();
  const cursor = parsed.flags.cursor ? decodeCursor(String(parsed.flags.cursor), now) : undefined;
  const query: QueueQuery = {
    ...(context.job ? { jobId: context.job } : {}),
    ...(context.project ? { projectId: context.project } : {}),
    count: Number(parsed.flags.limit ?? 20),
    start: cursor?.position ?? 0,
    scanLimit: 5000,
  };

  queueRequest(query);
  const server = context.config?.servers[context.server!];
  const binding = (): CursorBinding => ({
    command: 'queue.list',
    server: context.server!,
    count: query.count,
    filterHash: createHash('sha256')
      .update(
        JSON.stringify({
          serverUrl: context.serverUrl,
          filters: queueRequest(query).filters,
          allowedProjects: server?.allowedProjects?.toSorted() ?? null,
        }),
      )
      .digest('hex'),
  });

  // A job-only cursor binds its observed owning project after the exact job read.
  if (cursor && query.projectId) assertCursor(cursor, binding());
  if (
    cursor &&
    (cursor.command !== 'queue.list' ||
      cursor.server !== context.server ||
      cursor.count !== query.count)
  )
    throw new DomainError('USAGE_ERROR', 'Cursor belongs to another queue query', 2);
  const session = await openReadSession(context, signal);

  try {
    const budget = { deadline: context.deadline };
    const policy = new ProjectPolicy(session.reader, server?.allowedProjects, budget);

    if (query.jobId) {
      const job = await session.reader.getJob({ id: query.jobId }, budget);

      if (job.state === 'unavailable') throw job.error;
      if (query.projectId && job.value.projectId !== query.projectId)
        throw new DomainError('CONTEXT_MISMATCH', 'Selected queue job belongs to another project');
      query.projectId = job.value.projectId;
      await policy.assert(query.projectId);
    } else {
      await policy.project(query.projectId!);
      await policy.assert(query.projectId!);
    }

    const bound = binding();

    if (cursor) assertCursor(cursor, bound);
    const read = await session.reader.listQueue(query, budget);

    if (read.state === 'unavailable') throw read.error;
    const page = read.value;
    const limitations = [
      ...page.limitations,
      {
        code: 'BEST_EFFORT_PAGINATION',
        message: 'Queued executions can start, leave or change position between observations',
        source: 'queue',
      },
    ];

    if (session.nativeVersion !== '1.5.0')
      limitations.push({
        code: 'UNVERIFIED_VERSION',
        message: 'Native version has not been release-certified',
        source: 'context',
      });
    const expiresAt = cursor?.expiresAt ?? now + 1800000,
      continuationNow = Date.now();

    if (page.position !== null && expiresAt <= continuationNow)
      limitations.push({
        code: 'CURSOR_EXPIRED',
        message: 'The query cursor expired during acquisition; continuation is unavailable',
        source: 'queue',
      });
    const token =
      page.position === null || expiresAt <= continuationNow
        ? null
        : encodeCursor(
            { ...bound, version: 1, position: page.position, expiresAt },
            continuationNow,
          );
    const output = response('queue.list', {
      items: page.items,
      page: {
        returned: page.items.length,
        total: null,
        totalKind: 'unknown',
        hasMore: page.hasMore,
        cursor: token,
      },
      selection: {
        projectId: query.projectId,
        jobId: query.jobId ?? null,
        pageSize: query.count,
        providerReturned: page.providerReturned,
        position: query.start,
        scanLimit: query.scanLimit,
        consistency: 'best_effort_offset',
        meaning: 'queued_executions',
      },
      ...(!page.items.length
        ? {
            emptyReason: page.hasMore
              ? 'No items returned in this page; scoped continuation remains'
              : 'No queued items returned; bounded queue exhaustion is unverified',
          }
        : {}),
    });

    output.context = {
      server: context.server!,
      project: query.projectId,
      ...(query.jobId ? { job: query.jobId } : {}),
    };
    output.meta.observedAt = read.provenance.observedAt;
    output.meta.counts = { childProcesses: session.transport.childProcesses };
    output.meta.limits = {
      maxBytes: Math.min(
        Number(parsed.flags['max-bytes'] ?? 16384),
        context.config?.limits?.maxBytes ?? 262144,
      ),
      maxChildProcesses: session.maxChildProcesses,
      concurrency: Math.min(3, context.config?.limits?.concurrency ?? 3),
    };
    output.meta.limitations = limitations;

    if (
      limitations.some((l) => !['BEST_EFFORT_PAGINATION', 'UNVERIFIED_VERSION'].includes(l.code))
    ) {
      output.status = 'partial';
      output.meta.complete = false;
    }

    if (!parsed.flags['no-hints']) {
      if (token)
        output.next = [
          {
            reason: 'Read the next bounded page of this queue scope',
            argv: [
              'teamcity-axi',
              'queue',
              'list',
              '--server',
              context.server!,
              `--project=${query.projectId}`,
              ...(query.jobId ? [`--job=${query.jobId}`] : []),
              '--limit',
              String(query.count),
              '--cursor',
              token,
            ],
          },
        ];
      else if (page.items.length)
        output.next = page.items.slice(0, 2).map((item) => ({
          reason: 'Observe this exact queued execution',
          argv: [
            'teamcity-axi',
            'run',
            'view',
            item.id,
            '--server',
            context.server!,
            `--project=${query.projectId}`,
            `--job=${item.jobId}`,
          ],
        }));
    }

    if (parsed.flags['require-complete'] && !output.meta.complete) process.exitCode = 1;

    return output;
  } finally {
    await session.transport.dispose();
  }
}
