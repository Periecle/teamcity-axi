import { test } from 'node:test';
import assert from 'node:assert/strict';
import { decode } from '@toon-format/toon';

import { parseRaw } from '../../dist/adapter/raw.js';

import { liveFixture } from '../../scripts/live-harness.mjs';
import { validateResponse } from '../../dist/output/schema.js';

test('restricted live run-list predicates preserve exclusive fractional windows despite coarse DTO dates', async () => {
  const f = await liveFixture();
  try {
    const p = f.contract.fixture.listPrecision;
    for (const [since, until, expected] of [
      [p.lowerExclusive, p.upperExclusive, [p.runId]],
      [p.upperExclusive, p.afterExact, []],
      [p.lowerExclusive, p.exactFinish, []],
    ]) {
      for (const flags of [['--json'], []]) {
        const wire = await f.wrapper([
          'run',
          'list',
          '--job',
          p.jobId,
          '--all-branches',
          '--since',
          since,
          '--until',
          until,
          '--fields',
          'finishedAt',
          ...flags,
        ]);
        assert.equal(wire.code, 0);
        const value = flags.length ? JSON.parse(wire.stdout) : decode(wire.stdout);
        validateResponse(value);
        assert.deepEqual(
          value.data.runs.map((run) => run.id),
          expected,
        );
        assert.equal(value.data.selection.window.since, since);
        assert.equal(value.data.selection.window.until, until);
        assert.equal(value.data.selection.window.bounds, 'exclusive');
        assert.equal(value.data.selection.timestampMembership, 'provider');
        assert.equal(value.data.selection.reportedTimestampPrecision, 'second');
        assert.equal(value.data.selection.filterTimestampPrecision, 'millisecond');
        if (expected.length) assert.equal(value.data.runs[0].finishedAt, p.finish);
        assert.equal(value.data.page.total, expected.length);
        assert.equal(value.data.page.hasMore, false);
      }
    }
  } finally {
    await f.close();
  }
});

test('restricted live unknown-result filtering includes queued evidence and preserves finished candidate counts', async () => {
  const f = await liveFixture();
  try {
    for (const flags of [['--json'], []]) {
      const queuedWire = await f.wrapper([
        'run',
        'list',
        '--job',
        f.contract.fixture.queueJobIds[0],
        '--all-branches',
        '--state',
        'queued',
        '--result',
        'unknown',
        ...flags,
      ]);
      assert.equal(queuedWire.code, 0);
      const queued = flags.length ? JSON.parse(queuedWire.stdout) : decode(queuedWire.stdout);
      validateResponse(queued);
      assert.deepEqual(
        queued.data.runs.map((run) => run.id),
        [f.contract.fixture.queuedRunIds[0]],
      );
      assert.equal(queued.data.runs[0].state, 'queued');
      assert.equal(queued.data.runs[0].result, 'unknown');
      assert.equal(queued.data.selection.timestampBasis, null);
      assert.equal(queued.data.selection.window, undefined);
      assert.equal(queued.data.selection.resultBasis, 'normalized_candidates');
      assert.equal(queued.data.aggregates.unknown, 1);
      const p = f.contract.fixture.listPrecision;
      const finishedWire = await f.wrapper([
        'run',
        'list',
        '--job',
        p.jobId,
        '--all-branches',
        '--result',
        'unknown',
        '--since',
        p.lowerExclusive,
        '--until',
        p.upperExclusive,
        ...flags,
      ]);
      assert.equal(finishedWire.code, 0);
      const finished = flags.length ? JSON.parse(finishedWire.stdout) : decode(finishedWire.stdout);
      validateResponse(finished);
      assert.deepEqual(finished.data.runs, []);
      assert.equal(finished.data.selection.providerReturned, 1);
      assert.equal(finished.data.page.total, 0);
      assert.equal(finished.data.page.hasMore, false);
      assert.equal(finished.status, 'ok');
    }
  } finally {
    await f.close();
  }
});

test('restricted live finished-empty scope returns exact zero while lookup-capped searches keep continuation', async () => {
  const f = await liveFixture();
  try {
    for (const name of ['exhaustion-cap-empty', 'exhaustion-cap-positive']) {
      const wire = await f.native(f.contract.records[name].args);
      assert.equal(wire.code, 0);
      const body = parseRaw({
        stdout: Buffer.from(wire.stdout),
        stderr: Buffer.from(wire.stderr),
        exitCode: wire.code,
        signal: wire.signal,
      }).body;
      assert.equal(body.count, name.endsWith('empty') ? 0 : 1);
      assert.ok(body.nextHref.includes('lookupLimit:2'));
    }
    const p = f.contract.fixture.exhaustion;
    for (const flags of [['--json'], []]) {
      const wire = await f.wrapper([
        'run',
        'list',
        '--job',
        p.emptyFinishedJobId,
        '--all-branches',
        '--since',
        p.since,
        '--until',
        p.until,
        '--require-complete',
        ...flags,
      ]);
      assert.equal(wire.code, 0);
      assert.equal(wire.stderr, '');
      const value = flags.length ? JSON.parse(wire.stdout) : decode(wire.stdout);
      validateResponse(value);
      assert.equal(value.status, 'ok');
      assert.equal(value.meta.complete, true);
      assert.deepEqual(value.data.runs, []);
      assert.deepEqual(value.data.page, {
        returned: 0,
        total: 0,
        totalKind: 'exact',
        hasMore: false,
        cursor: null,
      });
      assert.equal(value.context.job, p.emptyFinishedJobId);
      assert.equal(value.context.project, f.contract.fixture.projectId);
      assert.equal(value.data.selection.exhaustionBasis, 'verified_server_pagination');
      assert.equal(value.data.selection.consistency, 'best_effort_offset');
      assert.equal(value.data.selection.window.since, p.since);
      assert.equal(value.data.selection.window.until, p.until);
      assert.equal(value.meta.counts.childProcesses, 4);
      assert.equal(value.next, undefined);
    }
  } finally {
    await f.close();
  }
});
