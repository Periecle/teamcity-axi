import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { parseRaw } from '../../dist/adapter/raw.js';
import {
  normalizeProject,
  normalizeIdentity,
  normalizeServer,
  normalizeLogTail,
} from '../../dist/adapter/metadata.js';
import { ProjectPolicy } from '../../dist/context/project-policy.js';
const contract = JSON.parse(
  readFileSync('tests/fixtures/teamcity-2026.2-native-1.5.0/contract.json', 'utf8'),
);
const body = (name) => {
  const r = contract.records[name];
  return parseRaw({
    stdout: Buffer.from(r.stdout),
    stderr: Buffer.from(r.stderr),
    exitCode: r.code,
    signal: r.signal,
  }).body;
};
test('recorded project, server and current identity normalize without raw or personal fields', () => {
  const project = normalizeProject(body('project'), 'AxiContract', []);
  assert.equal(project.parentProjectId, '_Root');
  assert.equal(normalizeProject(body('root-project'), '_Root', []).parentProjectId, null);
  assert.equal(normalizeServer(body('server'), []).buildNumber, '238924');
  const current = normalizeIdentity(
    { ...body('identity'), token: 'canary', name: 'not-public' },
    'http://127.0.0.1:32768',
  );
  assert.deepEqual(Object.keys(current), ['fingerprint']);
  assert.match(current.fingerprint, /^sha256:[a-f0-9]{32}$/);
  assert.notEqual(
    current.fingerprint,
    normalizeIdentity(body('identity'), 'https://other.invalid').fingerprint,
  );
  assert.throws(
    () => normalizeProject(body('project'), 'Other', []),
    (e) => e.code === 'CONTEXT_MISMATCH',
  );
  const { parentProjectId, ...unknown } = body('project');
  assert.throws(
    () => normalizeProject(unknown, 'AxiContract', []),
    (e) => e.code === 'UPSTREAM_SCHEMA_MISMATCH',
  );
  assert.throws(() => normalizeIdentity({ id: 0, username: 'guest' }, 'https://a.invalid'));
});
test('native log overdelivery is capped and credentials are removed before text previews', () => {
  const dto = JSON.parse(contract.records['log-probe'].stdout);
  const tail = normalizeLogTail(dto, '1', 1, []);
  assert.equal(tail.providerReturned, 2);
  assert.equal(tail.messages.length, 1);
  assert.equal(tail.truncated, true);
  assert.equal(tail.messages[0].id, String(dto.messages.at(-1).id));
  const secret = 'secret-' + 'z'.repeat(2500);
  const sanitized = normalizeLogTail(
    { ...dto, messages: [{ ...dto.messages[0], text: secret }] },
    '1',
    1,
    [secret],
  );
  assert.equal(sanitized.messages[0].text, '[REDACTED]');
  assert.throws(
    () => normalizeLogTail({ ...dto, run_id: '2' }, '1', 1, []),
    (e) => e.code === 'CONTEXT_MISMATCH',
  );
  assert.throws(() =>
    normalizeLogTail({ ...dto, messages: [dto.messages[0], dto.messages[0]] }, '1', 1, []),
  );
});
test('policy admits only observed ancestor chains and caches within one invocation', async () => {
  const calls = [];
  const reader = {
    async getProject({ id }) {
      calls.push(id);
      return {
        state: 'available',
        value: { id, parentProjectId: id === 'Child' ? 'Allowed' : '_Root' },
      };
    },
  };
  const policy = new ProjectPolicy(reader, ['Allowed'], { deadline: Date.now() + 1000 });
  await policy.assert('Child');
  await policy.assert('Child');
  assert.deepEqual(calls, ['Child']);
  await assert.rejects(policy.assert('Allowed_Evil'), (e) => e.code === 'POLICY_DENIED');
  await assert.rejects(policy.assert(null), (e) => e.code === 'POLICY_DENIED');
  const cycle = new ProjectPolicy(
    {
      async getProject({ id }) {
        return { state: 'available', value: { id, parentProjectId: id === 'a' ? 'b' : 'a' } };
      },
    },
    ['Allowed'],
    { deadline: Date.now() + 1000 },
  );
  await assert.rejects(cycle.assert('a'), (e) => e.code === 'POLICY_DENIED');
  let depth = 0;
  const deep = new ProjectPolicy(
    {
      async getProject() {
        depth++;
        return { state: 'available', value: { parentProjectId: String(depth) } };
      },
    },
    ['Allowed'],
    { deadline: Date.now() + 1000 },
  );
  await assert.rejects(deep.assert('first'), (e) => e.code === 'POLICY_DENIED');
  assert.equal(depth, 8);
  const unavailable = new ProjectPolicy(
    {
      async getProject() {
        return {
          state: 'unavailable',
          error: Object.assign(Error('denied'), { code: 'PERMISSION_DENIED' }),
        };
      },
    },
    ['Allowed'],
    { deadline: Date.now() + 1000 },
  );
  await assert.rejects(unavailable.assert('Other'), (e) => e.code === 'PERMISSION_DENIED');
});

test('the eighth observed ancestor is accepted without a ninth read', async () => {
  let count = 0;
  const policy = new ProjectPolicy(
    {
      async getProject({ id }) {
        count++;
        return { state: 'available', value: { id, parentProjectId: 'p' + count } };
      },
    },
    ['p8'],
    { deadline: Date.now() + 1000 },
  );
  await policy.assert('p0');
  assert.equal(count, 8);
});
