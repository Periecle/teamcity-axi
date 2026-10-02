import { DomainError } from '../domain/errors.js';
import type { JobSnapshot, StatusQuery } from '../domain/teamcity.js';
import { apiPath, branchCondition, idCondition } from './locator.js';
import { jobFields, normalizeJob } from './jobs.js';
import { identity, normalizeRun, object } from './run.js';
import { nextPosition } from './continuation.js';

export const statusRunFields =
  'id,buildTypeId,number,state,status,branchName,personal,composite,buildType(id,projectId),revisions(revision(version,vcs-root-instance(id,vcs-root-id)))';

export function statusRequest(query: StatusQuery): string {
  if (
    !Array.isArray(query.jobIds) ||
    query.jobIds.length < 1 ||
    query.jobIds.length > 5 ||
    new Set(query.jobIds).size !== query.jobIds.length
  )
    throw new DomainError('USAGE_ERROR', 'Status requires one to five distinct job identities', 2);

  const selectors = [
    ...query.jobIds.map((value) => ({ value, limit: 256 })),
    ...(query.branch !== undefined ? [{ value: query.branch, limit: 4096 }] : []),
  ];

  for (const { value: id, limit } of selectors) {
    if (
      typeof id !== 'string' ||
      !id ||
      id.length > limit ||
      !id.isWellFormed() ||
      /[\u0000-\u001f\u007f-\u009f\u202a-\u202e\u2066-\u2069]/.test(id)
    )
      throw new DomainError('USAGE_ERROR', 'Invalid status selector', 2);
  }

  const branch =
    query.branch === undefined ? 'branch:(default:any)' : `branch:${branchCondition(query.branch)}`;
  const locator = `defaultFilter:false,state:any,${branch},count:20,start:0,lookupLimit:5000`;
  const fields = `count,buildType(${jobFields},builds($locator:(${locator}),count,nextHref,build(${statusRunFields})))`;

  return apiPath(
    'buildTypes',
    [
      ...query.jobIds.map((id) => `item:${idCondition(id)}`),
      `count:${query.jobIds.length}`,
      'start:0',
    ],
    fields,
  );
}

export function normalizeStatus(
  input: unknown,
  query: StatusQuery,
  serverUrl: string,
  secrets: readonly string[],
): JobSnapshot[] {
  const dto = object(input);
  const values = dto.buildType === undefined && dto.count === 0 ? [] : dto.buildType;

  if (
    !Array.isArray(values) ||
    values.length > query.jobIds.length ||
    !Number.isSafeInteger(dto.count) ||
    dto.count !== values.length
  )
    throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid bounded status collection');

  const jobs = new Set<string>();
  const runs = new Set<string>();

  return values.map((value) => {
    const item = object(value);
    const job = normalizeJob(item, secrets);

    if (!query.jobIds.includes(job.id))
      throw new DomainError('CONTEXT_MISMATCH', 'Status returned another job');

    if (jobs.has(job.id))
      throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Status repeated a job');

    jobs.add(job.id);
    const builds = object(item.builds);
    const values = builds.build === undefined && builds.count === 0 ? [] : builds.build;

    if (
      !Array.isArray(values) ||
      values.length > 20 ||
      !Number.isSafeInteger(builds.count) ||
      builds.count !== values.length
    )
      throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid status run page');

    const limitations: JobSnapshot['page']['limitations'] = [];
    let previous = Number.MAX_SAFE_INTEGER + 1;
    const normalized = values.map((value) => {
      const result = normalizeRun(value, serverUrl, secrets);
      const run = result.run;

      if (
        run.jobId !== job.id ||
        result.projectId !== job.projectId ||
        (query.branch !== undefined && run.branch !== query.branch)
      )
        throw new DomainError('CONTEXT_MISMATCH', 'Status candidate is outside its verified scope');

      if (runs.has(run.id) || Number(run.id) >= previous)
        throw new DomainError(
          'UPSTREAM_SCHEMA_MISMATCH',
          'Status candidates are not distinct newest-first executions',
        );

      runs.add(run.id);
      previous = Number(identity(run.id, true));
      const roots = run.revisions?.map((revision) => revision.vcsRootId) ?? [];

      if (new Set(roots).size !== roots.length)
        throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Status candidate repeats a VCS root');

      limitations.push(...result.limitations);

      return run;
    });

    if (builds.nextHref !== undefined) {
      try {
        if (typeof builds.nextHref !== 'string') throw new Error();

        nextPosition(builds.nextHref, {
          serverUrl,
          resource: 'builds',
          filters: [
            'defaultFilter:false',
            'state:any',
            `buildType:${idCondition(job.id)}`,
            query.branch === undefined
              ? 'branch:(default:any)'
              : `branch:${branchCondition(query.branch)}`,
          ],
          fields: `count,nextHref,build(${statusRunFields})`,
          count: 20,
          start: 0,
          scanLimit: 5000,
        });
      } catch {
        limitations.push({
          code: 'UNSAFE_CONTINUATION',
          message: 'Status candidate continuation could not be verified; it was not followed',
          source: 'run',
        });
      }
    }

    return {
      job,
      page: {
        runs: normalized,
        providerReturned: normalized.length,
        position: null,
        hasMore: null,
        limitations,
      },
    };
  });
}
