const api = path => ['api', path, '-X', 'GET', '--include', '--raw', '-H', 'Accept: application/json', '--no-input'];
export const operations = [
  ['server', api('/app/rest/server?fields=version,buildNumber')],
  ['run-view', api('/app/rest/builds/id:482193?fields=id,buildTypeId,number,state,status,branchName,statusText,personal,composite,buildType(id,name,projectId),revisions(revision(version,vcs-root-instance(id,vcs-root-id))),startDate,finishDate')],
  ['run-list', api('/app/rest/builds?locator=buildType:(id:Payments_Build),count:20,lookupLimit:5000&fields=count,nextHref,build(id,buildTypeId,state,status,branchName)')],
  ['problems', api('/app/rest/problemOccurrences?locator=build:(id:482193),count:20&fields=count,nextHref,problemOccurrence(id,type,identity,details,build(id))')],
  ['tests', api('/app/rest/testOccurrences?locator=build:(id:482193),count:20&fields=count,nextHref,testOccurrence(id,name,status,muted,ignored,details,build(id),test(id))')],
  ['dependencies', api('/app/rest/builds?locator=snapshotDependency:(to:(id:482193),recursive:false),defaultFilter:false,count:20,lookupLimit:5000&fields=count,nextHref,build(id,buildTypeId,state,status)')],
  ['changes', api('/app/rest/changes?locator=build:(id:482193),count:10&fields=count,nextHref,change(id,version,comment,vcsRootInstance(vcs-root-id))')],
  ['jobs', api('/app/rest/buildTypes?locator=project:(id:Payments),count:20&fields=count,nextHref,buildType(id,name,projectId,paused)')],
  ['queue', api('/app/rest/buildQueue?locator=buildType:(id:Payments_Build),count:20&fields=count,nextHref,build(id,buildTypeId,state,branchName,waitReason)')],
  ['agents', api('/app/rest/agents?locator=pool:(id:1),count:20&fields=count,nextHref,agent(id,name,connected,enabled,authorized,pool(id,name))')],
  ['log-tail', ['run','log','482193','--tail','80','--json','--no-input']],
  ['failure-summary', ['run','log','482193','--failed','--json','--no-input']],
];
export const errorModes = ['denied','missing','expired','malformed','html'];
