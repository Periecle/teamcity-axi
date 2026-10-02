import { isUtf8 } from 'node:buffer';

import { DomainError } from '../domain/errors.js';
import type { Captured } from '../transport/process.js';

export interface RawResponse {
  status: number;
  body: unknown;
  retryAfter: string | null;
}

export class HttpReadError extends DomainError {
  constructor(
    readonly httpStatus: number,
    readonly retryAfter: string | null,
  ) {
    super(
      httpStatus === 401
        ? 'AUTH_REQUIRED'
        : httpStatus === 403
          ? 'PERMISSION_DENIED'
          : httpStatus === 404
            ? 'NOT_FOUND'
            : 'UPSTREAM_FAILURE',
      httpStatus === 401
        ? 'Native read requires valid authentication'
        : httpStatus === 403
          ? 'The server denied this read'
          : httpStatus === 404
            ? 'The requested resource was not found'
            : 'The server could not perform this read',
      1,
      httpStatus === 429 || httpStatus >= 500,
    );
  }
}

const statusError = (status: number, retryAfter: string | null): DomainError =>
  new HttpReadError(status, retryAfter);

export function parseRaw(captured: Captured): RawResponse {
  if (!isUtf8(captured.stdout))
    throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Native response is not UTF-8');
  const text = captured.stdout.toString('utf8');
  const boundary = text.indexOf('\n\n');
  const crlf = text.indexOf('\r\n\r\n');
  const offset = boundary < 0 ? crlf : crlf < 0 ? boundary : Math.min(boundary, crlf);
  const separator = offset === crlf ? '\r\n\r\n' : '\n\n';

  if (offset < 0 || offset > 16384)
    throw new DomainError('UPSTREAM_FAILURE', 'Native raw response has no supported HTTP envelope');
  const lines = text.slice(0, offset).split(/\r?\n/);
  const match = /^HTTP\/1\.1 ([1-5]\d\d) [\x20-\x7e]{0,100}$/.exec(lines.shift() ?? '');

  if (!match || lines.length > 64)
    throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid native HTTP preamble');
  const headers = new Map<string, string>();

  for (const line of lines) {
    const header = /^([!#$%&'*+.^_`|~0-9A-Za-z-]+):[ \t]*([^\r\n\u0000]*)$/.exec(line);

    if (!header || line.length > 4096)
      throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid native HTTP headers');
    const name = header[1]!.toLowerCase();

    if (['content-type', 'retry-after'].includes(name)) {
      if (headers.has(name))
        throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Ambiguous native HTTP headers');
      headers.set(name, header[2]!);
    }
  }

  const status = Number(match[1]);

  if (status < 200 || status >= 300) throw statusError(status, headers.get('retry-after') ?? null);
  if (captured.exitCode !== 0 || captured.signal !== null)
    throw new DomainError(
      'UPSTREAM_FAILURE',
      'Native process did not complete the successful response',
    );
  const contentType = headers.get('content-type')?.split(';')[0]?.trim().toLowerCase();

  if (contentType === 'text/html')
    throw new DomainError('AUTH_REQUIRED', 'Server returned an HTML authentication or proxy page');
  if (
    !contentType ||
    !(contentType === 'application/json' || /^application\/[a-z0-9.+-]+\+json$/.test(contentType))
  )
    throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Native read did not return JSON content');
  let body: unknown;

  try {
    body = JSON.parse(text.slice(offset + separator.length));
  } catch {
    throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Malformed JSON from native read');
  }

  return { status, body, retryAfter: headers.get('retry-after') ?? null };
}
