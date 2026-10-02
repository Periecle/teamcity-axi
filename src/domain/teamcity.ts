import type { DomainError } from './errors.js';
import type { Limitation } from './response.js';

export interface Run {
  id: string;
  jobId: string;
  state: 'queued' | 'running' | 'finished' | 'unknown';
  result: 'success' | 'failure' | 'error' | 'canceled' | 'failed_to_start' | 'unknown';
  number?: string;
  branch?: string | null;
  statusText?: string;
  personal?: boolean | null;
  composite?: boolean | null;
  queuedAt?: string | null;
  startedAt?: string | null;
  finishedAt?: string | null;
  durationMs?: number | null;
  webUrl?: string | null;
  revisions?: { vcsRootId: string; revision: string }[];
  rawStatus?: string | null;
}

export interface RunRef {
  id: string;
}

export interface Job {
  id: string;
  name: string;
  projectId: string;
  paused: boolean | null;
}

export interface Project {
  id: string;
  name: string;
  parentProjectId: string | null;
  archived: boolean | null;
}

export interface ServerInfo {
  version: string;
  buildNumber: string;
}

export interface AuthenticatedIdentity {
  fingerprint: string;
}

export interface LogTail {
  runId: string;
  messages: {
    id: string;
    text: string;
    level: number;
    status: number;
    timestamp?: string | null;
  }[];
  providerReturned: number;
  truncated: boolean;
  limitations: Limitation[];
}

export interface Problem {
  id: string;
  runId: string;
  type: string;
  description: string;
  identity?: string;
}

export interface TestOccurrence {
  id: string;
  runId: string;
  name: string;
  result: 'success' | 'failure' | 'ignored' | 'unknown';
  testId?: string;
  durationMs: number | null;
  muted: boolean | null;
  ignored: boolean | null;
  details?: string;
  rawStatus?: string;
}

export interface EvidenceQuery {
  runId: string;
  count: number;
  start: number;
  scanLimit: number;
  failed?: boolean;
  muted?: boolean;
}

export interface EvidencePage<T> {
  items: T[];
  providerReturned: number;
  position: number | null;
  hasMore: boolean | null;
  limitations: Limitation[];
}

export interface RelatedQuery {
  runId: string;
  count: number;
  start: number;
  scanLimit: number;
  files?: boolean;
}

export interface Change {
  id: string;
  version: string;
  vcsRootId: string;
  message: string;
  timestamp: string | null;
  files?: string[];
  fileCoverage?: { returned: number; providerReturned: number; omitted: number };
}

export interface ScopedRun {
  run: Run;
  projectId: string | null;
}

export interface RunQuery {
  jobId?: string;
  projectId?: string;
  branch?: string;
  state?: 'queued' | 'running' | 'finished';
  result?: 'success' | 'failure' | 'error';
  revision?: string;
  vcsRootId?: string;
  window?: { since: string; until: string };
  count: number;
  start: number;
  scanLimit: number;
  allowedProjects?: readonly string[];
}

export interface RunPage {
  runs: Run[];
  providerReturned: number;
  position: number | null;
  hasMore: boolean | null;
  limitations: Limitation[];
}

export interface Budget {
  deadline: number;
  // Optional absolute invocation launch ceiling, leaving reserved capacity unused.
  maxChildProcesses?: number;
}

export interface Provenance {
  observedAt: string;
  operation: string;
  projectId: string | null;
  limitations: Limitation[];
}

export type ReadResult<T> =
  | { state: 'available'; value: T; provenance: Provenance }
  | { state: 'unavailable'; error: DomainError; provenance: Provenance };

// The first vertical adapter implements this slice. Additional read primitives
// extend this interface as their recorded contracts and tests are added.
export interface TeamCityReader {
  listChanges(query: RelatedQuery, budget: Budget): Promise<ReadResult<EvidencePage<Change>>>;
  listSnapshotDependencies(
    query: RelatedQuery,
    budget: Budget,
  ): Promise<ReadResult<EvidencePage<ScopedRun>>>;
  getSnapshotDependencyCount(ref: RunRef, budget: Budget): Promise<ReadResult<number>>;
  listProblems(query: EvidenceQuery, budget: Budget): Promise<ReadResult<EvidencePage<Problem>>>;
  listTests(
    query: EvidenceQuery,
    budget: Budget,
  ): Promise<ReadResult<EvidencePage<TestOccurrence>>>;
  getProblem(ref: { runId: string; id: string }, budget: Budget): Promise<ReadResult<Problem>>;
  getTest(ref: { runId: string; id: string }, budget: Budget): Promise<ReadResult<TestOccurrence>>;
  getServer(budget: Budget): Promise<ReadResult<ServerInfo>>;
  getIdentity(budget: Budget): Promise<ReadResult<AuthenticatedIdentity>>;
  getProject(ref: { id: string }, budget: Budget): Promise<ReadResult<Project>>;
  getLogTail(ref: RunRef, tail: number, budget: Budget): Promise<ReadResult<LogTail>>;
  getRun(ref: RunRef, budget: Budget): Promise<ReadResult<Run>>;
  getJob(ref: { id: string }, budget: Budget): Promise<ReadResult<Job>>;
  listRuns(query: RunQuery, budget: Budget): Promise<ReadResult<RunPage>>;
}
