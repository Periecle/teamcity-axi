import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {mkdtemp,mkdir,writeFile,rm,readFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {createHash} from 'node:crypto';
import {decode} from '@toon-format/toon';
import {mockServer,longCanary} from '../fixtures/mock-server.mjs';
import {parse} from '../../dist/cli/parser.js';
import {validateResponse} from '../../dist/output/schema.js';
async function fixture() {
  const binary=process.env.TEAMCITY_AXI_TEST_BINARY;
  if(!binary)throw Error('TEAMCITY_AXI_TEST_BINARY is required; this suite never skips');
  const manifest=JSON.parse(await readFile('docs/compatibility.json','utf8'));
  const sha=createHash('sha256').update(await readFile(binary)).digest('hex');
  assert.ok(manifest.artifacts.some(a=>a.binarySha256===sha),'The native binary must be checksum verified');
  const server=await mockServer();const dir=await mkdtemp(join(tmpdir(),'axi-view-'));
  await mkdir(join(dir,'teamcity-axi'));
  const config={schemaVersion:'1.0',readOnly:true,defaultServer:'work',binaryPath:resolve(binary),servers:{work:{url:server.base,allowHttpLoopback:true,allowedProjects:['Payments']}}};
  await writeFile(join(dir,'teamcity-axi','config.json'),JSON.stringify(config),{mode:0o600});
  const call=(args=[],signal=null)=>new Promise((resolve,reject)=>{
    const child=spawn(process.execPath,[resolveBin,'run','view','482193',...args],{env:{HOME:dir,XDG_CONFIG_HOME:dir,PATH:process.env.PATH,TEAMCITY_URL:server.base,TEAMCITY_TOKEN:'fixture-only-token',APP_TOKEN:longCanary},cwd:dir,stdio:['ignore','pipe','pipe']});
    const out=[],err=[];child.stdout.on('data',v=>out.push(v));child.stderr.on('data',v=>err.push(v));child.on('error',reject);
    if(signal)setTimeout(()=>child.kill(signal),500);
    child.on('close',(code)=>{const stdout=Buffer.concat(out).toString(),stderr=Buffer.concat(err).toString();resolve({code,stdout,stderr,value:args.includes('--json')?JSON.parse(stdout):decode(stdout)});});
  });
  return {server,config,dir,call,close:async()=>{await server.close();await rm(dir,{recursive:true,force:true});}};
}
const resolveBin=resolve('bin/teamcity-axi.mjs');
test('actual wrapper and released CLI observe an exact failed run successfully in both formats',async()=>{
  const f=await fixture();try {
    for(const args of [['--json'],[]]) {
      const r=await f.call(args);assert.equal(r.code,0);assert.equal(r.stderr,'');validateResponse(r.value);
      assert.equal(r.value.status,'ok');assert.equal(r.value.data.run.id,'482193');assert.equal(r.value.data.run.result,'failure');assert.equal(r.value.context.server,'work');assert.equal(r.value.context.job,'Payments_Build');assert.equal(r.value.meta.counts.childProcesses,2);
      assert.ok(!r.stdout.includes('fixture-only-token'));assert.ok(f.server.requests.every(q=>q.method==='GET'&&q.authenticated));
    }
    const projected=await f.call(['--fields','number','--json']);assert.deepEqual(Object.keys(projected.value.data.run).sort(),['id','jobId','number','result','state']);
    const mismatch=await f.call(['--job','Other','--json']);assert.equal(mismatch.code,1);assert.equal(mismatch.value.error.code,'CONTEXT_MISMATCH');assert.equal(mismatch.value.context.server,'work');
  }finally{await f.close();}
});
test('permission, authentication, wrong identity, malformed and oversized reads never become empty success',async()=>{
  const f=await fixture();try {
    for(const [mode,code] of [['denied','PERMISSION_DENIED'],['expired','AUTH_REQUIRED'],['missing','NOT_FOUND'],['malformed','UPSTREAM_SCHEMA_MISMATCH'],['html','AUTH_REQUIRED'],['wrong-id','CONTEXT_MISMATCH'],['invalid-identity','UPSTREAM_SCHEMA_MISMATCH'],['huge','INPUT_LIMIT_EXCEEDED']]) {
      f.server.setMode(mode);const r=await f.call(['--json']);assert.equal(r.code,1,mode);assert.equal(r.stderr,'',mode);assert.equal(r.value.status,'error',mode);assert.equal(r.value.error.code,code,mode);assert.equal(r.value.data,undefined,mode);assert.equal(r.value.context.server,'work',mode);validateResponse(r.value);
    }
    f.server.setMode('unknown-enum');const unknown=await f.call(['--json']);assert.equal(unknown.code,0);assert.equal(unknown.value.data.run.result,'unknown');assert.equal(unknown.value.data.run.state,'unknown');
  }finally{await f.close();}
});
test('preview expansion is bounded and next actions parse; SIGINT/deadline remain read-only',async()=>{
  const f=await fixture();try {
    f.server.setMode('huge-text');const preview=await f.call(['--json','--job','Payments_Build','--project','Payments']);assert.equal(preview.code,0);assert.equal(preview.value.meta.truncated,true);assert.equal(Array.from(preview.value.data.run.statusText).length,1200);
    for(const action of preview.value.next)parse(action.argv.slice(1));
    assert.ok(preview.value.next[0].argv.includes('--job'));assert.ok(preview.value.next[0].argv.includes('--project'));
    const full=await f.call(['--full','--max-bytes','2048','--json']);assert.equal(full.code,1);assert.ok(Buffer.byteLength(full.stdout)<=2048);assert.equal(full.value.context.server,'work');
    f.server.setMode('hang');const timeout=await f.call(['--timeout','200ms','--json']);assert.equal(timeout.code,1);assert.equal(timeout.value.error.code,'DEADLINE_EXCEEDED');
    const interrupted=await f.call(['--json'],'SIGINT');assert.equal(interrupted.code,130);assert.equal(interrupted.value.error.code,'INTERRUPTED');assert.equal(interrupted.stderr,'');
    assert.ok(f.server.requests.every(q=>q.method==='GET'));
  }finally{await f.close();}
});
test('released CLI output redacts long credentials before previews and marks omitted revisions partial',async()=>{
  const f=await fixture();try {
    f.server.setMode('long-secret');const secret=await f.call(['--json']);assert.equal(secret.code,0);assert.equal(secret.value.data.run.rawStatus,'[REDACTED]');assert.equal(secret.value.data.run.statusText,'[REDACTED]');assert.ok(!secret.stdout.includes(longCanary.slice(0,80)));
    f.server.setMode('decorated-secret');const decorated=await f.call(['--json']);assert.equal(decorated.value.data.run.rawStatus,'[REDACTED]');assert.equal(decorated.value.data.run.statusText,'[REDACTED]');assert.ok(!decorated.stdout.includes(longCanary.slice(0,80)));
    f.server.setMode('missing-revisions');const missing=await f.call(['--json']);assert.equal(missing.code,0);assert.equal(missing.value.status,'partial');assert.equal(missing.value.meta.complete,false);assert.equal(missing.value.data.run.revisions,undefined);
    const strict=await f.call(['--json','--require-complete']);assert.equal(strict.code,1);assert.equal(strict.value.status,'partial');
  }finally{await f.close();}
});
