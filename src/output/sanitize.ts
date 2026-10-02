const secretName = /(?:token|password|secret|authorization|cookie|api[_-]?key|private[_-]?key)/i;
export function secretMatchers(patterns: readonly string[] = []): RegExp[] {
  // Deliberately restricted regular expressions: no groups, repetition, alternation or backrefs.
  // Character classes, anchors, literal names and .* cover bounded environment-name matching.
  return patterns.map(pattern => {
    if (pattern.length > 128 || /[()+?{}|\\]/.test(pattern) || pattern.replaceAll('.*','').includes('*') || (pattern.match(/\.\*/g)?.length ?? 0) > 1) throw new Error('Unsafe secret-name pattern');
    return new RegExp(pattern,'i');
  });
}
export function knownSecrets(env: NodeJS.ProcessEnv, patterns: readonly string[] = []): string[] {
  const matchers=secretMatchers(patterns);
  return [...new Set(Object.entries(env).filter(([name, value]) => value && (secretName.test(name) || matchers.some(p=>p.test(name)))).map(([,value]) => value!))].sort((a,b) => b.length - a.length);
}
export function sanitizeText(text: string, secrets: readonly string[]): string {
  // Redact before previews, grouping, hashing or output measurements.
  for (const value of secrets) if (value) text = text.split(value).join('[REDACTED]');
  text = text
    .replace(/\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g, '')
    .replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, '')
    .replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/g, '')
    .replace(/[\u202a-\u202e\u2066-\u2069]/g, c => `\\u${c.charCodeAt(0).toString(16).padStart(4,'0')}`);
  // Removing terminal sequences can join fragments into a known credential.
  for (const value of secrets) if (value) text = text.split(value).join('[REDACTED]');
  return text
    .replace(/-----BEGIN (?:[A-Z]+ )?PRIVATE KEY-----[\s\S]*?-----END (?:[A-Z]+ )?PRIVATE KEY-----/g, '[REDACTED PRIVATE KEY]')
    .replace(/\b((?:Bearer|Basic)\s+)[^\s"\\,;]+/gi, '$1[REDACTED]')
    .replace(/\b(token|password|secret|api[_-]?key)\s*[:=]\s*[^\s"\\,;]+/gi, '$1=[REDACTED]')
    .replace(/(https?:\/\/)[^/@\s]+:[^/@\s]+@/gi, '$1[REDACTED]@');
}
export function sanitize(value: unknown, secrets: readonly string[], depth = 0, protectedKeys: ReadonlySet<string> = new Set(), matchers: readonly RegExp[] = [], path: readonly string[] = []): unknown {
  if (depth > 30) return '[INPUT_DEPTH_LIMIT]';
  if (typeof value === 'string') return sanitizeText(value, secrets);
  if (Array.isArray(value)) return value.map(v => sanitize(v, secrets, depth + 1, protectedKeys, matchers, path));
  if (value !== null && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([k,v]) => {
      const structural = protectedKeys.has(k) && (depth === 0 || path[0] === 'meta');
      const sensitive = !structural && (secretName.test(k) || matchers.some(p=>p.test(k)));
      return [protectedKeys.has(k) ? k : sanitizeText(k, secrets), sensitive ? '[REDACTED]' : sanitize(v, secrets, depth + 1, protectedKeys, matchers, [...path,k])];
    }));
  }
  return value;
}
