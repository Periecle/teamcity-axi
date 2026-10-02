import type { Run } from './teamcity.js';

export type Expansion =
  | 'complete'
  | 'not_requested'
  | 'depth_limit'
  | 'node_limit'
  | 'call_limit'
  | 'permission_denied'
  | 'unavailable';

export interface GraphNode {
  run: Run;
  expansion: Expansion;
  dependencyCount: number | null;
  observedDependencies: number | null;
}

export interface GraphEdge {
  fromRunId: string;
  toRunId: string;
  kind: 'snapshot';
}

export interface Graph {
  rootRunId: string;
  complete: boolean;
  nodes: GraphNode[];
  edges: GraphEdge[];
  cycles: GraphEdge[];
  unexpanded: number;
}

export function graphRun(run: Run): Run {
  return {
    id: run.id,
    jobId: run.jobId,
    state: run.state,
    result: run.result,
    branch: run.branch ?? null,
    ...(run.revisions ? { revisions: run.revisions } : {}),
    ...(run.personal !== undefined ? { personal: run.personal } : {}),
    ...(run.composite !== undefined ? { composite: run.composite } : {}),
    ...(run.result === 'unknown' ? { rawStatus: run.rawStatus ?? null } : {}),
  };
}

export function sameGraphObservation(left: Run, right: Run): boolean {
  const comparable = (run: Run) => ({
    ...graphRun(run),
    ...(run.revisions
      ? {
          revisions: [...run.revisions].sort(
            (a, b) =>
              a.vcsRootId.localeCompare(b.vcsRootId) || a.revision.localeCompare(b.revision),
          ),
        }
      : {}),
  });

  return JSON.stringify(comparable(left)) === JSON.stringify(comparable(right));
}
