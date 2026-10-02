import {writeFile,mkdir} from 'node:fs/promises';
import {captureNative} from './native-harness.mjs';
const result = await captureNative(process.env.TEAMCITY_AXI_TEST_BINARY);
await mkdir(new URL('../tests/fixtures/native-v1.5.0/',import.meta.url),{recursive:true});
await writeFile(new URL('../tests/fixtures/native-v1.5.0/contract.json',import.meta.url),JSON.stringify(result,null,2)+'\n');
console.log(`Recorded ${Object.keys(result.records).length} released-binary mock-server observations`);
