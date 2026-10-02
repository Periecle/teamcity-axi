import type { ExecutionContext } from '../context/resolve.js';
import type { ProcessLimits } from './process.js';

export type ReadProfile = 'simple' | 'graph' | 'status' | 'watch';

export function readProfile(command: string): ReadProfile {
  if (command === 'run.watch') return 'watch';

  if (command === 'status') return 'status';

  if (command === 'run.tree' || command === 'run.failure') return 'graph';

  return 'simple';
}

export function readLimits(
  context: ExecutionContext,
  profile: ReadProfile = 'simple',
): ProcessLimits {
  return {
    deadline: context.deadline,
    concurrency: Math.min(
      profile === 'watch' ? 1 : profile === 'status' ? 2 : 3,
      context.config?.limits?.concurrency ?? 3,
    ),
    maxChildren: Math.min(
      profile === 'watch' ? 32 : profile === 'graph' ? 24 : profile === 'status' ? 6 : 8,
      context.config?.limits?.maxChildProcesses ?? 256,
    ),
    stdoutBytes: profile === 'status' || profile === 'watch' ? 1048576 : 2097152,
    stderrBytes: 65536,
  };
}

export function publicLimits(limits: ProcessLimits, maxBytes: number) {
  return {
    maxBytes,
    maxChildProcesses: limits.maxChildren,
    concurrency: limits.concurrency,
    deadline: limits.deadline,
    stdoutCaptureBytes: limits.stdoutBytes,
    stderrCaptureBytes: limits.stderrBytes,
  };
}
