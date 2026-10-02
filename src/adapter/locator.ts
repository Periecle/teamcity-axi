import { DomainError } from '../domain/errors.js';

// Literal value encoding follows the pinned official CLI and public REST docs.
// Live server round-trips remain an independent contract gate.
export function literal(value: string): string {
  if (
    !value ||
    !value.isWellFormed() ||
    Buffer.byteLength(value) > 4096 ||
    /[\u0000-\u001f\u007f]/.test(value)
  )
    throw new DomainError('USAGE_ERROR', 'Invalid locator value', 2);

  return `($base64:${Buffer.from(value, 'utf8').toString('base64url')})`;
}

export function idCondition(value: string): string {
  return `(id:${literal(value)})`;
}

export function branchCondition(value: string): string {
  return `(name:(value:${literal(value)}))`;
}

export function apiPath(
  resource:
    | 'builds'
    | 'problemOccurrences'
    | 'testOccurrences'
    | 'changes'
    | 'buildTypes'
    | 'buildQueue'
    | 'agents',
  parts: readonly string[],
  fields: string,
): string {
  // Callers supply only adapter-owned structural pieces; no raw user locators.
  return `/app/rest/${resource}?${new URLSearchParams({ locator: parts.join(','), fields })}`;
}
