import {test} from 'node:test';
import assert from 'node:assert/strict';
import {nextPosition} from '../../dist/adapter/continuation.js';
const request={serverUrl:'https://tc.example/teamcity',resource:'builds',filters:['buildType:(id:Build)','branch:(name:(value:($base64:YSwp)))'],fields:'count,nextHref,build(id)',count:20,start:0,scanLimit:5000};
const next=(patch={})=>'/teamcity/app/rest/builds?'+new URLSearchParams({locator:[...request.filters,'count:20','start:20','lookupLimit:5000'].join(','),fields:request.fields,...patch});
test('continuation is reduced to a bounded position and never forwarded as a URL',()=>{
  assert.equal(nextPosition(next(),request),20);assert.equal(nextPosition(next().replace('/teamcity/','/'),request),20);
  assert.equal(nextPosition('https://tc.example'+next(),request),20);
  assert.throws(()=>nextPosition('https://tc.example'+next().replace('/teamcity/','/'),request),e=>e.code==='UPSTREAM_SCHEMA_MISMATCH');
  for(const href of ['https://evil.example'+next(),'//tc.example'+next(),next().replace('/builds?','/agents?'),next().replace('/teamcity/','/teamcity/../'),next().replace('/teamcity/','/teamcity/%2e%2e/'),next().replace('/teamcity/','/teamcity/%252e%252e/'),next({fields:'build(parameters)'}),next({extra:'x'}),next({locator:'buildType:(id:Other),count:20,start:20'}),next({locator:[...request.filters,'count:100','start:20'].join(',')}),next({locator:[...request.filters,'count:20','start:20','lookupLimit:10000'].join(',')}),next({locator:[...request.filters,'count:20','start:0'].join(',')}),next({locator:[...request.filters,'count:20','start:5000'].join(',')})]){
    assert.throws(()=>nextPosition(href,request),e=>e.code==='UPSTREAM_SCHEMA_MISMATCH');
  }
});
