import { DomainError } from '../domain/errors.js';
import type { Limitation } from '../domain/response.js';
import type { EvidencePage, QueueItem, QueueQuery } from '../domain/teamcity.js';
import { sanitizeText } from '../output/sanitize.js';
import { nextPosition } from './continuation.js';
import { apiPath, idCondition } from './locator.js';
import { identity, object, timestamp } from './run.js';

export const queueFields =
  'id,buildTypeId,state,branchName,queuedDate,waitReason,buildType(id,projectId)';

function invalid(): never {
  throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid scoped queue metadata');
}

export function queueRequest(query: QueueQuery) {
  if (!query.jobId && !query.projectId)
    throw new DomainError('CONTEXT_REQUIRED', 'Queue list requires a job or project', 2);

  for (const id of [query.jobId, query.projectId])
    if (
      id !== undefined &&
      (typeof id !== 'string' ||
        !id ||
        id.length > 256 ||
        !id.isWellFormed() ||
        /[\u0000-\u001f\u007f-\u009f\u202a-\u202e\u2066-\u2069]/.test(id))
    )
      throw new DomainError('USAGE_ERROR', 'Invalid queue scope identity', 2);

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
    throw new DomainError('USAGE_ERROR', 'Invalid bounded queue query', 2);

  const resource = 'buildQueue';
  const filters = [
    ...(query.jobId ? [`buildType:${idCondition(query.jobId)}`] : []),
    ...(query.projectId ? [`project:${idCondition(query.projectId)}`] : []),
  ];
  const fields = `count,nextHref,build(${queueFields})`;

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

export function normalizeQueuePage(
  value: unknown,
  query: QueueQuery,
  serverUrl: string,
  secrets: readonly string[],
): EvidencePage<QueueItem> {
  const dto = object(value);

  if (
    !Array.isArray(dto.build) ||
    dto.build.length > query.count ||
    !Number.isSafeInteger(dto.count) ||
    dto.count !== dto.build.length
  )
    invalid();

  const ids = new Set<string>(),
    limitations: Limitation[] = [];

  const note = (code: string, message: string) => {
    if (!limitations.some((l) => l.code === code))
      limitations.push({ code, message, source: 'queue' });
  };

  const items = dto.build.map((value) => {
    const build = object(value),
      id = identity(build.id, true),
      jobId = identity(build.buildTypeId);
    const buildType = object(build.buildType),
      projectId = identity(buildType.projectId);

    if (identity(buildType.id) !== jobId) invalid();

    if (
      (query.jobId && jobId !== query.jobId) ||
      (query.projectId && projectId !== query.projectId)
    )
      throw new DomainError(
        'CONTEXT_MISMATCH',
        'Queued execution belongs to another selected scope',
      );

    if (ids.has(id)) invalid();

    ids.add(id);

    if (
      typeof build.state !== 'string' ||
      (build.branchName !== undefined &&
        build.branchName !== null &&
        typeof build.branchName !== 'string') ||
      (build.waitReason !== undefined &&
        build.waitReason !== null &&
        typeof build.waitReason !== 'string')
    )
      invalid();

    const knownState = ['queued', 'running', 'finished'].includes(build.state);
    const state: QueueItem['state'] = knownState ? (build.state as QueueItem['state']) : 'unknown';

    if (!knownState) note('UNKNOWN_QUEUE_STATE', 'The server supplied an unknown queue lifecycle');
    else if (state !== 'queued')
      note(
        'QUEUE_STATE_CHANGED',
        'An observed queue item is no longer queued; this page is provisional',
      );

    const times: Limitation[] = [];
    const queuedAt = timestamp(build.queuedDate, 'queued', times);

    for (const limitation of times) note(limitation.code, limitation.message);

    return {
      id,
      jobId,
      state,
      branch: typeof build.branchName === 'string' ? sanitizeText(build.branchName, secrets) : null,
      queuedAt,
      waitReason:
        typeof build.waitReason === 'string' && build.waitReason
          ? sanitizeText(build.waitReason, secrets)
          : null,
      ...(!knownState ? { rawState: sanitizeText(build.state, secrets) } : {}),
    };
  });
  let position: number | null = null,
    hasMore: boolean | null = null;

  if (dto.nextHref !== undefined) {
    try {
      if (typeof dto.nextHref !== 'string' || !dto.nextHref) invalid();

      position = nextPosition(dto.nextHref, { ...queueRequest(query), ...query, serverUrl });
      hasMore = true;
    } catch {
      note(
        'UNSAFE_CONTINUATION',
        'Useful queue items retained; unsafe continuation was not followed',
      );
    }
  } else
    note(
      'SCAN_COVERAGE_UNKNOWN',
      'No continuation returned; bounded lookup does not prove queue exhaustion',
    );

  return { items, providerReturned: dto.build.length, position, hasMore, limitations };
}
