import { asDomainError, DomainError } from '../domain/errors.js';
import type {
  Budget,
  ReadResult,
  Run,
  RunRef,
  TeamCityReader,
  Provenance,
  Job,
  RunQuery,
  RunPage,
} from '../domain/teamcity.js';
import type { ProcessTransport } from '../transport/process.js';
import { parseRaw } from './raw.js';
import { identity, normalizeRun, object } from './run.js';
import { literal, apiPath } from './locator.js';
import { normalizeRunPage, runFilters } from './run-page.js';
import { sanitizeText } from '../output/sanitize.js';

import { isUtf8 } from 'node:buffer';

import {
  normalizeIdentity,
  normalizeLogTail,
  normalizeProject,
  normalizeServer,
} from './metadata.js';
import type { AuthenticatedIdentity, LogTail, Project, ServerInfo } from '../domain/teamcity.js';

// Frozen in tests/fixtures/native-operations.mjs and the released-binary capture.
export const runDetailFields =
  'id,buildTypeId,number,state,status,branchName,statusText,personal,composite,buildType(id,name,projectId),revisions(revision(version,vcs-root-instance(id,vcs-root-id))),startDate,finishDate';

export class NativeTeamCityReader implements TeamCityReader {
  constructor(
    private readonly transport: ProcessTransport,
    private readonly serverUrl: string,
    private readonly secrets: readonly string[] = [],
  ) {}

  private async metadata<T>(
    operation: string,
    path: string,
    budget: Budget,
    normalize: (body: unknown) => T,
  ): Promise<ReadResult<T>> {
    const provenance: Provenance = {
      observedAt: new Date().toISOString(),
      operation,
      projectId: null,
      limitations: [],
    };

    try {
      if (Date.now() >= budget.deadline)
        throw new DomainError('DEADLINE_EXCEEDED', 'Overall deadline exceeded', 1, true);
      const captured = await this.transport.execute({ kind: 'api', path });
      const value = normalize(parseRaw(captured).body);

      provenance.observedAt = new Date().toISOString();

      return { state: 'available', value, provenance };
    } catch (error) {
      return { state: 'unavailable', error: asDomainError(error), provenance };
    }
  }

  getServer(budget: Budget): Promise<ReadResult<ServerInfo>> {
    return this.metadata(
      'server.detail',
      '/app/rest/server?fields=version,buildNumber',
      budget,
      (body) => normalizeServer(body, this.secrets),
    );
  }

  getIdentity(budget: Budget): Promise<ReadResult<AuthenticatedIdentity>> {
    return this.metadata(
      'identity.current',
      '/app/rest/users/current?fields=id,username',
      budget,
      (body) => normalizeIdentity(body, this.serverUrl),
    );
  }

  async getProject(ref: { id: string }, budget: Budget): Promise<ReadResult<Project>> {
    const id = identity(ref.id);
    const result = await this.metadata(
      'project.detail',
      `/app/rest/projects/id:${literal(id)}?fields=id,name,parentProjectId,archived`,
      budget,
      (body) => normalizeProject(body, id, this.secrets),
    );

    if (result.state === 'available') result.provenance.projectId = result.value.id;

    return result;
  }

  async getLogTail(ref: RunRef, tail: number, budget: Budget): Promise<ReadResult<LogTail>> {
    const provenance: Provenance = {
      observedAt: new Date().toISOString(),
      operation: 'log.tail',
      projectId: null,
      limitations: [],
    };

    try {
      const id = identity(ref.id, true);

      if (Date.now() >= budget.deadline)
        throw new DomainError('DEADLINE_EXCEEDED', 'Overall deadline exceeded', 1, true);
      const captured = await this.transport.execute({ kind: 'log', runId: id, tail });

      if (captured.exitCode !== 0 || captured.signal)
        throw new DomainError(
          'UPSTREAM_FAILURE',
          'Structured log capability could not be verified',
        );
      if (!isUtf8(captured.stdout))
        throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid structured log encoding');
      let dto: unknown;

      try {
        dto = JSON.parse(captured.stdout.toString('utf8'));
      } catch {
        throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid structured log document');
      }

      const value = normalizeLogTail(dto, id, tail, this.secrets);

      provenance.observedAt = new Date().toISOString();

      return { state: 'available', value, provenance };
    } catch (error) {
      return { state: 'unavailable', error: asDomainError(error), provenance };
    }
  }

  async getRun(ref: RunRef, budget: Budget): Promise<ReadResult<Run>> {
    const provenance: Provenance = {
      observedAt: new Date().toISOString(),
      operation: 'run.detail',
      projectId: null,
      limitations: [],
    };

    try {
      const id = identity(ref.id, true);

      if (Date.now() >= budget.deadline)
        throw new DomainError('DEADLINE_EXCEEDED', 'Overall deadline exceeded', 1, true);
      const captured = await this.transport.execute({
        kind: 'api',
        path: `/app/rest/builds/id:${id}?fields=${runDetailFields}`,
      });
      const normalized = normalizeRun(parseRaw(captured).body, this.serverUrl, this.secrets);

      if (normalized.run.id !== id)
        throw new DomainError(
          'CONTEXT_MISMATCH',
          'Server returned a different execution than the requested ID',
        );
      provenance.observedAt = new Date().toISOString();
      provenance.projectId = normalized.projectId;
      provenance.limitations = normalized.limitations;

      return { state: 'available', value: normalized.run, provenance };
    } catch (error) {
      return { state: 'unavailable', error: asDomainError(error), provenance };
    }
  }

  async getJob(ref: { id: string }, budget: Budget): Promise<ReadResult<Job>> {
    const provenance: Provenance = {
      observedAt: new Date().toISOString(),
      operation: 'job.detail',
      projectId: null,
      limitations: [],
    };

    try {
      const id = identity(ref.id);

      if (Date.now() >= budget.deadline)
        throw new DomainError('DEADLINE_EXCEEDED', 'Overall deadline exceeded', 1, true);
      const captured = await this.transport.execute({
        kind: 'api',
        path: `/app/rest/buildTypes/id:${literal(id)}?fields=id,name,projectId,paused`,
      });
      const dto = object(parseRaw(captured).body);
      const observedId = identity(dto.id),
        projectId = identity(dto.projectId);

      if (observedId !== id)
        throw new DomainError('CONTEXT_MISMATCH', 'Server returned a different job');
      if (
        typeof dto.name !== 'string' ||
        (dto.paused !== undefined && typeof dto.paused !== 'boolean')
      )
        throw new DomainError('UPSTREAM_SCHEMA_MISMATCH', 'Invalid job metadata');
      provenance.projectId = projectId;
      provenance.observedAt = new Date().toISOString();

      return {
        state: 'available',
        value: {
          id,
          name: sanitizeText(dto.name, this.secrets),
          projectId,
          paused: typeof dto.paused === 'boolean' ? dto.paused : null,
        },
        provenance,
      };
    } catch (error) {
      return { state: 'unavailable', error: asDomainError(error), provenance };
    }
  }

  async listRuns(query: RunQuery, budget: Budget): Promise<ReadResult<RunPage>> {
    const provenance: Provenance = {
      observedAt: new Date().toISOString(),
      operation: 'run.page',
      projectId: query.projectId ?? null,
      limitations: [],
    };

    try {
      const filters = runFilters(query),
        fields = `count,nextHref,build(${runDetailFields})`;

      if (Date.now() >= budget.deadline)
        throw new DomainError('DEADLINE_EXCEEDED', 'Overall deadline exceeded', 1, true);
      const captured = await this.transport.execute({
        kind: 'api',
        path: apiPath(
          'builds',
          [
            ...filters,
            `count:${query.count}`,
            `start:${query.start}`,
            `lookupLimit:${query.scanLimit}`,
          ],
          fields,
        ),
      });
      const page = normalizeRunPage(
        parseRaw(captured).body,
        query,
        {
          serverUrl: this.serverUrl,
          resource: 'builds',
          filters,
          fields,
          count: query.count,
          start: query.start,
          scanLimit: query.scanLimit,
        },
        this.secrets,
      );

      provenance.observedAt = new Date().toISOString();
      provenance.limitations = page.limitations;

      return { state: 'available', value: page, provenance };
    } catch (error) {
      return { state: 'unavailable', error: asDomainError(error), provenance };
    }
  }
}
