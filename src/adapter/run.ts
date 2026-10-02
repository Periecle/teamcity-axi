import { DomainError } from '../domain/errors.js';
import type { Run } from '../domain/teamcity.js';
import type { Limitation } from '../domain/response.js';
import { sanitizeText } from '../output/sanitize.js';

function invalid(message = 'Run detail does not match the supported DTO contract'): never {
  throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', message);
}

export function object(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) invalid();

  return value as Record<string, unknown>;
}

export function identity(value: unknown, numeric = false): string {
  if (typeof value === 'number') {
    if (!Number.isSafeInteger(value) || value <= 0) invalid('Unsafe upstream numeric identity');

    value = String(value);
  }

  if (
    typeof value !== 'string' ||
    value.length < 1 ||
    value.length > 256 ||
    !value.isWellFormed() ||
    /[\u0000-\u001f\u007f-\u009f\u202a-\u202e\u2066-\u2069]/.test(value)
  )
    invalid('Missing or invalid upstream identity');

  if (numeric && (!/^[1-9]\d*$/.test(value) || !Number.isSafeInteger(Number(value))))
    invalid('Unsafe upstream run identity');

  return value;
}

function text(value: unknown): string | undefined {
  if (value === undefined) return undefined;

  if (typeof value !== 'string') invalid();

  return value;
}

function boolean(value: unknown): boolean | null {
  if (value === undefined || value === null) return null;

  if (typeof value !== 'boolean') invalid();

  return value;
}

export function timestamp(value: unknown, field: string, limitations: Limitation[]): string | null {
  if (value === undefined || value === null || value === '') return null;

  if (typeof value !== 'string') invalid();

  const m = /^(\d{4})(\d\d)(\d\d)T(\d\d)(\d\d)(\d\d)([+-])(\d\d)(\d\d)$/.exec(value);

  if (m) {
    const [, y, month, day, hour, minute, second, sign, zh, zm] = m;
    const valid =
      Number(y) >= 100 &&
      Number(month) >= 1 &&
      Number(month) <= 12 &&
      Number(day) >= 1 &&
      Number(day) <= new Date(Date.UTC(Number(y), Number(month), 0)).getUTCDate() &&
      Number(hour) < 24 &&
      Number(minute) < 60 &&
      Number(second) < 60 &&
      Number(zh) <= 23 &&
      Number(zm) < 60;

    if (valid) {
      const epoch =
        Date.UTC(
          Number(y),
          Number(month) - 1,
          Number(day),
          Number(hour),
          Number(minute),
          Number(second),
        ) -
        (sign === '-' ? -1 : 1) * (Number(zh) * 60 + Number(zm)) * 60000;

      return new Date(epoch).toISOString();
    }
  }

  limitations.push({
    code: 'INVALID_TIMESTAMP',
    message: `The server supplied an invalid ${field} timestamp`,
    source: 'run',
  });

  return null;
}

export function normalizeRun(
  input: unknown,
  serverUrl: string,
  secrets: readonly string[] = [],
): { run: Run; projectId: string | null; limitations: Limitation[] } {
  const dto = object(input);
  const limitations: Limitation[] = [];
  const id = identity(dto.id, true);
  const jobId = identity(dto.buildTypeId);
  const buildType = dto.buildType === undefined ? undefined : object(dto.buildType);

  if (buildType && identity(buildType.id) !== jobId) invalid('Conflicting run job identities');

  const projectId = buildType?.projectId === undefined ? null : identity(buildType.projectId);

  if (
    typeof dto.state !== 'string' ||
    (typeof dto.status !== 'string' && !(dto.state === 'queued' && dto.status === undefined))
  )
    invalid('Run lifecycle and result metadata are required');

  const state = (
    ['queued', 'running', 'finished'].includes(dto.state) ? dto.state : 'unknown'
  ) as Run['state'];
  const results = new Map<string, Run['result']>([
    ['SUCCESS', 'success'],
    ['FAILURE', 'failure'],
    ['ERROR', 'error'],
  ]);
  const result =
    typeof dto.status === 'string' ? (results.get(dto.status) ?? 'unknown') : 'unknown';

  if (state === 'unknown')
    limitations.push({
      code: 'UNKNOWN_LIFECYCLE',
      message: 'The upstream lifecycle is unknown to this adapter',
      source: 'run',
      runId: id,
    });

  if (result === 'unknown')
    limitations.push({
      code: dto.status === undefined ? 'RESULT_UNAVAILABLE' : 'UNKNOWN_RESULT',
      message:
        dto.status === undefined
          ? 'The queued execution has no reported result'
          : 'The upstream result is unknown to this adapter',
      source: 'run',
      runId: id,
    });

  const revisions: NonNullable<Run['revisions']> = [];

  if (dto.revisions !== undefined) {
    const collection = object(dto.revisions);

    if (!Array.isArray(collection.revision) || collection.revision.length > 100)
      invalid('Invalid revision collection');

    for (const value of collection.revision) {
      const revision = object(value);
      const root = object(revision['vcs-root-instance']);

      revisions.push({
        vcsRootId: identity(root['vcs-root-id']),
        revision: identity(revision.version),
      });
    }
  } else
    limitations.push({
      code: 'MISSING_REVISION_METADATA',
      message: 'The server omitted revision metadata; revision coverage is unknown',
      source: 'run',
      runId: id,
    });

  const startedAt = timestamp(dto.startDate, 'start', limitations),
    finishedAt = timestamp(dto.finishDate, 'finish', limitations),
    queuedAt = timestamp(dto.queuedDate, 'queue', limitations);
  let webUrl: string | null = null;

  if (dto.webUrl !== undefined) {
    const value = text(dto.webUrl)!;

    try {
      const url = new URL(value);
      const base = new URL(serverUrl);

      if (
        url.origin === base.origin &&
        !url.username &&
        !url.password &&
        url.pathname.startsWith(base.pathname.replace(/\/$/, '') + '/')
      )
        webUrl = url.toString();
      else
        limitations.push({
          code: 'UNSAFE_WEB_URL',
          message: 'Untrusted run web link omitted',
          source: 'run',
          runId: id,
        });
    } catch {
      limitations.push({
        code: 'UNSAFE_WEB_URL',
        message: 'Invalid run web link omitted',
        source: 'run',
        runId: id,
      });
    }
  }

  const run: Run = {
    id,
    jobId,
    state,
    result,
    branch: text(dto.branchName) ?? null,
    personal: boolean(dto.personal),
    composite: boolean(dto.composite),
    startedAt,
    finishedAt,
    queuedAt,
    webUrl,
    ...(dto.revisions !== undefined ? { revisions } : {}),
    rawStatus:
      result === 'unknown' && typeof dto.status === 'string'
        ? sanitizeText(dto.status, secrets).slice(0, 80)
        : null,
  };
  const number = text(dto.number),
    statusText = text(dto.statusText);

  if (number !== undefined) run.number = sanitizeText(number, secrets);

  if (statusText !== undefined) run.statusText = sanitizeText(statusText, secrets);

  if (startedAt && finishedAt) {
    const duration = Date.parse(finishedAt) - Date.parse(startedAt);

    if (duration >= 0) run.durationMs = duration;
    else
      limitations.push({
        code: 'INVALID_DURATION',
        message: 'Finish precedes start; duration omitted',
        source: 'run',
        runId: id,
      });
  }

  return { run, projectId, limitations };
}
