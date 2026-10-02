import {writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {liveFixture,verifyLiveCapture} from './live-harness.mjs';
const outputPath=process.env.TEAMCITY_AXI_LIVE_OUTPUT;
if(!outputPath)throw Error('Set TEAMCITY_AXI_LIVE_OUTPUT to an explicit destination for sanitized test captures');
const f=await liveFixture();
try {
  const records={};
  for(const [name,record] of Object.entries(f.contract.records))records[name]={args:record.args,...await f.native(record.args,name==='invalid-auth')};
  verifyLiveCapture(records,f.contract);
  const result={...f.contract,observedAt:new Date().toISOString(),records};
  await writeFile(resolve(outputPath),JSON.stringify(result,null,2)+'\n',{mode:0o600,flag:'wx'});
  console.log(`Recorded ${Object.keys(records).length} sanitized live native observations`);
}finally{await f.close();}
