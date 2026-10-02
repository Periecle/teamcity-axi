import type {DomainError} from './errors.js';
import type {Limitation} from './response.js';
export interface Run {
  id:string; jobId:string; state:'queued'|'running'|'finished'|'unknown';
  result:'success'|'failure'|'error'|'canceled'|'failed_to_start'|'unknown';
  number?:string; branch?:string|null; statusText?:string; personal?:boolean|null;
  composite?:boolean|null; queuedAt?:string|null; startedAt?:string|null;
  finishedAt?:string|null; durationMs?:number|null; webUrl?:string|null;
  revisions?:{vcsRootId:string;revision:string}[]; rawStatus?:string|null;
}
export interface RunRef {id:string}
export interface Budget {deadline:number}
export interface Provenance {observedAt:string;operation:string;projectId:string|null;limitations:Limitation[]}
export type ReadResult<T> = {state:'available';value:T;provenance:Provenance} | {state:'unavailable';error:DomainError;provenance:Provenance};
// The first vertical adapter implements this slice. Additional read primitives
// extend this interface as their recorded contracts and tests are added.
export interface TeamCityReader {getRun(ref:RunRef,budget:Budget):Promise<ReadResult<Run>>}
