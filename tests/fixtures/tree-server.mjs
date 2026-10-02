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
      if (['changed', 'changed-metadata', 'provisional', 'final-unavailable'].includes(mode)) {
        value = { ...value, state: mode === 'changed' && detailReads > 1 ? 'finished' : 'running' };
        if (mode === 'changed' && detailReads > 1) value.status = 'SUCCESS';
        if (mode === 'changed-metadata' && detailReads > 1) value.branchName = 'changed-branch';
      }
      return send(value);
    }

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
