import { DomainError } from '../domain/errors.js';
import type { Budget, Project, TeamCityReader } from '../domain/teamcity.js';

// Invocation-local ancestry observations. IDs and names never imply parentage.
export class ProjectPolicy {
  private readonly projects = new Map<string, Promise<Project>>();

  constructor(
    private readonly reader: TeamCityReader,
    private readonly roots: readonly string[] | undefined,
    private readonly budget: Budget,
  ) {}

  project(id: string): Promise<Project> {
    let observation = this.projects.get(id);

    if (!observation) {
      observation = this.reader.getProject({ id }, this.budget).then((read) => {
        if (read.state === 'unavailable') throw read.error;

        return read.value;
      });
      this.projects.set(id, observation);
    }

    return observation;
  }

  async assert(id: string | null): Promise<void> {
    if (!this.roots) return;
    if (!id)
      throw new DomainError('POLICY_DENIED', 'Project identity is unavailable for trusted policy');
    const seen = new Set<string>();
    let current: string | null = id;

    for (let depth = 0; current !== null && depth < 8; depth++) {
      if (this.roots.includes(current)) return;
      if (seen.has(current)) break;
      seen.add(current);
      const project = await this.project(current);

      current = project.parentProjectId;
    }

    if (current !== null && this.roots.includes(current)) return;

    throw new DomainError(
      'POLICY_DENIED',
      'Project ancestry does not establish an allowed subtree',
    );
  }
}
