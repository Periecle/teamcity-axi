import {asDomainError,DomainError} from '../domain/errors.js';
import type {Budget,ReadResult,Run,RunRef,TeamCityReader,Provenance} from '../domain/teamcity.js';
import type {ProcessTransport} from '../transport/process.js';
import {parseRaw} from './raw.js';
import {identity,normalizeRun} from './run.js';
// Frozen in tests/fixtures/native-operations.mjs and the released-binary capture.
export const runDetailFields='id,buildTypeId,number,state,status,branchName,statusText,personal,composite,buildType(id,name,projectId),revisions(revision(version,vcs-root-instance(id,vcs-root-id))),startDate,finishDate';
export class NativeTeamCityReader implements TeamCityReader {
  constructor(private readonly transport:ProcessTransport,private readonly serverUrl:string,private readonly secrets:readonly string[]=[]) {}
  async getRun(ref:RunRef,budget:Budget):Promise<ReadResult<Run>> {
    const provenance:Provenance={observedAt:new Date().toISOString(),operation:'run.detail',projectId:null,limitations:[]};
    try {
      const id=identity(ref.id,true);
      if(Date.now()>=budget.deadline)throw new DomainError('DEADLINE_EXCEEDED','Overall deadline exceeded',1,true);
      const captured=await this.transport.execute({kind:'api',path:`/app/rest/builds/id:${id}?fields=${runDetailFields}`});
      const normalized=normalizeRun(parseRaw(captured).body,this.serverUrl,this.secrets);
      if(normalized.run.id!==id)throw new DomainError('CONTEXT_MISMATCH','Server returned a different execution than the requested ID');
      provenance.observedAt=new Date().toISOString();provenance.projectId=normalized.projectId;provenance.limitations=normalized.limitations;
      return {state:'available',value:normalized.run,provenance};
    }catch(error){return {state:'unavailable',error:asDomainError(error),provenance};}
  }
}
