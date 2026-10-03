//go:build live

package liveserver

import (
	"strings"
	"testing"

	"github.com/Periecle/teamcity-axi/internal/axi"
)

func TestLiveExceptionalOutcomesAcrossViewListWatchAndFailure(t *testing.T) {
	f := fixture(t)
	p := axi.Obj(f.Contract.Fixture["lifecycle"])
	jobs := axi.Obj(p["jobs"])
	for _, entry := range []struct{ id, job, result string }{{axi.Str(p, "queuedCanceledRunId"), axi.Str(jobs, "blocked"), "canceled"}, {axi.Str(p, "runningCanceledRunId"), axi.Str(jobs, "slow"), "canceled"}, {axi.Str(p, "failedToStartRunId"), "AxiContract_Vcs", "failed_to_start"}, {axi.Str(p, "compositeRunId"), axi.Str(jobs, "composite"), "success"}} {
		for _, format := range []string{"json", "toon"} {
			result := wire(t, f, []string{"run", "view", entry.id, "--format", format}, 0)
			value := decode(t, result, format)
			equal(t, at(value.Data, "run", "id"), entry.id)
			equal(t, at(value.Data, "run", "result"), entry.result)
			equal(t, at(value.Data, "run", "state"), "finished")
			equal(t, at(value.Data, "run", "composite"), entry.id == axi.Str(p, "compositeRunId"))
			require(t, !strings.Contains(result.Stdout, "canceledInfo"), "raw cancellation metadata propagated")
		}
		list := jsonCall(t, f, []string{"run", "list", "--job", entry.job, "--all-branches", "--result", entry.result}, 0)
		found := false
		for _, run := range objects(list.Data["runs"]) {
			if run["id"] == entry.id && run["result"] == entry.result {
				found = true
			}
		}
		require(t, found, "exceptional run missing from list")
		equal(t, at(list.Data, "aggregates", entry.result), len(objects(list.Data["runs"])))
		code := 1
		if entry.result == "success" {
			code = 0
		}
		watch := jsonCall(t, f, []string{"run", "watch", entry.id, "--check"}, code)
		equal(t, at(watch.Data, "run", "result"), entry.result)
		equal(t, at(watch.Data, "check", "passed"), entry.result == "success")
		failure := jsonCall(t, f, []string{"run", "failure", entry.id}, 0)
		equal(t, at(failure.Data, "run", "result"), entry.result)
		assessment := "failure_observed"
		if entry.result == "success" {
			assessment = "not_failed"
		}
		equal(t, failure.Data["assessment"], assessment)
	}
	ordinary := jsonCall(t, f, []string{"run", "list", "--job", "AxiContract_Vcs", "--all-branches", "--result", "failure"}, 0)
	equal(t, objects(ordinary.Data["runs"]), []axi.Object{})
	assertPermissions(t, f, "outcome-permissions")
}
func TestLiveWatchFixedGreenRedQueuedCheckAndDeadline(t *testing.T) {
	f := fixture(t)
	p := f.Contract.Fixture
	for _, format := range []string{"json", "toon"} {
		value := decode(t, wire(t, f, []string{"run", "watch", axi.Str(p, "vcsRunId"), "--check", "--format", format}, 0), format)
		equal(t, value.Data["outcome"], "finished")
		equal(t, at(value.Data, "check", "passed"), true)
		equal(t, at(value.Data, "run", "id"), p["vcsRunId"])
		equal(t, at(value.Meta, "counts", "childProcesses"), 2)
	}
	jsonCall(t, f, []string{"run", "watch", axi.Str(p, "failedRunId")}, 0)
	jsonCall(t, f, []string{"run", "watch", axi.Str(p, "failedRunId"), "--check"}, 1)
	queued := jsonCall(t, f, []string{"run", "watch", axi.Strings(p["queuedRunIds"])[0], "--timeout", "700ms", "--check"}, 1)
	equal(t, queued.Data["outcome"], "deadline")
	equal(t, at(queued.Data, "run", "state"), "queued")
	equal(t, queued.Meta["complete"], false)
	notes := axi.Limitations(queued.Meta["limitations"])
	found := false
	for _, note := range notes {
		if note.Code == "DEADLINE_EXCEEDED" {
			found = true
		}
	}
	require(t, found, "watch deadline not reported")
}
