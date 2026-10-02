import { createHash } from 'node:crypto';

import { DomainError } from '../domain/errors.js';
import type { AuthenticatedIdentity, LogTail, Project, ServerInfo } from '../domain/teamcity.js';
import { sanitizeText } from '../output/sanitize.js';
import { identity, object } from './run.js';

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

  if (identity(dto.run_id, true) !== expectedId)
    throw new DomainError('CONTEXT_MISMATCH', 'Log belongs to another execution');
  if (!Array.isArray(dto.messages) || dto.messages.length > 1001) invalid();
  const seen = new Set<string>();
  let textTruncated = false;
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

    if (seen.has(id)) invalid();
    seen.add(id);
    const text = Array.from(sanitizeText(m.text, secrets));

    if (text.length > 2000) textTruncated = true;

    return {
      id,
      text: text.slice(0, 2000).join(''),
      level: Number(m.level),
      status: Number(m.status),
    };
  });

  return {
    runId: expectedId,
    messages: messages.slice(-tail),
    providerReturned: messages.length,
    truncated: messages.length > tail || textTruncated,
  };
}
