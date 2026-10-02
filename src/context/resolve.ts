import { realpath, stat, open } from 'node:fs/promises';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { homedir } from 'node:os';
import { join, dirname, relative, isAbsolute, sep } from 'node:path';

import { parse as parseToml } from 'smol-toml';

import { DomainError } from '../domain/errors.js';

import { constants } from 'node:fs';
import { isUtf8 } from 'node:buffer';

import { validateConfig } from '../output/schema.js';
import { canonicalUrl } from './url.js';
import { secretMatchers } from '../output/sanitize.js';
import type { Parsed } from '../cli/parser.js';

const exec = promisify(execFile);

interface ServerConfig {
  url: string;
  allowedProjects?: string[];
  allowHttpLoopback?: boolean;
  forwardHeaderEnvNames?: string[];
}

export interface UserConfig {
  schemaVersion: '1.0';
  readOnly: true;
  servers: Record<string, ServerConfig>;
  defaultServer?: string;
  binaryPath?: string;
  allowWorkspaceBinary?: boolean;
  allowConfigSymlinks?: boolean;
  secretNamePatterns?: string[];
  limits?: { concurrency?: number; maxBytes?: number; maxChildProcesses?: number };
}

interface Binding {
  url: string;
  project?: string;
  job?: string;
  jobs?: string[];
  paths?: Record<string, Omit<Binding, 'url' | 'paths'>>;
}

interface Overlay {
  schemaVersion: '1.0';
  vcsRoots?: { server: string; remote: string; rootId: string }[];
}

export interface ExecutionContext {
  deadline: number;
  cwd: string;
  repositoryRoot?: string;
  head?: string;
  dirty?: boolean;
  server?: string;
  serverUrl?: string;
  project?: string;
  job?: string;
  jobs: readonly string[];
  branch?: string;
  revision?: string;
  vcsRootId?: string;
  sources: Readonly<Record<string, string>>;
  config?: Readonly<UserConfig>;
}

function failure(message: string): never {
  throw new DomainError('USAGE_ERROR', message, 2);
}

function object(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function id(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    value.length > 0 &&
    value.length <= 256 &&
    !/[\u0000-\u001f\u007f]/.test(value)
  );
}

async function boundedRead(path: string, trusted = false): Promise<string | undefined> {
  let file;

  try {
    file = await open(path, constants.O_RDONLY | constants.O_NONBLOCK);
  } catch (e) {
    if ((e as NodeJS.ErrnoException).code === 'ENOENT') return undefined;

    throw new DomainError('USAGE_ERROR', 'Cannot read configuration', 2);
  }

  try {
    const metadata = await file.stat();

    if (!metadata.isFile()) failure('Configuration must be a regular file');

    if (
      trusted &&
      process.platform !== 'win32' &&
      ((metadata.mode & 0o022) !== 0 || (process.getuid && metadata.uid !== process.getuid()))
    )
      throw new DomainError(
        'POLICY_DENIED',
        'Trusted configuration must be owned by the current user and not writable by other users',
      );

    const buffer = Buffer.alloc(65537);
    const { bytesRead } = await file.read(buffer, 0, buffer.length, 0);

    if (bytesRead > 65536) failure('Configuration exceeds the 64 KiB input limit');

    const data = buffer.subarray(0, bytesRead);

    if (!isUtf8(data)) failure('Configuration must be UTF-8');

    return data.toString('utf8');
  } finally {
    await file.close();
  }
}

async function git(
  cwd: string,
  args: string[],
  env: NodeJS.ProcessEnv,
  deadline: number,
): Promise<string | undefined> {
  const gitEnv: NodeJS.ProcessEnv = {
    PATH: env.PATH,
    HOME: env.HOME,
    LANG: 'C',
    GIT_TERMINAL_PROMPT: '0',
    GIT_CONFIG_NOSYSTEM: '1',
  };

  if (Date.now() >= deadline)
    throw new DomainError('DEADLINE_EXCEEDED', 'Overall deadline exceeded', 1, true);

  try {
    return (
      await exec('git', ['-c', 'core.fsmonitor=false', ...args], {
        cwd,
        env: gitEnv,
        timeout: Math.min(1000, deadline - Date.now()),
        maxBuffer: 131072,
        encoding: 'utf8',
      })
    ).stdout.trim();
  } catch {
    if (Date.now() >= deadline)
      throw new DomainError('DEADLINE_EXCEEDED', 'Overall deadline exceeded', 1, true);

    return undefined;
  }
}

function freeze<T>(value: T): T {
  if (value && typeof value === 'object') {
    for (const child of Object.values(value)) freeze(child);

    Object.freeze(value);
  }

  return value;
}

function nativeBindings(value: unknown): Binding[] {
  if (
    !object(value) ||
    Object.keys(value).some((k) => k !== 'server') ||
    (value.server !== undefined && !Array.isArray(value.server))
  )
    failure('Invalid native repository binding');

  const bindings = value.server ?? [];

  if ((bindings as unknown[]).length > 100) failure('Too many native server bindings');

  for (const b of bindings as unknown[]) {
    if (
      !object(b) ||
      typeof b.url !== 'string' ||
      b.url.length < 1 ||
      b.url.length > 2048 ||
      Object.keys(b).some((k) => !['url', 'project', 'job', 'jobs', 'paths'].includes(k))
    )
      failure('Invalid native server binding');

    const scopes: unknown[] = [b];

    if (b.paths !== undefined) {
      if (!object(b.paths) || Object.keys(b.paths).length > 100) failure('Invalid path bindings');

      for (const [path, scope] of Object.entries(b.paths)) {
        if (
          isAbsolute(path) ||
          path.split('/').some((p) => p === '.' || p === '..') ||
          /[\\\u0000-\u001f]/.test(path)
        )
          failure('Invalid native binding path');

        if (
          !object(scope) ||
          Object.keys(scope).some((k) => !['project', 'job', 'jobs'].includes(k))
        )
          failure('Invalid path scope');

        scopes.push(scope);
      }
    }

    for (const scope of scopes) {
      const s = scope as Record<string, unknown>;

      if (
        (s.project !== undefined && !id(s.project)) ||
        (s.job !== undefined && !id(s.job)) ||
        (s.jobs !== undefined &&
          (!Array.isArray(s.jobs) || s.jobs.length > 100 || s.jobs.some((j) => !id(j))))
      )
        failure('Invalid binding scope identifiers');
    }
  }

  return bindings as Binding[];
}

function effective(binding: Binding, rel: string): Binding {
  const paths = Object.entries(binding.paths ?? {})
    .filter(([p]) => rel === p || rel.startsWith(p + '/'))
    .sort(([a], [b]) => b.length - a.length);
  const scoped = paths[0]?.[1];

  return {
    ...binding,
    ...(scoped?.project ? { project: scoped.project } : {}),
    ...(scoped?.job ? { job: scoped.job } : {}),
    ...(scoped?.jobs?.length ? { jobs: scoped.jobs } : {}),
  };
}

async function safeRepositoryFile(
  path: string,
  root: string,
  allowSymlinks: boolean,
): Promise<string | undefined> {
  let resolved;

  try {
    resolved = await realpath(path);
  } catch (e) {
    if ((e as NodeJS.ErrnoException).code === 'ENOENT') return undefined;

    failure('Cannot resolve repository configuration');
  }

  const rel = relative(root, resolved);

  if (!allowSymlinks && (rel === '..' || rel.startsWith('..' + sep) || isAbsolute(rel)))
    throw new DomainError('POLICY_DENIED', 'Repository configuration escapes the worktree');

  return boundedRead(resolved);
}

export async function resolveContext(
  parsed: Parsed,
  env: NodeJS.ProcessEnv = process.env,
): Promise<ExecutionContext> {
  const deadline =
    Date.now() +
    Number(
      parsed.flags.timeout ??
        (parsed.descriptor.name === 'status'
          ? 5000
          : ['run.tree', 'run.failure'].includes(parsed.descriptor.name)
            ? 20000
            : 10000),
    );

  const within = async <T>(operation: Promise<T>): Promise<T> => {
    const remaining = deadline - Date.now();

    if (remaining <= 0)
      throw new DomainError('DEADLINE_EXCEEDED', 'Overall deadline exceeded', 1, true);

    let timer: NodeJS.Timeout | undefined;

    try {
      return await Promise.race([
        operation,
        new Promise<never>((_, reject) => {
          timer = setTimeout(
            () =>
              reject(new DomainError('DEADLINE_EXCEEDED', 'Overall deadline exceeded', 1, true)),
            remaining,
          );
        }),
      ]);
    } finally {
      if (timer) clearTimeout(timer);
    }
  };

  let cwd: string;

  try {
    cwd = await within(realpath(String(parsed.flags.cwd ?? process.cwd())));

    if (!(await within(stat(cwd))).isDirectory()) failure('--cwd is not a directory');
  } catch (e) {
    if (e instanceof DomainError && e.code === 'DEADLINE_EXCEEDED') throw e;

    failure('Cannot resolve --cwd');
  }

  const configPath = join(
    env.XDG_CONFIG_HOME ?? join(env.HOME ?? homedir(), '.config'),
    'teamcity-axi',
    'config.json',
  );
  const raw = await within(boundedRead(configPath, true));
  let config: UserConfig | undefined;

  if (raw !== undefined) {
    try {
      config = JSON.parse(raw) as UserConfig;
    } catch {
      failure('Invalid trusted configuration JSON');
    }

    validateConfig('user-config', config);

    try {
      secretMatchers(config?.secretNamePatterns);
    } catch {
      failure('Unsafe trusted secret-name pattern');
    }
  }

  const repositoryRoot = await git(cwd, ['rev-parse', '--show-toplevel'], env, deadline);
  const head = repositoryRoot
    ? await git(cwd, ['rev-parse', '--verify', 'HEAD'], env, deadline)
    : undefined;
  const branchRef = repositoryRoot
    ? await git(cwd, ['symbolic-ref', '--quiet', '--short', 'HEAD'], env, deadline)
    : undefined;
  const changes = repositoryRoot
    ? await git(cwd, ['status', '--porcelain=v1', '--untracked-files=normal'], env, deadline)
    : undefined;
  let bindings: Binding[] = [];
  let overlay: Overlay | undefined;

  if (repositoryRoot) {
    let dir = cwd;

    for (;;) {
      const text = await within(
        safeRepositoryFile(
          join(dir, 'teamcity.toml'),
          repositoryRoot,
          config?.allowConfigSymlinks === true,
        ),
      );

      if (text !== undefined) {
        let value: unknown;

        try {
          value = parseToml(text);
        } catch {
          failure('Malformed teamcity.toml');
        }

        bindings = nativeBindings(value).map((b) =>
          effective(b, relative(dir, cwd).split('\\').join('/')),
        );
        break;
      }

      if (dir === repositoryRoot) break;

      const parent = dirname(dir);

      if (parent === dir) break;

      dir = parent;
    }

    const text = await within(
      safeRepositoryFile(
        join(repositoryRoot, '.teamcity-axi.json'),
        repositoryRoot,
        config?.allowConfigSymlinks === true,
      ),
    );

    if (text !== undefined) {
      try {
        overlay = JSON.parse(text) as Overlay;
      } catch {
        failure('Malformed repository overlay JSON');
      }

      validateConfig('repository-config', overlay);
    }
  }

  const sources: Record<string, string> = {};
  const registered = Object.entries(config?.servers ?? {}).map(([alias, s]) => ({
    alias,
    ...s,
    url: canonicalUrl(s.url, s.allowHttpLoopback),
  }));

  if (new Set(registered.map((s) => s.url)).size !== registered.length)
    throw new DomainError(
      'AMBIGUOUS_CONTEXT',
      'Trusted aliases must not register the same canonical server URL',
      2,
    );

  const explicit = parsed.flags.server ?? env.TEAMCITY_AXI_SERVER;
  let selected: (typeof registered)[number] | undefined;

  if (explicit !== undefined) {
    selected = registered.find((s) => s.alias === explicit);
    sources.server = parsed.flags.server ? 'flag' : 'TEAMCITY_AXI_SERVER';

    if (!selected) throw new DomainError('UNTRUSTED_SERVER', 'Server alias is not registered');
  } else if (env.TEAMCITY_URL) {
    const inherited = canonicalUrl(env.TEAMCITY_URL, true);

    selected = registered.find((s) => s.url === inherited);
    sources.server = 'TEAMCITY_URL';

    if (!selected)
      throw new DomainError('UNTRUSTED_SERVER', 'Inherited server URL is not registered');
  } else if (bindings.length) {
    if (bindings.length > 1)
      throw new DomainError(
        'AMBIGUOUS_CONTEXT',
        'Repository has multiple applicable servers; select --server',
        2,
      );

    const bound = canonicalUrl(bindings[0]!.url, true);

    selected = registered.find((s) => s.url === bound);
    sources.server = 'repository';

    if (!selected)
      throw new DomainError('UNTRUSTED_SERVER', 'Repository selects an unregistered server');
  } else if (config?.defaultServer) {
    selected = registered.find((s) => s.alias === config.defaultServer);
    sources.server = 'default';

    if (!selected) failure('defaultServer is not registered');
  }

  if (
    !(parsed.descriptor.name === 'doctor' && parsed.flags.offline) &&
    env.TEAMCITY_TOKEN &&
    (!env.TEAMCITY_URL ||
      !selected ||
      canonicalUrl(env.TEAMCITY_URL, selected.allowHttpLoopback) !== selected.url)
  )
    throw new DomainError(
      'AUTH_CONTEXT_MISMATCH',
      'Inherited token is not bound to the selected server',
    );

  const matched = selected
    ? bindings.filter((b) => canonicalUrl(b.url, true) === selected!.url)
    : [];

  if (matched.length > 1)
    throw new DomainError(
      'AMBIGUOUS_CONTEXT',
      'Duplicate repository bindings for the selected server',
      2,
    );

  const binding = matched[0];
  const project =
    parsed.flags.project !== undefined ? String(parsed.flags.project) : binding?.project;
  const job = parsed.flags.job !== undefined ? String(parsed.flags.job) : binding?.job;

  if ((project !== undefined && !id(project)) || (job !== undefined && !id(job)))
    failure('Invalid project/job identity');

  if (project) sources.project = parsed.flags.project ? 'flag' : 'repository';

  if (job) sources.job = parsed.flags.job ? 'flag' : 'repository';

  let branch = parsed.flags['all-branches']
    ? undefined
    : parsed.flags['literal-branch'] !== undefined
      ? String(parsed.flags['literal-branch'])
      : parsed.flags.branch !== undefined
        ? String(parsed.flags.branch)
        : branchRef;

  if (parsed.flags['literal-branch'] === undefined && branch === '@this') {
    if (!branchRef)
      throw new DomainError('CONTEXT_REQUIRED', 'No current logical branch is available', 2);

    branch = branchRef;
  }

  if (branch !== undefined)
    sources.branch = parsed.flags.branch || parsed.flags['literal-branch'] ? 'flag' : 'git';

  let revision = parsed.flags.revision !== undefined ? String(parsed.flags.revision) : head;

  if (revision === '@head') {
    if (!head) throw new DomainError('CONTEXT_REQUIRED', 'No committed Git HEAD is available', 2);

    revision = head;
  }

  let vcsRootId =
    parsed.flags['vcs-root'] !== undefined ? String(parsed.flags['vcs-root']) : undefined;

  if (!vcsRootId && selected) {
    const mappings = overlay?.vcsRoots?.filter((m) => m.server === selected!.alias) ?? [];

    if (
      mappings.length === 1 &&
      repositoryRoot &&
      (await git(cwd, ['config', '--get', `remote.${mappings[0]!.remote}.url`], env, deadline))
    )
      vcsRootId = mappings[0]!.rootId;
  }

  if (vcsRootId !== undefined && !id(vcsRootId)) failure('Invalid VCS root identity');

  if (revision !== undefined && !id(revision)) failure('Invalid revision identity');

  const jobs =
    parsed.flags.job !== undefined
      ? [job!]
      : binding?.jobs?.length
        ? [...new Set(binding.jobs)]
        : job
          ? [job]
          : [];

  if (Date.now() >= deadline)
    throw new DomainError('DEADLINE_EXCEEDED', 'Overall deadline exceeded', 1, true);

  return freeze({
    deadline,
    cwd,
    ...(repositoryRoot ? { repositoryRoot } : {}),
    ...(head ? { head } : {}),
    ...(changes !== undefined ? { dirty: changes !== '' } : {}),
    ...(selected ? { server: selected.alias, serverUrl: selected.url } : {}),
    ...(project ? { project } : {}),
    ...(job ? { job } : {}),
    jobs,
    ...(branch !== undefined ? { branch } : {}),
    ...(revision ? { revision } : {}),
    ...(vcsRootId ? { vcsRootId } : {}),
    sources,
    ...(config ? { config } : {}),
  });
}

export function publicContext(context: ExecutionContext): Record<string, unknown> | undefined {
  if (!context.server) return undefined;

  return {
    server: context.server,
    ...(context.project ? { project: context.project } : {}),
    ...(context.job ? { job: context.job } : {}),
    ...(context.jobs.length ? { jobs: [...context.jobs] } : {}),
    ...(context.branch !== undefined ? { branch: context.branch } : {}),
    ...(context.revision ? { revision: context.revision } : {}),
    ...(context.vcsRootId ? { vcsRootId: context.vcsRootId } : {}),
  };
}
