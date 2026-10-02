import {test} from 'node:test';
import assert from 'node:assert/strict';
import {captureNative} from '../../scripts/native-harness.mjs';
import {operations} from '../fixtures/native-operations.mjs';
test('released CLI transport contract preserves context path and independent errors', async () => {
  const result = await captureNative(process.env.TEAMCITY_AXI_TEST_BINARY);
  assert.equal(result.probes.version.stdout,'teamcity version 1.5.0\n');
  assert.equal(result.probes.version.requests.length,0);
  assert.equal(result.probes.help.requests.length,0);
  for (const [name,entry] of Object.entries(result.records)) {
    assert.ok(entry.requests.every(r=>r.method === 'GET' && r.path.startsWith('/teamcity/') && r.authenticated),name);
    assert.ok(!entry.stdout.includes('fixture-only-token'),name);
    assert.ok(!entry.stderr.includes('fixture-only-token'),name);
  }
  for (const name of ['run-view','run-list','problems','tests','dependencies','changes','jobs','queue','agents']) {
    assert.equal(result.records[name].code,0,name);
    assert.match(result.records[name].stdout,/^HTTP\/1\.1 200 OK\n/);
    const input = new URL(operations.find(([n])=>n === name)[1][1],'http://fixture.invalid');
    assert.equal(result.records[name].requests.length,1,name);
    assert.deepEqual(result.records[name].requests[0],{method:'GET',path:'/teamcity'+input.pathname,query:Object.fromEntries(input.searchParams),authenticated:true},name);
  }
  for (const [mode,status] of [['denied',403],['missing',404],['expired',401]]) {
    assert.notEqual(result.records[`error-${mode}`].code,0);
    assert.match(result.records[`error-${mode}`].stdout,new RegExp(`^HTTP/1.1 ${status} `));
  }
  const omitted = result.records['summary-denied'];
  assert.equal(omitted.code,0);
  assert.equal(JSON.parse(omitted.stdout).failed_tests,undefined);
  assert.ok(omitted.requests.some(r=>r.path.endsWith('/testOccurrences')));
  assert.notEqual(result.records['logs-unsupported'].code,0);
  assert.ok(result.records['logs-unsupported'].requests.every(r=>!r.path.includes('downloadBuildLog')));
});
