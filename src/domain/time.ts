import { usage } from './errors.js';

interface Instant {
  secondMs: number;
  fraction: string;
}

function instant(value: string): Instant {
  const match = /^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d+))?(Z|[+-]\d\d:\d\d)$/.exec(value);

  if (!match || value.length > 4096) usage('Expected a bounded RFC 3339 timestamp');

  const [year, month, day, hour, minute, second] = match[1]!.split(/[-T:]/).map(Number);
  const leap = year! % 4 === 0 && (year! % 100 !== 0 || year! % 400 === 0);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  const secondMs = Date.parse(`${match[1]}${match[3]}`);

  if (
    month! < 1 ||
    month! > 12 ||
    day! < 1 ||
    day! > days[month! - 1]! ||
    hour! > 23 ||
    minute! > 59 ||
    second! > 59 ||
    !Number.isFinite(secondMs)
  )
    usage('Invalid RFC 3339 calendar timestamp');

  return { secondMs, fraction: (match[2] ?? '').replace(/0+$/, '').padEnd(3, '0') };
}

function utc(secondMs: number, fraction: string): string {
  const seconds = new Date(secondMs).toISOString().slice(0, -5);

  if (!/^\d{4}-/.test(seconds)) usage('Timestamp is outside the RFC 3339 year range');

  return `${seconds}.${fraction}Z`;
}

export function canonicalTimestamp(value: string): string {
  const { secondMs, fraction } = instant(value);

  return utc(secondMs, fraction);
}

export function compareTimestamps(left: string, right: string): number {
  const a = instant(left),
    b = instant(right);

  if (a.secondMs !== b.secondMs) return a.secondMs < b.secondMs ? -1 : 1;

  const length = Math.max(a.fraction.length, b.fraction.length);
  const af = a.fraction.padEnd(length, '0'),
    bf = b.fraction.padEnd(length, '0');

  return af === bf ? 0 : af < bf ? -1 : 1;
}

export function shiftTimestamp(value: string, seconds: number): string {
  const { secondMs, fraction } = instant(value);

  return utc(secondMs + seconds * 1000, fraction);
}

// The verified server compares integer milliseconds, while its DTO reports
// whole seconds. Strict after(floor) and before(ceil) preserve exclusive bounds.
export function providerDate(value: string, condition: 'after' | 'before'): string | null {
  const { secondMs, fraction } = instant(value);
  const rounded =
    secondMs +
    Number(fraction.slice(0, 3)) +
    (condition === 'before' && /[1-9]/.test(fraction.slice(3)) ? 1 : 0);
  const date = new Date(rounded).toISOString();

  if (!/^\d{4}-/.test(date)) return null;

  const seconds = date.slice(0, 19).replaceAll('-', '').replaceAll(':', '');
  const milliseconds = date.slice(19, 23);

  return seconds + (milliseconds === '.000' ? '' : milliseconds) + '+0000';
}
