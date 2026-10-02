import { parse } from './parser.js';
import { help } from './registry.js';
import { asDomainError, DomainError } from '../domain/errors.js';
import { response } from '../domain/response.js';
import type { Response } from '../domain/response.js';

export async function main(args: readonly string[]): Promise<void> {
  let format: 'json' | 'toon' =
    args.includes('--json') ||
    args.some((v, i) => v === '--format=json' || (v === 'json' && args[i - 1] === '--format'))
      ? 'json'
      : 'toon';
  let command = 'status';
  let maxBytes = 6144;
  let output: Response;
  let patterns: readonly string[] = [];
  let safeContext: Record<string, unknown> | undefined;
  let additionalSecretNames: readonly string[] = [];
  let terminationCode = 0;

  try {
    const parsed = parse(args);

    format = parsed.format;
    command = parsed.descriptor.name;
    maxBytes = Number(
      parsed.flags['max-bytes'] ??
        (command === 'status'
          ? 6144
          : ['run.tree', 'run.failure'].includes(command)
            ? 24576
            : 16384),
    );

    if (parsed.flags.help) {
      process.stdout.write(
        format === 'json'
          ? JSON.stringify(
              parsed.home
                ? { commands: (await import('./registry.js')).registry }
                : parsed.descriptor,
            ) + '\n'
          : help(parsed.home ? undefined : parsed.descriptor),
      );

      return;
    }

    if (command === 'schema') {
      const { descriptor } = await import('./registry.js');
      const d = descriptor(parsed.positional!);

      if (!d) throw new DomainError('USAGE_ERROR', 'Unknown schema command', 2);
      const { packagedSchema } = await import('../output/schema.js');

      output = response(command, {
        descriptor: d,
        envelope: packagedSchema('response'),
        ...(d.name === 'run.failure'
          ? { payload: packagedSchema('failure') }
          : d.name === 'run.view'
            ? { payload: packagedSchema('run-view') }
            : d.name === 'run.list'
              ? { payload: packagedSchema('run-list') }
              : d.name === 'context.show'
                ? { payload: packagedSchema('context-show') }
                : d.name === 'doctor'
                  ? { payload: packagedSchema('doctor') }
                  : [
                        'run.problems',
                        'run.tests',
                        'run.log',
                        'run.changes',
                        'run.tree',
                        'job.view',
                        'job.list',
                        'queue.list',
                      ].includes(d.name)
                    ? { payload: packagedSchema(d.name.replace('.', '-')) }
                    : {}),
      });
      maxBytes = Number(parsed.flags['max-bytes'] ?? 65536);
    } else {
      const { resolveContext, publicContext } = await import('../context/resolve.js');
      const context = await resolveContext(parsed);

      patterns = context.config?.secretNamePatterns ?? [];
      additionalSecretNames = context.server
        ? (context.config?.servers[context.server]?.forwardHeaderEnvNames ?? [])
        : [];
      maxBytes = Math.min(maxBytes, context.config?.limits?.maxBytes ?? 262144);
      const scope = publicContext(context);

      if (context.server)
        safeContext = {
          server: context.server,
          ...(parsed.flags.job ? { job: String(parsed.flags.job) } : {}),
          ...(parsed.flags.project ? { project: String(parsed.flags.project) } : {}),
        };

      if (command === 'context.show' && !parsed.flags.verify) {
        const { localContext } = await import('../commands/diagnostics.js');

        output = localContext(context);
      } else if (command === 'status' && parsed.home && context.jobs.length === 0) {
        output = response(command, {
          mode: 'unconfigured',
          checkout: {
            head: context.head ?? null,
            branch: context.branch ?? null,
            dirty: context.dirty ?? null,
          },
          message: 'Configure a trusted server and repository binding to observe this checkout',
        });
        if (scope) output.context = scope;
        if (!parsed.flags['no-hints'])
          output.next = [
            { reason: 'Inspect local context', argv: ['teamcity-axi', 'context', 'show'] },
          ];
      } else if (
        [
          'run.view',
          'run.list',
          'run.problems',
          'run.tests',
          'run.log',
          'run.changes',
          'run.tree',
          'run.failure',
          'job.view',
          'job.list',
          'queue.list',
          'context.show',
          'doctor',
        ].includes(command)
      ) {
        if (!context.server && !(command === 'doctor' && parsed.flags.offline))
          throw new DomainError('CONTEXT_REQUIRED', 'Select a registered trusted server', 2);
        const controller = new AbortController();

        const interrupt = () => {
          terminationCode = 130;
          controller.abort();
        };

        const terminate = () => {
          terminationCode = 143;
          controller.abort();
        };

        process.on('SIGINT', interrupt);
        process.on('SIGTERM', terminate);

        try {
          if (command === 'run.view') {
            const { viewRun } = await import('../commands/run-view.js');

            output = await viewRun(parsed, context, controller.signal);
          } else if (command === 'run.list') {
            const { listRuns } = await import('../commands/run-list.js');

            output = await listRuns(parsed, context, controller.signal);
          } else if (command === 'queue.list') {
            const { listQueue } = await import('../commands/queue.js');

            output = await listQueue(parsed, context, controller.signal);
          } else if (command === 'job.view' || command === 'job.list') {
            const { viewJob, listJobs } = await import('../commands/jobs.js');

            output = await (command === 'job.view' ? viewJob : listJobs)(
              parsed,
              context,
              controller.signal,
            );
          } else if (command === 'run.failure') {
            const { readFailure } = await import('../commands/run-failure.js');

            output = await readFailure(parsed, context, controller.signal);
          } else if (command === 'run.tree') {
            const { readTree } = await import('../commands/run-tree.js');

            output = await readTree(parsed, context, controller.signal);
          } else if (command === 'run.changes') {
            const { readChanges } = await import('../commands/run-changes.js');

            output = await readChanges(parsed, context, controller.signal);
          } else if (['run.problems', 'run.tests', 'run.log'].includes(command)) {
            const { readEvidence } = await import('../commands/run-evidence.js');

            output = await readEvidence(parsed, context, controller.signal);
          } else {
            const { diagnose } = await import('../commands/diagnostics.js');

            output = await diagnose(parsed, context, controller.signal);
          }
        } finally {
          process.removeListener('SIGINT', interrupt);
          process.removeListener('SIGTERM', terminate);
        }

        if (parsed.flags.debug)
          process.stderr.write(
            JSON.stringify({ command, childProcesses: output.meta.counts?.childProcesses }) + '\n',
          );
      } else {
        if (!context.server)
          throw new DomainError('CONTEXT_REQUIRED', 'Select a registered trusted server', 2);
        if (command === 'status' && context.jobs.length === 0)
          throw new DomainError('CONTEXT_REQUIRED', 'Select a job or repository tracked jobs', 2);

        // Explicitly gated during foundation work; never substitute synthetic remote data.
        throw new DomainError(
          'DEPENDENCY_UNSUPPORTED',
          'Remote command services are not implemented in this development build',
        );
      }
    }
  } catch (error) {
    const domain = asDomainError(error);

    output = {
      schemaVersion: '1.0',
      command,
      status: 'error',
      ...(safeContext ? { context: safeContext } : {}),
      error: domain.publicValue(),
      meta: { observedAt: new Date().toISOString(), complete: false, truncated: false },
    };
    process.exitCode = terminationCode || domain.exitCode;
  }

  try {
    const { render } = await import('../output/render.js');
    const { knownSecrets } = await import('../output/sanitize.js');
    const result = render(
      output,
      format,
      maxBytes,
      knownSecrets(process.env, patterns, additionalSecretNames),
      patterns,
    );

    if (result.response.status === 'error' && !process.exitCode) process.exitCode = 1;
    process.stdout.write(result.document);
  } catch {
    // A contract violation must not expose source data or stack traces on stderr.
    const fallback = response(command, {});

    delete fallback.data;
    fallback.status = 'error';
    fallback.meta.complete = false;
    fallback.error = {
      code: 'INTERNAL_ERROR',
      message: 'Cannot render the normalized result safely',
      retryable: false,
    };
    const { render } = await import('../output/render.js');
    const { knownSecrets } = await import('../output/sanitize.js');

    process.stdout.write(
      render(
        fallback,
        format,
        Math.max(2048, maxBytes),
        knownSecrets(process.env, patterns, additionalSecretNames),
        patterns,
      ).document,
    );
    process.exitCode = 1;
  }
}
