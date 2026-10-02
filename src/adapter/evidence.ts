import { DomainError } from '../domain/errors.js';
import type { EvidencePage, EvidenceQuery, Problem, TestOccurrence } from '../domain/teamcity.js';
import type { Limitation } from '../domain/response.js';
import { sanitizeText } from '../output/sanitize.js';
import { identity, object } from './run.js';
import { apiPath } from './locator.js';
import { nextPosition } from './continuation.js';

export const problemFields = 'id,type,identity,details,build(id)';

export const testFields = 'id,name,status,duration,muted,ignored,details,build(id),test(id)';

export type EvidenceKind = 'problems' | 'tests';

function invalid(message = 'Invalid independent evidence response'): never {
  throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', message);
}

export function occurrenceLocator(kind: EvidenceKind, id: string, runId: string): string {
  const pattern =
    kind === 'problems'
      ? /^build:\(id:([1-9]\d*)\),problem:\(id:([1-9]\d*)\)$/
      : /^build:\(id:([1-9]\d*)\),id:(\d+)$/;
  const matched = pattern.exec(id);

  if (
    !matched ||
    !Number.isSafeInteger(Number(matched[1])) ||
    !Number.isSafeInteger(Number(matched[2]))
  )
    throw new DomainError(
      'USAGE_ERROR',
      'Use an exact supported occurrence ID, not a test definition or raw locator',
      2,
    );
  if (matched[1] !== runId)
    throw new DomainError('CONTEXT_MISMATCH', 'Occurrence ID belongs to a different execution');

  // Structural syntax is wrapper-owned; only validated integer components are copied.
  return kind === 'problems'
    ? `build:(id:${runId}),problem:(id:${matched[2]})`
    : `build:(id:${runId}),id:${matched[2]}`;
}

function occurrence(value: unknown, kind: EvidenceKind, runId: string): Record<string, unknown> {
  const dto = object(value),
    id = identity(dto.id);

  if (identity(object(dto.build).id, true) !== runId)
    throw new DomainError('CONTEXT_MISMATCH', 'Evidence belongs to a different execution');

  try {
    occurrenceLocator(kind, id, runId);
  } catch (error) {
    if (error instanceof DomainError && error.code === 'CONTEXT_MISMATCH') throw error;
    invalid('Unsupported upstream occurrence identity');
  }

  return dto;
}

function text(value: unknown, required = true): string | undefined {
  if (value === undefined && !required) return undefined;
  if (typeof value !== 'string') invalid();

  return value;
}

function flag(value: unknown): boolean | null {
  if (value === undefined || value === null) return null;
  if (typeof value !== 'boolean') invalid();

  return value;
}

export function normalizeProblem(
  value: unknown,
  runId: string,
  secrets: readonly string[],
): Problem {
  const dto = occurrence(value, 'problems', runId);
  const explicitIdentity = text(dto.identity, false);

  return {
    id: identity(dto.id),
    runId,
    type: sanitizeText(identity(dto.type), secrets),
    description: sanitizeText(text(dto.details)!, secrets),
    ...(explicitIdentity !== undefined
      ? { identity: sanitizeText(explicitIdentity, secrets) }
      : {}),
  };
}

export function normalizeTest(
  value: unknown,
  runId: string,
  secrets: readonly string[],
  limitations: Limitation[],
): TestOccurrence {
  const dto = occurrence(value, 'tests', runId),
    status = text(dto.status)!,
    muted = flag(dto.muted),
    ignored = flag(dto.ignored);

  if (
    dto.duration !== undefined &&
    (!Number.isSafeInteger(dto.duration) || Number(dto.duration) < 0)
  )
    invalid();
  const result =
    ignored === true || status === 'IGNORED'
      ? 'ignored'
      : status === 'SUCCESS'
        ? 'success'
        : status === 'FAILURE'
          ? 'failure'
          : 'unknown';

  if (result === 'unknown')
    limitations.push({
      code: 'UNKNOWN_TEST_RESULT',
      message: 'Test occurrence has an unknown outcome',
      source: 'tests',
      runId,
    });
  if (muted === null || ignored === null)
    limitations.push({
      code: 'TEST_FLAGS_UNAVAILABLE',
      message: 'Muted or ignored state was not supplied',
      source: 'tests',
      runId,
    });
  const details = text(dto.details, false);

  return {
    id: identity(dto.id),
    runId,
    name: sanitizeText(text(dto.name)!, secrets),
    result,
    muted,
    ignored,
    durationMs: typeof dto.duration === 'number' ? dto.duration : null,
    ...(dto.test !== undefined ? { testId: identity(object(dto.test).id) } : {}),
    ...(details !== undefined ? { details: sanitizeText(details, secrets) } : {}),
    ...(result === 'unknown'
      ? { rawStatus: Array.from(sanitizeText(status, secrets)).slice(0, 80).join('') }
      : {}),
  };
}

export function evidenceRequest(kind: EvidenceKind, query: EvidenceQuery) {
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
    throw new DomainError('USAGE_ERROR', 'Invalid bounded evidence query', 2);
  if (kind === 'problems' && (query.failed !== undefined || query.muted !== undefined))
    throw new DomainError('USAGE_ERROR', 'Problem pages do not support test filters', 2);
  const filters = [
    `build:(id:${runId})`,
    ...(query.failed ? ['status:FAILURE'] : []),
    ...(query.muted !== undefined ? [`muted:${query.muted}`] : []),
  ];
  const resource = kind === 'problems' ? 'problemOccurrences' : 'testOccurrences';
  const fields = `count,nextHref,${kind === 'problems' ? 'problemOccurrence' : 'testOccurrence'}(${kind === 'problems' ? problemFields : testFields})`;
  const path = apiPath(
    resource,
    [...filters, `count:${query.count}`, `start:${query.start}`, `lookupLimit:${query.scanLimit}`],
    fields,
  );

  return { path, resource, fields, filters };
}

export function normalizeEvidencePage(
  kind: 'problems',
  value: unknown,
  query: EvidenceQuery,
  serverUrl: string,
  secrets: readonly string[],
): EvidencePage<Problem>;

export function normalizeEvidencePage(
  kind: 'tests',
  value: unknown,
  query: EvidenceQuery,
  serverUrl: string,
  secrets: readonly string[],
): EvidencePage<TestOccurrence>;

export function normalizeEvidencePage(
  kind: EvidenceKind,
  value: unknown,
  query: EvidenceQuery,
  serverUrl: string,
  secrets: readonly string[],
): EvidencePage<Problem | TestOccurrence> {
  const dto = object(value),
    rows = dto[kind === 'problems' ? 'problemOccurrence' : 'testOccurrence'];

  if (
    !Array.isArray(rows) ||
    rows.length > query.count ||
    !Number.isSafeInteger(dto.count) ||
    dto.count !== rows.length
  )
    invalid('Invalid bounded occurrence collection');
  const request = evidenceRequest(kind, query),
    limitations: Limitation[] = [],
    ids = new Set<string>();
  const items: (Problem | TestOccurrence)[] = [];

  for (const row of rows) {
    const item =
      kind === 'problems'
        ? normalizeProblem(row, query.runId, secrets)
        : normalizeTest(row, query.runId, secrets, limitations);

    if (ids.has(item.id)) invalid('Duplicate occurrence ID in one page');
    ids.add(item.id);

    if (kind === 'tests') {
      const test = item as TestOccurrence;

      if (
        (query.failed && test.result !== 'failure') ||
        (query.muted !== undefined && test.muted !== null && test.muted !== query.muted)
      )
        throw new DomainError('CONTEXT_MISMATCH', 'Test page does not match declared filters');
      if (query.muted !== undefined && test.muted === null) continue;
    }

    items.push(item);
  }

  let position: number | null = null,
    hasMore: boolean | null = null;

  if (dto.nextHref !== undefined) {
    try {
      if (typeof dto.nextHref !== 'string') invalid();
      position = nextPosition(dto.nextHref, {
        ...request,
        serverUrl,
        count: query.count,
        start: query.start,
        scanLimit: query.scanLimit,
      });
      hasMore = true;
    } catch {
      limitations.push({
        code: 'UNSAFE_CONTINUATION',
        message: 'Occurrence rows retained; unsafe continuation was not followed',
        source: kind,
        runId: query.runId,
      });
    }
  } else
    limitations.push({
      code: 'SCAN_COVERAGE_UNKNOWN',
      message: 'No continuation returned; bounded lookup does not prove complete retained evidence',
      source: kind,
      runId: query.runId,
    });

  return { items, providerReturned: rows.length, position, hasMore, limitations };
}
