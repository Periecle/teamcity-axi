import { createHash } from 'node:crypto';

import { DomainError } from '../domain/errors.js';
import { response } from '../domain/response.js';
import type { Response } from '../domain/response.js';
import type {
  EvidencePage,
  EvidenceQuery,
  Problem,
  TestOccurrence,
  Run,
  LogTail,
  ReadResult,
} from '../domain/teamcity.js';
import type { Parsed } from '../cli/parser.js';
import type { ExecutionContext } from '../context/resolve.js';
import { ProjectPolicy } from '../context/project-policy.js';
import { decodeCursor, assertCursor, encodeCursor } from '../adapter/cursor.js';
import type { CursorBinding } from '../adapter/cursor.js';
import { occurrenceLocator, evidenceRequest } from '../adapter/evidence.js';
import type { EvidenceKind } from '../adapter/evidence.js';
import { openReadSession } from './read-session.js';

function primaryRun(run: Run): Run {
  return {
    id: run.id,
    jobId: run.jobId,
    state: run.state,
    result: run.result,
    branch: run.branch ?? null,
    ...(run.revisions ? { revisions: run.revisions } : {}),
    ...(run.result === 'unknown' ? { rawStatus: run.rawStatus ?? null } : {}),
  };
}

function preview<T extends Problem | TestOccurrence>(item: T, full: boolean, output: Response): T {
  const limit = full ? 32768 : 'description' in item ? 1200 : 2000;
  const field = 'description' in item ? 'description' : 'details';
  const value = item[field as keyof T];

  if (typeof value !== 'string' || Array.from(value).length <= limit) return item;

  output.meta.truncated = true;

  if (full) {
    output.status = 'partial';
    output.meta.complete = false;
    (output.meta.limitations ??= []).push({
      code: 'TEXT_HARD_LIMIT',
      message: 'Selected text exceeds the bounded full-text ceiling',
      source: 'description' in item ? 'problems' : 'tests',
      runId: item.runId,
    });
  }

  return { ...item, [field]: Array.from(value).slice(0, limit).join('') };
}

function projection(test: TestOccurrence, fields: string | undefined): TestOccurrence {
  if (!fields) return test;

  const keep = new Set([
    'id',
    'runId',
    'name',
    'result',
    'muted',
    'ignored',
    ...(test.result === 'unknown' ? ['rawStatus'] : []),
    ...fields.split(','),
  ]);

  return Object.fromEntries(
    Object.entries(test).filter(([key]) => keep.has(key)),
  ) as unknown as TestOccurrence;
}

function boundedLog(log: LogTail, parsed: Parsed, output: Response) {
  const retained = log.messages,
    contains = parsed.flags.contains === undefined ? undefined : String(parsed.flags.contains);
  const matching =
    contains === undefined ? retained : retained.filter((m) => m.text.includes(contains));
  const limit = parsed.flags.full ? 32768 : 2000;
  const messages = matching.map((m) => {
    const text = Array.from(m.text);

    if (text.length > limit) {
      output.meta.truncated = true;

      if (parsed.flags.full) {
        output.status = 'partial';
        output.meta.complete = false;
        (output.meta.limitations ??= []).push({
          code: 'TEXT_HARD_LIMIT',
          message: 'Log message exceeds the bounded full-text ceiling',
          source: 'log',
          runId: log.runId,
        });
      }
    }

    return { ...m, runId: log.runId, text: text.slice(0, limit).join('') };
  });

  return {
    messages,
    window: {
      kind: 'tail',
      requested: Number(parsed.flags.tail ?? 80),
      providerReturned: log.providerReturned,
      retained: retained.length,
      firstMessageId: retained[0]?.id ?? null,
      lastMessageId: retained.at(-1)?.id ?? null,
      omittedProviderMessages: log.providerReturned - retained.length,
      matched: messages.length,
      ...(contains !== undefined ? { contains } : {}),
    },
    ...(contains !== undefined && !messages.length
      ? { emptyReason: 'No literal matches in the declared retained tail window' }
      : {}),
  };
}

export async function readEvidence(
  parsed: Parsed,
  context: ExecutionContext,
  signal: AbortSignal,
): Promise<Response> {
  const command = parsed.descriptor.name,
    runId = parsed.positional!,
    now = Date.now();
  const kind: EvidenceKind = command === 'run.problems' ? 'problems' : 'tests';
  const selected = parsed.flags.problem ?? parsed.flags.test;

  if (selected !== undefined) occurrenceLocator(kind, String(selected), runId);

  const cursor = parsed.flags.cursor ? decodeCursor(String(parsed.flags.cursor), now) : undefined;
  const query: EvidenceQuery = {
    runId,
    count: Number(parsed.flags.limit ?? 20),
    start: cursor?.position ?? 0,
    scanLimit: 5000,
    ...((parsed.flags.failed || parsed.flags.muted) && command === 'run.tests'
      ? { failed: true }
      : {}),
    ...(command === 'run.tests' && parsed.flags.muted
      ? { muted: true }
      : command === 'run.tests' && parsed.flags.failed && !parsed.flags['include-muted']
        ? { muted: false }
        : {}),
  };

  if (command !== 'run.log') evidenceRequest(kind, query);

  const session = await openReadSession(context, signal);

  try {
    const budget = { deadline: context.deadline },
      read = await session.reader.getRun({ id: runId }, budget);

    if (read.state === 'unavailable') throw read.error;

    if (parsed.flags.job !== undefined && read.value.jobId !== parsed.flags.job)
      throw new DomainError('CONTEXT_MISMATCH', 'Requested run belongs to another job');

    if (parsed.flags.project !== undefined && read.provenance.projectId !== parsed.flags.project)
      throw new DomainError('CONTEXT_MISMATCH', 'Requested run belongs to another project');

    const policy = new ProjectPolicy(
      session.reader,
      context.config?.servers[context.server!]?.allowedProjects,
      budget,
    );

    await policy.assert(read.provenance.projectId);
    const output = response(command, { run: primaryRun(read.value) });

    output.context = {
      server: context.server!,
      job: read.value.jobId,
      branch: read.value.branch ?? null,
      ...(read.provenance.projectId ? { project: read.provenance.projectId } : {}),
    };
    output.meta.limitations = [...read.provenance.limitations];
    const scopeArgs = [
      '--server',
      context.server!,
      '--job',
      read.value.jobId,
      ...(read.provenance.projectId ? ['--project', read.provenance.projectId] : []),
    ];

    if (command === 'run.log') {
      let providerTruncated = false;

      if (parsed.flags.failed) {
        const results = await Promise.all([
          session.reader.listProblems({ runId, count: 20, start: 0, scanLimit: 5000 }, budget),
          session.reader.listTests(
            { runId, count: 20, start: 0, scanLimit: 5000, failed: true, muted: false },
            budget,
          ),
          session.reader.getLogTail({ id: runId }, 80, budget),
        ]);

        if (results.some((r) => r.state === 'unavailable' && r.error.code === 'INTERRUPTED'))
          throw new DomainError('INTERRUPTED', 'Invocation interrupted');

        const [problems, tests, log] = results;

        providerTruncated = log.state === 'available' && log.value.truncated;

        const source = (read: ReadResult<unknown>, source: string) => {
          if (read.state === 'unavailable') {
            output.status = 'partial';
            output.meta.complete = false;
            output.meta.limitations!.push({
              code: read.error.code,
              message: 'Independent source is unavailable',
              source,
              runId,
            });

            return {
              availability: 'unavailable',
              complete: false,
              coverage: 'unavailable',
              errorCode: read.error.code,
            };
          }

          const value = read.value as EvidencePage<unknown> | LogTail;
          const limitations = 'items' in value ? value.limitations : read.provenance.limitations;

          output.meta.limitations!.push(...limitations);

          return {
            availability: 'available',
            complete: limitations.length === 0,
            coverage: source === 'log' ? 'tail_window' : 'bounded_page',
          };
        };

        const sources = {
          problems: source(problems, 'problems'),
          tests: source(tests, 'tests'),
          log: source(log, 'log'),
        };

        output.data = {
          ...output.data,
          mode: 'failed',
          sources,
          ...(problems.state === 'available'
            ? {
                problems: problems.value.items.map((p) =>
                  preview(p, Boolean(parsed.flags.full), output),
                ),
              }
            : {}),
          ...(tests.state === 'available'
            ? {
                tests: tests.value.items.map((t) => preview(t, Boolean(parsed.flags.full), output)),
              }
            : {}),
          ...(log.state === 'available' ? boundedLog(log.value, parsed, output) : {}),
        };
      } else {
        const log = await session.reader.getLogTail(
          { id: runId },
          Number(parsed.flags.tail ?? 80),
          budget,
        );

        if (log.state === 'unavailable') {
          if (
            [
              'INTERRUPTED',
              'DEADLINE_EXCEEDED',
              'INPUT_LIMIT_EXCEEDED',
              'CONTEXT_MISMATCH',
              'UPSTREAM_SCHEMA_MISMATCH',
            ].includes(log.error.code)
          )
            throw log.error;

          throw new DomainError(
            'CAPABILITY_UNAVAILABLE',
            'Bounded structured log messages are unavailable',
          );
        }

        output.meta.limitations.push(...log.provenance.limitations);
        output.data = { ...output.data, mode: 'tail', ...boundedLog(log.value, parsed, output) };
        providerTruncated = log.value.truncated;
      }

      if (output.meta.truncated && !parsed.flags.full)
        output.next = [
          {
            reason: 'Expand bounded log text',
            argv: [
              'teamcity-axi',
              'run',
              'log',
              runId,
              ...scopeArgs,
              ...(parsed.flags.failed
                ? ['--failed']
                : [
                    '--tail',
                    String(parsed.flags.tail ?? 80),
                    ...(parsed.flags.contains !== undefined
                      ? ['--contains', String(parsed.flags.contains)]
                      : []),
                  ]),
              '--full',
            ],
          },
        ];

      if (providerTruncated) output.meta.truncated = true;
    } else {
      const filterHash = createHash('sha256')
        .update(
          JSON.stringify({
            serverUrl: context.serverUrl,
            runId,
            jobId: read.value.jobId,
            projectId: read.provenance.projectId,
            request: evidenceRequest(kind, query).filters,
            allowedProjects:
              context.config?.servers[context.server!]?.allowedProjects?.toSorted() ?? null,
          }),
        )
        .digest('hex');
      const binding: CursorBinding = {
        command,
        server: context.server!,
        count: query.count,
        filterHash,
      };

      if (cursor) assertCursor(cursor, binding);

      let items: (Problem | TestOccurrence)[],
        page: EvidencePage<Problem | TestOccurrence> | undefined;

      if (selected !== undefined) {
        const detail =
          kind === 'problems'
            ? await session.reader.getProblem({ runId, id: String(selected) }, budget)
            : await session.reader.getTest({ runId, id: String(selected) }, budget);

        if (detail.state === 'unavailable') throw detail.error;

        output.meta.limitations.push(...detail.provenance.limitations);
        items = [detail.value];
      } else {
        const readPage =
          kind === 'problems'
            ? await session.reader.listProblems(query, budget)
            : await session.reader.listTests(query, budget);

        if (readPage.state === 'unavailable') throw readPage.error;

        page = readPage.value;
        items = page.items;
        output.meta.limitations.push(...page.limitations);
      }

      const continuationNow = Date.now(),
        expiresAt = cursor?.expiresAt ?? now + 1800000;
      const token =
        page?.position !== null && page?.position !== undefined && expiresAt > continuationNow
          ? encodeCursor(
              { ...binding, version: 1, position: page.position, expiresAt },
              continuationNow,
            )
          : null;

      if (page?.position !== null && page?.position !== undefined && expiresAt <= continuationNow)
        output.meta.limitations.push({
          code: 'CURSOR_EXPIRED',
          message: 'Cursor expired during acquisition; useful rows are retained',
          source: kind,
          runId,
        });

      const projected = items.map((item) =>
        kind === 'tests'
          ? projection(
              item as TestOccurrence,
              parsed.flags.fields === undefined ? undefined : String(parsed.flags.fields),
            )
          : item,
      );
      const previewTarget = projected.find(
        (item) =>
          Array.from('description' in item ? item.description : (item.details ?? '')).length >
          (kind === 'problems' ? 1200 : 2000),
      );
      const displayed = projected.map((item) => preview(item, Boolean(parsed.flags.full), output));

      output.data = {
        ...output.data,
        [kind]: displayed,
        selection: {
          kind: selected !== undefined ? 'occurrence' : 'page',
          pageSize: selected !== undefined ? 1 : query.count,
          providerReturned: page?.providerReturned ?? 1,
          position: query.start,
          scanLimit: 5000,
          consistency: 'best_effort_offset',
          ...(selected !== undefined ? { occurrenceId: String(selected) } : {}),
          ...(kind === 'tests'
            ? {
                filter: parsed.flags.muted
                  ? 'muted_failures'
                  : parsed.flags.failed
                    ? parsed.flags['include-muted']
                      ? 'failures_including_muted'
                      : 'unmuted_failures'
                    : 'all',
              }
            : {}),
        },
        page: {
          returned: displayed.length,
          total: selected !== undefined ? 1 : null,
          totalKind: selected !== undefined ? 'exact' : 'unknown',
          hasMore: selected !== undefined ? false : page!.hasMore,
          cursor: token,
        },
      };
      output.next = [];

      if (token)
        output.next.push({
          reason: 'Read the next bounded occurrence page',
          argv: [
            'teamcity-axi',
            'run',
            kind,
            runId,
            ...scopeArgs,
            '--limit',
            String(query.count),
            '--cursor',
            token,
            ...(parsed.flags.failed ? ['--failed'] : []),
            ...(parsed.flags.muted ? ['--muted'] : []),
            ...(parsed.flags['include-muted'] ? ['--include-muted'] : []),
            ...(parsed.flags.fields ? ['--fields', String(parsed.flags.fields)] : []),
            ...(parsed.flags.full ? ['--full'] : []),
          ],
        });

      if (previewTarget && !parsed.flags.full)
        output.next.push({
          reason: 'Expand an exact occurrence',
          argv: [
            'teamcity-axi',
            'run',
            kind,
            runId,
            ...scopeArgs,
            kind === 'problems' ? '--problem' : '--test',
            previewTarget.id,
            '--full',
          ],
        });

      if (!output.next.length) delete output.next;
    }

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

    if (!output.meta.limitations.length) delete output.meta.limitations;

    output.meta.counts = { childProcesses: session.transport.childProcesses };
    output.meta.observedAt = new Date().toISOString();

    if (parsed.flags['no-hints']) delete output.next;

    if (parsed.flags['require-complete'] && output.status === 'partial') process.exitCode = 1;

    return output;
  } finally {
    await session.transport.dispose();
  }
}
