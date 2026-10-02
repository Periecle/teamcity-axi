import { DomainError } from '../domain/errors.js';
import type { ExecutionContext } from '../context/resolve.js';
import { ProcessTransport, resolveBinary } from '../transport/process.js';
import { NativeTeamCityReader } from '../adapter/reader.js';
import { knownSecrets } from '../output/sanitize.js';

export async function openReadSession(
  context: ExecutionContext,
  signal: AbortSignal,
  profile: 'simple' | 'graph' = 'simple',
) {
  const binary = await resolveBinary(
    context.config?.binaryPath,
    context.repositoryRoot,
    context.config?.allowWorkspaceBinary,
  );
  const server = context.config?.servers[context.server!];
  const maxChildProcesses = Math.min(
    profile === 'graph' ? 24 : 8,
    context.config?.limits?.maxChildProcesses ?? 256,
  );
  const transport = await ProcessTransport.create({
    binary,
    serverUrl: context.serverUrl!,
    env: process.env,
    signal,
    ...(server?.forwardHeaderEnvNames ? { headerNames: server.forwardHeaderEnvNames } : {}),
    limits: {
      deadline: context.deadline,
      concurrency: Math.min(3, context.config?.limits?.concurrency ?? 3),
      maxChildren: maxChildProcesses,
      stdoutBytes: 2097152,
      stderrBytes: 65536,
    },
  });

  try {
    const capture = await transport.execute({ kind: 'version' });
    const match = /^teamcity version ([a-zA-Z0-9.+-]{1,80})\r?\n$/.exec(
      capture.stdout.toString('utf8'),
    );

    if (capture.exitCode !== 0 || capture.signal || !match)
      throw new DomainError(
        'DEPENDENCY_UNSUPPORTED',
        'Native executable has an unsupported version response',
      );

    const reader = new NativeTeamCityReader(
      transport,
      context.serverUrl!,
      knownSecrets(process.env, context.config?.secretNamePatterns, server?.forwardHeaderEnvNames),
    );

    return { reader, transport, nativeVersion: match[1]!, maxChildProcesses };
  } catch (error) {
    await transport.dispose();

    throw error;
  }
}
