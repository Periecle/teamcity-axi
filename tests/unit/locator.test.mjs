import { test } from 'node:test';
import assert from 'node:assert/strict';
import { literal, idCondition, branchCondition, apiPath } from '../../dist/adapter/locator.js';
test('untrusted names cannot introduce additional locator dimensions', () => {
  for (const value of [
    'feature/refund',
    'a,b',
    'branch:(default:any)',
    'a:b',
    '$base64',
    '()',
    'space branch',
    '日🦊',
    '@this',
    '-',
  ]) {
    const encoded = literal(value);
    assert.match(encoded, /^\(\$base64:[A-Za-z0-9_-]+\)$/);
    assert.equal(Buffer.from(encoded.slice(9, -1), 'base64url').toString('utf8'), value);
    assert.equal(idCondition(value), `(id:${encoded})`);
    assert.equal(branchCondition(value), `(name:(value:${encoded}))`);
    const path = apiPath(
      'builds',
      [`buildType:${idCondition(value)}`, `branch:${branchCondition(value)}`, 'count:20'],
      'count,nextHref,build(id)',
    );
    const query = new URL(path, 'https://adapter.invalid').searchParams;
    assert.equal(query.size, 2);
    assert.equal(
      query.get('locator'),
      `buildType:(id:${encoded}),branch:(name:(value:${encoded})),count:20`,
    );
  }
  for (const bad of ['', '\n', 'a'.repeat(4097), '\ud800', '\udfff'])
    assert.throws(
      () => literal(bad),
      (e) => e.code === 'USAGE_ERROR',
    );
});
