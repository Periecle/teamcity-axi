import { createServer } from 'node:http';
export const run = {
  id: 482193,
  buildTypeId: 'Payments_Build',
  number: '42',
  state: 'finished',
  status: 'FAILURE',
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
    let value;
    if (path === '/app/rest/server') value = wire.server;
    else if (path === '/app/rest/users/current') value = { id: 2, username: 'fixture-reader' };
    else if (path.startsWith('/app/rest/projects/id:')) {
      const literal = /\(\$base64:([A-Za-z0-9_-]+)\)/.exec(path)?.[1];
      const id = literal ? Buffer.from(literal, 'base64url').toString() : path.split('id:')[1];
      value = {
        id,
        name: 'Fixture project',
        archived: false,
        ...(id === '_Root' ? {} : { parentProjectId: '_Root' }),
      };
      if (mode === 'project-cycle') value.parentProjectId = id;
      if (mode === 'project-wrong-id') value.id = 'Other';
    } else if (path.startsWith('/app/rest/buildTypes/id:'))
      value = { id: run.buildTypeId, name: 'Build', projectId: 'Payments', paused: false };
    else if (path.endsWith('/snapshot-dependencies'))
      return error(406, 'This subresource does not provide the supported JSON collection');
    else if (path === '/app/rest/builds/id:482193')
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
    else if (path === '/app/messages') value = wire.messages;
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
    res.end(JSON.stringify(value));
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const base = `http://127.0.0.1:${server.address().port}/teamcity`;
  return {
    base,
    requests,
    setMode(value) {
      mode = value;
    },
    close: () =>
      new Promise((resolve) => {
        server.closeAllConnections();
        server.close(resolve);
      }),
  };
}
