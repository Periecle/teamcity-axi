import { createHash } from 'node:crypto';

import { DomainError } from '../domain/errors.js';
import { response } from '../domain/response.js';
import type { Response } from '../domain/response.js';
import type { RunQuery } from '../domain/teamcity.js';
import type { Parsed } from '../cli/parser.js';
import type { ExecutionContext } from '../context/resolve.js';
import { ProcessTransport, resolveBinary } from '../transport/process.js';
import { NativeTeamCityReader } from '../adapter/reader.js';
import { decodeCursor, assertCursor, encodeCursor } from '../adapter/cursor.js';
import type { CursorBinding } from '../adapter/cursor.js';
import { runFilters } from '../adapter/run-page.js';
import { knownSecrets } from '../output/sanitize.js';
import { ProjectPolicy } from '../context/project-policy.js';

function wholeSecond(value: string): string {
  const fraction = /\.(\d+)(?:Z|[+-]\d\d:\d\d)$/.exec(value)?.[1];

  if (fraction && /[1-9]/.test(fraction))
    throw new DomainError(
      'USAGE_ERROR',
      'Finish-time filters currently require whole-second timestamps',
      2,
    );
  const canonical = new Date(value).toISOString();

  if (!canonical.endsWith('.000Z'))
    throw new DomainError(
      'USAGE_ERROR',
      'Finish-time filters currently require whole-second timestamps',
      2,
    );

  return canonical;
}

export async function listRuns(
  parsed: Parsed,
  context: ExecutionContext,
  signal: AbortSignal,
): Promise<Response> {
  if (!context.job && !context.project)
    throw new DomainError('CONTEXT_REQUIRED', 'Select a job or project for run list', 2);
  const result = parsed.flags.result === undefined ? undefined : String(parsed.flags.result);

  if (result && !['success', 'failure', 'error'].includes(result))
    throw new DomainError(
      'DEPENDENCY_UNSUPPORTED',
      'This outcome filter requires explicit metadata not yet verified by the adapter',
    );
  if (parsed.flags.revision && !context.vcsRootId)
    throw new DomainError(
      'CONTEXT_REQUIRED',
      'Revision filtering requires an exact VCS root or unambiguous repository mapping',
      2,
    );
  const now = Date.now(),
    cursor = parsed.flags.cursor ? decodeCursor(String(parsed.flags.cursor), now) : undefined;
  const state = String(parsed.flags.state ?? 'finished') as NonNullable<RunQuery['state']>;
  const until = parsed.flags.until
    ? wholeSecond(String(parsed.flags.until))
    : (cursor?.window?.until ?? new Date(Math.floor(now / 1000) * 1000).toISOString());
  const since = parsed.flags.since
    ? wholeSecond(String(parsed.flags.since))
    : (cursor?.window?.since ?? new Date(Date.parse(until) - 7 * 86400000).toISOString());
  const window = state === 'finished' ? { since, until } : undefined;
  const server = context.config?.servers[context.server!]!;
  const query: RunQuery = {
    ...(context.job ? { jobId: context.job } : {}),
    ...(context.project ? { projectId: context.project } : {}),
    ...(context.branch !== undefined ? { branch: context.branch } : {}),
    state,
    ...(result ? { result: result as RunQuery['result'] & string } : {}),
    ...(parsed.flags.revision
      ? { revision: context.revision!, vcsRootId: context.vcsRootId! }
      : {}),
    ...(window ? { window } : {}),
    count: Number(parsed.flags.limit ?? 20),
    start: cursor?.position ?? 0,
    scanLimit: 5000,
    ...(server?.allowedProjects ? { allowedProjects: server.allowedProjects } : {}),
  };

  // Reject invalid typed selectors before launching a native child.
  runFilters(query);
  const binary = await resolveBinary(
    context.config?.binaryPath,
    context.repositoryRoot,
    context.config?.allowWorkspaceBinary,
  );
  const transport = await ProcessTransport.create({
    binary,
    serverUrl: context.serverUrl!,
    env: process.env,
    signal,
    ...(server?.forwardHeaderEnvNames ? { headerNames: server.forwardHeaderEnvNames } : {}),
    limits: {
      deadline: context.deadline,
      concurrency: Math.min(3, context.config?.limits?.concurrency ?? 3),
      maxChildren: Math.min(8, context.config?.limits?.maxChildProcesses ?? 8),
      stdoutBytes: 2097152,
      stderrBytes: 65536,
    },
  });

  try {
    const version = await transport.execute({ kind: 'version' }),
      matched = /^teamcity version ([a-zA-Z0-9.+-]{1,80})\r?\n$/.exec(
        version.stdout.toString('utf8'),
      );

    if (version.exitCode !== 0 || !matched)
      throw new DomainError(
        'DEPENDENCY_UNSUPPORTED',
        'Native executable has an unsupported version response',
      );
    const reader = new NativeTeamCityReader(
      transport,
      context.serverUrl!,
      knownSecrets(process.env, context.config?.secretNamePatterns, server?.forwardHeaderEnvNames),
    );
    const policy = new ProjectPolicy(reader, server?.allowedProjects, {
      deadline: context.deadline,
    });

    if (query.jobId) {
      const job = await reader.getJob({ id: query.jobId }, { deadline: context.deadline });

      if (job.state === 'unavailable') throw job.error;
      if (query.projectId && query.projectId !== job.value.projectId)
        throw new DomainError('CONTEXT_MISMATCH', 'Selected job belongs to another project');
      await policy.assert(job.value.projectId);
      query.projectId = job.value.projectId;
    } else {
      await policy.project(query.projectId!);
      await policy.assert(query.projectId!);
    }

    if (server?.allowedProjects) query.allowedProjects = [query.projectId!];
    const filterHash = createHash('sha256')
      .update(
        JSON.stringify({
          serverUrl: context.serverUrl,
          filters: runFilters(query),
          revision: query.revision ?? null,
          vcsRootId: query.vcsRootId ?? null,
          allowedProjects: server?.allowedProjects?.toSorted() ?? null,
          window: window ?? null,
        }),
      )
      .digest('hex');
    const binding: CursorBinding = {
      command: 'run.list',
      server: context.server!,
      filterHash,
      count: query.count,
      ...(window ? { window } : {}),
    };

    if (cursor) assertCursor(cursor, binding);
    const read = await reader.listRuns(query, { deadline: context.deadline });

    if (read.state === 'unavailable') throw read.error;
    const page = read.value,
      limitations = [
        ...page.limitations,
        {
          code: 'BEST_EFFORT_PAGINATION',
          message: 'Offset pages can change when executions are inserted or deleted',
          source: 'run' as const,
        },
      ];

    if (matched[1] !== '1.5.0')
      limitations.push({
        code: 'UNVERIFIED_VERSION',
        message: 'Native version has not been release-certified',
        source: 'context',
      });
    if (parsed.flags.revision && context.dirty)
      limitations.push({
        code: 'DIRTY_WORKTREE',
        message: 'Remote revision matches do not cover uncommitted local changes',
        source: 'context',
      });
    const expiresAt = cursor?.expiresAt ?? now + 1800000;
    const continuationNow = Date.now();

    if (page.position !== null && expiresAt <= continuationNow)
      limitations.push({
        code: 'CURSOR_EXPIRED',
        message: 'The query cursor expired during acquisition; continuation is unavailable',
        source: 'run',
      });
    const token =
      page.position === null || expiresAt <= continuationNow
        ? null
        : encodeCursor(
            {
              ...binding,
              version: 1,
              position: page.position,
              expiresAt,
            },
            continuationNow,
          );
    const keep = new Set([
      'id',
      'jobId',
      'state',
      'result',
      ...(query.branch === undefined ? ['branch'] : []),
      ...(parsed.flags.revision ? ['revisions'] : []),
      ...String(parsed.flags.fields ?? 'branch').split(','),
    ]);
    const runs = page.runs.map((run) =>
      Object.fromEntries(
        Object.entries(run).filter(
          ([key]) => keep.has(key) || (key === 'rawStatus' && run.result === 'unknown'),
        ),
      ),
    );
    const output = response('run.list', {
      runs,
      page: {
        returned: runs.length,
        total: null,
        totalKind: 'unknown',
        hasMore: page.hasMore,
        cursor: token,
      },
      selection: {
        pageSize: query.count,
        providerReturned: page.providerReturned,
        position: query.start,
        scanLimit: query.scanLimit,
        consistency: 'best_effort_offset',
        timestampBasis: window ? 'finishTime' : null,
        ...(window ? { window: { ...window, bounds: 'exclusive' } } : {}),
      },
      aggregates: {
        scope: 'returnedPage',
        failure: runs.filter((r) => r.result === 'failure').length,
        success: runs.filter((r) => r.result === 'success').length,
        unknown: runs.filter((r) => r.result === 'unknown').length,
      },
      ...(runs.length
        ? {}
        : {
            emptyReason: page.hasMore
              ? 'No matches in this page; bounded continuation remains'
              : 'No rows returned; bounded search exhaustion is unverified',
          }),
    });

    output.context = {
      server: context.server!,
      ...(query.projectId ? { project: query.projectId } : {}),
      ...(query.jobId ? { job: query.jobId } : {}),
      ...(query.branch !== undefined ? { branch: query.branch } : {}),
      ...(query.revision ? { revision: query.revision, vcsRootId: query.vcsRootId } : {}),
    };
    output.meta.observedAt = read.provenance.observedAt;
    output.meta.counts = { childProcesses: transport.childProcesses };
    output.meta.limitations = limitations;

    if (
      limitations.some(
        (l) =>
          ![
            'BEST_EFFORT_PAGINATION',
            'UNVERIFIED_VERSION',
            'UNKNOWN_LIFECYCLE',
            'UNKNOWN_RESULT',
          ].includes(l.code),
      )
    ) {
      output.status = 'partial';
      output.meta.complete = false;
    }

    if (token && !parsed.flags['no-hints'])
      output.next = [
        {
          reason: 'Read the next bounded page of this query',
          argv: [
            'teamcity-axi',
            'run',
            'list',
            '--server',
            context.server!,
            ...(query.jobId ? ['--job', query.jobId] : []),
            ...(query.projectId ? ['--project', query.projectId] : []),
            ...(query.branch !== undefined
              ? ['--literal-branch', query.branch]
              : ['--all-branches']),
            '--state',
            state,
            ...(result ? ['--result', result] : []),
            ...(query.revision
              ? ['--revision', query.revision, '--vcs-root', query.vcsRootId!]
              : []),
            ...(parsed.flags.fields ? ['--fields', String(parsed.flags.fields)] : []),
            '--limit',
            String(query.count),
            '--cursor',
            token,
          ],
        },
      ];
    if (parsed.flags['require-complete'] && output.status === 'partial') process.exitCode = 1;

    return output;
  } finally {
    await transport.dispose();
  }
}
