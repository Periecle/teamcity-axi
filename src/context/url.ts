import { DomainError } from '../domain/errors.js';
export function canonicalUrl(input: string, allowHttpLoopback = false): string {
  let url: URL;
  try {
    url = new URL(input);
  } catch {
    throw new DomainError('USAGE_ERROR', 'Invalid server URL', 2);
  }
  if (
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    /[\r\n\u0000\\]/.test(input) ||
    /%2f|%5c|%2e/i.test(input) ||
    input.split(/[/?#]/).some((p) => p === '.' || p === '..')
  )
    throw new DomainError('USAGE_ERROR', 'Unsafe server URL', 2);
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname);
  if (url.protocol !== 'https:' && !(allowHttpLoopback && loopback && url.protocol === 'http:'))
    throw new DomainError(
      'UNTRUSTED_SERVER',
      'HTTPS is required except explicitly trusted loopback development servers',
    );
  return url.toString().replace(/\/$/, '');
}
