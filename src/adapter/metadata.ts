import { createHash } from 'node:crypto';

import { DomainError } from '../domain/errors.js';
import type { AuthenticatedIdentity, LogTail, Project, ServerInfo } from '../domain/teamcity.js';
import { sanitizeText } from '../output/sanitize.js';
import { identity, object } from './run.js';
import type { Limitation } from '../domain/response.js';

function logTimestamp(value: unknown, limitations: Limitation[]): string | null {
  if (typeof value !== 'string') invalid();
  const matched = /^(\d{4})-(\d\d)-(\d\d)T(\d\d):(\d\d):(\d\d)(\.\d{1,3})?(Z|[+-]\d\d:?\d\d)$/.exec(
    value,
  );

  if (matched) {
    const [, y, m, d, h, min, s, , zone] = matched;
    const digits = zone!.replace(/[^0-9]/g, '');

    if (
      Number(y) >= 100 &&
      Number(m) >= 1 &&
      Number(m) <= 12 &&
      Number(d) >= 1 &&
      Number(d) <= new Date(Date.UTC(Number(y), Number(m), 0)).getUTCDate() &&
      Number(h) < 24 &&
      Number(min) < 60 &&
      Number(s) < 60 &&
      (zone === 'Z' || (Number(digits.slice(0, 2)) <= 23 && Number(digits.slice(2)) < 60))
    ) {
      const canonicalZone = value.replace(/([+-]\d\d)(\d\d)$/, '$1:$2');

      return new Date(canonicalZone).toISOString();
    }
  }

  limitations.push({
    code: 'INVALID_TIMESTAMP',
    message: 'The server supplied an invalid log timestamp',
    source: 'log',
  });

  return null;
}

function invalid(): never {
  throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid scoped metadata response');
}

function preview(value: unknown, secrets: readonly string[], limit: number): string {
  if (typeof value !== 'string' || !value) invalid();

  return Array.from(sanitizeText(value, secrets)).slice(0, limit).join('');
}

export function normalizeProject(
  value: unknown,
  expectedId: string,
  secrets: readonly string[],
): Project {
  const dto = object(value),
    id = identity(dto.id);

  if (id !== expectedId)
    throw new DomainError('CONTEXT_MISMATCH', 'Server returned a different project');
  if (dto.archived !== undefined && typeof dto.archived !== 'boolean') invalid();
  // The recorded root omits its parent; omission on any other project is unknown ancestry.
  const parentProjectId =
    dto.parentProjectId === undefined && id === '_Root' ? null : identity(dto.parentProjectId);

  if (parentProjectId === id) invalid();

  return {
    id,
    name: preview(dto.name, secrets, 200),
    parentProjectId,
    archived: typeof dto.archived === 'boolean' ? dto.archived : null,
  };
}

export function normalizeServer(value: unknown, secrets: readonly string[]): ServerInfo {
  const dto = object(value);

  return { version: preview(dto.version, secrets, 100), buildNumber: identity(dto.buildNumber) };
}

export function normalizeIdentity(value: unknown, serverUrl: string): AuthenticatedIdentity {
  const dto = object(value),
    id = identity(dto.id, true);

  identity(dto.username);

  return {
    fingerprint:
      'sha256:' +
      createHash('sha256')
        .update(JSON.stringify([serverUrl, id]))
        .digest('hex')
        .slice(0, 32),
  };
}

export function normalizeLogTail(
  value: unknown,
  expectedId: string,
  tail: number,
  secrets: readonly string[],
): LogTail {
  const dto = object(value);

  if (!Number.isInteger(tail) || tail < 1 || tail > 1000)
    throw new DomainError('USAGE_ERROR', 'Invalid bounded log tail', 2);

  if (identity(dto.run_id, true) !== expectedId)
    throw new DomainError('CONTEXT_MISMATCH', 'Log belongs to another execution');
  if (!Array.isArray(dto.messages) || dto.messages.length > 1001) invalid();
  const seen = new Set<string>();
  const limitations: Limitation[] = [];
  let previousId = -1;
  const messages = dto.messages.map((value) => {
    const m = object(value);

    if (
      !Number.isSafeInteger(m.id) ||
      Number(m.id) < 0 ||
      typeof m.text !== 'string' ||
      !Number.isSafeInteger(m.level) ||
      !Number.isSafeInteger(m.status)
    )
      invalid();
    const id = String(m.id);

    if (seen.has(id) || Number(m.id) <= previousId) invalid();
    previousId = Number(m.id);
    seen.add(id);
    const text = sanitizeText(m.text, secrets);

    return {
      id,
      text,
      level: Number(m.level),
      status: Number(m.status),
      ...(m.timestamp !== undefined ? { timestamp: logTimestamp(m.timestamp, limitations) } : {}),
    };
  });

  return {
    runId: expectedId,
    messages: messages.slice(-tail),
    providerReturned: messages.length,
    truncated: messages.length > tail,
    limitations,
  };
}
