import type { Parsed } from '../cli/parser.js';
import { DomainError } from '../domain/errors.js';
import { response } from '../domain/response.js';
import type { Response } from '../domain/response.js';
import type { Job, TeamCityReader } from '../domain/teamcity.js';
import { publicContext } from '../context/resolve.js';
import type { ExecutionContext } from '../context/resolve.js';
import { ProjectPolicy } from '../context/project-policy.js';
import { readLimits } from '../transport/limits.js';
import { ProcessTransport, resolveBinary } from '../transport/process.js';
import { NativeTeamCityReader } from '../adapter/reader.js';
import { knownSecrets } from '../output/sanitize.js';

const capabilityNames = [
  'structuredRunDetail',
  'boundedRunPages',
  'independentProblemPages',
  'independentTestPages',
  'snapshotDependencyPages',
  'structuredLogTail',
  'boundedChangesPages',
  'scopedQueueRead',
  'safeAgentRead',
] as const;

type Capability = {
  name: (typeof capabilityNames)[number];
  required: boolean;
  state: 'available' | 'unavailable' | 'not_probed';
  errorCode?: string;
  runId?: string;
};

export function localContext(context: ExecutionContext): Response {
  const output = response('context.show', {
    scope: publicContext(context) ?? null,
    checkout: {
      head: context.head ?? null,
      branch: context.branch ?? null,
      dirty: context.dirty ?? null,
    },
    sources: context.sources,
    readOnly: true,
    verification: {
      requested: false,
      authentication: 'not_checked',
      identityFingerprint: null,
      jobs: [],
      project: null,
      policy:
        context.server && context.config?.servers[context.server]?.allowedProjects
          ? 'not_checked'
          : 'not_configured',
      nativeVersion: null,
    },
  });
  const scope = publicContext(context);

  if (scope) output.context = scope;

  return output;
}

async function verifiedScope(
  context: ExecutionContext,
  reader: TeamCityReader,
  policy: ProjectPolicy,
) {
  const jobIds = [...new Set([...(context.job ? [context.job] : []), ...context.jobs])];

  if (jobIds.length > 5)
    throw new DomainError(
      'INPUT_LIMIT_EXCEEDED',
      'Context verification allows at most five selected jobs',
    );

  const jobs: Job[] = [];

  for (const id of jobIds) {
    const read = await reader.getJob({ id }, { deadline: context.deadline });

    if (read.state === 'unavailable') throw read.error;

    if (context.project && read.value.projectId !== context.project)
      throw new DomainError('CONTEXT_MISMATCH', 'Selected job belongs to a different project');

    await policy.assert(read.value.projectId);
    jobs.push({ ...read.value, name: Array.from(read.value.name).slice(0, 200).join('') });
  }

  const projectId = context.project ?? (jobs.length === 1 ? jobs[0]!.projectId : undefined);
  const project = projectId ? await policy.project(projectId) : null;

  if (projectId) await policy.assert(projectId);

  const identity = await reader.getIdentity({ deadline: context.deadline });

  if (identity.state === 'unavailable') throw identity.error;

  return { jobs, project, identityFingerprint: identity.value.fingerprint };
}

export async function diagnose(
  parsed: Parsed,
  context: ExecutionContext,
  signal: AbortSignal,
): Promise<Response> {
  const offline = parsed.descriptor.name === 'doctor' && parsed.flags.offline === true;

  if (!offline && !context.server)
    throw new DomainError('CONTEXT_REQUIRED', 'Select a registered trusted server', 2);

  if (
    !offline &&
    parsed.descriptor.name === 'doctor' &&
    !context.project &&
    !context.job &&
    !context.jobs.length
  )
    throw new DomainError(
      'CONTEXT_REQUIRED',
      'Online doctor requires a selected project or job for bounded probes',
      2,
    );

  const binary = await resolveBinary(
    context.config?.binaryPath,
    context.repositoryRoot,
    context.config?.allowWorkspaceBinary,
  );
  const server = context.server ? context.config?.servers[context.server] : undefined;
  const limits = readLimits(context);
  const transport = await ProcessTransport.create({
    binary,
    serverUrl: context.serverUrl ?? 'https://offline.invalid',
    env: offline
      ? Object.fromEntries(
          Object.entries(process.env).filter(([name]) => !name.startsWith('TEAMCITY_')),
        )
      : process.env,
    signal,
    ...(!offline && server?.forwardHeaderEnvNames
      ? { headerNames: server.forwardHeaderEnvNames }
      : {}),
    limits,
  });

  try {
    const captured = await transport.execute({ kind: 'version' });
    const matched = /^teamcity version ([a-zA-Z0-9.+-]{1,80})\r?\n$/.exec(
      captured.stdout.toString('utf8'),
    );

    if (captured.exitCode !== 0 || captured.signal || !matched)
      throw new DomainError(
        'DEPENDENCY_UNSUPPORTED',
        'Native executable has an unsupported version response',
      );

    const version = matched[1]!;
    const reader = new NativeTeamCityReader(
      transport,
      context.serverUrl ?? 'https://offline.invalid',
      knownSecrets(process.env, context.config?.secretNamePatterns, server?.forwardHeaderEnvNames),
    );
    const policy = new ProjectPolicy(reader, server?.allowedProjects, {
      deadline: context.deadline,
    });
    const scope = offline ? null : await verifiedScope(context, reader, policy);
    let output: Response;

    if (parsed.descriptor.name === 'context.show') {
      output = localContext(context);
      output.data!.verification = {
        requested: true,
        authentication: 'authenticated',
        identityFingerprint: scope!.identityFingerprint,
        jobs: scope!.jobs,
        project: scope!.project,
        policy: server?.allowedProjects
          ? scope!.jobs.length || scope!.project
            ? 'verified'
            : 'not_checked'
          : 'not_configured',
        nativeVersion: version,
      };

      if (server?.allowedProjects && !scope!.jobs.length && !scope!.project) {
        output.status = 'partial';
        output.meta.complete = false;
        output.meta.limitations = [
          {
            code: 'POLICY_SCOPE_UNVERIFIED',
            message:
              'Authentication is verified; select a project or job to verify narrowing policy',
            source: 'context',
          },
        ];
      }
    } else {
      const capabilities: Capability[] = capabilityNames.map((name) => ({
        name,
        required: name !== 'structuredLogTail',
        state: 'not_probed',
      }));
      const set = (name: Capability['name'], values: Omit<Capability, 'name' | 'required'>) =>
        Object.assign(
          capabilities.find((c) => c.name === name)!,
          values,
        );
      let serverInfo = null;

      if (!offline) {
        const read = await reader.getServer({ deadline: context.deadline });

        if (read.state === 'unavailable') throw read.error;

        serverInfo = read.value;
        const jobId = scope!.jobs[0]?.id,
          projectId = jobId ? scope!.jobs[0]!.projectId : scope!.project!.id;
        const page = await reader.listRuns(
          {
            ...(jobId ? { jobId } : {}),
            projectId,
            count: 1,
            start: 0,
            scanLimit: 5000,
            ...(server?.allowedProjects ? { allowedProjects: [projectId] } : {}),
          },
          { deadline: context.deadline },
        );

        if (page.state === 'unavailable') throw page.error;

        set('boundedRunPages', { state: 'available' });
        const sample = page.value.runs[0];

        if (sample) {
          const detail = await reader.getRun({ id: sample.id }, { deadline: context.deadline });

          if (detail.state === 'unavailable') throw detail.error;

          if (detail.value.jobId !== sample.jobId || detail.provenance.projectId !== projectId)
            throw new DomainError('CONTEXT_MISMATCH', 'Capability probe changed execution scope');

          await policy.assert(detail.provenance.projectId);
          set('structuredRunDetail', { state: 'available', runId: sample.id });
          const log = await reader.getLogTail({ id: sample.id }, 1, { deadline: context.deadline });

          if (log.state === 'unavailable') {
            if (log.error.code === 'INTERRUPTED') throw log.error;

            set('structuredLogTail', {
              state: 'unavailable',
              errorCode: log.error.code,
              runId: sample.id,
            });
          } else set('structuredLogTail', { state: 'available', runId: sample.id });
        }
      }

      output = response('doctor', {
        offline,
        executable: {
          name: binary.split('/').at(-1),
          resolved: true,
          version,
          versionStatus: version === '1.5.0' ? 'recorded' : 'unverified_version',
        },
        target: { registered: Boolean(context.server) },
        server: serverInfo,
        authentication: {
          state: offline ? 'not_checked' : 'authenticated',
          identityFingerprint: scope?.identityFingerprint ?? null,
        },
        policy: {
          readOnly: true,
          state: offline ? 'not_checked' : server?.allowedProjects ? 'verified' : 'not_configured',
        },
        liveCertified: false,
        output: { formats: ['json', 'toon'], schemaVersion: '1.0' },
        capabilities,
        limits: {
          concurrency: limits.concurrency,
          maxChildProcesses: limits.maxChildren,
          stdoutCaptureBytes: limits.stdoutBytes,
          stderrCaptureBytes: limits.stderrBytes,
          maxBytes: Math.min(
            Number(parsed.flags['max-bytes'] ?? 16384),
            context.config?.limits?.maxBytes ?? 262144,
          ),
          remainingMs: Math.max(0, context.deadline - Date.now()),
        },
      });

      if (!offline && capabilities.some((c) => c.required && c.state === 'not_probed')) {
        output.status = 'partial';
        output.meta.complete = false;
        output.meta.limitations = [
          {
            code: 'CAPABILITIES_NOT_PROBED',
            message: 'Capabilities without a scoped adapter or retained sample remain unverified',
            source: 'context',
          },
        ];
      }

      const missingLog = capabilities.find(
        (c) => c.name === 'structuredLogTail' && c.state === 'unavailable',
      );

      if (missingLog)
        (output.meta.limitations ??= []).push({
          code: 'OPTIONAL_LOG_UNAVAILABLE',
          message: 'Structured log tail is unavailable; metadata reads remain usable',
          source: 'log',
        });

      if (
        missingLog &&
        ['DEADLINE_EXCEEDED', 'INPUT_LIMIT_EXCEEDED'].includes(missingLog.errorCode ?? '')
      ) {
        output.status = 'partial';
        output.meta.complete = false;
        (output.meta.limitations ??= []).push({
          code: missingLog.errorCode!,
          message: 'Optional log probe exhausted its shared budget; acquired metadata is retained',
          source: 'log',
        });
      }

      const contextScope = publicContext(context);

      if (contextScope) output.context = contextScope;
    }

    output.meta.counts = { childProcesses: transport.childProcesses };

    if (version !== '1.5.0')
      (output.meta.limitations ??= []).push({
        code: 'UNVERIFIED_VERSION',
        message: 'Native version has not been release-certified',
        source: 'context',
      });

    if (parsed.flags['require-complete'] && output.status === 'partial') process.exitCode = 1;

    return output;
  } finally {
    await transport.dispose();
  }
}
