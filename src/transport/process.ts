import { spawn } from 'node:child_process';
import { access, realpath, mkdtemp, rm } from 'node:fs/promises';
import { constants } from 'node:fs';
import { tmpdir } from 'node:os';
import { isAbsolute, join, relative, delimiter, sep } from 'node:path';

import { DomainError } from '../domain/errors.js';
import { canonicalUrl } from '../context/url.js';

export type Operation =
  | { kind: 'api'; path: string }
  | { kind: 'log'; runId: string; tail: number }
  | { kind: 'version' };

export interface Captured {
  stdout: Buffer;
  stderr: Buffer;
  exitCode: number | null;
  signal: NodeJS.Signals | null;
}

export interface ProcessLimits {
  deadline: number;
  concurrency: number;
  maxChildren: number;
  stdoutBytes: number;
  stderrBytes: number;
}

export interface TransportOptions {
  binary: string;
  serverUrl: string;
  env: NodeJS.ProcessEnv;
  limits: ProcessLimits;
  signal?: AbortSignal;
  headerNames?: readonly string[];
}

const readFamilies = new Set([
  'server',
  'builds',
  'buildTypes',
  'projects',
  'buildQueue',
  'agents',
  'testOccurrences',
  'problemOccurrences',
  'changes',
]);

function inside(path: string, root: string) {
  const rel = relative(root, path);

  return rel === '' || (!(rel === '..' || rel.startsWith('..' + sep)) && !isAbsolute(rel));
}

export async function resolveBinary(
  path: string | undefined,
  repository: string | undefined,
  allowWorkspace = false,
  env: NodeJS.ProcessEnv = process.env,
): Promise<string> {
  const paths = path
    ? [path]
    : (env.PATH ?? '')
        .split(delimiter)
        .filter(isAbsolute)
        .map((p) => join(p, 'teamcity'));

  if (path && !isAbsolute(path))
    throw new DomainError('USAGE_ERROR', 'Trusted binaryPath must be absolute', 2);

  for (const candidate of paths) {
    let resolved: string;

    try {
      resolved = await realpath(candidate);
      await access(resolved, constants.X_OK);
    } catch {
      continue;
    }

    if (
      repository &&
      inside(resolved, repository) &&
      !(allowWorkspace && path && resolved === (await realpath(path)))
    )
      throw new DomainError(
        'POLICY_DENIED',
        'Native executable resolves inside the untrusted workspace',
      );

    return resolved;
  }

  throw new DomainError('DEPENDENCY_MISSING', 'A trusted official teamcity executable is required');
}

export function childEnvironment(
  options: Pick<TransportOptions, 'env' | 'serverUrl' | 'headerNames'>,
): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = {};
  const names = [
    'HOME',
    'PATH',
    'XDG_CONFIG_HOME',
    'XDG_DATA_HOME',
    'XDG_RUNTIME_DIR',
    'USER',
    'LOGNAME',
    'LANG',
    'LC_ALL',
    'SSL_CERT_FILE',
    'SSL_CERT_DIR',
    'NODE_EXTRA_CA_CERTS',
    'HTTPS_PROXY',
    'HTTP_PROXY',
    'ALL_PROXY',
    'NO_PROXY',
    'https_proxy',
    'http_proxy',
    'all_proxy',
    'no_proxy',
  ];

  for (const name of names) if (options.env[name] !== undefined) env[name] = options.env[name];

  if (options.env.TEAMCITY_TOKEN) {
    if (
      !options.env.TEAMCITY_URL ||
      canonicalUrl(options.env.TEAMCITY_URL, true) !== canonicalUrl(options.serverUrl, true)
    )
      throw new DomainError(
        'AUTH_CONTEXT_MISMATCH',
        'Inherited token is not bound to the frozen trusted server',
      );

    env.TEAMCITY_TOKEN = options.env.TEAMCITY_TOKEN;
  }

  for (const name of options.headerNames ?? []) {
    if (
      !/^TEAMCITY_HEADER_[A-Z0-9_]+$/.test(name) ||
      /(?:AUTHORIZATION|HOST|COOKIE|PROXY_AUTHORIZATION|CONNECTION|TRANSFER_ENCODING)/.test(name)
    )
      throw new DomainError('POLICY_DENIED', 'Unsafe custom header environment name');

    const value = options.env[name];

    if (value !== undefined) {
      if (/[\r\n\u0000]/.test(value))
        throw new DomainError('POLICY_DENIED', 'Invalid custom header value');

      env[name] = value;
    }
  }

  return {
    ...env,
    TEAMCITY_URL: options.serverUrl,
    TEAMCITY_RO: '1',
    TEAMCITY_NO_UPDATE: '1',
    DO_NOT_TRACK: '1',
    NO_COLOR: '1',
    TERM: 'dumb',
  };
}

function argv(operation: Operation): string[] {
  if (operation.kind === 'version') return ['--version'];

  if (operation.kind === 'log') {
    if (
      !/^[1-9]\d*$/.test(operation.runId) ||
      !Number.isSafeInteger(Number(operation.runId)) ||
      !Number.isInteger(operation.tail) ||
      operation.tail < 1 ||
      operation.tail > 1000
    )
      throw new DomainError('USAGE_ERROR', 'Invalid bounded log request', 2);

    return [
      'run',
      'log',
      operation.runId,
      '--tail',
      String(operation.tail),
      '--json',
      '--no-input',
    ];
  }

  const path = operation.path;
  const rawPath = path.split('?')[0]!;

  if (
    Buffer.byteLength(path) > 16384 ||
    !path.startsWith('/app/rest/') ||
    /[\r\n\u0000#\\]/.test(path) ||
    /%2f|%5c|%2e/i.test(rawPath) ||
    rawPath.split('/').some((p) => p === '.' || p === '..')
  )
    throw new DomainError('POLICY_DENIED', 'Unsafe adapter path');

  let url: URL;

  try {
    url = new URL(path, 'https://adapter.invalid');
  } catch {
    throw new DomainError('POLICY_DENIED', 'Invalid adapter path');
  }

  if (
    url.origin !== 'https://adapter.invalid' ||
    (!readFamilies.has(url.pathname.split('/')[3] ?? '') &&
      !(
        url.pathname === '/app/rest/users/current' &&
        url.searchParams.get('fields') === 'id,username' &&
        [...url.searchParams.keys()].length === 1
      )) ||
    [...url.searchParams.keys()].some((k) => !['locator', 'fields'].includes(k))
  )
    throw new DomainError(
      'POLICY_DENIED',
      'Resource or query is outside the read-only adapter surface',
    );

  let decoded: string;

  try {
    decoded = decodeURIComponent(url.pathname);
  } catch {
    throw new DomainError('POLICY_DENIED', 'Unsafe path encoding');
  }

  if (
    decoded.split('/').some((p) => p === '.' || p === '..') ||
    path.split('/').some((p) => p === '.' || p === '..') ||
    /%2f|%5c|%2e/i.test(url.pathname)
  )
    throw new DomainError('POLICY_DENIED', 'Unsafe path encoding');

  return [
    'api',
    path,
    '-X',
    'GET',
    '--include',
    '--raw',
    '-H',
    'Accept: application/json',
    '--no-input',
  ];
}

export class ProcessTransport {
  private active = 0;
  private launches = 0;
  private readonly waiting: (() => void)[] = [];
  private readonly env: NodeJS.ProcessEnv;
  private readonly children = new Set<number>();
  private disposed = false;

  private constructor(
    private readonly options: TransportOptions,
    private readonly cwd: string,
  ) {
    this.env = childEnvironment(options);
  }

  static async create(options: TransportOptions): Promise<ProcessTransport> {
    const { limits } = options;

    if (
      !isAbsolute(options.binary) ||
      !Number.isInteger(limits.concurrency) ||
      limits.concurrency < 1 ||
      limits.concurrency > 8 ||
      !Number.isInteger(limits.maxChildren) ||
      limits.maxChildren < 1 ||
      limits.maxChildren > 256 ||
      !Number.isFinite(limits.deadline) ||
      !Number.isInteger(limits.stdoutBytes) ||
      limits.stdoutBytes < 1 ||
      limits.stdoutBytes > 2097152 ||
      !Number.isInteger(limits.stderrBytes) ||
      limits.stderrBytes < 1 ||
      limits.stderrBytes > 65536
    )
      throw new DomainError('INTERNAL_ERROR', 'Invalid process limits');

    // Validate the environment before creating invocation-owned filesystem state.
    childEnvironment(options);

    return new ProcessTransport(options, await mkdtemp(join(tmpdir(), 'teamcity-axi-child-')));
  }

  get childProcesses() {
    return this.launches;
  }

  private check() {
    if (this.disposed || this.options.signal?.aborted)
      throw new DomainError('INTERRUPTED', 'Invocation interrupted');

    if (Date.now() >= this.options.limits.deadline)
      throw new DomainError('DEADLINE_EXCEEDED', 'Overall deadline exceeded', 1, true);
  }

  private async acquire(maxChildProcesses?: number): Promise<void> {
    this.check();

    while (this.active >= this.options.limits.concurrency) {
      await new Promise<void>((resolve) => this.waiting.push(resolve));
      this.check();
    }

    if (maxChildProcesses !== undefined && this.launches >= maxChildProcesses)
      throw new DomainError(
        'CALL_LIMIT_EXCEEDED',
        'Reserved child launch capacity cannot be consumed',
      );

    if (this.launches >= this.options.limits.maxChildren)
      throw new DomainError('INPUT_LIMIT_EXCEEDED', 'Child launch budget exhausted');

    this.active++;
    this.launches++;
  }

  private kill(pid: number, signal: NodeJS.Signals) {
    try {
      process.kill(process.platform === 'win32' ? pid : -pid, signal);
    } catch {
      /* Already reaped. */
    }
  }

  async execute(operation: Operation, maxChildProcesses?: number): Promise<Captured> {
    if (
      maxChildProcesses !== undefined &&
      (!Number.isInteger(maxChildProcesses) || maxChildProcesses < 0 || maxChildProcesses > 256)
    )
      throw new DomainError('INTERNAL_ERROR', 'Invalid reserved launch ceiling');

    const args = argv(operation);

    await this.acquire(maxChildProcesses);

    try {
      return await new Promise<Captured>((resolve, reject) => {
        const child = spawn(this.options.binary, args, {
          cwd: this.cwd,
          env: this.env,
          shell: false,
          detached: process.platform !== 'win32',
          stdio: ['ignore', 'pipe', 'pipe'],
        });

        if (child.pid) this.children.add(child.pid);

        const stdout: Buffer[] = [],
          stderr: Buffer[] = [];
        let outBytes = 0,
          errBytes = 0,
          failure: DomainError | undefined,
          killTimer: NodeJS.Timeout | undefined;

        const stop = (error: DomainError) => {
          if (failure) return;

          failure = error;

          if (child.pid) {
            this.kill(child.pid, 'SIGTERM');
            killTimer = setTimeout(() => this.kill(child.pid!, 'SIGKILL'), 200);
          }
        };

        const onAbort = () => stop(new DomainError('INTERRUPTED', 'Invocation interrupted'));
        const timer = setTimeout(
          () => stop(new DomainError('DEADLINE_EXCEEDED', 'Overall deadline exceeded', 1, true)),
          Math.max(1, this.options.limits.deadline - Date.now()),
        );

        this.options.signal?.addEventListener('abort', onAbort, { once: true });

        if (this.options.signal?.aborted) onAbort();

        child.stdout.on('data', (chunk: Buffer) => {
          outBytes += chunk.length;

          if (outBytes > this.options.limits.stdoutBytes)
            stop(new DomainError('INPUT_LIMIT_EXCEEDED', 'Child stdout capture limit exceeded'));

          if (!failure) stdout.push(chunk);
        });
        child.stderr.on('data', (chunk: Buffer) => {
          errBytes += chunk.length;

          if (errBytes > this.options.limits.stderrBytes)
            stop(new DomainError('INPUT_LIMIT_EXCEEDED', 'Child stderr capture limit exceeded'));

          if (!failure) stderr.push(chunk);
        });
        child.on('error', () => {
          failure ??= new DomainError(
            'DEPENDENCY_MISSING',
            'Cannot start the selected native executable',
          );
        });
        child.on('close', (exitCode, signal) => {
          clearTimeout(timer);

          if (killTimer) clearTimeout(killTimer);

          this.options.signal?.removeEventListener('abort', onAbort);

          if (child.pid) {
            this.kill(child.pid, 'SIGKILL');
            this.children.delete(child.pid);
          }

          if (failure) reject(failure);
          else
            resolve({
              stdout: Buffer.concat(stdout),
              stderr: Buffer.concat(stderr),
              exitCode,
              signal,
            });
        });
      });
    } finally {
      this.active--;

      for (const wake of this.waiting.splice(0)) wake();
    }
  }

  async dispose(): Promise<void> {
    this.disposed = true;

    for (const pid of this.children) this.kill(pid, 'SIGKILL');

    for (const wake of this.waiting.splice(0)) wake();

    while (this.active > 0) await new Promise<void>((resolve) => this.waiting.push(resolve));

    await rm(this.cwd, { recursive: true, force: true });
  }
}
