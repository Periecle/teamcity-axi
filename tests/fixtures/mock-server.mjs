import { createServer } from 'node:http';
export const run = {
  id: 482193,
  buildTypeId: 'Payments_Build',
  number: '42',
  state: 'finished',
  status: 'FAILURE',
  failedToStart: false,
  branchName: 'feature/refund',
  statusText: 'Tests failed',
  personal: false,
  composite: false,
  buildType: { id: 'Payments_Build', name: 'Build', projectId: 'Payments' },
  revisions: {
    count: 1,
    revision: [
      {
        version: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
        'vcs-root-instance': { id: '17', 'vcs-root-id': 'Payments_Git' },
      },
    ],
  },
  startDate: '20261001T140000+0000',
  finishDate: '20261001T140100+0000',
};
export const longCanary = 'fixture-secret-' + 'q'.repeat(1600);
export const wire = {
  server: { version: 'mock-contract', buildNumber: 'not-a-TeamCity-server' },
  builds: { count: 1, build: [run] },
  problems: {
    count: 1,
    problemOccurrence: [
      {
        id: 'problem-1',
        type: 'TC_TESTS_FAILED',
        identity: 'tests',
        details: 'One test failed',
        build: { id: 482193 },
      },
    ],
  },
  tests: {
    count: 2,
    failed: 2,
    testOccurrence: [
      {
        id: 'occ-1',
        name: 'PaymentServiceIT.shouldRefund',
        status: 'FAILURE',
        duration: 100,
        muted: false,
        ignored: false,
        details: 'Connection refused',
        build: { id: 482193 },
        test: { id: 'test-1' },
      },
      {
        id: 'occ-2',
        name: 'PaymentServiceIT.shouldRefund',
        status: 'FAILURE',
        duration: 200,
        muted: true,
        ignored: false,
        details: 'Muted failure',
        build: { id: 482193 },
        test: { id: 'test-2' },
      },
    ],
  },
  dependencies: {
    count: 1,
    build: [
      {
        ...run,
        id: 482188,
        buildTypeId: 'Payments_IntegrationTests',
        buildType: { id: 'Payments_IntegrationTests', name: 'Integration', projectId: 'Payments' },
      },
    ],
  },
  changes: {
    count: 1,
    change: [
      {
        id: '101',
        version: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
        comment: 'Fix refund',
        date: '20261001T135900+0000',
        vcsRootInstance: { 'vcs-root-id': 'Payments_Git' },
      },
    ],
  },
  jobs: {
    count: 1,
    buildType: [{ id: 'Payments_Build', name: 'Build', projectId: 'Payments', paused: false }],
  },
  queue: {
    count: 1,
    build: [
      {
        id: 482194,
        buildTypeId: 'Payments_Build',
        state: 'queued',
        branchName: 'feature/refund',
        waitReason: 'Waiting for compatible agent',
      },
    ],
  },
  agents: {
    count: 1,
    agent: [
      {
        id: 7,
        name: 'linux-1',
        connected: true,
        enabled: true,
        authorized: true,
        pool: { id: 1, name: 'Default' },
      },
    ],
  },
  messages: {
    messages: [
      {
        id: 12,
        text: 'Connection refused',
        level: 0,
        status: 4,
        timestamp: '2026-10-01T14:00:01Z',
      },
    ],
    lastMessageIndex: 12,
    focusIndex: 12,
    lastMessageIncluded: true,
  },
};
export async function mockServer() {
  const requests = [];
  let mode = 'ok';
  let statusRevision = run.revisions.revision[0].version;
  let watchReads = 0;
  const server = createServer((req, res) => {
    const url = new URL(req.url, 'http://localhost');
    requests.push({
      method: req.method,
      path: url.pathname,
      query: Object.fromEntries(url.searchParams),
      authenticated: req.headers.authorization === 'Bearer fixture-only-token',
    });
    res.setHeader('Content-Type', 'application/json');
    res.setHeader('Date', 'Thu, 01 Oct 2026 14:00:00 GMT');
    const error = (status, message) => {
      res.statusCode = status;
      res.end(JSON.stringify({ message }));
    };
    if (req.method !== 'GET') return error(405, 'Read only');
    if (!url.pathname.startsWith('/teamcity/')) return error(404, 'Context path missing');
    if (mode === 'denied') return error(403, 'Permission denied');
    if (mode === 'missing') return error(404, 'Not found');
    if (mode === 'expired') return error(401, 'Expired credentials');
    if (mode === 'malformed') return res.end('{broken');
    if (mode === 'html') {
      res.setHeader('Content-Type', 'text/html');
      return res.end('<html>Login</html>');
    }
    if (mode === 'hang') return;
    const path = url.pathname.slice('/teamcity'.length);
    if (mode === 'summary-denied' && path === '/app/rest/testOccurrences')
      return error(403, 'Cannot read tests');
    if (mode === 'logs-unsupported' && path === '/app/messages')
      return error(404, 'Capability not found');
    if (mode === 'logs-hang' && path === '/app/messages') return;
    if (mode === 'problems-denied' && path.startsWith('/app/rest/problemOccurrences'))
      return error(403, 'Problem source denied');
    if (mode === 'tests-denied' && path.startsWith('/app/rest/testOccurrences'))
      return error(403, 'Test source denied');
    if (
      path === '/app/rest/builds' &&
      url.searchParams.get('locator')?.includes('snapshotDependency:')
    ) {
      if (mode === 'dependencies-denied') return error(403, 'Dependency source denied');
      if (mode === 'dependencies-unsupported') return error(404, 'Dependency locator unavailable');
      if (mode === 'dependencies-huge')
        return res.end(
          JSON.stringify({ count: 1, build: [{ ...run, statusText: 'x'.repeat(3000000) }] }),
        );
      if (mode === 'dependencies-malformed')
        return res.end(JSON.stringify({ count: 0, build: wire.dependencies.build }));
    }
    if (path === '/app/rest/changes' && url.searchParams.get('locator')?.includes('start:')) {
      if (mode === 'changes-denied') return error(403, 'Change source denied');
      if (mode === 'changes-unsupported') return error(404, 'Change endpoint unavailable');
      if (mode === 'changes-huge')
        return res.end(
          JSON.stringify({
            count: 1,
            change: [{ ...wire.changes.change[0], comment: 'x'.repeat(3000000) }],
          }),
        );
      const locator = url.searchParams.get('locator');
      const count = Number(/(?:^|,)count:(\d+)/.exec(locator)?.[1] ?? 10);
      const start = Number(/(?:^|,)start:(\d+)/.exec(locator)?.[1] ?? 0);
      const includeFiles = url.searchParams.get('fields')?.includes('files(');
      let changes = [0, 1, 2].map((index) => ({
        ...wire.changes.change[0],
        id: String(101 + index),
        comment: `Synthetic change ${index}\n\nContextual fixture evidence only`,
        ...(includeFiles
          ? { files: { count: 1, file: [{ file: 'src/fixture.ts', changeType: 'edited' }] } }
          : {}),
      }));
      if (mode === 'changes-wrong-root')
        changes[0].vcsRootInstance = { 'vcs-root-id': 'Foreign_Git' };
      if (mode === 'changes-no-root') changes[0].vcsRootInstance = null;
      if (mode === 'changes-secret') changes[0].comment = longCanary;
      if (mode === 'changes-preview') changes[0].comment = '🦊'.repeat(2500) + '\nOther line';
      if (mode === 'changes-file-limit')
        changes[0].files = {
          count: 101,
          file: Array.from({ length: 101 }, () => ({ file: 'src/fixture.ts' })),
        };
      const selected = changes.slice(start, start + count);
      const nextHref =
        '/teamcity/app/rest/changes?' +
        new URLSearchParams({
          locator: locator.replace(/start:\d+/, `start:${start + count}`),
          fields: url.searchParams.get('fields'),
        });
      return res.end(
        JSON.stringify({
          count: selected.length,
          change: selected,
          ...(mode === 'changes-unsafe'
            ? { nextHref: 42 }
            : start + count < changes.length
              ? { nextHref }
              : {}),
        }),
      );
    }
    if (mode === 'evidence-huge' && path.startsWith('/app/rest/problemOccurrences'))
      return res.end(
        JSON.stringify({
          count: 1,
          problemOccurrence: [
            { ...wire.problems.problemOccurrence[0], details: 'x'.repeat(3000000) },
          ],
        }),
      );
    if (
      path.startsWith('/app/rest/problemOccurrences/') ||
      path.startsWith('/app/rest/testOccurrences/') ||
      ((path === '/app/rest/problemOccurrences' || path === '/app/rest/testOccurrences') &&
        url.searchParams.get('locator')?.includes('start:'))
    ) {
      const isProblem = path.includes('problemOccurrences'),
        key = isProblem ? 'problemOccurrence' : 'testOccurrence';
      let items = isProblem
        ? [
            { ...wire.problems.problemOccurrence[0], id: 'build:(id:482193),problem:(id:1)' },
            {
              ...wire.problems.problemOccurrence[0],
              id: 'build:(id:482193),problem:(id:2)',
              identity: 'another-problem',
            },
          ]
        : [
            {
              ...wire.tests.testOccurrence[0],
              id: 'build:(id:482193),id:2000000000',
              test: { id: '517450581327024597' },
            },
            {
              ...wire.tests.testOccurrence[1],
              id: 'build:(id:482193),id:2000000001',
              test: { id: '517450581327024598' },
            },
            {
              ...wire.tests.testOccurrence[0],
              id: 'build:(id:482193),id:2000000002',
              status: 'IGNORED',
              ignored: true,
            },
          ];
      if (mode === 'evidence-wrong-run')
        items = items.map((item) => ({ ...item, build: { id: 482100 } }));
      if (mode === 'evidence-duplicate-id') items[1].id = items[0].id;
      if (mode === 'evidence-unknown-test') items[0].status = 'FUTURE_RESULT';
      if (mode === 'evidence-no-flags') {
        delete items[0].muted;
        delete items[0].ignored;
      }
      if (mode === 'evidence-preview')
        items[1][isProblem ? 'details' : 'details'] = '🦊'.repeat(2500);
      if (mode === 'evidence-secret') items[0].details = longCanary;
      const locator = url.searchParams.get('locator') ?? '';
      if (locator.includes('status:FAILURE'))
        items = items.filter((item) => item.status === 'FAILURE');
      if (locator.includes('muted:false')) items = items.filter((item) => item.muted !== true);
      if (locator.includes('muted:true')) items = items.filter((item) => item.muted === true);
      if (path.endsWith('Occurrences')) {
        const count = Number(/(?:^|,)count:(\d+)/.exec(locator)?.[1] ?? 20),
          start = Number(/(?:^|,)start:(\d+)/.exec(locator)?.[1] ?? 0);
        const nextHref =
          '/teamcity' +
          path +
          '?' +
          new URLSearchParams({
            locator: locator.replace(/start:\d+/, `start:${start + count}`),
            fields: url.searchParams.get('fields'),
          });
        const rows = mode === 'evidence-empty' ? [] : items.slice(start, start + count);
        return res.end(
          JSON.stringify({
            count: rows.length,
            [key]: rows,
            ...(mode === 'evidence-unsafe'
              ? { nextHref: 'https://attacker.invalid/app/rest/testOccurrences' }
              : start + count <= items.length || mode === 'evidence-empty'
                ? { nextHref }
                : {}),
          }),
        );
      }
      const id = path.slice(path.lastIndexOf('/') + 1);
      const item = items.find((item) => item.id === id);
      if (!item) return error(404, 'Occurrence missing');
      return res.end(
        JSON.stringify(mode === 'evidence-wrong-id' ? { ...item, id: items[1].id } : item),
      );
    }
    let value;
    if (path === '/app/rest/server') value = wire.server;
    else if (path === '/app/rest/users/current') value = { id: 2, username: 'fixture-reader' };
    else if (path.startsWith('/app/rest/projects/id:')) {
      const literal = /\(\$base64:([A-Za-z0-9_-]+)\)/.exec(path)?.[1];
      const id = literal ? Buffer.from(literal, 'base64url').toString() : path.split('id:')[1];
      if (mode === 'agent-policy-hang' && id === 'Payments_Child') return;
      value = {
        id,
        name: 'Fixture project',
        archived: false,
        ...(id === '_Root' ? {} : { parentProjectId: '_Root' }),
      };
      if (mode === 'project-cycle') value.parentProjectId = id;
      if (mode === 'project-wrong-id') value.id = 'Other';
      if (id === 'Payments_Child') value.parentProjectId = 'Payments';
    } else if (path.startsWith('/app/rest/buildTypes/id:')) {
      if (mode === 'jobs-detail-denied') return error(403, 'Job unavailable');
      if (mode === 'jobs-detail-missing') return error(404, 'Job not found');
      value = { id: run.buildTypeId, name: 'Build', projectId: 'Payments', paused: false };
      if (mode === 'jobs-wrong-id') value.id = 'Unrelated';
      if (mode === 'jobs-leading-id') value.id = '--job';
      if (mode === 'jobs-foreign') value.projectId = 'Forbidden';
      if (mode === 'jobs-unknown-paused') delete value.paused;
      if (mode === 'jobs-secret')
        value = {
          ...value,
          name: 'fixture-only-token\x1b[31m',
          parameters: { secret: 'server-private-parameter' },
        };
    } else if (path.endsWith('/snapshot-dependencies'))
      return error(406, 'This subresource does not provide the supported JSON collection');
    else if (
      path === '/app/rest/builds/id:482193' &&
      url.searchParams.get('fields') === 'id,snapshot-dependencies(count)'
    )
      value = {
        id: mode === 'dependency-count-wrong-id' ? 482100 : 482193,
        'snapshot-dependencies': { count: 1 },
      };
    else if (path === '/app/rest/builds/id:482194') {
      const { status, ...queued } = run;
      value = { ...queued, id: 482194, state: 'queued', statusText: 'Queued' };
    } else if (path === '/app/rest/builds/id:482193')
      value =
        mode === 'decorated-secret'
          ? {
              ...run,
              status: longCanary.slice(0, 600) + '\x1b[31m' + longCanary.slice(600),
              statusText: longCanary.slice(0, 600) + '\x1b[31m' + longCanary.slice(600),
            }
          : mode === 'long-secret'
            ? { ...run, status: longCanary, statusText: longCanary }
            : mode === 'missing-revisions'
              ? Object.fromEntries(Object.entries(run).filter(([key]) => key !== 'revisions'))
              : mode === 'wrong-id'
                ? { ...run, id: 482100, status: 'SUCCESS' }
                : mode === 'invalid-identity'
                  ? { ...run, id: 9007199254740992 }
                  : mode === 'unknown-enum'
                    ? { ...run, status: 'FUTURE_RESULT', state: 'new_lifecycle' }
                    : mode === 'huge-text'
                      ? { ...run, statusText: '🦊'.repeat(100000) }
                      : mode === 'huge'
                        ? { ...run, statusText: 'x'.repeat(3000000) }
                        : run;
    else if (path === '/app/rest/builds')
      value = url.searchParams.get('locator')?.includes('snapshotDependency:')
        ? wire.dependencies
        : wire.builds;
    else if (path === '/app/rest/problemOccurrences') value = wire.problems;
    else if (path === '/app/rest/testOccurrences') value = wire.tests;
    else if (path === '/app/rest/changes') value = wire.changes;
    else if (path === '/app/rest/buildTypes') value = wire.jobs;
    else if (path === '/app/rest/buildQueue') value = wire.queue;
    else if (path === '/app/rest/agents') value = wire.agents;
    else if (path.startsWith('/app/rest/agents/id:')) value = wire.agents.agent[0];
    else if (path === '/app/messages')
      value =
        mode === 'log-window'
          ? {
              messages: [
                {
                  ...wire.messages.messages[0],
                  id: 12,
                  text: 'x'.repeat(2500) + 'literal[needle]',
                },
                { ...wire.messages.messages[0], id: 13, text: 'plain' },
              ],
              lastMessageIncluded: true,
              lastMessageIndex: 13,
              focusIndex: 13,
            }
          : mode === 'log-overdelivery'
            ? {
                ...wire.messages,
                messages: Array.from({ length: 81 }, (_, id) => ({
                  ...wire.messages.messages[0],
                  id,
                  text: 'plain',
                })),
                lastMessageIndex: 80,
                focusIndex: 80,
              }
            : wire.messages;
    else return error(404, 'No fixture for requested path');
    if (path === '/app/rest/builds' && mode.startsWith('list-')) {
      const locator = url.searchParams.get('locator'),
        count = Number(/(?:^|,)count:(\d+)/.exec(locator)?.[1] ?? 20),
        start = Number(/(?:^|,)start:(\d+)/.exec(locator)?.[1] ?? 0);
      const nextLocator = locator.replace(/(?:^|,)start:\d+/, (m) =>
        m.startsWith(',') ? `,start:${start + count}` : `start:${start + count}`,
      );
      const nextHref =
        '/teamcity/app/rest/builds?' +
        new URLSearchParams({ locator: nextLocator, fields: url.searchParams.get('fields') });
      value = {
        build: mode === 'list-empty' ? [] : [run],
        count: mode === 'list-empty' ? 0 : 1,
        nextHref,
      };
      if (mode === 'list-unsafe') value.nextHref = 'https://attacker.invalid/app/rest/builds';
      if (mode === 'list-escalating')
        value.nextHref = nextHref.replace('lookupLimit%3A5000', 'lookupLimit%3A10000');
      if (mode === 'list-wrong-branch') value.build = [{ ...run, branchName: 'another-branch' }];
      if (mode === 'list-unknown-result') value.build = [{ ...run, status: 'FUTURE_RESULT' }];
      if (mode === 'list-duplicate') value = { build: [run, run], count: 2 };
    }
    if (
      path === '/app/rest/buildTypes' &&
      url.searchParams.get('fields')?.includes('builds($locator:')
    ) {
      const ids = [
        ...url.searchParams.get('locator').matchAll(/item:\(id:\(\$base64:([^)]*)\)\)/g),
      ].map((match) => Buffer.from(match[1], 'base64url').toString());
      value = {
        count: ids.length,
        buildType: ids.map((id, index) => ({
          id,
          name: id,
          projectId: 'Payments',
          paused: false,
          builds: {
            count: 1,
            build: [
              {
                ...run,
                id: run.id + index,
                buildTypeId: id,
                buildType: { id, projectId: 'Payments' },
                status: 'SUCCESS',
              },
            ],
          },
        })),
      };
      const first = value.buildType[0];
      const selected = first.builds.build[0];
      for (const job of value.buildType)
        job.builds.build[0].revisions = {
          revision: [
            {
              version: statusRevision,
              'vcs-root-instance': { id: '17', 'vcs-root-id': 'Payments_Git' },
            },
          ],
        };
      if (mode === 'status-red') selected.status = 'FAILURE';
      if (mode === 'status-canceled') {
        selected.status = 'UNKNOWN';
        selected.canceledInfo = { timestamp: '20261001T110000+0000' };
      }
      if (mode === 'status-failed-to-start') {
        selected.status = 'FAILURE';
        selected.failedToStart = true;
      }
      if (mode === 'status-missing-outcome') delete selected.failedToStart;
      if (mode === 'status-composite') selected.composite = true;
      if (mode === 'status-stale')
        selected.revisions = {
          revision: [
            {
              version: 'b'.repeat(40),
              'vcs-root-instance': { id: '17', 'vcs-root-id': 'Payments_Git' },
            },
          ],
        };
      if (mode === 'status-unknown') delete selected.revisions;
      if (mode === 'status-personal') selected.personal = true;
      if (mode === 'status-newer-unknown') {
        first.builds.build.unshift({
          ...selected,
          id: selected.id + 100,
          revisions: { revision: [] },
        });
        first.builds.count++;
      }
      if (mode === 'status-running' || mode === 'status-queued')
        selected.state = mode.slice('status-'.length);
      if (mode === 'status-queued') delete selected.status;
      if (mode === 'status-multi-root')
        selected.revisions = {
          revision: [
            ...selected.revisions.revision,
            {
              version: 'c'.repeat(40),
              'vcs-root-instance': { id: '18', 'vcs-root-id': 'Other_Git' },
            },
          ],
        };
      if (mode === 'status-missing-job') {
        value.buildType.pop();
        value.count--;
      }
      if (mode === 'status-foreign') {
        first.projectId = 'Forbidden';
        selected.buildType.projectId = 'Forbidden';
      }
      if (mode === 'status-wrong-run') selected.buildTypeId = 'Foreign_Job';
      if (mode === 'status-unsafe-continuation')
        first.builds.nextHref = 'https://attacker.invalid/steal';
      if (mode === 'status-huge')
        selected.revisions = {
          revision: Array.from({ length: 100 }, (_, index) => ({
            version: 'v'.repeat(256),
            'vcs-root-instance': { id: String(index + 1), 'vcs-root-id': 'r'.repeat(245) + index },
          })),
        };
    }
    if (path === '/app/rest/builds/id:482193' && mode.startsWith('watch-')) {
      watchReads++;
      if (watchReads > 1 && mode === 'watch-vanish') return error(404, 'Execution removed');
      if (watchReads > 1 && mode === 'watch-inaccessible')
        return error(403, 'Execution no longer visible');
      value = { ...run, state: 'running', status: 'SUCCESS' };
      if (mode === 'watch-missing-revisions') delete value.revisions;
      if (mode === 'watch-success' || (mode === 'watch-transition' && watchReads > 1))
        value.state = 'finished';
      if (mode === 'watch-queued') {
        value.state = 'queued';
        delete value.status;
      }
    }
    if (path === '/app/rest/buildTypes' && mode.startsWith('jobs-')) {
      if (mode === 'jobs-denied') return error(403, 'Job page unavailable');
      if (mode === 'jobs-unsupported') return error(404, 'Job collection unavailable');
      const locator = url.searchParams.get('locator'),
        count = Number(/(?:^|,)count:(\d+)/.exec(locator)?.[1] ?? 20),
        start = Number(/(?:^|,)start:(\d+)/.exec(locator)?.[1] ?? 0);
      const nextLocator = locator.replace(/(?:^|,)start:\d+/, (m) =>
        m.startsWith(',') ? `,start:${start + count}` : `start:${start + count}`,
      );
      const nextHref =
        '/teamcity/app/rest/buildTypes?' +
        new URLSearchParams({ locator: nextLocator, fields: url.searchParams.get('fields') });
      const job = { ...wire.jobs.buildType[0] };
      value = { count: 1, buildType: [job], nextHref };
      if (mode === 'jobs-empty') value = { count: 0, buildType: [] };
      if (mode === 'jobs-empty-next') value = { count: 0, buildType: [], nextHref };
      if (mode === 'jobs-unsafe') value.nextHref = 'https://attacker.invalid/app/rest/buildTypes';
      if (mode === 'jobs-escalating')
        value.nextHref = nextHref.replace('lookupLimit%3A5000', 'lookupLimit%3A10000');
      if (mode === 'jobs-scope-change')
        value.nextHref = nextHref.replace('UGF5bWVudHM', 'Rm9yYmlkZGVu');
      if (mode === 'jobs-duplicate') value = { count: 2, buildType: [job, job] };
      if (mode === 'jobs-foreign') job.projectId = 'Forbidden';
      if (mode === 'jobs-leading-id') {
        job.id = '--job';
        delete value.nextHref;
      }
      if (mode === 'jobs-all-unknown')
        value = {
          count,
          buildType: Array.from({ length: count }, (_, i) => ({
            id: `Payments_Job${i}`,
            name: 'Build',
            projectId: 'Payments',
          })),
        };
      if (mode === 'jobs-malformed') value.count = 0;
      if (mode === 'jobs-unknown-paused') delete job.paused;
      if (mode === 'jobs-secret') job.name = 'fixture-only-token\x1b[31m';
      if (mode === 'jobs-huge') job.name = 'x'.repeat(3000000);
      if (mode === 'jobs-oversized')
        value = {
          count,
          buildType: Array.from({ length: count }, (_, i) => ({
            ...job,
            id: `Payments_Job${i}`,
            name: '🦊'.repeat(100),
          })),
        };
    }
    if (
      path === '/app/rest/buildQueue' &&
      url.searchParams.get('fields')?.includes('buildType(id,projectId)')
    ) {
      if (mode === 'queue-denied') return error(403, 'Queue source denied');
      if (mode === 'queue-unsupported') return error(404, 'Queue source unavailable');
      const locator = url.searchParams.get('locator'),
        count = Number(/(?:^|,)count:(\d+)/.exec(locator)?.[1] ?? 20),
        start = Number(/(?:^|,)start:(\d+)/.exec(locator)?.[1] ?? 0);
      const nextLocator = locator.replace(/(?:^|,)start:\d+/, (m) =>
        m.startsWith(',') ? `,start:${start + count}` : `start:${start + count}`,
      );
      const nextHref =
        '/teamcity/app/rest/buildQueue?' +
        new URLSearchParams({ locator: nextLocator, fields: url.searchParams.get('fields') });
      const build = {
        ...wire.queue.build[0],
        queuedDate: '20261001T140000+0000',
        buildType: { id: 'Payments_Build', projectId: 'Payments' },
      };
      value = { count: 1, build: [build] };
      if (
        mode === 'queue-page' ||
        mode === 'queue-empty-next' ||
        mode.startsWith('queue-unsafe') ||
        mode === 'queue-escalating'
      )
        value.nextHref = nextHref;
      if (mode === 'queue-empty' || mode === 'queue-empty-next') {
        value.count = 0;
        value.build = [];
      }
      if (mode === 'queue-unsafe') value.nextHref = 'https://attacker.invalid/app/rest/buildQueue';
      if (mode === 'queue-unsafe-scope')
        value.nextHref = nextHref.replace('UGF5bWVudHM', 'Rm9yYmlkZGVu');
      if (mode === 'queue-escalating')
        value.nextHref = nextHref.replace('lookupLimit%3A5000', 'lookupLimit%3A10000');
      if (mode === 'queue-foreign') build.buildType.projectId = 'Forbidden';
      if (mode === 'queue-wrong-job') {
        build.buildTypeId = 'Other';
        build.buildType.id = 'Other';
      }
      if (mode === 'queue-conflict') build.buildType.id = 'Other';
      for (const [name, invalid] of [
        ['queue-surrogate-id', 'bad\ud800job'],
        ['queue-control-id', 'bad\u0085job'],
        ['queue-bidi-id', 'bad\u202ejob'],
      ])
        if (mode === name) {
          build.buildTypeId = invalid;
          build.buildType.id = invalid;
        }
      if (mode === 'queue-duplicate') value = { count: 2, build: [build, build] };
      if (mode === 'queue-malformed') value.count = 0;
      if (mode === 'queue-unknown') build.state = 'future-state';
      if (mode === 'queue-running') build.state = 'running';
      if (mode === 'queue-finished') build.state = 'finished';
      if (mode === 'queue-no-reason') delete build.waitReason;
      if (mode === 'queue-secret') {
        build.branchName = 'fixture-only-token';
        build.waitReason = 'fixture-only-token\x1b[31m';
      }
      if (mode === 'queue-bad-date') build.queuedDate = '20260230T140000+0000';
      if (mode === 'queue-huge') build.waitReason = 'x'.repeat(3000000);
      if (mode === 'queue-oversized') build.waitReason = '🦊'.repeat(1000);
      if (mode === 'queue-many-unknown')
        value = {
          count,
          build: Array.from({ length: count }, (_, i) => ({
            ...build,
            id: 482194 + i,
            state: 'future-state',
            queuedDate: 'invalid',
          })),
        };
    }
    if (
      path.startsWith('/app/rest/agents') &&
      (path.includes('/id:') || url.searchParams.get('fields')?.includes('build('))
    ) {
      if (mode === 'agent-denied') {
        res.statusCode = 403;
        return res.end(JSON.stringify({ errors: [{ message: 'Forbidden fixture scope' }] }));
      }
      if (mode === 'agent-unsupported' || mode === 'agent-missing') {
        res.statusCode = 404;
        return res.end(JSON.stringify({ errors: [{ message: 'Unavailable fixture capability' }] }));
      }
      const agent = structuredClone(wire.agents.agent[0]);
      const locator = url.searchParams.get('locator') ?? '';
      const count = Number(/(?:^|,)count:(\d+)/.exec(locator)?.[1] ?? 20);
      const start = Number(/(?:^|,)start:(\d+)/.exec(locator)?.[1] ?? 0);
      if (mode === 'agent-idle') agent.build = null;
      if (mode === 'agent-state-mix') {
        agent.enabled = false;
        agent.authorized = false;
        agent.build = null;
      }
      if (mode === 'agent-zero-pool') agent.pool.id = 0;
      if (mode === 'agent-wrong-pool') agent.pool.id = 2;
      if (mode === 'agent-wrong-id') agent.id = 8;
      if (mode === 'agent-malformed-id') agent.id = 0;
      if (mode === 'agent-unknown') {
        delete agent.connected;
        agent.enabled = null;
        delete agent.authorized;
        delete agent.pool;
      }
      if (mode === 'agent-bad-state') agent.enabled = 'true';
      if (mode === 'agent-secret') {
        agent.name = 'fixture-only-token\x1b[31m';
        agent.pool.name = 'fixture-only-token';
      }
      if (mode === 'agent-huge') agent.name = 'x'.repeat(3000000);
      if (mode === 'agent-oversized') agent.name = '🦊'.repeat(1000);
      if (
        [
          'agent-active',
          'agent-foreign-active',
          'agent-conflict-active',
          'agent-policy-hang',
        ].includes(mode)
      ) {
        agent.build = {
          id: 482193,
          buildTypeId: 'Payments_Build',
          buildType: {
            id: 'Payments_Build',
            projectId:
              mode === 'agent-foreign-active'
                ? 'Forbidden'
                : mode === 'agent-policy-hang'
                  ? 'Payments_Child'
                  : 'Payments',
          },
        };
        if (mode === 'agent-conflict-active') agent.build.buildType.id = 'Other';
      }
      if (path.includes('/id:')) value = agent;
      else {
        const nextLocator = locator.replace(
          /(?:^|,)start:\d+/,
          (m) => (m.startsWith(',') ? ',' : '') + 'start:' + (start + count),
        );
        const nextHref =
          '/teamcity/app/rest/agents?' +
          new URLSearchParams({ locator: nextLocator, fields: url.searchParams.get('fields') });
        value = { count: 1, agent: [agent] };
        if (
          mode === 'agent-page' ||
          mode === 'agent-slow-page' ||
          mode === 'agent-empty-next' ||
          mode.startsWith('agent-unsafe')
        )
          value.nextHref = nextHref;
        if (mode === 'agent-empty' || mode === 'agent-empty-next')
          value = { ...value, count: 0, agent: [] };
        if (mode === 'agent-unsafe') value.nextHref = 'https://attacker.invalid/app/rest/agents';
        if (mode === 'agent-unsafe-scope')
          value.nextHref = nextHref.replace('UGF5bWVudHM', 'Rm9yYmlkZGVu');
        if (mode === 'agent-unsafe-filter')
          value.nextHref = nextHref.replace('defaultFilter%3Afalse', 'defaultFilter%3Atrue');
        if (mode === 'agent-duplicate') value = { count: 2, agent: [agent, agent] };
        if (mode === 'agent-malformed-page') value.count = 0;
        if (mode === 'agent-many-unknown')
          value = {
            count,
            agent: Array.from({ length: count }, (_, i) => ({
              id: i + 1,
              name: 'Synthetic agent ' + i,
            })),
          };
      }
    }
    if (mode === 'agent-slow-page' && path === '/app/rest/agents') {
      setTimeout(() => res.end(JSON.stringify(value)), 1000);
      return;
    }
    if (mode.startsWith('outcome-')) {
      const exceptional = { ...run };
      if (mode === 'outcome-canceled') {
        exceptional.status = 'UNKNOWN';
        exceptional.canceledInfo = {
          timestamp: '20261001T110000+0000',
          text: 'Private cancellation comment',
          user: { username: 'Private actor' },
        };
      }
      if (mode === 'outcome-failed-to-start') exceptional.failedToStart = true;
      if (mode === 'outcome-composite') {
        exceptional.composite = true;
        exceptional.status = 'SUCCESS';
      }
      if (mode === 'outcome-missing' || mode === 'outcome-many-missing')
        delete exceptional.failedToStart;
      if (
        path === '/app/rest/builds/id:482193' &&
        url.searchParams.get('fields') !== 'id,snapshot-dependencies(count)'
      )
        value = exceptional;
      if (
        path === '/app/rest/builds' &&
        !url.searchParams.get('locator')?.includes('snapshotDependency:')
      )
        value = { count: 1, build: [exceptional] };
      if (mode === 'outcome-many-missing' && path === '/app/rest/builds')
        value = {
          count: 100,
          build: Array.from({ length: 100 }, (_, index) => ({
            ...exceptional,
            id: exceptional.id + index,
          })),
        };
    }
    res.end(JSON.stringify(value));
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const base = `http://127.0.0.1:${server.address().port}/teamcity`;
  return {
    base,
    requests,
    setMode(value) {
      mode = value;
      watchReads = 0;
    },
    setStatusRevision(value) {
      statusRevision = value;
    },
    close: () =>
      new Promise((resolve) => {
        server.closeAllConnections();
        server.close(resolve);
      }),
  };
}
