import type { Graph } from './graph.js';
import type { Change, Run } from './teamcity.js';
import type { Limitation } from './response.js';

export type SourceKind = 'run' | 'problems' | 'tests' | 'log' | 'changes' | 'dependencies';

export interface SourceCoverage {
  id: string;
  runId: string;
  kind: SourceKind;
  state: 'complete' | 'partial' | 'unavailable' | 'not_requested' | 'budget_exhausted';
  returned: number | null;
  total: number | null;
  providerReturned?: number;
  window?: {
    requested: number;
    firstMessageId: string | null;
    lastMessageId: string | null;
    omittedProviderMessages: number;
  };
  reasonCode?: string;
  observedAt?: string;
  required: boolean;
  scope: 'execution' | 'bounded_page' | 'tail_window' | 'immediate_dependencies';
}

export interface Evidence {
  id: string;
  sourceRef: string;
  runId: string;
  kind: 'problem' | 'test' | 'log' | 'run';
  itemId: string;
  excerpt: string;
  observedAt: string;
  retrieve: { reason: string; argv: string[] };
}

export interface Finding {
  id: string;
  runId: string;
  kind: 'failed_test' | 'build_problem' | 'dependency_failure' | 'log_signal';
  claim: 'observation';
  summary: string;
  evidence: Evidence[];
}

export interface FailureReport {
  run: Run;
  assessment: 'not_failed' | 'in_progress' | 'failure_observed' | 'inconclusive';
  findings: Finding[];
  sources: SourceCoverage[];
  graph: Graph;
  changes?: Change[];
  selection: {
    maxDiagnosedRuns: number;
    diagnosedRunIds: string[];
    omittedDiagnosedRuns: number;
    graphReadAttempts: number;
    maxGraphReads: number;
    omittedGraphTargets: number;
    omittedFindings: number;
    omittedChanges: number;
    consistency: 'best_effort';
    dependencyReferences: 'unavailable';
  };
}

export interface Investigation {
  data: FailureReport;
  limitations: Limitation[];
  complete: boolean;
  truncated: boolean;
  projects: ReadonlyMap<string, string | null>;
}
