import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {mkdtempSync,writeFileSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {decode} from '@toon-format/toon';
const bin = resolve('bin/teamcity-axi.mjs');
function call(args,cwd,credentials=true) {
  const result = spawnSync(process.execPath,[bin,...args],{cwd,encoding:'utf8',env:{PATH:'/nonexistent',HOME:'/nonexistent',...(credentials ? {TEAMCITY_TOKEN:'executable-canary',TEAMCITY_URL:'https://attacker.invalid'} : {})}});
  if (result.error) throw result.error;
  return result;
}
test('version and help need no credentials, Git, dependency executable or initialization', () => {
  const dir = mkdtempSync(join(tmpdir(),'axi-boundary-'));
  try {
    // Malformed repository config must not be read by help/version/schema paths.
    writeFileSync(join(dir,'teamcity.toml'),'not = valid = toml');
    for (const flag of ['--version','-v','-V']) {
      const result = call([flag],dir); assert.equal(result.status,0); assert.equal(result.stdout,'0.1.0-dev.1\n'); assert.equal(result.stderr,'');
    }
    for (const args of [['--help'],['run','view','--help'],['run','tests','--help','--json']]) {
      const result = call(args,dir); assert.equal(result.status,0); assert.equal(result.stderr,''); assert.ok(!result.stdout.includes('executable-canary'));
    }
    const schema = call(['schema','run.view','--json'],dir);
    assert.equal(schema.status,0); assert.equal(JSON.parse(schema.stdout).data.descriptor.name,'run.view');
  } finally {rmSync(dir,{recursive:true,force:true});}
});
test('usage errors produce a valid document, exit 2 and silent stderr before child execution', () => {
  for (const args of [['run','list','--stat','failure','--json'],['run','view','9007199254740993','--json'],['run','start','--json'],['--'+ 'x'.repeat(3000),'--json']]) {
    const result = call(args); assert.equal(result.status,2); assert.equal(result.stderr,''); assert.equal(JSON.parse(result.stdout).error.code,'USAGE_ERROR');
  }
});
test('the local no-argument view is small and round-trips', () => {
  const result = call([],undefined,false); assert.equal(result.status,0); assert.equal(result.stderr,'');
  assert.ok(Buffer.byteLength(result.stdout) < 6144);
  assert.equal(decode(result.stdout).data.mode,'unconfigured');
});
test('short known secrets cannot corrupt public structural keys in either error rendering path',()=>{
  const result=spawnSync(process.execPath,[bin,'--json'],{encoding:'utf8',env:{PATH:'/nonexistent',HOME:'/nonexistent',TEAMCITY_TOKEN:'meta'}});
  if(result.error)throw result.error;
  assert.equal(result.status,1);assert.equal(result.stderr,'');assert.equal(JSON.parse(result.stdout).error.code,'AUTH_CONTEXT_MISMATCH');
});
