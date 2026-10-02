import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parse } from '../../dist/cli/parser.js';
const bad = (args) =>
  assert.throws(
    () => parse(args),
    (e) => e.code === 'USAGE_ERROR' && e.exitCode === 2,
  );
test('strict parsing rejects typos, invalid projections, duplicates, surplus and writes', () => {
  for (const args of [
    ['run', 'list', '--stat', 'failure'],
    ['run', 'list', '--limit', '1', '--limit', '2'],
    ['run', 'view', '1', 'extra'],
    ['run', 'start', '1'],
    ['api', '/app/rest/builds'],
    ['run', 'view', '9007199254740993'],
    ['run', 'view', '0'],
    ['run', 'list', '--fields', 'parameters'],
    ['run', 'list', '--fields', 'id,id'],
    ['run', 'log', '1', '--limit', '1'],
    ['run', 'list', '--limit', '1.5'],
    ['run', 'list', '--limit', '101'],
    ['run', 'view', '1', '--timeout', '1'],
    ['--json', '--format', 'toon'],
    ['run', 'list', '--branch', 'main', '--all-branches'],
    ['run', 'tests', '1', '--test', 'o1', '--failed'],
    ['run', 'tests', '1', '--include-muted'],
    ['run', 'log', '1', '--failed', '--tail', '80'],
    ['run', 'list', '--since', 'bad'],
    ['run', 'list', '--since', '2026-10-01T00:00:00Z', '--state', 'running'],
    ['run', 'list', '--since', '2026-02-30T00:00:00Z'],
  ])
    bad(args);
});
test('global flags are accepted before or after commands, IDs remain strings', () => {
  const first = parse(['--server', 'work', 'run', 'view', '482193', '--json']);
  const last = parse(['run', 'view', '482193', '--server', 'work', '--json']);
  assert.deepEqual(first, last);
  assert.equal(first.positional, '482193');
  assert.equal(parse(['run', 'tree', '1', '--depth', '0']).flags.depth, 0);
  assert.equal(parse(['run', 'watch', '1', '--interval', '5s']).flags.interval, 5000);
  assert.equal(parse(['run', 'view', '--help']).flags.help, true);
  assert.equal(parse(['--json', '--format', 'json']).format, 'json');
});

test('explicit inline string values preserve leading option-like literal text', () => {
  assert.equal(
    parse(['run', 'log', '1', '--contains=--error=Connection refused']).flags.contains,
    '--error=Connection refused',
  );
  assert.throws(
    () => parse(['run', 'log', '1', '--contains', '--full']),
    (error) => error.code === 'USAGE_ERROR',
  );
});
