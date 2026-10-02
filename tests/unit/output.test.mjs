import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { decode } from '@toon-format/toon';
import { response } from '../../dist/domain/response.js';
import { render } from '../../dist/output/render.js';
import { sanitizeText, knownSecrets, secretMatchers } from '../../dist/output/sanitize.js';
import { validateResponse, validateConfig } from '../../dist/output/schema.js';
// Serializer-only fixtures use the local schema command's generic envelope;
// domain payload examples separately validate their actual command contracts.
test('explicitly forwarded header values are redacted longest first regardless of environment name', () => {
  const secrets = knownSecrets(
    { APP_TOKEN: 'short-canary', TEAMCITY_HEADER_X_CUSTOM: 'prefix-short-canary-suffix' },
    [],
    ['TEAMCITY_HEADER_X_CUSTOM'],
  );
  assert.equal(sanitizeText('prefix-short-canary-suffix', secrets), '[REDACTED]');
  assert.equal(
    render(response('schema', { branch: 'prefix-short-canary-suffix' }), 'json', 16384, secrets)
      .response.data.branch,
    '[REDACTED]',
  );
});
test('all supplied response examples validate and JSON/TOON preserve logical values', () => {
  for (const file of readdirSync('examples')) {
    const value = JSON.parse(readFileSync(`examples/${file}`, 'utf8'));
    if (file === 'user-config.json' || file === 'repository-config.json') {
      validateConfig(file.replace('.json', ''), value);
      continue;
    }
    validateResponse(value);
    assert.deepEqual(
      decode(render(value, 'toon', 262144).document),
      JSON.parse(render(value, 'json', 262144).document),
    );
  }
  const value = response('schema', {
    strings: ['123', '001', 'true', 'null'],
    unicode: 'ą日本語🦊',
    nested: [
      { x: null, n: 1 },
      { x: '1', n: 0 },
    ],
  });
  assert.deepEqual(decode(render(value, 'toon', 16384).document), value);
  assert.equal(render(value, 'json', 16384, ['1.0']).response.schemaVersion, '1.0');
  assert.ok(render(value, 'json', 16384, ['meta']).response.meta);
});
test('trusted safe name patterns redact environment values and untrusted fields', () => {
  const secrets = knownSecrets({ COMPANY_CREDENTIAL: 'pattern-canary' }, ['^COMPANY_CREDENTIAL$']);
  assert.deepEqual(secrets, ['pattern-canary']);
  const value = response('schema', {
    branch: 'pattern-canary',
    message: 'protected-name-canary',
    COMPANY_CREDENTIAL: 'unknown-private-value',
  });
  const document = render(value, 'json', 2048, secrets, [
    '^COMPANY_CREDENTIAL$',
    '^message$',
  ]).document;
  assert.ok(!document.includes('pattern-canary'));
  assert.ok(!document.includes('unknown-private-value'));
  assert.ok(!document.includes('protected-name-canary'));
  assert.throws(() => secretMatchers(['(a+)+$']));
  assert.throws(() => secretMatchers(['.*.*.*']));
});
test('known secrets and terminal controls are removed before rendering in both formats', () => {
  const value = response('schema', {
    text: '\x1b[31mcanary-secret\x1b[0m\u202eevil',
    password: 'unknown',
    message: 'Bearer abc123',
  });
  for (const format of ['json', 'toon']) {
    const { document } = render(value, format, 2048, ['canary-secret']);
    assert.ok(!document.includes('canary-secret'));
    assert.ok(!document.includes('abc123'));
    assert.ok(!document.includes('\x1b'));
    assert.ok(!document.includes('\u202e'));
  }
  assert.equal(sanitizeText('\x1b]0;injected title\x07safe', []), 'safe');
  assert.equal(sanitizeText('\u202e', []), '\\u202e');
  assert.equal(
    sanitizeText('Authorization: Basic YWRtaW46cGFzc3dvcmQ=', []),
    'Authorization: Basic [REDACTED]',
  );
  assert.equal(
    sanitizeText(
      '-----BEGIN RSA PRIVATE KEY-----\nprivate body\n-----END RSA PRIVATE KEY-----',
      [],
    ),
    '[REDACTED PRIVATE KEY]',
  );
});
test('oversized evidence produces one valid bounded error instead of lying about retained evidence', () => {
  const value = response('run.view', {
    run: {
      id: '1',
      jobId: 'Build',
      state: 'finished',
      result: 'failure',
      statusText: '🦊'.repeat(100000),
    },
  });
  for (const format of ['json', 'toon']) {
    const result = render(value, format, 2048);
    assert.ok(Buffer.byteLength(result.document) <= 2048);
    assert.equal(result.response.status, 'error');
    assert.equal(result.response.meta.complete, false);
    assert.equal(result.response.meta.truncated, true);
    assert.equal(result.response.error.details.limit, 'maxBytes');
    assert.equal(result.response.error.details.ceiling, 2048);
    assert.ok(result.response.error.details.observed > 2048);
    validateResponse(result.response);
  }
  const scoped = response('run.view', {
    run: { id: '1', jobId: 'Build', state: 'finished', result: 'failure', statusText: 'important' },
  });
  scoped.context = { server: 'work', job: 'Payments_Build', branch: 'b'.repeat(5000) };
  const bounded = render(scoped, 'json', 2048).response;
  assert.equal(bounded.context.server, 'work');
  assert.equal(bounded.context.job, 'Payments_Build');
  scoped.context = {
    server: 'a'.repeat(64),
    job: '日'.repeat(256),
    project: '日'.repeat(256),
    branch: 'b'.repeat(5000),
  };
  for (const format of ['json', 'toon']) {
    const r = render(scoped, format, 2048);
    assert.ok(Buffer.byteLength(r.document) <= 2048);
    assert.equal(r.response.context.job, scoped.context.job);
    assert.equal(r.response.context.project, scoped.context.project);
    validateResponse(r.response);
  }
  scoped.context.vcsRootId = '日'.repeat(256);
  for (const format of ['json', 'toon']) {
    const r = render(scoped, format, 2048);
    assert.ok(Buffer.byteLength(r.document) <= 2048);
    assert.equal(r.response.context.server, scoped.context.server);
    assert.equal(r.response.status, 'error');
    validateResponse(r.response);
  }
});

test('large pages and graphs retain unknown outcomes without exceeding the diagnostic schema ceiling', () => {
  for (const count of [100, 200]) {
    const value = response('schema', {
      runs: Array.from({ length: count }, (_, index) => ({
        id: String(index + 1),
        result: 'unknown',
      })),
    });
    value.status = 'partial';
    value.meta.complete = false;
    value.meta.limitations = Array.from({ length: count }, (_, index) => ({
      code: 'OUTCOME_METADATA_UNAVAILABLE',
      source: 'run',
      runId: String(index + 1),
      message: 'Explicit execution outcome metadata is unavailable',
    }));
    value.meta.limitations.push({
      code: 'SCAN_COVERAGE_UNKNOWN',
      source: 'run',
      message: 'Exhaustion is unverified',
    });
    for (const format of ['json', 'toon']) {
      const output = render(value, format, 65536).response;
      validateResponse(output);
      assert.equal(output.status, 'partial');
      assert.equal(output.meta.complete, false);
      assert.equal(output.data.runs.length, count);
      assert.ok(output.data.runs.every((run) => run.result === 'unknown'));
      assert.equal(output.meta.limitations.length, 2);
      assert.equal(output.meta.limitations[0].runId, undefined);
      assert.ok(
        output.meta.limitations[0].message.includes(`${count} distinct executions affected`),
      );
      assert.ok(output.meta.limitations.some((note) => note.code === 'SCAN_COVERAGE_UNKNOWN'));
    }
  }
});

test('renderer introduces actual limit metadata only when byte reduction begins, including removed hints', () => {
  const value = response('run.view', {
    run: { id: '1', jobId: 'Build', state: 'finished', result: 'failure' },
  });
  const limits = {
    maxBytes: 4096,
    maxChildProcesses: 4,
    concurrency: 1,
    deadline: Date.now() + 10000,
    stdoutCaptureBytes: 2097152,
    stderrCaptureBytes: 65536,
  };
  for (const format of ['json', 'toon']) {
    const clean = render(value, format, 2048, [], [], limits);
    assert.equal(clean.response.meta.limits, undefined);
    assert.deepEqual(clean.response, value);
    const hints = {
      ...value,
      next: [
        {
          reason: 'Optional read',
          argv: [
            'teamcity-axi',
            'run',
            'list',
            '--job',
            'Build',
            '--literal-branch',
            'x'.repeat(3000),
          ],
        },
      ],
    };
    const reduced = render(hints, format, 2048, [], [], limits);
    assert.equal(reduced.response.status, 'ok');
    assert.equal(reduced.response.next, undefined);
    assert.deepEqual(reduced.response.meta.limits, { ...limits, maxBytes: 2048 });
    assert.ok(Buffer.byteLength(reduced.document) <= 2048);
    const oversized = response('run.view', {
      run: {
        id: '1',
        jobId: 'Build',
        state: 'finished',
        result: 'failure',
        statusText: 'x'.repeat(100000),
      },
    });
    const error = render(oversized, format, 2048, [], [], limits);
    assert.equal(error.response.status, 'error');
    assert.deepEqual(error.response.meta.limits, { ...limits, maxBytes: 2048 });
    assert.equal(error.response.error.details.ceiling, 2048);
    assert.ok(Buffer.byteLength(error.document) <= 2048);
  }
});
