import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, rm, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { parse } from '../../dist/cli/parser.js';
import { resolveContext } from '../../dist/context/resolve.js';
import { canonicalUrl } from '../../dist/context/url.js';
async function workspace() {
  const dir = await mkdtemp(join(tmpdir(), 'axi-context-'));
  const repo = join(dir, 'repo');
  const configHome = join(dir, 'config');
  await mkdir(repo);
  await mkdir(join(configHome, 'teamcity-axi'), { recursive: true });
  const config = {
    schemaVersion: '1.0',
    readOnly: true,
    defaultServer: 'work',
    servers: {
      work: { url: 'https://TEAMCITY.example.test:443/teamcity/' },
      other: { url: 'https://other.example.test' },
    },
  };
  await writeFile(join(configHome, 'teamcity-axi', 'config.json'), JSON.stringify(config), {
    mode: 0o600,
  });
  const git = (args, cwd = repo) =>
    execFileSync('git', args, {
      cwd,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
      env: { PATH: process.env.PATH, HOME: dir, GIT_CONFIG_NOSYSTEM: '1' },
    }).trim();
  git(['init', '-b', 'main']);
  git([
    '-c',
    'user.name=Fixture',
    '-c',
    'user.email=fixture@example.invalid',
    '-c',
    'commit.gpgsign=false',
    'commit',
    '--allow-empty',
    '-m',
    'Fixture',
  ]);
  const env = { HOME: dir, PATH: process.env.PATH, XDG_CONFIG_HOME: configHome };
  const ctx = async (args = [], cwd = repo, extra = {}) =>
    resolveContext(parse(['context', 'show', '--cwd', cwd, ...args]), { ...env, ...extra });
  return {
    dir,
    repo,
    configHome,
    config,
    env,
    git,
    ctx,
    close: () => rm(dir, { recursive: true, force: true }),
  };
}
test('canonical trusted URLs preserve context paths and reject normalized traversal', () => {
  assert.equal(canonicalUrl('https://EXAMPLE.test:443/teamcity/'), 'https://example.test/teamcity');
  for (const url of [
    'https://host/teamcity/%2e%2e/other',
    'https://host/teamcity/../other',
    'https://user:secret@host',
    'https://host/?a=1',
    'https://host/#a',
    'http://host',
    'http://127.0.0.1',
  ])
    assert.throws(() => canonicalUrl(url));
  assert.equal(
    canonicalUrl('http://127.0.0.1:8111/teamcity', true),
    'http://127.0.0.1:8111/teamcity',
  );
});
test('native TOML deepest paths overlay defaults and explicit flags win', async () => {
  const f = await workspace();
  try {
    await mkdir(join(f.repo, 'packages', 'payments', 'src'), { recursive: true });
    await writeFile(
      join(f.repo, 'teamcity.toml'),
      '[[server]]\nurl="https://teamcity.example.test/teamcity"\nproject="Base"\njob="Base_Build"\njobs=["Base_Build","Deploy"]\n[server.paths."packages"]\nproject="Packages"\n[server.paths."packages/payments"]\njob="Payments_Build"\n',
    );
    const context = await f.ctx([], join(f.repo, 'packages', 'payments', 'src'));
    assert.equal(context.server, 'work');
    assert.equal(context.project, 'Base');
    assert.equal(context.job, 'Payments_Build');
    assert.equal(context.branch, 'main');
    assert.equal(context.sources.server, 'repository');
    assert.ok(Object.isFrozen(context));
    assert.ok(Object.isFrozen(context.jobs));
    assert.deepEqual(context.jobs, ['Base_Build', 'Deploy']);
    assert.equal(
      (await f.ctx(['--job', 'Explicit'], join(f.repo, 'packages', 'payments', 'src'))).job,
      'Explicit',
    );
    assert.deepEqual((await f.ctx(['--job', 'Explicit'])).jobs, ['Explicit']);
  } finally {
    await f.close();
  }
});
test('two worktrees retain distinct branches and trusted server selections', async () => {
  const f = await workspace();
  try {
    const second = join(f.dir, 'second');
    f.git(['worktree', 'add', '-b', 'feature/refund', second]);
    const [a, b] = await Promise.all([
      f.ctx(['--server', 'work']),
      f.ctx(['--server', 'other'], second),
    ]);
    assert.equal(a.branch, 'main');
    assert.equal(b.branch, 'feature/refund');
    assert.equal(a.server, 'work');
    assert.equal(b.server, 'other');
    assert.equal(b.repositoryRoot, second);
  } finally {
    await f.close();
  }
});
test('repository cannot register targets, redirect credentials or smuggle overlay privileges', async () => {
  const f = await workspace();
  try {
    await writeFile(
      join(f.repo, 'teamcity.toml'),
      '[[server]]\nurl="https://attacker.invalid"\njob="Build"\n',
    );
    await assert.rejects(f.ctx(), (e) => e.code === 'UNTRUSTED_SERVER');
    await assert.rejects(
      f.ctx(['--server', 'other'], f.repo, {
        TEAMCITY_URL: 'https://teamcity.example.test/teamcity',
        TEAMCITY_TOKEN: 'context-canary',
      }),
      (e) => e.code === 'AUTH_CONTEXT_MISMATCH',
    );
    await assert.rejects(
      f.ctx(['--server', 'work'], f.repo, { TEAMCITY_TOKEN: 'context-canary' }),
      (e) => e.code === 'AUTH_CONTEXT_MISMATCH',
    );
    await rm(join(f.repo, 'teamcity.toml'));
    await writeFile(
      join(f.repo, '.teamcity-axi.json'),
      JSON.stringify({ schemaVersion: '1.0', binaryPath: '/tmp/evil' }),
    );
    await assert.rejects(f.ctx(), (e) => e.code === 'USAGE_ERROR');
    await rm(join(f.repo, '.teamcity-axi.json'));
    const escape = join(f.dir, 'outside.json');
    await writeFile(escape, JSON.stringify({ schemaVersion: '1.0' }));
    await symlink(escape, join(f.repo, '.teamcity-axi.json'));
    await assert.rejects(f.ctx(), (e) => e.code === 'POLICY_DENIED');
  } finally {
    await f.close();
  }
});
test('detached HEAD, dirty checkout, literal @this and VCS remote mapping stay explicit', async () => {
  const f = await workspace();
  try {
    f.git(['remote', 'add', 'origin', 'https://git.example.test/payments.git']);
    await writeFile(
      join(f.repo, '.teamcity-axi.json'),
      JSON.stringify({
        schemaVersion: '1.0',
        vcsRoots: [{ server: 'work', remote: 'origin', rootId: 'Payments_Git' }],
      }),
    );
    const context = await f.ctx();
    assert.equal(context.vcsRootId, 'Payments_Git');
    assert.equal(context.dirty, true);
    assert.equal((await f.ctx(['--literal-branch', '@this'])).branch, '@this');
    f.git(['checkout', '--detach']);
    const detached = await f.ctx();
    assert.equal(detached.branch, undefined);
    assert.ok(detached.head);
    await assert.rejects(f.ctx(['--branch', '@this']), (e) => e.code === 'CONTEXT_REQUIRED');
    assert.equal((await f.ctx(['--revision', '@head'])).revision, detached.head);
    await assert.rejects(f.ctx(['--vcs-root', 'x'.repeat(257)]), (e) => e.code === 'USAGE_ERROR');
  } finally {
    await f.close();
  }
});
test('special/oversized/non-UTF8 configs and duplicate canonical aliases fail closed', async () => {
  const f = await workspace();
  try {
    execFileSync('mkfifo', [join(f.repo, 'teamcity.toml')]);
    const start = Date.now();
    await assert.rejects(f.ctx(['--timeout', '500ms']), (e) => e.code === 'USAGE_ERROR');
    assert.ok(Date.now() - start < 500);
    await rm(join(f.repo, 'teamcity.toml'));
    await writeFile(join(f.repo, 'teamcity.toml'), 'x'.repeat(65537));
    await assert.rejects(f.ctx(), (e) => e.code === 'USAGE_ERROR');
    await writeFile(join(f.repo, 'teamcity.toml'), Buffer.from([0xff, 0xfe]));
    await assert.rejects(f.ctx(), (e) => e.code === 'USAGE_ERROR');
    await rm(join(f.repo, 'teamcity.toml'));
    f.config.servers.other = { url: 'https://teamcity.example.test/teamcity' };
    await writeFile(join(f.configHome, 'teamcity-axi', 'config.json'), JSON.stringify(f.config));
    await assert.rejects(f.ctx(), (e) => e.code === 'AMBIGUOUS_CONTEXT');
  } finally {
    await f.close();
  }
});
