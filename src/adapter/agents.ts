import { DomainError } from '../domain/errors.js';
import type { Limitation } from '../domain/response.js';
import type { Agent, AgentQuery, AgentScope, EvidencePage } from '../domain/teamcity.js';
import { sanitizeText } from '../output/sanitize.js';
import { nextPosition } from './continuation.js';
import { apiPath, idCondition } from './locator.js';
import { identity, object } from './run.js';

export const agentFields =
  'id,name,connected,enabled,authorized,pool(id,name),build(id,buildTypeId,buildType(id,projectId))';

function invalid(): never {
  throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid safe agent metadata');
}

export function validateAgentId(value: string, pool = false): void {
  if (
    typeof value !== 'string' ||
    !(pool ? /^(0|[1-9]\d*)$/ : /^[1-9]\d*$/).test(value) ||
    !Number.isSafeInteger(Number(value))
  )
    throw new DomainError('USAGE_ERROR', 'Invalid exact numeric agent or pool identity', 2);
}

export function agentFilters(scope: AgentScope): string[] {
  for (const value of [scope.projectId, scope.jobId])
    if (
      value !== undefined &&
      (typeof value !== 'string' ||
        !value ||
        value.length > 256 ||
        !value.isWellFormed() ||
        /[\u0000-\u001f\u007f-\u009f\u202a-\u202e\u2066-\u2069]/.test(value))
    )
      throw new DomainError('USAGE_ERROR', 'Invalid agent scope identity', 2);
  if (scope.poolId !== undefined) validateAgentId(scope.poolId, true);

  return [
    ...(scope.poolId !== undefined ? [`pool:(id:${scope.poolId})`] : []),
    ...(scope.jobId
      ? [`compatible:(buildType:${idCondition(scope.jobId)})`]
      : scope.projectId
        ? [`compatible:(buildType:(project:${idCondition(scope.projectId)}))`]
        : []),
  ];
}

export function agentRequest(query: AgentQuery) {
  const filters = [...agentFilters(query), 'defaultFilter:false'];

  if (!query.jobId && !query.projectId && query.poolId === undefined)
    throw new DomainError('CONTEXT_REQUIRED', 'Agent list requires a pool, job or project', 2);
  if (
    !Number.isInteger(query.count) ||
    query.count < 1 ||
    query.count > 100 ||
    !Number.isInteger(query.start) ||
    query.start < 0 ||
    query.start >= query.scanLimit ||
    !Number.isInteger(query.scanLimit) ||
    query.scanLimit < 1 ||
    query.scanLimit > 5000
  )
    throw new DomainError('USAGE_ERROR', 'Invalid bounded agent query', 2);
  const resource = 'agents';
  const fields = `count,nextHref,agent(${agentFields})`;

  return {
    resource,
    filters,
    fields,
    path: apiPath(
      resource,
      [
        ...filters,
        `count:${query.count}`,
        `start:${query.start}`,
        `lookupLimit:${query.scanLimit}`,
      ],
      fields,
    ),
  };
}

export function normalizeAgent(
  value: unknown,
  scope: AgentScope,
  secrets: readonly string[],
): { agent: Agent; limitations: Limitation[] } {
  const dto = object(value);
  const id = identity(dto.id, true);
  const limitations: Limitation[] = [];
  const note = (code: string, message: string) =>
    limitations.push({ code, message, source: 'agent' });

  if (typeof dto.name !== 'string' || !dto.name) invalid();

  const state = (key: 'connected' | 'enabled' | 'authorized'): boolean | null => {
    if (dto[key] === undefined || dto[key] === null) {
      if (!limitations.some((l) => l.code === 'AGENT_STATUS_UNAVAILABLE'))
        note('AGENT_STATUS_UNAVAILABLE', 'Some agent availability states were not reported');

      return null;
    }

    if (typeof dto[key] !== 'boolean') invalid();

    return dto[key];
  };

  let pool: Agent['pool'] = null;

  if (dto.pool !== undefined && dto.pool !== null) {
    const value = object(dto.pool);

    if (typeof value.id === 'number' && (!Number.isSafeInteger(value.id) || value.id < 0))
      invalid();
    const poolId = identity(typeof value.id === 'number' ? String(value.id) : value.id);

    if (!/^(0|[1-9]\d*)$/.test(poolId) || !Number.isSafeInteger(Number(poolId))) invalid();
    if (value.name !== undefined && value.name !== null && typeof value.name !== 'string')
      invalid();
    pool = {
      id: poolId,
      name: typeof value.name === 'string' ? sanitizeText(value.name, secrets) : null,
    };
  } else note('AGENT_POOL_UNAVAILABLE', 'The agent pool was not reported');
  if (scope.poolId !== undefined && pool?.id !== scope.poolId)
    throw new DomainError('CONTEXT_MISMATCH', 'Agent belongs to another selected pool');
  let activeRun: Agent['activeRun'] = null;
  let activeRunState: Agent['activeRunState'] = 'not_reported';

  if (dto.build === null) activeRunState = 'idle';
  else if (dto.build !== undefined) {
    const build = object(dto.build);
    const jobId = identity(build.buildTypeId);
    const type = object(build.buildType);

    if (identity(type.id) !== jobId) invalid();
    activeRun = { id: identity(build.id, true), jobId, projectId: identity(type.projectId) };
    activeRunState = 'reported';

    if (scope.projectId && activeRun.projectId !== scope.projectId) {
      activeRun = null;
      activeRunState = 'unavailable';
      note(
        'ACTIVE_RUN_OUTSIDE_SCOPE',
        'The active execution pointer is outside the selected project',
      );
    }
  } else
    note('ACTIVE_RUN_UNREPORTED', 'The active execution was not reported; idleness is unverified');

  return {
    agent: {
      id,
      name: sanitizeText(dto.name, secrets),
      connected: state('connected'),
      enabled: state('enabled'),
      authorized: state('authorized'),
      pool,
      activeRun,
      activeRunState,
    },
    limitations,
  };
}

export function normalizeAgentPage(
  value: unknown,
  query: AgentQuery,
  serverUrl: string,
  secrets: readonly string[],
): EvidencePage<Agent> {
  const dto = object(value);

  if (
    !Array.isArray(dto.agent) ||
    dto.agent.length > query.count ||
    !Number.isSafeInteger(dto.count) ||
    dto.count !== dto.agent.length
  )
    invalid();
  const ids = new Set<string>();
  const limitations: Limitation[] = [];

  const note = (limitation: Limitation) => {
    if (!limitations.some((l) => l.code === limitation.code)) limitations.push(limitation);
  };

  const items = dto.agent.map((value) => {
    const { agent, limitations: notes } = normalizeAgent(value, query, secrets);

    if (ids.has(agent.id)) invalid();
    ids.add(agent.id);
    notes.forEach(note);

    return agent;
  });
  let position: number | null = null;

  if (dto.nextHref !== undefined) {
    try {
      if (typeof dto.nextHref !== 'string') invalid();
      position = nextPosition(dto.nextHref, { ...agentRequest(query), ...query, serverUrl });
    } catch {
      note({
        code: 'UNSAFE_CONTINUATION',
        message: 'Agent continuation could not be safely reconstructed',
        source: 'agent',
      });
    }
  } else
    note({
      code: 'SCAN_COVERAGE_UNKNOWN',
      message: 'Bounded agent collection exhaustion is unverified',
      source: 'agent',
    });

  return {
    items,
    providerReturned: items.length,
    position,
    hasMore: position === null ? null : true,
    limitations,
  };
}
