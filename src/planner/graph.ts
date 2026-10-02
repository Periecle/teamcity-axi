import { asDomainError, DomainError } from '../domain/errors.js';
import type { Limitation } from '../domain/response.js';
import type { Budget, Run, TeamCityReader } from '../domain/teamcity.js';
import type { Expansion, Graph, GraphEdge, GraphNode } from '../domain/graph.js';
import { graphRun, sameGraphObservation } from '../domain/graph.js';

interface Options {
  root: Run;
  rootProjectId?: string | null;
  reader: Pick<TeamCityReader, 'getSnapshotDependencyCount' | 'listSnapshotDependencies'>;
  policy: { assert(projectId: string | null): Promise<void> };
  budget: Budget;
  depth: number;
  maxNodes: number;
  maxGraphReads: number;
  canRead?: () => boolean;
}

interface Node extends GraphNode {
  depth: number;
  projectId?: string | null;
}

const order = (a: string, b: string) => Number(a) - Number(b);

function cycles(nodes: GraphNode[], edges: GraphEdge[], rootId: string): GraphEdge[] {
  const visited = new Set<string>(),
    path = new Set<string>(),
    result: GraphEdge[] = [];
  const outgoing = new Map<string, GraphEdge[]>();

  for (const edge of edges)
    outgoing.set(edge.fromRunId, [...(outgoing.get(edge.fromRunId) ?? []), edge]);

  function visit(id: string) {
    if (visited.has(id)) return;
    visited.add(id);
    path.add(id);

    for (const edge of outgoing.get(id) ?? []) {
      if (path.has(edge.toRunId)) result.push(edge);
      else visit(edge.toRunId);
    }

    path.delete(id);
  }

  visit(rootId);
  for (const node of nodes) visit(node.run.id);

  return result;
}

export async function traverseSnapshotGraph(options: Options) {
  if (
    !Number.isInteger(options.depth) ||
    options.depth < 0 ||
    options.depth > 12 ||
    !Number.isInteger(options.maxNodes) ||
    options.maxNodes < 1 ||
    options.maxNodes > 200 ||
    !Number.isInteger(options.maxGraphReads) ||
    options.maxGraphReads < 0 ||
    options.maxGraphReads > 24
  )
    throw new DomainError('USAGE_ERROR', 'Invalid bounded graph request', 2);
  const root: Node = {
    run: options.root,
    expansion: 'complete',
    dependencyCount: null,
    observedDependencies: null,
    depth: 0,
    ...(options.rootProjectId !== undefined ? { projectId: options.rootProjectId } : {}),
  };
  const nodes = new Map<string, Node>([[root.run.id, root]]),
    queue = [root];
  const edges = new Map<string, GraphEdge>(),
    omitted = new Set<string>(),
    depths = new Map<string, number>([[root.run.id, 0]]);
  const limitations: Limitation[] = [];
  const sharedLimits = new Set<string>();
  let graphReadAttempts = 0;
  const rank: Record<Expansion, number> = {
    complete: 0,
    not_requested: 0,
    depth_limit: 1,
    node_limit: 2,
    call_limit: 3,
    unavailable: 4,
    permission_denied: 5,
  };

  function mark(node: Node, expansion: Expansion) {
    if (rank[expansion] > rank[node.expansion]) node.expansion = expansion;
  }

  function limit(node: Node, code: string, message: string, expansion: Expansion = 'unavailable') {
    mark(node, expansion);
    const shared = ['depth_limit', 'node_limit', 'call_limit'].includes(expansion);

    if (shared && sharedLimits.has(code)) return;
    if (shared) sharedLimits.add(code);
    limitations.push({
      code,
      message,
      source: 'dependencies',
      ...(shared ? {} : { runId: node.run.id }),
    });
  }

  function failed(node: Node, error: unknown) {
    const domain = asDomainError(error);

    if (domain.code === 'INTERRUPTED') throw domain;
    const expansion = ['POLICY_DENIED', 'PERMISSION_DENIED'].includes(domain.code)
      ? 'permission_denied'
      : ['CALL_LIMIT_EXCEEDED', 'DEADLINE_EXCEEDED'].includes(domain.code)
        ? 'call_limit'
        : 'unavailable';

    limit(
      node,
      domain.code,
      'An independent graph read or scoped target is unavailable',
      expansion,
    );
  }

  function spend() {
    if (Date.now() >= options.budget.deadline)
      throw new DomainError('DEADLINE_EXCEEDED', 'Shared graph deadline exhausted');
    if (graphReadAttempts >= options.maxGraphReads || options.canRead?.() === false)
      throw new DomainError('CALL_LIMIT_EXCEEDED', 'Shared graph read capacity exhausted');
    graphReadAttempts++;
  }

  for (let index = 0; index < queue.length; index++) {
    const node = queue[index]!;

    if (node.depth >= options.depth) {
      limit(
        node,
        'GRAPH_DEPTH_LIMIT',
        'Dependencies were not requested beyond the declared depth',
        'depth_limit',
      );
      continue;
    }

    try {
      spend();
      const count = await options.reader.getSnapshotDependencyCount(
        { id: node.run.id },
        options.budget,
      );

      if (count.state === 'unavailable') {
        failed(node, count.error);
        if (['CALL_LIMIT_EXCEEDED', 'DEADLINE_EXCEEDED'].includes(count.error.code)) continue;
      } else {
        node.dependencyCount = count.value;

        if (count.value === 0) {
          node.observedDependencies = 0;
          continue;
        }
      }
    } catch (error) {
      failed(node, error);
      continue;
    }

    const observed = new Set<string>();
    let start = 0;

    for (;;) {
      try {
        spend();
        const read = await options.reader.listSnapshotDependencies(
          { runId: node.run.id, count: 100, start, scanLimit: 5000 },
          options.budget,
        );

        if (read.state === 'unavailable') {
          failed(node, read.error);
          break;
        }

        const page = read.value;
        const accepted = new Set<string>();
        let duplicate = false;

        node.observedDependencies = observed.size;

        for (const child of [...page.items].sort((a, b) => order(a.run.id, b.run.id))) {
          if (observed.has(child.run.id)) {
            duplicate = true;
            continue;
          }

          observed.add(child.run.id);
          node.observedDependencies = observed.size;

          try {
            await options.policy.assert(child.projectId);
          } catch (error) {
            failed(node, error);
            continue;
          }

          let target = nodes.get(child.run.id);

          if (
            target &&
            (target.run.jobId !== child.run.jobId ||
              (target.projectId !== undefined && target.projectId !== child.projectId))
          ) {
            limit(
              node,
              'CONTEXT_MISMATCH',
              'Conflicting execution identities were returned for a shared graph node',
            );
            continue;
          }

          if (!target) {
            if (nodes.size >= options.maxNodes) {
              omitted.add(child.run.id);
              limit(
                node,
                'GRAPH_NODE_LIMIT',
                'Discovered target could not be retained within the unique-node limit',
                'node_limit',
              );
              continue;
            }

            target = {
              run: child.run,
              projectId: child.projectId,
              depth: node.depth + 1,
              expansion: 'complete',
              dependencyCount: null,
              observedDependencies: null,
            };
            nodes.set(target.run.id, target);
            queue.push(target);
            depths.set(target.run.id, target.depth);
          } else if (!sameGraphObservation(target.run, child.run)) {
            limit(
              node,
              'RUN_STATE_CHANGED',
              'A shared execution changed lifecycle, result or scoped metadata between observations',
            );
          }

          accepted.add(child.run.id);

          const key = `${node.run.id}:${target.run.id}`;

          if (!edges.has(key)) {
            if (edges.size >= 2000) {
              limit(node, 'GRAPH_EDGE_LIMIT', 'Graph edge ceiling reached', 'node_limit');
              continue;
            }

            edges.set(key, { fromRunId: node.run.id, toRunId: target.run.id, kind: 'snapshot' });
          }
        }

        const relevant = page.limitations.filter(
          (l) =>
            l.code !== 'SCAN_COVERAGE_UNKNOWN' &&
            (l.source !== 'run' || (l.runId !== undefined && accepted.has(l.runId))),
        );

        limitations.push(
          ...relevant.map((l) => ({ ...l, source: 'dependencies', runId: l.runId ?? node.run.id })),
        );

        if (duplicate) {
          limit(
            node,
            'DUPLICATE_DEPENDENCY',
            'A dependency appeared repeatedly across offset pages',
          );
          break;
        }

        if (relevant.some((l) => l.code === 'UNSAFE_CONTINUATION')) {
          mark(node, 'unavailable');
          break;
        }

        if (node.dependencyCount !== null && observed.size >= node.dependencyCount) {
          if (observed.size !== node.dependencyCount || page.hasMore === true)
            limit(
              node,
              'DEPENDENCY_COUNT_MISMATCH',
              'Dependency pages conflict with the independently observed count',
            );
          break;
        }

        if (page.position === null) {
          if (node.dependencyCount !== null)
            limit(
              node,
              'DEPENDENCY_COUNT_MISMATCH',
              'Returned dependencies do not reconcile with the scoped count',
            );
          else
            limit(
              node,
              'DEPENDENCY_COUNT_UNAVAILABLE',
              'Observed rows do not establish complete expansion without a scoped count',
            );
          break;
        }

        start = page.position;
      } catch (error) {
        failed(node, error);
        break;
      }
    }
  }

  const retained = [...nodes.values()].sort(
    (a, b) => a.depth - b.depth || order(a.run.id, b.run.id),
  );
  const publicNodes = retained.map(({ run, expansion, dependencyCount, observedDependencies }) => ({
    run: graphRun(run),
    expansion,
    dependencyCount,
    observedDependencies,
  }));
  const publicEdges = [...edges.values()].sort(
    (a, b) => order(a.fromRunId, b.fromRunId) || order(a.toRunId, b.toRunId),
  );
  const cycleEdges = cycles(publicNodes, publicEdges, root.run.id);

  if (cycleEdges.length)
    limitations.push({
      code: 'GRAPH_CYCLE_DETECTED',
      message: 'The inspected execution graph contains a cycle',
      source: 'dependencies',
      runId: root.run.id,
    });
  const unexpanded = publicNodes.filter((n) => n.expansion !== 'complete').length;
  const graph: Graph = {
    rootRunId: root.run.id,
    complete: unexpanded === 0,
    nodes: publicNodes,
    edges: publicEdges,
    cycles: cycleEdges,
    unexpanded,
  };

  const projects = new Map(retained.map((node) => [node.run.id, node.projectId ?? null]));

  return { graph, limitations, depths, projects, graphReadAttempts, omittedTargets: omitted.size };
}
