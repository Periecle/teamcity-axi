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
  JobQuery,
  QueueItem,
  QueueQuery,
} from '../domain/teamcity.js';
import type { ProcessTransport } from '../transport/process.js';
import { parseRaw } from './raw.js';
import { identity, normalizeRun } from './run.js';
import { literal, apiPath } from './locator.js';
import { normalizeRunPage, runFilters } from './run-page.js';

import { isUtf8 } from 'node:buffer';

import { jobFields, jobLimitations, jobRequest, normalizeJob, normalizeJobPage } from './jobs.js';
import { queueRequest, normalizeQueuePage } from './queue.js';
import { normalizeStatus, statusRequest } from './status.js';
import type { JobSnapshot, StatusQuery } from '../domain/teamcity.js';
import {
  agentFields,
  agentFilters,
  agentRequest,
  normalizeAgent,
  normalizeAgentPage,
  validateAgentId,
} from './agents.js';
import type { Agent, AgentQuery, AgentScope } from '../domain/teamcity.js';

import {
  normalizeIdentity,
  normalizeLogTail,
  normalizeProject,
  normalizeServer,
} from './metadata.js';
import type { AuthenticatedIdentity, LogTail, Project, ServerInfo } from '../domain/teamcity.js';
import type { EvidenceQuery, EvidencePage, Problem, TestOccurrence } from '../domain/teamcity.js';
import {
  evidenceRequest,
  normalizeEvidencePage,
  normalizeProblem,
  normalizeTest,
  occurrenceLocator,
  problemFields,
  testFields,
} from './evidence.js';
import type { Change, RelatedQuery, ScopedRun } from '../domain/teamcity.js';
import {
  relatedRequest,
  normalizeChangePage,
  normalizeDependencyPage,
  normalizeDependencyCount,
} from './related.js';

// Frozen in tests/fixtures/native-operations.mjs and the released-binary capture.
export const runDetailFields =
  'id,buildTypeId,number,state,status,failedToStart,canceledInfo(timestamp),branchName,statusText,personal,composite,buildType(id,name,projectId),revisions(revision(version,vcs-root-instance(id,vcs-root-id))),startDate,finishDate';

export class NativeTeamCityReader implements TeamCityReader {
  constructor(
    private readonly transport: ProcessTransport,
    private readonly serverUrl: string,
    private readonly secrets: readonly string[] = [],
  ) {}

  listChanges(query: RelatedQuery, budget: Budget): Promise<ReadResult<EvidencePage<Change>>> {
    const request = relatedRequest('changes', query);

    return this.metadata('changes.page', request.path, budget, (body) =>
      normalizeChangePage(body, query, this.serverUrl, this.secrets),
    );
  }

  listSnapshotDependencies(
    query: RelatedQuery,
    budget: Budget,
  ): Promise<ReadResult<EvidencePage<ScopedRun>>> {
    const request = relatedRequest('dependencies', query, runDetailFields);

    return this.metadata('dependencies.page', request.path, budget, (body) =>
      normalizeDependencyPage(body, query, this.serverUrl, runDetailFields, this.secrets),
    );
  }

  getSnapshotDependencyCount(ref: RunRef, budget: Budget): Promise<ReadResult<number>> {
    const id = identity(ref.id, true);

    return this.metadata(
      'dependencies.count',
      `/app/rest/builds/id:${id}?fields=id,snapshot-dependencies(count)`,
      budget,
      (body) => normalizeDependencyCount(body, id),
    );
  }

  async listProblems(
    query: EvidenceQuery,
    budget: Budget,
  ): Promise<ReadResult<EvidencePage<Problem>>> {
    const request = evidenceRequest('problems', query);

    return this.metadata('problems.page', request.path, budget, (body) =>
      normalizeEvidencePage('problems', body, query, this.serverUrl, this.secrets),
    );
  }

  async listTests(
    query: EvidenceQuery,
    budget: Budget,
  ): Promise<ReadResult<EvidencePage<TestOccurrence>>> {
    const request = evidenceRequest('tests', query);

    return this.metadata('tests.page', request.path, budget, (body) =>
      normalizeEvidencePage('tests', body, query, this.serverUrl, this.secrets),
    );
  }

  async getProblem(
    ref: { runId: string; id: string },
    budget: Budget,
  ): Promise<ReadResult<Problem>> {
    const locator = occurrenceLocator('problems', ref.id, ref.runId);

    return this.metadata(
      'problem.detail',
      `/app/rest/problemOccurrences/${locator}?fields=${problemFields}`,
      budget,
      (body) => {
        const value = normalizeProblem(body, ref.runId, this.secrets);

        if (value.id !== ref.id)
          throw new DomainError('CONTEXT_MISMATCH', 'Server returned another problem occurrence');

        return value;
      },
    );
  }

  async getTest(
    ref: { runId: string; id: string },
    budget: Budget,
  ): Promise<ReadResult<TestOccurrence>> {
    const locator = occurrenceLocator('tests', ref.id, ref.runId),
      limitations: Provenance['limitations'] = [];
    const result = await this.metadata(
      'test.detail',
      `/app/rest/testOccurrences/${locator}?fields=${testFields}`,
      budget,
      (body) => {
        const value = normalizeTest(body, ref.runId, this.secrets, limitations);

        if (value.id !== ref.id)
          throw new DomainError('CONTEXT_MISMATCH', 'Server returned another test occurrence');

        return value;
      },
    );

    result.provenance.limitations = limitations;

    return result;
  }

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

      const captured = await this.transport.execute(
        { kind: 'api', path },
        budget.maxChildProcesses,
      );
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

      const captured = await this.transport.execute(
        { kind: 'log', runId: id, tail },
        budget.maxChildProcesses,
      );

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
      provenance.limitations = value.limitations;

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

      const captured = await this.transport.execute(
        {
          kind: 'api',
          path: `/app/rest/builds/id:${id}?fields=${runDetailFields}`,
        },
        budget.maxChildProcesses,
      );
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
    const id = identity(ref.id);
    const result = await this.metadata(
      'job.detail',
      `/app/rest/buildTypes/id:${literal(id)}?fields=${jobFields}`,
      budget,
      (body) => {
        const job = normalizeJob(body, this.secrets);

        if (job.id !== id)
          throw new DomainError('CONTEXT_MISMATCH', 'Server returned a different job');

        return job;
      },
    );

    if (result.state === 'available') {
      result.provenance.projectId = result.value.projectId;
      result.provenance.limitations = jobLimitations(result.value);
    }

    return result;
  }

  async readStatus(query: StatusQuery, budget: Budget): Promise<ReadResult<JobSnapshot[]>> {
    return this.metadata('status.snapshot', statusRequest(query), budget, (body) =>
      normalizeStatus(body, query, this.serverUrl, this.secrets),
    );
  }

  async listQueue(query: QueueQuery, budget: Budget): Promise<ReadResult<EvidencePage<QueueItem>>> {
    const request = queueRequest(query);
    const result = await this.metadata('queue.page', request.path, budget, (body) =>
      normalizeQueuePage(body, query, this.serverUrl, this.secrets),
    );

    result.provenance.projectId = query.projectId ?? null;

    if (result.state === 'available') result.provenance.limitations = result.value.limitations;

    return result;
  }

  async getAgent(ref: AgentScope & { id: string }, budget: Budget): Promise<ReadResult<Agent>> {
    validateAgentId(ref.id);
    const filters = agentFilters(ref);
    let limitations: Provenance['limitations'] = [];
    const result = await this.metadata(
      'agent.detail',
      `/app/rest/agents/${[`id:${ref.id}`, ...filters].join(',')}?fields=${agentFields}`,
      budget,
      (body) => {
        const normalized = normalizeAgent(body, ref, this.secrets);

        if (normalized.agent.id !== ref.id)
          throw new DomainError('CONTEXT_MISMATCH', 'Server returned a different agent');

        limitations = normalized.limitations;

        return normalized.agent;
      },
    );

    result.provenance.projectId = ref.projectId ?? null;
    result.provenance.limitations = limitations;

    return result;
  }

  async listAgents(query: AgentQuery, budget: Budget): Promise<ReadResult<EvidencePage<Agent>>> {
    const request = agentRequest(query);
    const result = await this.metadata('agents.page', request.path, budget, (body) =>
      normalizeAgentPage(body, query, this.serverUrl, this.secrets),
    );

    result.provenance.projectId = query.projectId ?? null;

    if (result.state === 'available') result.provenance.limitations = result.value.limitations;

    return result;
  }

  async listJobs(query: JobQuery, budget: Budget): Promise<ReadResult<EvidencePage<Job>>> {
    const request = jobRequest(query);
    const result = await this.metadata('job.page', request.path, budget, (body) =>
      normalizeJobPage(body, query, this.serverUrl, this.secrets),
    );

    result.provenance.projectId = query.projectId;

    if (result.state === 'available') result.provenance.limitations = result.value.limitations;

    return result;
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

      const captured = await this.transport.execute(
        {
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
        },
        budget.maxChildProcesses,
      );
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

      if (
        page.providerReturned < query.count &&
        page.limitations.some((note) => note.code === 'SCAN_COVERAGE_UNKNOWN')
      ) {
        const server = await this.getServer(budget);

        if (server.state === 'unavailable' && server.error.code === 'INTERRUPTED')
          throw server.error;

        // This exact server contract signals lookup-cap truncation through
        // nextHref even for empty pages. Other versions retain uncertainty.
        if (
          server.state === 'available' &&
          server.value.version === '2026.2 (build 238924)' &&
          server.value.buildNumber === '238924'
        ) {
          page.hasMore = false;
          page.limitations = page.limitations.filter(
            (note) => note.code !== 'SCAN_COVERAGE_UNKNOWN',
          );
        }
      }

      provenance.observedAt = new Date().toISOString();
      provenance.limitations = page.limitations;

      return { state: 'available', value: page, provenance };
    } catch (error) {
      return { state: 'unavailable', error: asDomainError(error), provenance };
    }
  }
}
