import { createHash } from 'node:crypto';

import { DomainError } from '../domain/errors.js';
import { response } from '../domain/response.js';
import type { Parsed } from '../cli/parser.js';
import type { ExecutionContext } from '../context/resolve.js';
import { ProjectPolicy } from '../context/project-policy.js';
import { decodeCursor, assertCursor, encodeCursor } from '../adapter/cursor.js';
import { relatedRequest } from '../adapter/related.js';
import { openReadSession } from './read-session.js';

export async function readChanges(parsed: Parsed, context: ExecutionContext, signal: AbortSignal) {
  const runId = parsed.positional!,
    now = Date.now();
  const cursor = parsed.flags.cursor ? decodeCursor(String(parsed.flags.cursor), now) : undefined;
  const query = {
    runId,
    count: Number(parsed.flags.limit ?? 10),
    start: cursor?.position ?? 0,
    scanLimit: 5000,
    files: Boolean(parsed.flags.files),
  };
  const request = relatedRequest('changes', query);
  const session = await openReadSession(context, signal);

  try {
    const budget = { deadline: context.deadline },
      read = await session.reader.getRun({ id: runId }, budget);

    if (read.state === 'unavailable') throw read.error;

    if (parsed.flags.job !== undefined && parsed.flags.job !== read.value.jobId)
      throw new DomainError('CONTEXT_MISMATCH', 'Requested run belongs to another job');

    if (parsed.flags.project !== undefined && parsed.flags.project !== read.provenance.projectId)
      throw new DomainError('CONTEXT_MISMATCH', 'Requested run belongs to another project');

    const allowedProjects = context.config?.servers[context.server!]?.allowedProjects;

    await new ProjectPolicy(session.reader, allowedProjects, budget).assert(
      read.provenance.projectId,
    );
    const binding = {
      command: 'run.changes',
      server: context.server!,
      count: query.count,
      filterHash: createHash('sha256')
        .update(
          JSON.stringify({
            serverUrl: context.serverUrl,
            runId,
            jobId: read.value.jobId,
            projectId: read.provenance.projectId,
            filters: request.filters,
            files: query.files,
            allowedProjects: allowedProjects?.toSorted() ?? null,
          }),
        )
        .digest('hex'),
    };

    if (cursor) assertCursor(cursor, binding);

    const page = await session.reader.listChanges(query, budget);

    if (page.state === 'unavailable') throw page.error;

    const output = response('run.changes', {});

    output.context = {
      server: context.server!,
      job: read.value.jobId,
      branch: read.value.branch ?? null,
      ...(read.provenance.projectId ? { project: read.provenance.projectId } : {}),
    };
    output.meta.limitations = [...read.provenance.limitations, ...page.value.limitations];
    const roots = read.value.revisions?.map((r) => r.vcsRootId);
    let previewed = false;
    const keep = parsed.flags.fields
      ? new Set([
          'id',
          'version',
          'vcsRootId',
          'message',
          ...String(parsed.flags.fields).split(','),
        ])
      : undefined;
    const changes = page.value.items.map((change) => {
      if (roots && !roots.includes(change.vcsRootId))
        throw new DomainError(
          'CONTEXT_MISMATCH',
          'Change belongs to a VCS root absent from the selected run',
        );

      const fullText = Array.from(change.message);
      const limit = parsed.flags.full ? 32768 : 200;
      const display = parsed.flags.full
        ? fullText
        : Array.from(change.message.split('\n')[0] ?? '');
      const message = display.slice(0, limit).join('');

      if (message !== change.message) {
        output.meta.truncated = true;
        previewed = true;

        if (parsed.flags.full)
          output.meta.limitations!.push({
            code: 'TEXT_HARD_LIMIT',
            message: 'Change message exceeds the bounded full-text ceiling',
            source: 'changes',
            runId,
          });
      }

      if (change.fileCoverage?.omitted) output.meta.truncated = true;

      return Object.fromEntries(
        Object.entries({ ...change, message }).filter(
          ([key]) => !keep || keep.has(key) || (key === 'fileCoverage' && keep.has('files')),
        ),
      );
    });
    const expiresAt = cursor?.expiresAt ?? now + 1800000,
      continuationNow = Date.now();
    const token =
      page.value.position !== null && expiresAt > continuationNow
        ? encodeCursor(
            { ...binding, version: 1, position: page.value.position, expiresAt },
            continuationNow,
          )
        : null;

    if (page.value.position !== null && expiresAt <= continuationNow)
      output.meta.limitations.push({
        code: 'CURSOR_EXPIRED',
        message: 'Cursor expired during acquisition; useful rows retained',
        source: 'changes',
        runId,
      });

    output.data = {
      run: {
        id: read.value.id,
        jobId: read.value.jobId,
        state: read.value.state,
        result: read.value.result,
        branch: read.value.branch ?? null,
        ...(read.value.revisions ? { revisions: read.value.revisions } : {}),
        ...(read.value.result === 'unknown' ? { rawStatus: read.value.rawStatus ?? null } : {}),
      },
      changes,
      selection: {
        runId,
        pageSize: query.count,
        providerReturned: page.value.providerReturned,
        position: query.start,
        scanLimit: query.scanLimit,
        filesRequested: query.files,
        consistency: 'best_effort_offset',
        meaning: 'changes_associated_with_run',
      },
      page: {
        returned: changes.length,
        total: null,
        totalKind: 'unknown',
        hasMore: page.value.hasMore,
        cursor: token,
      },
    };
    const scopeArgs = [
      '--server',
      context.server!,
      '--job',
      read.value.jobId,
      ...(read.provenance.projectId ? ['--project', read.provenance.projectId] : []),
    ];
    const options = [
      '--limit',
      String(query.count),
      ...(query.files ? ['--files'] : []),
      ...(parsed.flags.fields ? ['--fields', String(parsed.flags.fields)] : []),
    ];

    output.next = [];

    if (token)
      output.next.push({
        reason: 'Read the next bounded change page',
        argv: [
          'teamcity-axi',
          'run',
          'changes',
          runId,
          ...scopeArgs,
          ...options,
          '--cursor',
          token,
          ...(parsed.flags.full ? ['--full'] : []),
        ],
      });

    if (previewed && !parsed.flags.full)
      output.next.push({
        reason: 'Expand messages in this bounded change page',
        argv: [
          'teamcity-axi',
          'run',
          'changes',
          runId,
          ...scopeArgs,
          ...options,
          ...(parsed.flags.cursor ? ['--cursor', String(parsed.flags.cursor)] : []),
          '--full',
        ],
      });

    if (!output.next.length) delete output.next;

    if (session.nativeVersion !== '1.5.0')
      output.meta.limitations.push({
        code: 'UNVERIFIED_VERSION',
        message: 'Native version has not been release-certified',
        source: 'context',
      });

    if (
      output.meta.limitations.some(
        (l) => !['UNVERIFIED_VERSION', 'UNKNOWN_LIFECYCLE', 'UNKNOWN_RESULT'].includes(l.code),
      )
    ) {
      output.status = 'partial';
      output.meta.complete = false;
    }

    output.meta.counts = { childProcesses: session.transport.childProcesses };

    if (parsed.flags['no-hints']) delete output.next;

    if (parsed.flags['require-complete'] && output.status === 'partial') process.exitCode = 1;

    return output;
  } finally {
    await session.transport.dispose();
  }
}
