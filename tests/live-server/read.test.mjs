import {test} from 'node:test';
import assert from 'node:assert/strict';
import {decode} from '@toon-format/toon';
import {liveFixture} from '../../scripts/live-harness.mjs';
import {parseRaw} from '../../dist/adapter/raw.js';
import {validateResponse} from '../../dist/output/schema.js';
const apiBody=r=>parseRaw({stdout:Buffer.from(r.stdout),stderr:Buffer.from(r.stderr),exitCode:r.code,signal:r.signal}).body;
test('pinned CLI confirms the live server and restricted permission inventory with bounded reads',async()=>{
  const f=await liveFixture();try {
    const version=await f.native(['--version']);assert.equal(version.stdout,'teamcity version 1.5.0\n');assert.equal(version.code,0);
    const server=apiBody(await f.native(f.contract.records.server.args));assert.equal(server.buildNumber,f.contract.server.buildNumber);
    const permissions=apiBody(await f.native(f.contract.records.permissions.args)).permissionAssignment;
    assert.deepEqual(permissions.map(p=>p.permission.id).sort(),['change_own_profile','view_project','view_project']);
    assert.ok(permissions.filter(p=>p.permission.id==='view_project').every(p=>!p.isGlobalScope&&[f.contract.fixture.projectId,'_Root'].includes(p.project.id)));
    for(const name of ['problems','tests','dependencies','changes','queue','jobs','agents','pages','encoded-page','empty-page'])apiBody(await f.native(f.contract.records[name].args));
    const unsupported=await f.native(f.contract.records['dependencies-unsupported'].args);assert.equal(unsupported.code,1);assert.throws(()=>apiBody(unsupported),e=>e.code==='UPSTREAM_FAILURE');
    const log=await f.native(f.contract.records.log.args);assert.equal(log.code,0);assert.equal(JSON.parse(log.stdout).run_id,f.contract.fixture.failedRunId);
  }finally{await f.close();}
});
test('actual wrapper returns exact failed observation and preserves denied, missing and mismatch errors',async()=>{
  const f=await liveFixture();try {
    for(const format of ['json','toon']){
      const r=await f.wrapper(['run','view',f.contract.fixture.failedRunId,'--format',format]);assert.equal(r.code,0);assert.equal(r.stderr,'');
      const v=format==='json'?JSON.parse(r.stdout):decode(r.stdout);validateResponse(v);assert.equal(v.status,'ok');assert.equal(v.data.run.id,f.contract.fixture.failedRunId);assert.equal(v.data.run.result,'failure');assert.equal(v.context.server,'sandbox');
    }
    for(const [id,flags,code] of [[f.contract.fixture.deniedRunId,[],'PERMISSION_DENIED'],[f.contract.fixture.missingRunId,[],'NOT_FOUND'],[f.contract.fixture.failedRunId,['--job','AnotherJob'],'CONTEXT_MISMATCH']]){
      const r=await f.wrapper(['run','view',id,'--json',...flags]);const v=JSON.parse(r.stdout);assert.equal(r.code,1);assert.equal(v.error.code,code);assert.equal(v.data,undefined);validateResponse(v);
    }
    const expired=await f.native(f.contract.records['invalid-auth'].args,true);assert.throws(()=>apiBody(expired),e=>e.code==='AUTH_REQUIRED');
  }finally{await f.close();}
});
