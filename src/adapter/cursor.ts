import { DomainError } from '../domain/errors.js';

import { isUtf8 } from 'node:buffer';

export interface CursorBinding {
  command: string;
  server: string;
  filterHash: string;
  count: number;
  window?: { since: string; until: string };
}

export interface Cursor extends CursorBinding {
  version: 1;
  position: number;
  expiresAt: number;
}

function invalid(): never {
  throw new DomainError(
    'USAGE_ERROR',
    'Cursor is invalid, expired, or belongs to a different query',
    2,
  );
}

function valid(value: unknown, now: number): value is Cursor {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;

  const v = value as Record<string, unknown>;

  if (
    Object.keys(v).some(
      (k) =>
        ![
          'version',
          'command',
          'server',
          'filterHash',
          'count',
          'position',
          'expiresAt',
          'window',
        ].includes(k),
    )
  )
    return false;

  if (
    v.version !== 1 ||
    typeof v.command !== 'string' ||
    !/^[a-z]+\.[a-z]+$/.test(v.command) ||
    typeof v.server !== 'string' ||
    v.server.length < 1 ||
    v.server.length > 256 ||
    typeof v.filterHash !== 'string' ||
    !/^[a-f0-9]{64}$/.test(v.filterHash)
  )
    return false;

  if (
    !Number.isInteger(v.count) ||
    Number(v.count) < 1 ||
    Number(v.count) > 100 ||
    !Number.isInteger(v.position) ||
    Number(v.position) < 1 ||
    Number(v.position) >= 5000 ||
    !Number.isSafeInteger(v.expiresAt) ||
    Number(v.expiresAt) <= now ||
    Number(v.expiresAt) > now + 1800000
  )
    return false;

  if (v.window !== undefined) {
    if (!v.window || typeof v.window !== 'object' || Array.isArray(v.window)) return false;

    const w = v.window as Record<string, unknown>;

    if (
      Object.keys(w).length !== 2 ||
      !['since', 'until'].every(
        (k) =>
          typeof w[k] === 'string' &&
          /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$/.test(w[k] as string) &&
          Number.isFinite(Date.parse(w[k] as string)) &&
          new Date(w[k] as string).toISOString() === w[k],
      )
    )
      return false;

    if (Date.parse(w.since as string) > Date.parse(w.until as string)) return false;
  }

  return true;
}

export function encodeCursor(value: Cursor, now = Date.now()): string {
  if (!valid(value, now)) return invalid();

  const encoded = Buffer.from(JSON.stringify(value), 'utf8').toString('base64url');

  if (Buffer.byteLength(encoded) > 4096) return invalid();

  return encoded;
}

export function decodeCursor(token: string, now = Date.now()): Cursor {
  if (Buffer.byteLength(token) > 4096 || !/^[A-Za-z0-9_-]+$/.test(token)) return invalid();

  const bytes = Buffer.from(token, 'base64url');

  if (!isUtf8(bytes) || bytes.toString('base64url') !== token) return invalid();

  let value: unknown;

  try {
    value = JSON.parse(bytes.toString('utf8'));
  } catch {
    return invalid();
  }

  if (!valid(value, now)) return invalid();

  return value;
}

export function assertCursor(cursor: Cursor, binding: CursorBinding): void {
  if (
    cursor.command !== binding.command ||
    cursor.server !== binding.server ||
    cursor.filterHash !== binding.filterHash ||
    cursor.count !== binding.count ||
    cursor.window?.since !== binding.window?.since ||
    cursor.window?.until !== binding.window?.until
  )
    return invalid();
}
