import { createHash } from 'node:crypto';

import { assertCursor, decodeCursor, encodeCursor } from '../adapter/cursor.js';
import type { CursorBinding } from '../adapter/cursor.js';
import { agentFilters, agentRequest, validateAgentId } from '../adapter/agents.js';
import type { Parsed } from '../cli/parser.js';
import { ProjectPolicy } from '../context/project-policy.js';
import type { ExecutionContext } from '../context/resolve.js';
import { DomainError } from '../domain/errors.js';
import { response } from '../domain/response.js';
import type { Limitation, Response } from '../domain/response.js';
import type { Agent, AgentQuery, AgentScope, EvidencePage } from '../domain/teamcity.js';
import { openReadSession } from './read-session.js';

export async function readAgents(
  parsed: Parsed,
  context: ExecutionContext,
  signal: AbortSignal,
): Promise<Response> {
  const list = parsed.descriptor.name === 'agent.list';
  const now = Date.now();
  const cursor = parsed.flags.cursor ? decodeCursor(String(parsed.flags.cursor), now) : undefined;
  const query: AgentQuery = {
    ...(context.job ? { jobId: context.job } : {}),
    ...(context.project ? { projectId: context.project } : {}),
    ...(parsed.flags.pool !== undefined ? { poolId: String(parsed.flags.pool) } : {}),
    count: Number(parsed.flags.limit ?? 20),
    start: cursor?.position ?? 0,
    scanLimit: 5000,
  };

  if (list) agentRequest(query);
  else {
    validateAgentId(parsed.positional!);
    agentFilters(query);
  }

  const scope = (): AgentScope => ({
    ...(query.jobId ? { jobId: query.jobId } : {}),
    ...(query.projectId ? { projectId: query.projectId } : {}),
    ...(query.poolId !== undefined ? { poolId: query.poolId } : {}),
  });
  const server = context.config?.servers[context.server!];
  const binding = (): CursorBinding => ({
    command: 'agent.list',
    server: context.server!,
    count: query.count,
    filterHash: createHash('sha256')
      .update(
        JSON.stringify({
          serverUrl: context.serverUrl,
          scope: scope(),
          allowedProjects: server?.allowedProjects?.toSorted() ?? null,
        }),
      )
      .digest('hex'),
  });

  if (cursor && (!query.jobId || query.projectId)) assertCursor(cursor, binding());

  if (
    cursor &&
    (cursor.command !== 'agent.list' ||
      cursor.server !== context.server ||
      cursor.count !== query.count)
  )
    throw new DomainError('USAGE_ERROR', 'Cursor belongs to another agent query', 2);

  const session = await openReadSession(context, signal);

  try {
    const budget = { deadline: context.deadline };
    const policy = new ProjectPolicy(session.reader, server?.allowedProjects, budget);

    if (query.jobId) {
      const job = await session.reader.getJob({ id: query.jobId }, budget);

      if (job.state === 'unavailable') throw job.error;

      if (query.projectId && query.projectId !== job.value.projectId)
        throw new DomainError('CONTEXT_MISMATCH', 'Selected agent job belongs to another project');

      query.projectId = job.value.projectId;
      await policy.assert(query.projectId);
    } else if (query.projectId) {
      await policy.project(query.projectId);
      await policy.assert(query.projectId);
    }

    if (cursor) assertCursor(cursor, binding());

    const read = list
      ? await session.reader.listAgents(query, budget)
      : await session.reader.getAgent({ ...scope(), id: parsed.positional! }, budget);

    if (read.state === 'unavailable') throw read.error;

    const limitations = [...read.provenance.limitations];

    const note = (code: string, message: string) => {
      if (!limitations.some((l) => l.code === code))
        limitations.push({ code, message, source: 'agent' });
    };

    const page = list ? (read.value as EvidencePage<Agent>) : null;
    const agents = page ? page.items : [read.value as Agent];

    for (const agent of agents) {
      if (!agent.activeRun) continue;

      try {
        await policy.assert(agent.activeRun.projectId);
      } catch (error) {
        if (error instanceof DomainError && error.code === 'INTERRUPTED') throw error;

        agent.activeRun = null;
        agent.activeRunState = 'unavailable';
        note(
          'ACTIVE_RUN_SCOPE_UNVERIFIED',
          'The active execution pointer could not be admitted by current project policy',
        );
      }
    }

    if (session.nativeVersion !== '1.5.0')
      note('UNVERIFIED_VERSION', 'Native version has not been release-certified');

    let token: string | null = null;

    if (page) {
      note(
        'BEST_EFFORT_PAGINATION',
        'Agent state and compatibility can change between page observations',
      );
      const expiresAt = cursor?.expiresAt ?? now + 1800000;
      const continuationNow = Date.now();

      if (page.position !== null) {
        if (expiresAt <= continuationNow)
          note('CURSOR_EXPIRED', 'Agent continuation expired during acquisition');
        else
          token = encodeCursor(
            { ...binding(), version: 1, position: page.position, expiresAt },
            continuationNow,
          );
      }
    }

    const selection = {
      projectId: query.projectId ?? null,
      jobId: query.jobId ?? null,
      poolId: query.poolId ?? null,
      meaning:
        query.jobId || query.projectId ? 'compatible_agents' : list ? 'pool_agents' : 'exact_agent',
    };
    const output = response(
      parsed.descriptor.name,
      page
        ? {
            agents,
            selection: {
              ...selection,
              pageSize: query.count,
              providerReturned: page.providerReturned,
              position: query.start,
              scanLimit: query.scanLimit,
              consistency: 'best_effort_offset',
            },
            page: {
              returned: agents.length,
              total: null,
              totalKind: 'unknown',
              hasMore: page.hasMore,
              cursor: token,
            },
            ...(!agents.length
              ? { emptyReason: 'No agents returned; bounded scope exhaustion is unverified' }
              : {}),
          }
        : { agent: agents[0], selection },
    );

    output.context = {
      server: context.server!,
      ...(query.projectId ? { project: query.projectId } : {}),
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
      limitations.some(
        (l: Limitation) => !['BEST_EFFORT_PAGINATION', 'UNVERIFIED_VERSION'].includes(l.code),
      )
    ) {
      output.status = 'partial';
      output.meta.complete = false;
    }

    const scopeFlags = [
      '--server',
      context.server!,
      ...(query.projectId ? [`--project=${query.projectId}`] : []),
      ...(query.jobId ? [`--job=${query.jobId}`] : []),
      ...(query.poolId !== undefined ? [`--pool=${query.poolId}`] : []),
    ];

    if (!parsed.flags['no-hints']) {
      if (token)
        output.next = [
          {
            reason: 'Continue this bounded agent scope',
            argv: [
              'teamcity-axi',
              'agent',
              'list',
              ...scopeFlags,
              '--limit',
              String(query.count),
              '--cursor',
              token,
            ],
          },
        ];
      else if (page && agents.length)
        output.next = agents.slice(0, 2).map((agent) => ({
          reason: 'Observe this exact agent in the same scope',
          argv: ['teamcity-axi', 'agent', 'view', agent.id, ...scopeFlags],
        }));
      else if (!page && agents[0]?.activeRun) {
        const run = agents[0].activeRun;

        output.next = [
          {
            reason: 'Observe this reported active execution',
            argv: [
              'teamcity-axi',
              'run',
              'view',
              run.id,
              '--server',
              context.server!,
              `--job=${run.jobId}`,
              `--project=${run.projectId}`,
            ],
          },
        ];
      }
    }

    if (parsed.flags['require-complete'] && !output.meta.complete) process.exitCode = 1;

    return output;
  } finally {
    await session.transport.dispose();
  }
}
