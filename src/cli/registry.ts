export interface Flag {
  type: 'boolean' | 'string' | 'integer' | 'duration';
  description: string;
  choices?: readonly string[];
  min?: number;
  max?: number;
}

export interface Descriptor {
  name: string;
  summary: string;
  positional?: 'runId' | 'id' | 'command';
  flags: Record<string, Flag>;
  fields?: readonly string[];
}

const bool = (description: string): Flag => ({ type: 'boolean', description });

const str = (description: string, choices?: readonly string[]): Flag => ({
  type: 'string',
  description,
  ...(choices ? { choices } : {}),
});

const int = (description: string, min: number, max: number): Flag => ({
  type: 'integer',
  description,
  min,
  max,
});

const dur = (description: string, min: number, max: number): Flag => ({
  type: 'duration',
  description,
  min,
  max,
});

export const globalFlags: Record<string, Flag> = {
  help: bool('Show local command help'),
  version: bool('Show application version'),
  format: str('Output serialization', ['toon', 'json']),
  json: bool('Alias for --format json'),
  server: str('Registered trusted server alias'),
  cwd: str('Resolve worktree context from this directory'),
  timeout: dur('Overall deadline (ms, s or m)', 1, 1_800_000),
  'max-bytes': int('Serialized stdout byte ceiling', 2048, 262144),
  'require-complete': bool('Fail if the requested answer is partial'),
  'no-hints': bool('Omit optional read suggestions'),
  debug: bool('Redacted metadata on stderr'),
};

const scope = { project: str('Exact project ID'), job: str('Exact job ID') };

const branch = {
  branch: str('Logical branch or @this'),
  'literal-branch': str('Literal logical branch'),
  'all-branches': bool('Include all logical branches'),
};

const revision = {
  revision: str('Commit identity or @head'),
  'vcs-root': str('Exact VCS root ID'),
};

const page = {
  limit: int('Maximum requested rows', 1, 100),
  cursor: str('Opaque bounded continuation'),
};

const projection = {
  fields: str('Comma separated public fields'),
  full: bool('Expand text previews within budgets'),
};

const graph = {
  depth: int('Maximum snapshot traversal depth', 0, 12),
  'max-nodes': int('Maximum unique executions', 1, 200),
};

const runFields = [
  'id',
  'jobId',
  'branch',
  'state',
  'result',
  'number',
  'statusText',
  'personal',
  'composite',
  'queuedAt',
  'startedAt',
  'finishedAt',
  'durationMs',
  'webUrl',
  'revisions',
];

export const registry: readonly Descriptor[] = [
  {
    name: 'status',
    summary: 'CI orientation for the exact current checkout',
    flags: {
      ...scope,
      ...branch,
      ...revision,
      check: bool('Assert completed successful exact checkout'),
    },
  },
  {
    name: 'context.show',
    summary: 'Resolve local trusted context',
    flags: { ...scope, ...branch, ...revision, verify: bool('Verify selected scope remotely') },
  },
  {
    name: 'doctor',
    summary: 'Inspect dependency, authentication and capabilities',
    flags: { ...scope, offline: bool('Never perform network reads') },
  },
  {
    name: 'schema',
    summary: 'Inspect a packaged command contract',
    positional: 'command',
    flags: {},
  },
  {
    name: 'run.list',
    summary: 'Read one bounded scoped execution page',
    flags: {
      ...scope,
      ...branch,
      ...revision,
      ...page,
      fields: projection.fields!,
      state: str('Lifecycle filter', ['queued', 'running', 'finished']),
      result: str('Outcome filter', [
        'success',
        'failure',
        'error',
        'canceled',
        'failed_to_start',
        'unknown',
      ]),
      since: str('RFC 3339 finish-time lower bound'),
      until: str('RFC 3339 finish-time upper bound'),
    },
    fields: runFields,
  },
  {
    name: 'run.view',
    summary: 'Observe one exact execution',
    positional: 'runId',
    flags: { ...scope, ...projection },
    fields: runFields,
  },
  {
    name: 'run.problems',
    summary: 'Read independent problem occurrences',
    positional: 'runId',
    flags: {
      ...scope,
      ...page,
      problem: str('Exact problem occurrence ID'),
      full: projection.full!,
    },
  },
  {
    name: 'run.tests',
    summary: 'Read independent test occurrences',
    positional: 'runId',
    flags: {
      ...scope,
      ...page,
      ...projection,
      failed: bool('Unmuted failures'),
      muted: bool('Muted failures'),
      'include-muted': bool('Include muted failures with --failed'),
      test: str('Exact test occurrence ID'),
    },
    fields: [
      'id',
      'runId',
      'name',
      'result',
      'testId',
      'suite',
      'durationMs',
      'muted',
      'ignored',
      'details',
    ],
  },
  {
    name: 'run.log',
    summary: 'Read a bounded structured log tail',
    positional: 'runId',
    flags: {
      ...scope,
      tail: int('Tail messages', 1, 1000),
      contains: str('Literal filter within the fetched window'),
      failed: bool('Failure-oriented evidence'),
      full: projection.full!,
    },
  },
  {
    name: 'run.changes',
    summary: 'Read bounded contextual changes',
    positional: 'runId',
    flags: { ...scope, ...page, ...projection, files: bool('Include bounded changed file names') },
    fields: ['id', 'version', 'vcsRootId', 'message', 'timestamp', 'files'],
  },
  {
    name: 'run.tree',
    summary: 'Traverse bounded snapshot execution graph',
    positional: 'runId',
    flags: { ...scope, ...graph },
  },
  {
    name: 'run.failure',
    summary: 'Investigate independent failure evidence',
    positional: 'runId',
    flags: {
      ...scope,
      ...graph,
      full: projection.full!,
      'max-diagnosed-runs': int('Maximum diagnosed executions including root', 1, 10),
    },
  },
  {
    name: 'run.watch',
    summary: 'Observe one execution until terminal or deadline',
    positional: 'runId',
    flags: {
      ...scope,
      interval: dur('Poll interval (ms, s or m)', 5000, 1_800_000),
      check: bool('Assert terminal success'),
    },
  },
  { name: 'job.list', summary: 'Read scoped jobs', flags: { project: scope.project!, ...page } },
  {
    name: 'job.view',
    summary: 'Read safe exact job metadata',
    positional: 'id',
    flags: { project: scope.project! },
  },
  { name: 'queue.list', summary: 'Read scoped queued executions', flags: { ...scope, ...page } },
  {
    name: 'agent.list',
    summary: 'Read scoped agent availability',
    flags: { ...scope, ...page, pool: str('Exact agent pool ID') },
  },
  {
    name: 'agent.view',
    summary: 'Read safe exact agent metadata',
    positional: 'id',
    flags: { ...scope, pool: str('Assert exact agent pool ID') },
  },
];

export function descriptor(name: string): Descriptor | undefined {
  return registry.find((d) => d.name === name);
}

export function flagsFor(d: Descriptor) {
  return { ...globalFlags, ...d.flags };
}

export function help(d?: Descriptor): string {
  if (!d)
    return `teamcity-axi <command> [arguments] [flags]\nRead-only TeamCity observations.\n\n${registry.map((c) => `  ${c.name.replaceAll('.', ' ')}  ${c.summary}`).join('\n')}\n\nUse <command> --help for valid flags. Default output: TOON.\n`;

  return `teamcity-axi ${d.name.replaceAll('.', ' ')}${d.positional ? ` <${d.positional}>` : ''}\n${d.summary}\n\n${Object.entries(
    flagsFor(d),
  )
    .map(([n, f]) => `  --${n}${f.type === 'boolean' ? '' : ' VALUE'}  ${f.description}`)
    .join('\n')}\n`;
}
