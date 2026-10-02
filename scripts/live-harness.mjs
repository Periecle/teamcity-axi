import { spawn } from 'node:child_process';
import { open, readFile, mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createHash } from 'node:crypto';
import { constants } from 'node:fs';
import { sanitize, sanitizeText } from '../dist/output/sanitize.js';
import { canonicalUrl } from '../dist/context/url.js';
import { parseRaw } from '../dist/adapter/raw.js';
// Test tooling only. Setup, administrator credentials and mutations are absent.
export async function liveFixture() {
  const credentialPath = process.env.TEAMCITY_AXI_LIVE_CREDENTIALS,
    binary = process.env.TEAMCITY_AXI_TEST_BINARY;
  if (!credentialPath || !binary)
    throw Error(
      'Live tests require credential configuration path and checksum-verified native binary; they never skip',
    );
  const handle = await open(
    credentialPath,
    constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK,
  );
  let credentials;
  try {
    const stat = await handle.stat();
    if (
      !stat.isFile() ||
      stat.size > 65536 ||
      (stat.mode & 0o077) !== 0 ||
      (process.getuid && stat.uid !== process.getuid())
    )
      throw Error('Live credential configuration must be an owned private regular file');
    const buffer = Buffer.alloc(65537);
    let used = 0;
    while (used < buffer.length) {
      const { bytesRead } = await handle.read(buffer, used, buffer.length - used, null);
      if (!bytesRead) break;
      used += bytesRead;
    }
    if (used > 65536) throw Error('Live credential configuration exceeds input limit');
    try {
      credentials = JSON.parse(buffer.subarray(0, used).toString('utf8'));
    } catch {
      throw Error('Invalid live credential configuration JSON');
    }
  } finally {
    await handle.close();
  }
  if (
    !credentials ||
    typeof credentials !== 'object' ||
    typeof credentials.token !== 'string' ||
    !credentials.token ||
    credentials.token.length > 8192 ||
    typeof credentials.serverUrl !== 'string'
  )
    throw Error('Invalid live credential configuration');
  const serverUrl = canonicalUrl(credentials.serverUrl, true);
  const contract = JSON.parse(
    await readFile(
      new URL('../tests/fixtures/teamcity-2026.2-native-1.5.0/contract.json', import.meta.url),
      'utf8',
    ),
  );
  const native = resolve(binary),
    sha = createHash('sha256')
      .update(await readFile(native))
      .digest('hex');
  if (sha !== contract.native.binarySha256)
    throw Error('Live contract requires its pinned native executable');
  const secrets = [
    credentials.token,
    ...(typeof credentials.password === 'string' ? [credentials.password] : []),
  ];
  const dir = await mkdtemp(join(tmpdir(), 'axi-live-'));
  const env = {
    PATH: process.env.PATH,
    HOME: dir,
    XDG_CONFIG_HOME: dir,
    TEAMCITY_URL: serverUrl,
    TEAMCITY_TOKEN: credentials.token,
    TEAMCITY_RO: '1',
    TEAMCITY_NO_UPDATE: '1',
    DO_NOT_TRACK: '1',
    NO_COLOR: '1',
    TERM: 'dumb',
  };
  try {
    await mkdir(join(dir, 'teamcity-axi'));
    await writeFile(
      join(dir, 'teamcity-axi', 'config.json'),
      JSON.stringify({
        schemaVersion: '1.0',
        readOnly: true,
        defaultServer: 'sandbox',
        binaryPath: native,
        servers: {
          sandbox: {
            url: serverUrl,
            allowHttpLoopback: true,
            allowedProjects: [contract.fixture.projectId],
          },
        },
      }),
      { mode: 0o600 },
    );
  } catch (error) {
    await rm(dir, { recursive: true, force: true });
    throw error;
  }
  const invoke = (executable, args, expired = false) =>
    new Promise((resolveCall, reject) => {
      const child = spawn(executable, args, {
        cwd: dir,
        env: { ...env, ...(expired ? { TEAMCITY_TOKEN: 'invalid-synthetic-token' } : {}) },
        detached: process.platform !== 'win32',
        stdio: ['ignore', 'pipe', 'pipe'],
      });
      const out = [],
        err = [];
      let outBytes = 0,
        errBytes = 0,
        failure;
      const kill = () => {
        if (child.pid)
          try {
            process.kill(process.platform === 'win32' ? child.pid : -child.pid, 'SIGKILL');
          } catch {}
      };
      const timer = setTimeout(() => {
        failure = Error('Live operation exceeded its deadline');
        kill();
      }, 10000);
      child.stdout.on('data', (b) => {
        outBytes += b.length;
        if (outBytes > 2097152) {
          failure = Error('Live stdout exceeded capture limit');
          kill();
        } else out.push(b);
      });
      child.stderr.on('data', (b) => {
        errBytes += b.length;
        if (errBytes > 65536) {
          failure = Error('Live stderr exceeded capture limit');
          kill();
        } else err.push(b);
      });
      child.on('error', () => {
        failure = Error('Cannot start live test executable');
      });
      child.on('close', (code, signal) => {
        clearTimeout(timer);
        kill();
        if (failure) return reject(failure);
        const nativeJson = executable === native && (args[0] === 'api' || args[0] === 'run');
        const stdout = Buffer.concat(out).toString('utf8');
        resolveCall({
          code,
          signal,
          stdout: nativeJson ? sanitizeLiveStdout(stdout, secrets) : sanitizeText(stdout, secrets),
          stderr:
            executable === native
              ? errBytes
                ? '[Native stderr omitted from public live captures]'
                : ''
              : sanitizeText(Buffer.concat(err).toString('utf8'), secrets),
        });
      });
    });
  return {
    contract,
    serverUrl,
    dir,
    native: (args, expired = false) => invoke(native, args, expired),
    wrapper: (args) => invoke(process.execPath, [resolve('bin/teamcity-axi.mjs'), ...args]),
    close: () => rm(dir, { recursive: true, force: true }),
  };
}
export function verifyLiveCapture(records, contract) {
  const body = (name) => {
    const r = records[name];
    return parseRaw({
      stdout: Buffer.from(r.stdout),
      stderr: Buffer.from(r.stderr),
      exitCode: r.code,
      signal: r.signal,
    }).body;
  };
  if (body('server').buildNumber !== contract.server.buildNumber)
    throw Error('Captured server build differs from the test fixture');
  const permissions = body('permissions').permissionAssignment;
  if (
    !Array.isArray(permissions) ||
    JSON.stringify(permissions.map((p) => p.permission?.id).sort()) !==
      JSON.stringify(['change_own_profile', 'view_project', 'view_project']) ||
    permissions.some(
      (p) =>
        p.permission.id === 'view_project' &&
        (p.isGlobalScope || ![contract.fixture.projectId, '_Root'].includes(p.project?.id)),
    )
  )
    throw Error('Live identity does not match the restricted fixture permission inventory');
  for (const [name, id, job, status] of [
    ['run-detail', contract.fixture.failedRunId, contract.fixture.jobId, 'FAILURE'],
    ['green', contract.fixture.greenRunId, contract.fixture.greenJobId, 'SUCCESS'],
  ]) {
    const run = body(name);
    if (
      String(run.id) !== id ||
      run.buildTypeId !== job ||
      run.buildType?.id !== job ||
      run.buildType?.projectId !== contract.fixture.projectId ||
      run.state !== 'finished' ||
      run.status !== status
    )
      throw Error('Captured execution identity or outcome differs from the fixture');
  }
  for (const [name, expected] of [
    ['denied', 'PERMISSION_DENIED'],
    ['missing', 'NOT_FOUND'],
    ['invalid-auth', 'AUTH_REQUIRED'],
    ['bounded-jobs-denied', 'PERMISSION_DENIED'],
  ]) {
    let observed;
    try {
      body(name);
    } catch (error) {
      observed = error.code;
    }
    if (observed !== expected)
      throw Error('Captured error contract differs from the restricted test fixture');
  }
  for (const [name, expectedId] of [
    ['bounded-jobs', contract.fixture.jobId],
    ['bounded-jobs-next', contract.fixture.greenJobId],
  ]) {
    const page = body(name),
      job = page.buildType?.[0];
    if (
      page.count !== 1 ||
      page.buildType.length !== 1 ||
      job?.id !== expectedId ||
      job.projectId !== contract.fixture.projectId ||
      job.paused !== false ||
      typeof page.nextHref !== 'string'
    )
      throw Error('Captured job page identity or scope differs from the fixture');
  }
  const emptyJobs = body('bounded-jobs-empty');
  if (
    emptyJobs.count !== 0 ||
    !Array.isArray(emptyJobs.buildType) ||
    emptyJobs.buildType.length !== 0 ||
    emptyJobs.nextHref !== undefined
  )
    throw Error('Captured empty job page differs from the fixture');
}
export function sanitizeLiveStdout(stdout, secrets) {
  const index = stdout.startsWith('HTTP/1.1 ') ? stdout.indexOf('\n\n') : -1;
  if (stdout.startsWith('HTTP/1.1 ') && index < 0)
    throw Error('Native HTTP capture lacks a complete envelope');
  const headers =
    index < 0
      ? ''
      : sanitizeText(stdout.slice(0, index), secrets).replace(
          /^Set-Cookie:.*$/gim,
          'Set-Cookie: [REDACTED]',
        ) + '\n\n';
  const payload = index < 0 ? stdout : stdout.slice(index + 2);
  if (!payload) return headers;
  let decoded;
  try {
    decoded = JSON.parse(payload);
  } catch {
    return headers + '[Unparseable payload omitted from public live capture]';
  }
  const scrub = (value) => {
    if (typeof value === 'string')
      return value.replace(/;TCSESSIONID=[^/?#\s"\\]+/gi, ';TCSESSIONID=[REDACTED]');
    if (Array.isArray(value)) return value.map(scrub);
    if (value && typeof value === 'object')
      return Object.fromEntries(Object.entries(value).map(([key, v]) => [key, scrub(v)]));
    return value;
  };
  const sanitized = scrub(sanitize(decoded, secrets));
  return (
    headers +
    (JSON.stringify(decoded) === JSON.stringify(sanitized) ? payload : JSON.stringify(sanitized))
  );
}
