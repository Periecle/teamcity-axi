import {spawn} from 'node:child_process';
import {writeFileSync} from 'node:fs';
const args = process.argv.slice(2);
const mode = args[0] === '--version' ? 'inspect' : new URL(args[1],'https://fixture.invalid').searchParams.get('fields');
switch (mode) {
  case 'hang': setInterval(()=>{},1000); break;
  case 'grandchild': {
    const child = spawn(process.execPath,['-e',"process.on('SIGTERM',()=>{});setInterval(()=>{},1000)"],{stdio:'ignore'});
    writeFileSync(process.env.TEAMCITY_HEADER_X_FIXTURE_PID_PATH,String(child.pid));
    setInterval(()=>{},1000); break;
  }
  case 'huge': process.stdout.write('x'.repeat(3000000)); break;
  case 'huge-stderr': process.stderr.write('x'.repeat(100000)); break;
  case 'nonzero': process.stdout.write('{"valid":true}'); process.exitCode=17; break;
  case 'wait': setTimeout(()=>process.stdout.write('done'),100); break;
  default:
    process.stdout.write(JSON.stringify({args,cwd:process.cwd(),tokenPresent:Boolean(process.env.TEAMCITY_TOKEN),env:Object.fromEntries(Object.entries(process.env).filter(([k])=>k !== 'TEAMCITY_TOKEN' && k !== 'TEAMCITY_HEADER_X_FIXTURE_PID_PATH'))}));
}
