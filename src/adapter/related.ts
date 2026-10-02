import { DomainError } from '../domain/errors.js';
import type { Limitation } from '../domain/response.js';
import type { Change, EvidencePage, RelatedQuery, ScopedRun } from '../domain/teamcity.js';
import { sanitizeText } from '../output/sanitize.js';
import { apiPath } from './locator.js';
import { nextPosition } from './continuation.js';
import { identity, normalizeRun, object, timestamp } from './run.js';

export const changeFields = 'id,version,comment,date,vcsRootInstance(vcs-root-id)';

function invalid(): never {
  throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid bounded related-evidence response');
}

export function relatedRequest(
  kind: 'changes' | 'dependencies',
  query: RelatedQuery,
  runFields = '',
) {
  const runId = identity(query.runId, true);

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
    throw new DomainError('USAGE_ERROR', 'Invalid bounded related-evidence query', 2);
  const resource = kind === 'changes' ? 'changes' : 'builds';
  const filters =
    kind === 'changes'
      ? [`build:(id:${runId})`]
      : [`snapshotDependency:(to:(id:${runId}),recursive:false)`, 'defaultFilter:false'];
  const fields =
    kind === 'changes'
      ? `count,nextHref,change(${changeFields}${query.files ? ',files(count,file(file,changeType))' : ''})`
      : `count,nextHref,build(${runFields})`;

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

function continuation(
  dto: Record<string, unknown>,
  request: ReturnType<typeof relatedRequest>,
  query: RelatedQuery,
  serverUrl: string,
  source: string,
  limitations: Limitation[],
) {
  let position: number | null = null,
    hasMore: boolean | null = null;

  if (dto.nextHref !== undefined) {
    try {
      if (typeof dto.nextHref !== 'string' || !dto.nextHref) invalid();
      position = nextPosition(dto.nextHref, { ...request, ...query, serverUrl });
      hasMore = true;
    } catch {
      limitations.push({
        code: 'UNSAFE_CONTINUATION',
        message: 'Useful rows retained; unsafe continuation was not followed',
        source,
        runId: query.runId,
      });
    }
  } else
    limitations.push({
      code: 'SCAN_COVERAGE_UNKNOWN',
      message: 'No continuation returned; bounded lookup does not prove collection exhaustion',
      source,
      runId: query.runId,
    });

  return { position, hasMore };
}

function rows(dto: Record<string, unknown>, key: string, count: number): unknown[] {
  const value = dto[key];

  if (
    !Array.isArray(value) ||
    value.length > count ||
    !Number.isSafeInteger(dto.count) ||
    dto.count !== value.length
  )
    invalid();

  return value;
}

export function normalizeChangePage(
  value: unknown,
  query: RelatedQuery,
  serverUrl: string,
  secrets: readonly string[],
): EvidencePage<Change> {
  const dto = object(value),
    changes = rows(dto, 'change', query.count),
    limitations: Limitation[] = [];
  const ids = new Set<string>(),
    items: Change[] = [];

  for (const value of changes) {
    const change = object(value),
      id = identity(change.id, true);

    if (ids.has(id)) invalid();
    ids.add(id);
    if (typeof change.comment !== 'string') invalid();
    const version = identity(change.version);

    if (change.vcsRootInstance === null || change.vcsRootInstance === undefined) {
      limitations.push({
        code: 'CHANGE_ROOT_UNAVAILABLE',
        message: 'Change omitted because its VCS-root identity is unavailable',
        source: 'changes',
        runId: query.runId,
      });
      continue;
    }

    const timeLimitations: Limitation[] = [];
    const changeTimestamp = timestamp(change.date, 'change', timeLimitations);

    limitations.push(
      ...timeLimitations.map((l) => ({ ...l, source: 'changes', runId: query.runId })),
    );
    const item: Change = {
      id,
      version,
      vcsRootId: identity(object(change.vcsRootInstance)['vcs-root-id']),
      message: sanitizeText(change.comment, secrets),
      timestamp: changeTimestamp,
    };

    if (query.files) {
      if (change.files === undefined)
        limitations.push({
          code: 'CHANGE_FILES_UNAVAILABLE',
          message: 'Requested changed-file evidence was omitted',
          source: 'changes',
          runId: query.runId,
        });
      else {
        const files = object(change.files),
          entries = files.file;

        if (
          !Array.isArray(entries) ||
          !Number.isSafeInteger(files.count) ||
          files.count !== entries.length
        )
          invalid();
        const names: string[] = [];

        for (const value of entries.slice(0, 100)) {
          const file = object(value);

          if (typeof file.file !== 'string' || !file.file || file.file.length > 4096) invalid();
          const name = sanitizeText(file.file, secrets);

          if (Array.from(name).length > 4096) invalid();
          names.push(name);
        }

        item.files = names;
        item.fileCoverage = {
          returned: names.length,
          providerReturned: entries.length,
          omitted: entries.length - names.length,
        };
        if (entries.length > 100)
          limitations.push({
            code: 'CHANGE_FILE_LIMIT',
            message: 'Changed-file list reached its hundred-name ceiling',
            source: 'changes',
            runId: query.runId,
          });
      }
    }

    items.push(item);
  }

  return {
    items,
    providerReturned: changes.length,
    limitations,
    ...continuation(
      dto,
      relatedRequest('changes', query),
      query,
      serverUrl,
      'changes',
      limitations,
    ),
  };
}

export function normalizeDependencyPage(
  value: unknown,
  query: RelatedQuery,
  serverUrl: string,
  runFields: string,
  secrets: readonly string[],
): EvidencePage<ScopedRun> {
  const dto = object(value),
    builds = rows(dto, 'build', query.count),
    limitations: Limitation[] = [];
  const ids = new Set<string>();
  const items = builds.map((value) => {
    const normalized = normalizeRun(value, serverUrl, secrets);

    if (ids.has(normalized.run.id)) invalid();
    ids.add(normalized.run.id);
    limitations.push(
      ...normalized.limitations.map((limitation) => ({
        ...limitation,
        runId: limitation.runId ?? normalized.run.id,
      })),
    );

    return { run: normalized.run, projectId: normalized.projectId };
  });

  return {
    items,
    providerReturned: builds.length,
    limitations,
    ...continuation(
      dto,
      relatedRequest('dependencies', query, runFields),
      query,
      serverUrl,
      'dependencies',
      limitations,
    ),
  };
}

export function normalizeDependencyCount(value: unknown, runId: string): number {
  const dto = object(value);

  if (identity(dto.id, true) !== runId)
    throw new DomainError('CONTEXT_MISMATCH', 'Dependency count belongs to another execution');
  const count = object(dto['snapshot-dependencies']).count;

  if (!Number.isSafeInteger(count) || Number(count) < 0) invalid();

  return Number(count);
}
