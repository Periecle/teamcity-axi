import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync,readdirSync} from 'node:fs';
import {decode} from '@toon-format/toon';
import {response} from '../../dist/domain/response.js';
import {render} from '../../dist/output/render.js';
import {sanitizeText,knownSecrets,secretMatchers} from '../../dist/output/sanitize.js';
import {validateResponse,validateConfig} from '../../dist/output/schema.js';
test('all supplied response examples validate and JSON/TOON preserve logical values', () => {
  for (const file of readdirSync('examples')) {
    const value = JSON.parse(readFileSync(`examples/${file}`,'utf8'));
    if (file === 'user-config.json' || file === 'repository-config.json') {validateConfig(file.replace('.json',''),value); continue;}
    validateResponse(value);
    assert.deepEqual(decode(render(value,'toon',262144).document),JSON.parse(render(value,'json',262144).document));
  }
  const value = response('status',{strings:['123','001','true','null'],unicode:'ą日本語🦊',nested:[{x:null,n:1},{x:'1',n:0}]});
  assert.deepEqual(decode(render(value,'toon',16384).document),value);
  assert.equal(render(value,'json',16384,['1.0']).response.schemaVersion,'1.0');
  assert.ok(render(value,'json',16384,['meta']).response.meta);
});
test('trusted safe name patterns redact environment values and untrusted fields',()=>{
  const secrets=knownSecrets({COMPANY_CREDENTIAL:'pattern-canary'},['^COMPANY_CREDENTIAL$']);
  assert.deepEqual(secrets,['pattern-canary']);
  const value=response('status',{branch:'pattern-canary',message:'protected-name-canary',COMPANY_CREDENTIAL:'unknown-private-value'});
  const document=render(value,'json',2048,secrets,['^COMPANY_CREDENTIAL$','^message$']).document;
  assert.ok(!document.includes('pattern-canary'));assert.ok(!document.includes('unknown-private-value'));
  assert.ok(!document.includes('protected-name-canary'));
  assert.throws(()=>secretMatchers(['(a+)+$']));assert.throws(()=>secretMatchers(['.*.*.*']));
});
test('known secrets and terminal controls are removed before rendering in both formats', () => {
  const value = response('status',{text:'\x1b[31mcanary-secret\x1b[0m\u202eevil',password:'unknown',message:'Bearer abc123'});
  for (const format of ['json','toon']) {
    const {document} = render(value,format,2048,['canary-secret']);
    assert.ok(!document.includes('canary-secret')); assert.ok(!document.includes('abc123')); assert.ok(!document.includes('\x1b')); assert.ok(!document.includes('\u202e'));
  }
  assert.equal(sanitizeText('\x1b]0;injected title\x07safe',[]),'safe');
  assert.equal(sanitizeText('\u202e',[]),'\\u202e');
  assert.equal(sanitizeText('Authorization: Basic YWRtaW46cGFzc3dvcmQ=',[]),'Authorization: Basic [REDACTED]');
  assert.equal(sanitizeText('-----BEGIN RSA PRIVATE KEY-----\nprivate body\n-----END RSA PRIVATE KEY-----',[]),'[REDACTED PRIVATE KEY]');
});
test('oversized evidence produces one valid bounded error instead of lying about retained evidence', () => {
  const value = response('run.view',{text:'🦊'.repeat(100000)});
  for (const format of ['json','toon']) {
    const result = render(value,format,2048);
    assert.ok(Buffer.byteLength(result.document) <= 2048);
    assert.equal(result.response.status,'error'); assert.equal(result.response.meta.complete,false);
    assert.equal(result.response.meta.truncated,true);
    validateResponse(result.response);
  }
  const scoped = response('run.view',{text:'important'});
  scoped.context = {server:'work',job:'Payments_Build',branch:'b'.repeat(5000)};
  const bounded = render(scoped,'json',2048).response;
  assert.equal(bounded.context.server,'work'); assert.equal(bounded.context.job,'Payments_Build');
});
