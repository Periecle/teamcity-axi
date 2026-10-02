import {DomainError} from '../domain/errors.js';
import {response} from '../domain/response.js';
import type {Response} from '../domain/response.js';
import type {Parsed} from '../cli/parser.js';
import type {ExecutionContext} from '../context/resolve.js';
import {ProcessTransport,resolveBinary} from '../transport/process.js';
import {NativeTeamCityReader} from '../adapter/reader.js';
import {knownSecrets} from '../output/sanitize.js';
export async function viewRun(parsed:Parsed,context:ExecutionContext,signal:AbortSignal):Promise<Response> {
  const binary=await resolveBinary(context.config?.binaryPath,context.repositoryRoot,context.config?.allowWorkspaceBinary);
  const server=context.config?.servers[context.server!]!;
  const transport=await ProcessTransport.create({binary,serverUrl:context.serverUrl!,env:process.env,signal,...(server?.forwardHeaderEnvNames?{headerNames:server.forwardHeaderEnvNames}:{}),
    limits:{deadline:context.deadline,concurrency:Math.min(3,context.config?.limits?.concurrency??3),maxChildren:Math.min(8,context.config?.limits?.maxChildProcesses??8),stdoutBytes:2097152,stderrBytes:65536}});
  try {
    const version=await transport.execute({kind:'version'});
    const matched=/^teamcity version ([a-zA-Z0-9.+-]{1,80})\r?\n$/.exec(version.stdout.toString('utf8'));
    if(version.exitCode!==0||!matched)throw new DomainError('DEPENDENCY_UNSUPPORTED','Native executable has an unsupported version response');
    const reader=new NativeTeamCityReader(transport,context.serverUrl!,knownSecrets(process.env,context.config?.secretNamePatterns));
    const read=await reader.getRun({id:parsed.positional!},{deadline:context.deadline});
    if(read.state==='unavailable')throw read.error;
    if(parsed.flags.job!==undefined&&read.value.jobId!==parsed.flags.job)throw new DomainError('CONTEXT_MISMATCH','Requested run belongs to a different job');
    if(parsed.flags.project!==undefined&&read.provenance.projectId!==parsed.flags.project)throw new DomainError('CONTEXT_MISMATCH','Requested run belongs to a different project');
    // Exact-ID reads never bypass trusted narrowing. Ancestry expansion will be
    // added with separately verified project reads; unknown ancestry fails closed.
    if(server?.allowedProjects&&(!read.provenance.projectId||!server.allowedProjects.includes(read.provenance.projectId)))throw new DomainError('POLICY_DENIED','Run project is outside the verified trusted scope');
    let run=read.value;
    const output=response('run.view',{run});
    output.context={server:context.server!,job:run.jobId,branch:run.branch??null,...(read.provenance.projectId?{project:read.provenance.projectId}:{})};
    output.meta.observedAt=read.provenance.observedAt;
    output.meta.counts={childProcesses:transport.childProcesses};
    const limitations=[...read.provenance.limitations];
    if(matched[1]!=='1.5.0')limitations.push({code:'UNVERIFIED_VERSION',message:'Native version has not been release-certified',source:'context'});
    if(run.statusText&&!parsed.flags.full&&Array.from(run.statusText).length>1200){run={...run,statusText:Array.from(run.statusText).slice(0,1200).join('')};output.meta.truncated=true;output.next=[{reason:'Expand the execution summary',argv:['teamcity-axi','run','view',run.id,'--full','--server',context.server!,...(parsed.flags.job?['--job',String(parsed.flags.job)]:[]),...(parsed.flags.project?['--project',String(parsed.flags.project)]:[])]}];}
    if(parsed.flags.fields){const keep=new Set(['id','jobId','state','result',...String(parsed.flags.fields).split(',')]);run=Object.fromEntries(Object.entries(run).filter(([key])=>keep.has(key))) as typeof run;}
    output.data={run};
    if(limitations.length)output.meta.limitations=limitations;
    if(limitations.some(l=>!['UNKNOWN_LIFECYCLE','UNKNOWN_RESULT','UNVERIFIED_VERSION'].includes(l.code))){output.status='partial';output.meta.complete=false;}
    if(parsed.flags['no-hints'])delete output.next;
    if(parsed.flags['require-complete']&&output.status==='partial')process.exitCode=1;
    return output;
  }finally{await transport.dispose();}
}
