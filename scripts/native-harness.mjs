import { spawn } from 'node:child_process';
import { mkdtemp, rm, readFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createHash } from 'node:crypto';
import { mockServer } from '../tests/fixtures/mock-server.mjs';
import { operations, errorModes } from '../tests/fixtures/native-operations.mjs';
export async function captureNative(binary) {
  if (!binary)
    throw new Error('Set TEAMCITY_AXI_TEST_BINARY to the checksum-verified v1.5.0 binary');
  const path = resolve(binary);
  const sha256 = createHash('sha256')
    .update(await readFile(path))
    .digest('hex');
  const manifest = JSON.parse(
    await readFile(new URL('../docs/compatibility.json', import.meta.url), 'utf8'),
  );
  const artifact = manifest.artifacts.find((a) =>
    a.archive.includes(
      `${process.platform === 'darwin' ? 'darwin' : 'linux'}_${process.arch === 'x64' ? 'x86_64' : 'arm64'}.tar.gz`,
    ),
  );
  if (!artifact || sha256 !== artifact.binarySha256)
    throw new Error(
      'Native executable checksum does not match the pinned release for this platform',
    );
  const dir = await mkdtemp(join(tmpdir(), 'teamcity-axi-native-'));
  const server = await mockServer();
  const env = {
    PATH: process.env.PATH,
    HOME: dir,
    XDG_CONFIG_HOME: dir,
    TEAMCITY_URL: server.base,
    TEAMCITY_TOKEN: 'fixture-only-token',
    TEAMCITY_RO: '1',
    TEAMCITY_NO_UPDATE: '1',
    DO_NOT_TRACK: '1',
    NO_COLOR: '1',
    TERM: 'dumb',
  };
  const call = (argv) =>
    new Promise((resolve, reject) => {
      const child = spawn(path, argv, {
        env,
        cwd: dir,
        stdio: ['ignore', 'pipe', 'pipe'],
        timeout: 10000,
      });
      const stdout = [],
        stderr = [];
      child.stdout.on('data', (d) => stdout.push(d));
      child.stderr.on('data', (d) => stderr.push(d));
      child.on('error', reject);
      child.on('close', (code, signal) =>
        resolve({
          argv,
          code,
          signal,
          stdout: Buffer.concat(stdout).toString(),
          stderr: Buffer.concat(stderr).toString(),
          requests: server.requests.splice(0),
        }),
      );
    });
  try {
    const probes = { version: await call(['--version']), help: await call(['api', '--help']) };
    if (
      probes.version.code !== 0 ||
      probes.version.stdout !== `teamcity version ${manifest.nativeVersion}\n`
    )
      throw new Error('Native executable version does not match the compatibility manifest');
    const records = {};
    for (const [name, argv] of operations) records[name] = await call(argv);
    for (const mode of errorModes) {
      server.setMode(mode);
      records[`error-${mode}`] = await call(operations[1][1]);
    }
    server.setMode('summary-denied');
    records['summary-denied'] = await call(operations.at(-1)[1]);
    server.setMode('logs-unsupported');
    records['logs-unsupported'] = await call(operations.find(([n]) => n === 'log-tail')[1]);
    return JSON.parse(
      JSON.stringify({
        nativeVersion: manifest.nativeVersion,
        binarySha256: sha256,
        sourceRevision: manifest.sourceRevision,
        serverKind: 'synthetic-mock-not-live-TeamCity',
        platform: process.platform,
        architecture: process.arch,
        probes,
        records,
      })
        .replaceAll(server.base, 'http://127.0.0.1:PORT/teamcity')
        .replaceAll(dir, '<isolated-config>'),
    );
  } finally {
    await server.close();
    await rm(dir, { recursive: true, force: true });
  }
}
