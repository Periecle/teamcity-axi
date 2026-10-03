//go:build live

package liveserver

import (
	"testing"

	"github.com/Periecle/teamcity-axi/internal/axi"
)

func TestLiveRestrictedJobsDirectScopeMetadataAndUnknownExhaustion(t *testing.T) {
	f := fixture(t)
	p := f.Contract.Fixture
	args := []string{"job", "list", "--project", axi.Str(p, "projectId"), "--limit", "1"}
	first := jsonCall(t, f, args, 0)
	job := objects(first.Data["jobs"])[0]
	equal(t, job["id"], p["jobId"])
	equal(t, job["paused"], false)
	equal(t, at(first.Data, "page", "hasMore"), true)
	equal(t, at(first.Data, "page", "total"), nil)
	equal(t, at(first.Data, "selection", "membership"), "direct")
	equal(t, at(first.Meta, "counts", "childProcesses"), 3)
	parseActions(t, first.Next)
	next := jsonCall(t, f, action(t, first.Next, ""), 0)
	equal(t, objects(next.Data["jobs"])[0]["id"], p["greenJobId"])
	equal(t, at(next.Data, "selection", "position"), 1)
	toonValue := decode(t, wire(t, f, args, 0), "toon")
	equal(t, toonValue.Data["jobs"], first.Data["jobs"])
	for _, format := range []string{"json", "toon"} {
		value := decode(t, wire(t, f, []string{"job", "view", axi.Str(p, "jobId"), "--project", axi.Str(p, "projectId"), "--format", format}, 0), format)
		equal(t, value.Data["job"], job)
		equal(t, at(value.Meta, "counts", "childProcesses"), 2)
		equal(t, len(axi.Obj(value.Data["job"])), 4)
	}
	all := jsonCall(t, f, []string{"job", "list", "--project", axi.Str(p, "projectId")}, 0)
	expected := append([]string{"AxiContract_Fail", "AxiContract_Green"}, axi.Strings(p["queueJobIds"])...)
	expected = append(expected, "AxiContract_Vcs")
	equal(t, ids(objects(all.Data["jobs"]), "id"), expected)
	equal(t, all.Status, "partial")
	equal(t, at(all.Data, "page", "hasMore"), nil)
	equal(t, at(all.Data, "page", "total"), nil)
	strict := jsonCall(t, f, []string{"job", "list", "--project", axi.Str(p, "projectId"), "--require-complete", "--no-hints"}, 1)
	require(t, strict.Next == nil, "no-hints emitted next")
	denied := jsonCall(t, f, []string{"job", "list", "--project", "AxiDenied"}, 1)
	equal(t, denied.Error.Code, "PERMISSION_DENIED")
	mismatch := jsonCall(t, f, []string{"job", "view", axi.Str(p, "jobId"), "--project", "AxiDenied"}, 1)
	equal(t, mismatch.Error.Code, "CONTEXT_MISMATCH")
}
func TestLiveRestrictedQueuePositiveIDsContinuationAndWaitReason(t *testing.T) {
	f := fixture(t)
	p := f.Contract.Fixture
	queued, jobs := axi.Strings(p["queuedRunIds"]), axi.Strings(p["queueJobIds"])
	args := []string{"queue", "list", "--project", axi.Str(p, "projectId"), "--limit", "1"}
	first := jsonCall(t, f, args, 0)
	item := objects(first.Data["items"])[0]
	equal(t, item["id"], queued[0])
	equal(t, item["jobId"], jobs[0])
	equal(t, item["state"], "queued")
	equal(t, at(first.Data, "page", "hasMore"), true)
	equal(t, at(first.Data, "page", "total"), nil)
	equal(t, item["waitReason"], objects(api(t, native(t, f, "queue-project-positive"))["build"])[0]["waitReason"])
	timestamp := axi.Str(item, "queuedAt")
	require(t, len(timestamp) > 0 && timestamp[len(timestamp)-1] == 'Z', "queue timestamp not UTC")
	equal(t, item["branch"], nil)
	toonValue := decode(t, wire(t, f, args, 0), "toon")
	equal(t, toonValue.Data["items"], first.Data["items"])
	parseActions(t, first.Next)
	next := jsonCall(t, f, action(t, first.Next, ""), 0)
	equal(t, objects(next.Data["items"])[0]["id"], queued[1])
	equal(t, at(next.Data, "selection", "position"), 1)
	scoped := jsonCall(t, f, []string{"queue", "list", "--job", jobs[0]}, 0)
	equal(t, ids(objects(scoped.Data["items"]), "id"), []string{queued[0]})
	equal(t, scoped.Context["project"], p["projectId"])
	equal(t, scoped.Status, "partial")
	equal(t, at(scoped.Data, "page", "hasMore"), nil)
	parseActions(t, scoped.Next)
	detail := jsonCall(t, f, action(t, scoped.Next, ""), 0)
	equal(t, at(detail.Data, "run", "state"), "queued")
	empty := jsonCall(t, f, []string{"queue", "list", "--job", axi.Str(p, "jobId"), "--require-complete", "--no-hints"}, 1)
	equal(t, objects(empty.Data["items"]), []axi.Object{})
	equal(t, at(empty.Data, "page", "total"), nil)
	equal(t, at(empty.Data, "page", "hasMore"), nil)
	require(t, empty.Next == nil, "no-hints emitted next")
	denied := jsonCall(t, f, []string{"queue", "list", "--project", "AxiDenied"}, 1)
	equal(t, denied.Error.Code, "PERMISSION_DENIED")
}
func TestLiveScopedAgentsSeparateAvailabilityUnknownActivityAndDeniedPools(t *testing.T) {
	f := fixture(t)
	p := f.Contract.Fixture
	args := []string{"agent", "list", "--project", axi.Str(p, "projectId"), "--limit", "1"}
	first := jsonCall(t, f, args, 0)
	agent := objects(first.Data["agents"])[0]
	equal(t, agent["id"], p["agentId"])
	equal(t, at(agent, "pool", "id"), p["agentPoolId"])
	for _, key := range []string{"connected", "enabled", "authorized"} {
		equal(t, agent[key], true)
	}
	equal(t, agent["activeRunState"], "not_reported")
	equal(t, agent["activeRun"], nil)
	equal(t, first.Status, "partial")
	equal(t, at(first.Data, "page", "total"), nil)
	equal(t, at(first.Data, "page", "hasMore"), true)
	toonValue := decode(t, wire(t, f, args, 0), "toon")
	equal(t, toonValue.Data["agents"], first.Data["agents"])
	parseActions(t, first.Next)
	next := jsonCall(t, f, action(t, first.Next, ""), 0)
	equal(t, at(next.Data, "selection", "position"), 1)
	equal(t, objects(next.Data["agents"]), []axi.Object{})
	equal(t, at(next.Data, "page", "hasMore"), nil)
	for _, format := range []string{"json", "toon"} {
		value := decode(t, wire(t, f, []string{"agent", "view", axi.Str(p, "agentId"), "--job", axi.Str(p, "jobId"), "--format", format}, 0), format)
		equal(t, value.Data["agent"], agent)
		equal(t, value.Context["project"], p["projectId"])
	}
	job := jsonCall(t, f, []string{"agent", "list", "--job", axi.Str(p, "jobId")}, 0)
	equal(t, job.Data["agents"], first.Data["agents"])
	parseActions(t, job.Next)
	detail := jsonCall(t, f, action(t, job.Next, ""), 0)
	equal(t, at(detail.Data, "agent", "id"), p["agentId"])
	empty := jsonCall(t, f, []string{"agent", "list", "--job", axi.Strings(p["queueJobIds"])[0], "--require-complete", "--no-hints"}, 1)
	equal(t, objects(empty.Data["agents"]), []axi.Object{})
	equal(t, at(empty.Data, "page", "total"), nil)
	equal(t, at(empty.Data, "page", "hasMore"), nil)
	require(t, empty.Next == nil, "no-hints emitted next")
	for _, entry := range []struct {
		args []string
		code string
	}{{[]string{"agent", "list", "--project", "AxiDenied"}, "PERMISSION_DENIED"}, {[]string{"agent", "list", "--pool", axi.Str(p, "agentPoolId")}, "NOT_FOUND"}, {[]string{"agent", "view", "999999999"}, "NOT_FOUND"}, {[]string{"agent", "view", axi.Str(p, "agentId"), "--job", axi.Strings(p["queueJobIds"])[0]}, "NOT_FOUND"}} {
		value := jsonCall(t, f, entry.args, 1)
		equal(t, value.Error.Code, entry.code)
	}
}
