import { createServer } from 'node:http';

import { run } from './mock-server.mjs';

const rootId = 482193;
const node = (id) => ({
  ...run,
  id,
  buildTypeId: id === rootId ? run.buildTypeId : `Payments_Job_${id}`,
  buildType: {
    id: id === rootId ? run.buildTypeId : `Payments_Job_${id}`,
    name: 'Graph fixture',
    projectId: 'Payments',
  },
});

export async function treeServer() {
  let mode = 'dag';
  let detailReads = 0;
  const requests = [];
  const server = createServer((req, res) => {
    const url = new URL(req.url, 'http://localhost');
    const path = url.pathname.slice('/teamcity'.length);

    requests.push({ method: req.method, path, query: Object.fromEntries(url.searchParams) });
    res.setHeader('Content-Type', 'application/json');

    const send = (value, status = 200) => {
      res.statusCode = status;
      res.end(JSON.stringify(value));
    };

    if (req.method !== 'GET') return send({ message: 'Read only' }, 405);
    if (!url.pathname.startsWith('/teamcity/')) return send({ message: 'Missing prefix' }, 404);
    if (mode === 'root-denied') return send({ message: 'Denied' }, 403);

    const adjacency =
      mode === 'cycle'
        ? { 482193: [482190], 482190: [482191], 482191: [482193] }
        : ['changed', 'changed-metadata', 'provisional', 'final-unavailable'].includes(mode)
          ? { 482193: [] }
          : mode === 'wide'
            ? { 482193: Array.from({ length: 40 }, (_, i) => 482100 + i) }
            : { 482193: [482190, 482191], 482190: [482188], 482191: [482188], 482188: [] };
    const count = (id) => (adjacency[id] ?? []).length;
    const id = Number(/^\/app\/rest\/builds\/id:(\d+)$/.exec(path)?.[1]);

    if (Number.isSafeInteger(id) && id > 0) {
      if (mode === 'denied' && id === 482190) return send({ message: 'Denied child' }, 403);
      if (url.searchParams.get('fields') === 'id,snapshot-dependencies(count)')
        return send({
          id,
          'snapshot-dependencies': {
            count: count(id) + (mode === 'mismatch' && id === rootId ? 1 : 0),
          },
        });

      if (id !== rootId) return send({ message: 'Unexpected detail traversal' }, 400);
      detailReads++;
      if (mode === 'final-unavailable' && detailReads > 1)
        return send({ message: 'Final read denied' }, 403);
      let value = node(id);
      if (mode === 'success') value.status = 'SUCCESS';
      if (['changed', 'changed-metadata', 'provisional', 'final-unavailable'].includes(mode)) {
        value = { ...value, state: mode === 'changed' && detailReads > 1 ? 'finished' : 'running' };
        if (mode === 'changed' && detailReads > 1) value.status = 'SUCCESS';
        if (mode === 'changed-metadata' && detailReads > 1) value.branchName = 'changed-branch';
      }
      return send(value);
    }

    if (path === '/app/rest/problemOccurrences' || path === '/app/rest/testOccurrences') {
      const id = Number(/build:\(id:(\d+)\)/.exec(url.searchParams.get('locator'))?.[1]);
      if (!id) return send({ message: 'Missing source identity' }, 400);
      if (mode === 'sources-denied' && path.endsWith('problemOccurrences'))
        return send({ message: 'Independent source denied' }, 403);
      if (mode === 'logs-needed' && path.endsWith('testOccurrences'))
        return send({ message: 'Independent tests unavailable' }, 404);
      if (path.endsWith('problemOccurrences'))
        return send({
          count: 1,
          problemOccurrence: [
            {
              id: `build:(id:${id}),problem:(id:1)`,
              build: { id },
              type: 'SYNTHETIC',
              identity: 'synthetic',
              details: ['logs-needed', 'log-unicode'].includes(mode)
                ? ''
                : 'Explicit synthetic problem',
            },
          ],
        });
      if (mode === 'log-unicode') return send({ count: 0, testOccurrence: [] });
      const muted = url.searchParams.get('locator').includes('muted:true');
      return send({
        count: 2,
        testOccurrence: [1, 2].map((number) => ({
          id: `build:(id:${id}),id:${muted ? number + 2 : number}`,
          build: { id },
          name: 'same name',
          status: 'FAILURE',
          muted,
          ignored: false,
          duration: 25,
          details:
            mode === 'secret' ? 'fixture-only-token' + 'x'.repeat(3000) : 'Connection refused',
          test: { id: '517450581327024597' },
        })),
      });
    }
    if (path === '/app/messages') {
      if (mode === 'logs-needed') return send({ message: 'Log capability unavailable' }, 404);
      return send({
        messages: [
          {
            id: 12,
            text:
              mode === 'log-unicode'
                ? '--error=Connection refused ' + 'x'.repeat(52) + '🦊'
                : 'Connection refused',
            level: 0,
            status: 4,
            timestamp: '2026-10-02T00:00:00Z',
          },
        ],
        lastMessageIndex: 12,
        focusIndex: 12,
        lastMessageIncluded: true,
      });
    }
    if (path === '/app/rest/changes' && mode === 'changes-wide')
      return send({
        count: 10,
        change: Array.from({ length: 10 }, (_, i) => ({
          id: String(101 + i),
          version: 'a'.repeat(40),
          comment: '🦊'.repeat(1500),
          date: '20261001T135900+0000',
          vcsRootInstance: { 'vcs-root-id': 'Payments_Git' },
        })),
      });
    if (path === '/app/rest/changes')
      return send(
        mode === 'changes-positive'
          ? {
              count: 1,
              change: [
                {
                  id: '101',
                  version: 'a'.repeat(40),
                  comment: 'Synthetic contextual change\nFurther detail',
                  date: '20261001T135900+0000',
                  vcsRootInstance: { 'vcs-root-id': 'Payments_Git' },
                },
              ],
            }
          : { count: 0, change: [] },
      );

    if (path === '/app/rest/builds') {
      const locator = url.searchParams.get('locator');
      const parent = Number(
        /snapshotDependency:\(to:\(id:(\d+)\),recursive:false\)/.exec(locator)?.[1],
      );
      if (!parent || !locator.includes('defaultFilter:false'))
        return send({ message: 'Expected immediate dependency locator' }, 400);
      if (mode === 'denied' && parent === 482190) return send({ message: 'Denied child' }, 403);
      const start = Number(/(?:^|,)start:(\d+)/.exec(locator)?.[1]);
      let builds = (adjacency[parent] ?? []).map(node);
      if (mode === 'foreign')
        builds = builds.map((value) =>
          value.id === 482191
            ? {
                ...value,
                state: 'future',
                revisions: undefined,
                buildType: { ...value.buildType, projectId: 'Foreign' },
              }
            : value,
        );
      if (mode === 'missing-metadata') builds = builds.map(({ revisions, ...value }) => value);
      if (mode === 'invalid-timestamp')
        builds = builds.map((value) => ({ ...value, startDate: 'bad' }));
      const value = { count: builds.length, build: builds };
      if (mode === 'unsafe' && parent === rootId)
        value.nextHref = 'https://attacker.invalid/app/rest/builds';
      if (mode === 'empty' && parent === rootId && start === 0) {
        value.count = 0;
        value.build = [];
        value.nextHref =
          '/teamcity/app/rest/builds?' +
          new URLSearchParams({
            locator: locator.replace('start:0', 'start:100'),
            fields: url.searchParams.get('fields'),
          });
      }
      return send(value);
    }

    if (path.startsWith('/app/rest/projects/id:')) {
      const literal = /\(\$base64:([A-Za-z0-9_-]+)\)/.exec(path)?.[1];
      const projectId = literal
        ? Buffer.from(literal, 'base64url').toString()
        : path.split('id:')[1];
      return send({
        id: projectId,
        name: 'Fixture project',
        archived: false,
        ...(projectId === '_Root' ? {} : { parentProjectId: '_Root' }),
      });
    }
    return send({ message: 'Unexpected graph request' }, 404);
  });

  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));

  return {
    base: `http://127.0.0.1:${server.address().port}/teamcity`,
    requests,
    setMode(value) {
      mode = value;
      detailReads = 0;
      requests.length = 0;
    },
    close: () =>
      new Promise((resolve) => {
        server.closeAllConnections();
        server.close(resolve);
      }),
  };
}
