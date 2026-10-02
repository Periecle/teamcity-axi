import type { Limitation } from '../domain/response.js';
import type { JobSnapshot, Run } from '../domain/teamcity.js';

export type RevisionMatch = 'exact' | 'different' | 'unverified';

export interface StatusJob {
  jobId: string;
  required: true;
  availability: 'available' | 'unavailable';
  match: RevisionMatch;
  assessment: 'passed' | 'failed' | 'in_progress' | 'unverified';
  run: Run | null;
  activity: { queued: number; running: number; scope: 'returnedCandidates' } | null;
  candidateCount: number;
  limitations: Limitation[];
}

function revisionMatch(run: Run, revision?: string, root?: string): RevisionMatch {
  if (!revision || !root) return 'unverified';

  const matching = run.revisions?.filter((r) => r.vcsRootId === root);

  if (matching?.length !== 1) return 'unverified';

  return matching.every((r) => r.revision === revision) ? 'exact' : 'different';
}

export function assessStatusJob(
  id: string,
  snapshot: JobSnapshot | undefined,
  revision?: string,
  root?: string,
): StatusJob {
  const value: StatusJob = {
    jobId: id,
    required: true,
    availability: snapshot ? 'available' : 'unavailable',
    match: 'unverified',
    assessment: 'unverified',
    run: null,
    activity: null,
    candidateCount: snapshot?.page.providerReturned ?? 0,
    limitations: [],
  };
  const note = (code: string, message: string) =>
    value.limitations.push({ code, message, source: 'run' });

  if (!snapshot) {
    note(
      'REQUIRED_JOB_UNAVAILABLE',
      'The required job was not returned; absence and permission coverage are unverified',
    );

    return value;
  }

  const runs = snapshot.page.runs;
  const index = runs.findIndex((run) => revisionMatch(run, revision, root) === 'exact');
  const selected = index >= 0 ? runs[index] : runs[0];

  value.activity = {
    queued: runs.filter((run) => run.state === 'queued').length,
    running: runs.filter((run) => run.state === 'running').length,
    scope: 'returnedCandidates',
  };
  value.run = selected ?? null;
  value.match = selected ? revisionMatch(selected, revision, root) : 'unverified';

  if (snapshot.page.limitations.some((note) => note.code === 'UNSAFE_CONTINUATION')) {
    value.limitations.push(
      ...snapshot.page.limitations.filter((note) => note.code === 'UNSAFE_CONTINUATION'),
    );

    return value;
  }

  if (value.match !== 'exact' || !selected) {
    note(
      'EXACT_REVISION_UNVERIFIED',
      'No exact selected-root checkout was verified in the bounded candidate window',
    );

    return value;
  }

  if (selected.revisions?.some((r) => r.vcsRootId !== root)) {
    note(
      'OTHER_ROOTS_UNVERIFIED',
      'The selected root matches; other reported root revisions do not have local checkout verification',
    );

    return value;
  }

  if (selected.personal !== false) {
    note(
      'PERSONAL_CHECKOUT_UNVERIFIED',
      'A personal or unspecified checkout cannot certify the committed worktree',
    );

    return value;
  }

  if (runs.slice(0, index).some((run) => revisionMatch(run, revision, root) === 'unverified')) {
    note('NEWER_REVISION_UNVERIFIED', 'A newer candidate lacks selected-root revision evidence');

    return value;
  }

  if (selected.state === 'queued' || selected.state === 'running') {
    value.assessment = 'in_progress';
  } else if (selected.state === 'finished' && selected.result === 'success') {
    value.assessment = 'passed';
  } else if (
    selected.state === 'finished' &&
    ['failure', 'error', 'canceled', 'failed_to_start'].includes(selected.result)
  ) {
    value.assessment = 'failed';
  } else {
    note('RUN_OUTCOME_UNVERIFIED', 'The selected execution lacks a verified final outcome');
  }

  return value;
}
