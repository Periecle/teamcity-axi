import { DomainError } from '../domain/errors.js';
import type { Limitation } from '../domain/response.js';
import type { EvidencePage, Job, JobQuery } from '../domain/teamcity.js';
import { sanitizeText } from '../output/sanitize.js';
import { nextPosition } from './continuation.js';
import { apiPath, literal } from './locator.js';
import { identity, object } from './run.js';

export const jobFields = 'id,name,projectId,paused';

function invalid(): never {
  throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid safe job metadata');
}

export function normalizeJob(value: unknown, secrets: readonly string[]): Job {
  const dto = object(value);
  const id = identity(dto.id),
    projectId = identity(dto.projectId);

  if (typeof dto.name !== 'string' || (dto.paused !== undefined && typeof dto.paused !== 'boolean'))
    invalid();

  return {
    id,
    name: sanitizeText(dto.name, secrets),
    projectId,
    paused: typeof dto.paused === 'boolean' ? dto.paused : null,
  };
}

export function jobLimitations(job: Job): Limitation[] {
  return job.paused === null
    ? [
        {
          code: 'JOB_PAUSED_UNAVAILABLE',
          message: 'The selected job did not supply its paused state',
          source: 'job',
        },
      ]
    : [];
}

export function jobRequest(query: JobQuery) {
  const projectId = query.projectId;

  if (
    typeof projectId !== 'string' ||
    !projectId ||
    projectId.length > 256 ||
    !projectId.isWellFormed() ||
    /[\u0000-\u001f\u007f]/.test(projectId) ||
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
    throw new DomainError('USAGE_ERROR', 'Invalid bounded job query', 2);
  const resource = 'buildTypes';
  const filters = [`project:(id:${literal(projectId)})`];
  const fields = `count,nextHref,buildType(${jobFields})`;

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

export function normalizeJobPage(
  value: unknown,
  query: JobQuery,
  serverUrl: string,
  secrets: readonly string[],
): EvidencePage<Job> {
  const dto = object(value);

  if (
    !Array.isArray(dto.buildType) ||
    dto.buildType.length > query.count ||
    !Number.isSafeInteger(dto.count) ||
    dto.count !== dto.buildType.length
  )
    invalid();
  const ids = new Set<string>();
  const limitations: Limitation[] = [];
  const items = dto.buildType.map((value) => {
    const job = normalizeJob(value, secrets);

    if (job.projectId !== query.projectId)
      throw new DomainError('CONTEXT_MISMATCH', 'Job page contains another project');
    if (ids.has(job.id)) invalid();
    ids.add(job.id);
    if (job.paused === null && !limitations.some((l) => l.code === 'JOB_PAUSED_UNAVAILABLE'))
      limitations.push(...jobLimitations(job));

    return job;
  });
  let position: number | null = null,
    hasMore: boolean | null = null;

  if (dto.nextHref !== undefined) {
    try {
      if (typeof dto.nextHref !== 'string' || !dto.nextHref) invalid();
      position = nextPosition(dto.nextHref, { ...jobRequest(query), ...query, serverUrl });
      hasMore = true;
    } catch {
      limitations.push({
        code: 'UNSAFE_CONTINUATION',
        message: 'Useful jobs retained; unsafe continuation was not followed',
        source: 'job',
      });
    }
  } else
    limitations.push({
      code: 'SCAN_COVERAGE_UNKNOWN',
      message: 'No continuation returned; bounded lookup does not prove job collection exhaustion',
      source: 'job',
    });

  return { items, providerReturned: dto.buildType.length, position, hasMore, limitations };
}
