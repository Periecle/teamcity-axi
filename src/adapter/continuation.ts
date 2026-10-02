import { DomainError } from '../domain/errors.js';

const invalid = (): never => {
  throw new DomainError(
    'UPSTREAM_SCHEMA_MISMATCH',
    'Provider continuation cannot be safely reconstructed',
  );
};

function dimensions(locator: string): string[] {
  if (Buffer.byteLength(locator) > 16384) return invalid();
  let depth = 0,
    start = 0;
  const parts: string[] = [];

  for (let i = 0; i < locator.length; i++) {
    if (locator[i] === '(') {
      if (++depth > 8) return invalid();
    } else if (locator[i] === ')') {
      if (--depth < 0) return invalid();
    } else if (locator[i] === ',' && depth === 0) {
      parts.push(locator.slice(start, i));
      start = i + 1;
    }
  }

  if (depth !== 0) return invalid();
  parts.push(locator.slice(start));
  if (parts.some((p) => !p || !/^[A-Za-z][A-Za-z0-9]*:/.test(p))) return invalid();

  return parts;
}

export interface ContinuationRequest {
  serverUrl: string;
  resource: string;
  filters: readonly string[];
  fields: string;
  count: number;
  start: number;
  scanLimit: number;
}

export function nextPosition(href: string, request: ContinuationRequest): number {
  if (
    Buffer.byteLength(href) > 16384 ||
    !href ||
    href.startsWith('//') ||
    /[\u0000-\u0020\u007f\\#]/.test(href)
  )
    return invalid();
  const rawPath = href.split('?')[0]!;

  if (/%2e|%2f|%5c|%25/i.test(rawPath) || rawPath.split('/').some((p) => p === '.' || p === '..'))
    return invalid();
  const base = new URL(request.serverUrl);
  let url: URL;

  try {
    url = new URL(href, base);
  } catch {
    return invalid();
  }

  if (url.origin !== base.origin || url.username || url.password) return invalid();
  const prefix = base.pathname.replace(/\/$/, '');

  if (/^[A-Za-z][A-Za-z0-9+.-]*:/.test(href) && !url.pathname.startsWith(prefix + '/app/rest/'))
    return invalid();
  // A bare relative REST href is rebuilt against the frozen deployment prefix;
  // absolute hrefs must already belong to that same deployment context.
  const path = url.pathname.startsWith(prefix + '/app/rest/')
    ? url.pathname.slice(prefix.length)
    : url.pathname;

  if (path !== `/app/rest/${request.resource}`) return invalid();
  if (
    [...url.searchParams.keys()].some((k) => !['locator', 'fields'].includes(k)) ||
    url.searchParams.getAll('locator').length !== 1 ||
    url.searchParams.getAll('fields').length > 1
  )
    return invalid();
  if (url.searchParams.has('fields') && url.searchParams.get('fields') !== request.fields)
    return invalid();
  const page = new Map<string, number>();
  const filters: string[] = [];

  for (const part of dimensions(url.searchParams.get('locator')!)) {
    const key = part.slice(0, part.indexOf(':'));

    if (['count', 'start', 'lookupLimit'].includes(key)) {
      const value = part.slice(key.length + 1);

      if (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value)) || page.has(key))
        return invalid();
      page.set(key, Number(value));
    } else filters.push(part);
  }

  if (JSON.stringify(filters.toSorted()) !== JSON.stringify([...request.filters].toSorted()))
    return invalid();
  const start = page.get('start');

  if (
    page.get('count') !== request.count ||
    (page.get('lookupLimit') ?? request.scanLimit) !== request.scanLimit ||
    start === undefined ||
    start <= request.start ||
    start >= request.scanLimit
  )
    return invalid();

  return start;
}
