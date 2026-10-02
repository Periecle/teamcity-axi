import { DomainError } from '../domain/errors.js';
import type { RunQuery, RunPage } from '../domain/teamcity.js';
import { object, normalizeRun } from './run.js';
import { idCondition, branchCondition } from './locator.js';
import { nextPosition } from './continuation.js';
import type { ContinuationRequest } from './continuation.js';

function date(value: string): string {
  if (
    !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.000Z$/.test(value) ||
    !Number.isFinite(Date.parse(value)) ||
    new Date(value).toISOString() !== value
  )
    throw new DomainError(
      'USAGE_ERROR',
      'Finish-time queries require canonical whole-second timestamps',
      2,
    );

  return value.slice(0, 19).replaceAll('-', '').replaceAll(':', '') + '+0000';
}

export function runFilters(query: RunQuery): string[] {
  if (!query.jobId && !query.projectId)
    throw new DomainError('CONTEXT_REQUIRED', 'Run list requires a job or project', 2);
  if (
    !Number.isInteger(query.count) ||
    query.count < 1 ||
    query.count > 100 ||
    !Number.isInteger(query.scanLimit) ||
    query.scanLimit < 1 ||
    query.scanLimit > 5000 ||
    !Number.isInteger(query.start) ||
    query.start < 0 ||
    query.start >= query.scanLimit
  )
    throw new DomainError('USAGE_ERROR', 'Invalid bounded page request', 2);
  if (query.revision && !query.vcsRootId)
    throw new DomainError('CONTEXT_REQUIRED', 'Exact revision filtering requires a VCS root', 2);
  const filters = [
    'defaultFilter:false',
    ...(query.jobId ? [`buildType:${idCondition(query.jobId)}`] : []),
    ...(query.projectId ? [`project:${idCondition(query.projectId)}`] : []),
    query.branch === undefined ? 'branch:(default:any)' : `branch:${branchCondition(query.branch)}`,
  ];

  if (query.state !== undefined) {
    if (!['queued', 'running', 'finished'].includes(query.state))
      throw new DomainError('USAGE_ERROR', 'Invalid lifecycle filter', 2);
    filters.push(`state:${query.state}`);
  }

  if (query.result !== undefined) {
    if (!['success', 'failure', 'error'].includes(query.result))
      throw new DomainError(
        'DEPENDENCY_UNSUPPORTED',
        'Result filter requires separately verified explicit outcome metadata',
      );
    filters.push(`status:${query.result.toUpperCase()}`);
  }

  if (query.window) {
    if (
      query.state !== 'finished' ||
      Date.parse(query.window.since) > Date.parse(query.window.until)
    )
      throw new DomainError(
        'USAGE_ERROR',
        'Finish-time window requires ordered finished-run semantics',
        2,
      );
    filters.push(
      `finishDate:(date:${date(query.window.since)},condition:after)`,
      `finishDate:(date:${date(query.window.until)},condition:before)`,
    );
  }

  return filters;
}

export function normalizeRunPage(
  input: unknown,
  query: RunQuery,
  request: ContinuationRequest,
  secrets: readonly string[],
): RunPage {
  const dto = object(input);

  if (
    !Array.isArray(dto.build) ||
    dto.build.length > query.count ||
    !Number.isSafeInteger(dto.count) ||
    dto.count !== dto.build.length
  )
    throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid bounded run collection');
  const page: RunPage = {
    runs: [],
    providerReturned: dto.build.length,
    position: null,
    hasMore: null,
    limitations: [],
  };
  const ids = new Set<string>();

  for (const value of dto.build) {
    const { run, projectId, limitations } = normalizeRun(value, request.serverUrl, secrets);

    if (ids.has(run.id))
      throw new DomainError(
        'UPSTREAM_SCHEMA_MISMATCH',
        'Provider repeated an execution in one page',
      );
    ids.add(run.id);
    if (
      (query.jobId && run.jobId !== query.jobId) ||
      (query.projectId && projectId !== query.projectId) ||
      (query.branch !== undefined && run.branch !== query.branch) ||
      (query.state && run.state !== query.state) ||
      (query.result && run.result !== query.result)
    )
      throw new DomainError(
        'CONTEXT_MISMATCH',
        'Provider returned an execution outside the declared list scope',
      );
    if (query.allowedProjects && (!projectId || !query.allowedProjects.includes(projectId)))
      throw new DomainError('POLICY_DENIED', 'Run project is outside the verified trusted scope');
    page.limitations.push(...limitations);

    if (query.window) {
      if (!run.finishedAt) {
        page.limitations.push({
          code: 'FINISH_TIME_UNVERIFIED',
          message: 'Candidate omitted a valid finish timestamp',
          source: 'run',
          runId: run.id,
        });
        continue;
      }

      if (
        Date.parse(run.finishedAt) <= Date.parse(query.window.since) ||
        Date.parse(run.finishedAt) >= Date.parse(query.window.until)
      ) {
        page.limitations.push({
          code: 'FINISH_TIME_OUTSIDE_WINDOW',
          message: 'Candidate excluded after finish-time verification',
          source: 'run',
          runId: run.id,
        });
        continue;
      }
    }

    if (query.revision) {
      const roots = run.revisions?.filter((r) => r.vcsRootId === query.vcsRootId);

      if (!roots?.length) {
        page.limitations.push({
          code: 'REVISION_UNVERIFIED',
          message: 'Candidate lacks the requested VCS root revision',
          source: 'run',
          runId: run.id,
        });
        continue;
      }

      if (roots.some((r) => r.revision !== query.revision)) continue;
    }

    page.runs.push(run);
  }

  if (dto.nextHref !== undefined) {
    try {
      if (typeof dto.nextHref !== 'string' || !dto.nextHref) throw new Error();
      page.position = nextPosition(dto.nextHref, request);
      page.hasMore = true;
    } catch {
      page.limitations.push({
        code: 'UNSAFE_CONTINUATION',
        message: 'Provider continuation could not be safely reconstructed; search stopped',
        source: 'run',
      });
    }
  } else
    page.limitations.push({
      code: 'SCAN_COVERAGE_UNKNOWN',
      message: 'No continuation was supplied; bounded scan exhaustion is unverified',
      source: 'run',
    });

  return page;
}
