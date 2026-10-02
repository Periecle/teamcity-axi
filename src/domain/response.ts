export interface Limitation {
  code: string;
  message: string;
  runId?: string;
  source?: string;
}

export interface Response {
  schemaVersion: '1.0';
  command: string;
  status: 'ok' | 'partial' | 'error';
  context?: Record<string, unknown>;
  data?: Record<string, unknown>;
  error?: { code: string; message: string; retryable: boolean; details?: Record<string, unknown> };
  meta: {
    observedAt: string;
    complete: boolean;
    truncated: boolean;
    limitations?: Limitation[];
    counts?: { childProcesses: number };
    limits?: { maxBytes: number; maxChildProcesses: number; concurrency: number };
    omitted?: Record<string, number>;
  };
  next?: { reason: string; argv: string[] }[];
}

export function response(command: string, data: Record<string, unknown>): Response {
  return {
    schemaVersion: '1.0',
    command,
    status: 'ok',
    data,
    meta: { observedAt: new Date().toISOString(), complete: true, truncated: false },
  };
}
