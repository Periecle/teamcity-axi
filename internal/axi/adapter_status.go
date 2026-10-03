package axi

import (
	"strconv"
)

const StatusRunFields = "id,buildTypeId,number,state,status,failedToStart,canceledInfo(timestamp),branchName,personal,composite,buildType(id,projectId),revisions(revision(version,vcs-root-instance(id,vcs-root-id)))"

func StatusRequest(q ReadRequest) (result string, err error) {
	defer adapterRecover(&err)
	if len(q.JobIDs) < 1 || len(q.JobIDs) > 5 {
		adapterFail("USAGE_ERROR", "Status requires one to five distinct job identities", 2)
	}
	ids := map[string]bool{}
	parts := []string{}
	for _, id := range q.JobIDs {
		if ids[id] {
			adapterFail("USAGE_ERROR", "Status requires one to five distinct job identities", 2)
		}
		ids[id] = true
		if !safeIdentityText(id, 256, true) {
			adapterFail("USAGE_ERROR", "Invalid status selector", 2)
		}
		parts = append(parts, "item:"+idCondition(id))
	}
	branch := "branch:(default:any)"
	if q.Branch != "" || q.BranchSet {
		if !safeIdentityText(q.Branch, 4096, true) {
			adapterFail("USAGE_ERROR", "Invalid status selector", 2)
		}
		branch = "branch:" + branchCondition(q.Branch)
	}
	locator := "defaultFilter:false,state:any," + branch + ",count:20,start:0,lookupLimit:5000"
	fields := "count,buildType(" + JobFields + ",builds($locator:(" + locator + "),count,nextHref,build(" + StatusRunFields + ")))"
	parts = append(parts, "count:"+strconv.Itoa(len(q.JobIDs)), "start:0")
	return APIPath("buildTypes", parts, fields), nil
}
func NormalizeStatus(input any, q ReadRequest, server string, secrets []string) (result []Object, err error) {
	defer adapterRecover(&err)
	dto := adapterObject(input)
	if _, present := dto["buildType"]; !present {
		if n, ok := adapterNumber(dto["count"]); ok && n == 0 {
			dto = Object{"buildType": []any{}, "count": 0}
		}
	}
	rows := adapterRows(dto, "buildType", len(q.JobIDs))
	jobs, runs := map[string]bool{}, map[string]bool{}
	snapshots := []Object{}
	for _, row := range rows {
		item := adapterObject(row)
		job := normalizeJob(item, secrets)
		jobID := Str(job, "id")
		if !contains(q.JobIDs, jobID) {
			adapterFail("CONTEXT_MISMATCH", "Status returned another job", 1)
		}
		if jobs[jobID] {
			adapterInvalid("Status repeated a job")
		}
		jobs[jobID] = true
		builds := adapterObject(item["builds"])
		if _, present := builds["build"]; !present {
			if n, ok := adapterNumber(builds["count"]); ok && n == 0 {
				builds = Object{"build": []any{}, "count": 0}
			}
		}
		values := adapterRows(builds, "build", 20)
		notes := []Limitation{}
		normalized := []Object{}
		previous := int64(maxSafeInteger + 1)
		for _, value := range values {
			run, project, ls := normalizeRun(value, server, secrets)
			if Str(run, "jobId") != jobID || project != Str(job, "projectId") || (q.Branch != "" || q.BranchSet) && run["branch"] != q.Branch {
				adapterFail("CONTEXT_MISMATCH", "Status candidate is outside its verified scope", 1)
			}
			id := Str(run, "id")
			numeric, _ := strconv.ParseInt(id, 10, 64)
			if runs[id] || numeric >= previous {
				adapterInvalid("Status candidates are not distinct newest-first executions")
			}
			runs[id] = true
			previous = numeric
			roots := map[string]bool{}
			for _, revision := range Objects(run["revisions"]) {
				root := Str(revision, "vcsRootId")
				if roots[root] {
					adapterInvalid("Status candidate repeats a VCS root")
				}
				roots[root] = true
			}
			notes = append(notes, ls...)
			normalized = append(normalized, run)
		}
		if href, present := builds["nextHref"]; present {
			branch := "branch:(default:any)"
			if q.Branch != "" || q.BranchSet {
				branch = "branch:" + branchCondition(q.Branch)
			}
			r := ContinuationRequest{ServerURL: server, Resource: "builds", Filters: []string{"defaultFilter:false", "state:any", "buildType:" + idCondition(jobID), branch}, Fields: "count,nextHref,build(" + StatusRunFields + ")", Count: 20, Start: 0, ScanLimit: 5000}
			s, ok := href.(string)
			var e error
			if ok {
				_, e = NextPosition(s, r)
			}
			if !ok || e != nil {
				adapterNote(&notes, "UNSAFE_CONTINUATION", "Status candidate continuation could not be verified; it was not followed", "run", "")
			}
		}
		snapshots = append(snapshots, Object{"job": job, "page": Object{"runs": normalized, "providerReturned": len(normalized), "position": nil, "hasMore": nil, "limitations": notes}})
	}
	return snapshots, nil
}
