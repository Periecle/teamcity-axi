import { descriptor, flagsFor, globalFlags, registry } from './registry.js';
import type { Descriptor } from './registry.js';
import { usage } from '../domain/errors.js';

export interface Parsed {
  descriptor: Descriptor;
  flags: Record<string, string | number | boolean>;
  positional?: string;
  format: 'json' | 'toon';
  home: boolean;
}

const aliases: Record<string, string> = { h: 'help', v: 'version', V: 'version' };

export function parse(args: readonly string[]): Parsed {
  if (args.length > 100 || args.some((a) => Buffer.byteLength(a) > 4096))
    usage('Arguments exceed the input limit');
  const words: string[] = [];
  const flags: Parsed['flags'] = {};
  const known = Object.assign({}, globalFlags, ...registry.map((d) => d.flags));
  let literal = false;

  for (let i = 0; i < args.length; i++) {
    const arg = args[i]!;

    if (literal || !arg.startsWith('-')) {
      words.push(arg);
      continue;
    }

    if (arg === '--') {
      literal = true;
      continue;
    }

    const match = /^(?:--([a-z][a-z-]*)(?:=(.*))?|-([hvV]))$/s.exec(arg);

    if (!match) usage('Invalid option syntax');
    const name = match[1] ?? aliases[match[3]!]!;
    const spec = known[name];

    if (!spec) usage('Unknown flag', { validFlags: Object.keys(known) });
    if (Object.hasOwn(flags, name)) usage(`Duplicate singleton flag --${name}`);

    if (spec.type === 'boolean') {
      if (match[2] !== undefined) usage(`--${name} takes no value`);
      flags[name] = true;
      continue;
    }

    const value = match[2] ?? args[++i];

    if (value === undefined || value.startsWith('--') || value === '')
      usage(`--${name} requires a value`);

    if (spec.type === 'string') {
      if (spec.choices && !spec.choices.includes(value))
        usage(`Invalid --${name} value`, { choices: spec.choices });
      flags[name] = value;
    } else {
      const m = (spec.type === 'duration' ? /^(\d+)(ms|s|m)$/ : /^(\d+)$/).exec(value);

      if (!m)
        usage(
          `Invalid --${name}; expected ${spec.type === 'duration' ? 'duration with ms, s or m suffix' : 'integer'}`,
        );
      const n = Number(m[1]) * (m[2] === 's' ? 1000 : m[2] === 'm' ? 60000 : 1);

      if (!Number.isSafeInteger(n) || n < spec.min! || n > spec.max!)
        usage(`--${name} is outside its allowed range`, { min: spec.min, max: spec.max });
      flags[name] = n;
    }
  }

  const home = words.length === 0;
  let name = words.shift() ?? 'status';

  if (['run', 'job', 'queue', 'agent', 'context'].includes(name)) {
    const sub = words.shift();

    if (!sub) usage(`Missing ${name} subcommand`);
    name += `.${sub}`;
  }

  const d = descriptor(name);

  if (!d) usage('Unknown command', { commands: registry.map((c) => c.name) });
  for (const flag of Object.keys(flags))
    if (!Object.hasOwn(flagsFor(d), flag))
      usage(`--${flag} is not valid for ${name}`, { validFlags: Object.keys(flagsFor(d)) });
  const isHelp = flags.help === true;

  if (words.length > (d.positional ? 1 : 0)) usage('Unexpected positional arguments');
  if (d.positional && words.length === 0 && !isHelp) usage(`Missing ${d.positional}`);
  const positional = words[0];

  if (
    d.positional === 'runId' &&
    positional !== undefined &&
    (!/^[1-9]\d*$/.test(positional) || !Number.isSafeInteger(Number(positional)))
  )
    usage('Run ID must be a positive safe integer');
  if (positional !== undefined && /[\u0000-\u001f\u007f]/u.test(positional))
    usage('Control characters are not valid identifiers');
  if (flags.version) usage('Version must be requested as a standalone invocation');
  if (flags.json && flags.format !== undefined && flags.format !== 'json')
    usage('Conflicting --json and --format');

  const exclusive = (names: string[]) => {
    if (names.filter((n) => flags[n] !== undefined).length > 1)
      usage(`Conflicting flags: ${names.map((n) => '--' + n).join(', ')}`);
  };

  exclusive(['branch', 'literal-branch', 'all-branches']);
  exclusive(['failed', 'muted']);
  if (flags['include-muted'] && !flags.failed) usage('--include-muted requires --failed');
  if (
    flags.test &&
    ['failed', 'muted', 'include-muted', 'limit', 'cursor'].some((n) => flags[n] !== undefined)
  )
    usage('--test cannot be combined with filters or pagination');
  if (flags.problem && ['limit', 'cursor'].some((n) => flags[n] !== undefined))
    usage('--problem cannot be combined with pagination');
  if (
    name === 'run.log' &&
    flags.failed &&
    (flags.tail !== undefined || flags.contains !== undefined)
  )
    usage('--failed cannot be combined with --tail or --contains');
  for (const key of ['since', 'until'])
    if (
      flags[key] !== undefined &&
      !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)$/.test(String(flags[key]))
    )
      usage(`--${key} requires an RFC 3339 timestamp`);
  for (const key of ['since', 'until'])
    if (flags[key] !== undefined && !Number.isFinite(Date.parse(String(flags[key]))))
      usage(`Invalid --${key} timestamp`);

  for (const key of ['since', 'until'])
    if (flags[key] !== undefined) {
      const date = String(flags[key]);
      const year = Number(date.slice(0, 4)),
        month = Number(date.slice(5, 7)),
        day = Number(date.slice(8, 10));

      if (
        month < 1 ||
        month > 12 ||
        day < 1 ||
        day > new Date(Date.UTC(year, month, 0)).getUTCDate() ||
        Number(date.slice(11, 13)) > 23 ||
        Number(date.slice(14, 16)) > 59 ||
        Number(date.slice(17, 19)) > 59
      )
        usage(`Invalid --${key} calendar timestamp`);
    }

  if ((flags.since || flags.until) && flags.state !== undefined && flags.state !== 'finished')
    usage('Finish-time filters require finished executions');
  if (
    flags.since &&
    flags.until &&
    Date.parse(String(flags.since)) > Date.parse(String(flags.until))
  )
    usage('--since must precede --until');

  if (flags.fields !== undefined) {
    const fields = String(flags.fields).split(',');

    if (new Set(fields).size !== fields.length || fields.some((f) => !d.fields?.includes(f)))
      usage('Unknown or duplicate public projection field', { validFields: d.fields ?? [] });
  }

  if (flags.cursor && Buffer.byteLength(String(flags.cursor)) > 4096) usage('Cursor exceeds 4 KiB');

  return {
    descriptor: d,
    flags,
    ...(positional === undefined ? {} : { positional }),
    format: flags.json || flags.format === 'json' ? 'json' : 'toon',
    home,
  };
}
