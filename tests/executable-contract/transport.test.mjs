import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,readFile,writeFile,rm,stat,mkdir} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {ProcessTransport,resolveBinary,childEnvironment} from '../../dist/transport/process.js';
async function fixture(options = {}) {
  const dir = await mkdtemp(join(tmpdir(),'axi-process-test-'));
  const binary = join(dir,'teamcity');
  await writeFile(binary,`#!${process.execPath}\n`+await readFile('tests/fixtures/fake-teamcity.mjs','utf8'),{mode:0o700});
  const env = {HOME:dir,PATH:process.env.PATH,TEAMCITY_URL:'https://teamcity.example.test/teamcity',TEAMCITY_TOKEN:'transport-canary',TEAMCITY_GUEST:'1',TEAMCITY_HEADER_AUTHORIZATION:'Bearer evil',TEAMCITY_JOB:'Attacker',BUILD_URL:'https://attacker.invalid',NODE_OPTIONS:'--throw-deprecation',UNRELATED:'drop-me'};
  const transport = await ProcessTransport.create({binary,serverUrl:env.TEAMCITY_URL,env,
    limits:{deadline:Date.now()+5000,concurrency:3,maxChildren:8,stdoutBytes:2097152,stderrBytes:65536,...options.limits},...options});
  return {dir,binary,transport,dispose:async()=>{await transport.dispose();await rm(dir,{recursive:true,force:true});}};
}
test('frozen argv, environment and neutral CWD prevent implicit repository behavior', async () => {
  const f = await fixture();
  try {
    const captured = await f.transport.execute({kind:'api',path:'/app/rest/builds?fields=inspect'});
    const value = JSON.parse(captured.stdout.toString());
    assert.deepEqual(value.args,['api','/app/rest/builds?fields=inspect','-X','GET','--include','--raw','-H','Accept: application/json','--no-input']);
    assert.notEqual(value.cwd,process.cwd()); assert.notEqual(value.cwd,f.dir);
    assert.equal(value.tokenPresent,true); assert.equal(value.env.TEAMCITY_RO,'1');
    for (const k of ['TEAMCITY_NO_UPDATE','DO_NOT_TRACK','NO_COLOR']) assert.equal(value.env[k],'1');
    for (const k of ['TEAMCITY_GUEST','TEAMCITY_HEADER_AUTHORIZATION','TEAMCITY_JOB','BUILD_URL','NODE_OPTIONS','UNRELATED']) assert.equal(value.env[k],undefined);
    await f.transport.dispose(); await assert.rejects(stat(value.cwd));
  } finally {await f.dispose();}
});
test('origin-bound tokens, unsafe headers and workspace binaries fail before launch', async () => {
  for (const env of [{TEAMCITY_TOKEN:'canary'}, {TEAMCITY_TOKEN:'canary',TEAMCITY_URL:'https://other.test'}, {TEAMCITY_TOKEN:'canary',TEAMCITY_URL:'https://server.test/different'}]) {
    assert.throws(()=>childEnvironment({env,serverUrl:'https://server.test/teamcity'}),e=>e.code === 'AUTH_CONTEXT_MISMATCH');
  }
  assert.throws(()=>childEnvironment({env:{},serverUrl:'https://server.test',headerNames:['TEAMCITY_HEADER_HOST']}),e=>e.code === 'POLICY_DENIED');
  const f = await fixture();
  try {
    await assert.rejects(resolveBinary(f.binary,f.dir,false),e=>e.code === 'POLICY_DENIED'); assert.equal(await resolveBinary(f.binary,f.dir,true),f.binary);
    await mkdir(join(f.dir,'..hidden'));const hidden=join(f.dir,'..hidden','teamcity');await writeFile(hidden,'untrusted',{mode:0o700});
    await assert.rejects(resolveBinary(hidden,f.dir,false),e=>e.code === 'POLICY_DENIED');
  }
  finally {await f.dispose();}
});
test('unallowlisted, traversing and forged adapter operations never launch children', async () => {
  const f = await fixture();
  try {
    for (const path of ['https://evil.test/app/rest/builds','//evil.test/app/rest/builds','/app/rest/builds/../agents','/app/rest/builds/%2e%2e/agents','/app/rest/builds/%2e%2e/agents?fields=id','/app/rest/builds/id:123/..?fields=id','/app/rest/builds/%ZZ','/app/rest/users','/app/rest/builds?unexpected=1','/app/rest/builds#fragment']) {
      await assert.rejects(f.transport.execute({kind:'api',path}),e=>e.code === 'POLICY_DENIED');
    }
    assert.equal(f.transport.childProcesses,0);
  } finally {await f.dispose();}
});
test('shared child launch and concurrency bounds govern overlapping reads', async () => {
  const f = await fixture({limits:{deadline:Date.now()+5000,concurrency:1,maxChildren:3,stdoutBytes:2097152,stderrBytes:65536}});
  try {
    const start = Date.now();
    const results = await Promise.allSettled(Array.from({length:4},()=>f.transport.execute({kind:'api',path:'/app/rest/builds?fields=wait'})));
    assert.equal(results.filter(r=>r.status === 'fulfilled').length,3);
    assert.equal(f.transport.childProcesses,3);
    assert.ok(Date.now()-start >= 300);
    assert.equal(results.find(r=>r.status === 'rejected').reason.code,'INPUT_LIMIT_EXCEEDED');
  } finally {await f.dispose();}
});
test('oversized stdout and stderr are rejected, nonzero status remains distinct from valid JSON', async () => {
  const f = await fixture();
  try {
    for (const mode of ['huge','huge-stderr']) await assert.rejects(f.transport.execute({kind:'api',path:`/app/rest/builds?fields=${mode}`}),e=>e.code === 'INPUT_LIMIT_EXCEEDED');
    const result = await f.transport.execute({kind:'api',path:'/app/rest/builds?fields=nonzero'});
    assert.equal(result.exitCode,17); assert.deepEqual(JSON.parse(result.stdout),{valid:true});
  } finally {await f.dispose();}
});
test('deadline cleans up a process group including a descendant ignoring SIGTERM', async () => {
  const dir = await mkdtemp(join(tmpdir(),'axi-pid-test-'));
  const pidFile = join(dir,'pid');
  const f = await fixture({env:{HOME:dir,PATH:process.env.PATH,TEAMCITY_HEADER_X_FIXTURE_PID_PATH:pidFile},headerNames:['TEAMCITY_HEADER_X_FIXTURE_PID_PATH'],limits:{deadline:Date.now()+500,concurrency:1,maxChildren:1,stdoutBytes:2097152,stderrBytes:65536}});
  try {
    await assert.rejects(f.transport.execute({kind:'api',path:'/app/rest/builds?fields=grandchild'}),e=>e.code === 'DEADLINE_EXCEEDED');
    const pid = Number(await readFile(pidFile,'utf8'));
    // An exited Linux descendant can briefly remain an init-owned zombie; it cannot run.
    let alive = true;
    for (let i=0;i<30;i++) {
      try {
        if(process.platform==='linux') {const status=await readFile(`/proc/${pid}/stat`,'utf8'); alive = status.split(' ')[2] !== 'Z';}
        else {process.kill(pid,0);alive=true;}
      } catch {alive=false;}
      if (!alive) break; await new Promise(resolve=>setTimeout(resolve,20));
    }
    assert.equal(alive,false);
  } finally {await f.dispose();await rm(dir,{recursive:true,force:true});}
});
test('abort interrupts active and waiting reads without extra children', async () => {
  const controller = new AbortController();
  const f = await fixture({signal:controller.signal,limits:{deadline:Date.now()+5000,concurrency:1,maxChildren:5,stdoutBytes:2097152,stderrBytes:65536}});
  try {
    const pending = Promise.allSettled([f.transport.execute({kind:'api',path:'/app/rest/builds?fields=hang'}),f.transport.execute({kind:'api',path:'/app/rest/builds?fields=hang'})]);
    setTimeout(()=>controller.abort(),100);
    const results = await pending;
    assert.equal(f.transport.childProcesses,1); assert.ok(results.every(r=>r.status === 'rejected' && r.reason.code === 'INTERRUPTED'));
  } finally {await f.dispose();}
});
