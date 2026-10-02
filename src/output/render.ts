import { encode } from '@toon-format/toon';

import type { Response } from '../domain/response.js';
import { sanitize, secretMatchers } from './sanitize.js';
import { validateResponse } from './schema.js';

function serialize(value: Response, format: 'json' | 'toon') {
  return (format === 'json' ? JSON.stringify(value) : encode(value)) + '\n';
}

export interface Rendered {
  document: string;
  response: Response;
}

export function render(
  input: Response,
  format: 'json' | 'toon',
  maxBytes: number,
  secrets: readonly string[] = [],
  patterns: readonly string[] = [],
): Rendered {
  const keys = new Set([
    'schemaVersion',
    'command',
    'status',
    'context',
    'data',
    'error',
    'meta',
    'next',
    'observedAt',
    'complete',
    'truncated',
    'limitations',
    'counts',
    'code',
    'message',
    'retryable',
    'details',
    'reason',
    'argv',
    'server',
    'project',
    'job',
    'jobs',
    'branch',
    'revision',
    'vcsRootId',
    'revisionMatch',
    'searchScope',
    'since',
    'until',
    'timestampBasis',
    'scanLimit',
    'consistency',
    'maxBytes',
    'maxChildProcesses',
    'childProcesses',
    'retries',
    'graphNodes',
    'limits',
    'runId',
    'source',
    'limit',
    'observed',
  ]);
  let value = sanitize(
    structuredClone(input),
    secrets,
    0,
    keys,
    secretMatchers(patterns),
  ) as Response;

  // Public constants and wrapper-generated timestamps do not carry credential data.
  // A coincidental short secret matching them must not break the protocol itself.
  value.schemaVersion = input.schemaVersion;
  value.command = input.command;
  value.status = input.status;
  value.meta.observedAt = input.meta.observedAt;
  validateResponse(value);
  let document = serialize(value, format);

  if (Buffer.byteLength(document) <= maxBytes) return { document, response: value };
  delete value.next;
  document = serialize(value, format);
  if (Buffer.byteLength(document) <= maxBytes) return { document, response: value };
  // Never trim arbitrary arrays: evidence references, graphs and page totals would lie.
  // Until command-specific reducers prove their invariants, return an explicit bounded error.
  value = {
    schemaVersion: '1.0',
    command: input.command,
    status: 'error',
    ...(value.context ? { context: value.context } : {}),
    error: {
      code: 'INPUT_LIMIT_EXCEEDED',
      message:
        'Required output exceeds the byte budget. Narrow the query or increase --max-bytes within the configured ceiling.',
      retryable: false,
    },
    meta: {
      observedAt: input.meta.observedAt,
      complete: false,
      truncated: true,
      limitations: [
        {
          code: 'OUTPUT_LIMIT_EXCEEDED',
          message: 'No partial serialized stream was emitted',
          source: 'output',
        },
      ],
    },
  };
  validateResponse(value);
  document = serialize(value, format);

  if (Buffer.byteLength(document) > maxBytes) {
    // Keep the trusted destination and asserted IDs while omitting verbose branch data.
    if (value.context)
      value.context = Object.fromEntries(
        Object.entries(value.context).filter(([k]) =>
          ['server', 'project', 'job', 'vcsRootId'].includes(k),
        ),
      );
    document = serialize(value, format);
  }

  if (Buffer.byteLength(document) > maxBytes) {
    value.error!.message = 'Output exceeds byte budget';
    delete value.meta.limitations;
    document = serialize(value, format);
  }

  if (Buffer.byteLength(document) > maxBytes) {
    // Even asserted scope may exceed the envelope budget (e.g. three Unicode IDs).
    // Do not alter IDs into different targets. Retain only the trusted destination.
    if (value.context) value.context = { server: value.context.server };
    document = serialize(value, format);
  }

  if (Buffer.byteLength(document) > maxBytes)
    throw new Error('Output budget cannot hold the minimal envelope');
  validateResponse(value);

  return { document, response: value };
}
