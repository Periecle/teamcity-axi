//go:build live

package liveserver

import (
	"strings"
	"testing"

	"github.com/Periecle/teamcity-axi/internal/axi"
)

func TestLiveExclusiveFractionalRunWindowsDespiteCoarseDTODates(t *testing.T) {
	f := fixture(t)
	p := axi.Obj(f.Contract.Fixture["listPrecision"])
	for _, entry := range []struct {
		since, until string
		expected     []string
	}{{axi.Str(p, "lowerExclusive"), axi.Str(p, "upperExclusive"), []string{axi.Str(p, "runId")}}, {axi.Str(p, "upperExclusive"), axi.Str(p, "afterExact"), []string{}}, {axi.Str(p, "lowerExclusive"), axi.Str(p, "exactFinish"), []string{}}} {
		for _, format := range []string{"json", "toon"} {
			value := decode(t, wire(t, f, []string{"run", "list", "--job", axi.Str(p, "jobId"), "--all-branches", "--since", entry.since, "--until", entry.until, "--fields", "finishedAt", "--format", format}, 0), format)
			equal(t, ids(objects(value.Data["runs"]), "id"), entry.expected)
			equal(t, at(value.Data, "selection", "window", "since"), entry.since)
			equal(t, at(value.Data, "selection", "window", "until"), entry.until)
			equal(t, at(value.Data, "selection", "window", "bounds"), "exclusive")
			equal(t, at(value.Data, "selection", "timestampMembership"), "provider")
			equal(t, at(value.Data, "selection", "reportedTimestampPrecision"), "second")
			equal(t, at(value.Data, "selection", "filterTimestampPrecision"), "millisecond")
			if len(entry.expected) > 0 {
				equal(t, objects(value.Data["runs"])[0]["finishedAt"], p["finish"])
			}
			equal(t, at(value.Data, "page", "total"), len(entry.expected))
			equal(t, at(value.Data, "page", "hasMore"), false)
		}
	}
}
func TestLiveUnknownResultQueuedEvidenceAndFinishedCandidateCounts(t *testing.T) {
	f := fixture(t)
	p := f.Contract.Fixture
	precision := axi.Obj(p["listPrecision"])
	for _, format := range []string{"json", "toon"} {
		queued := decode(t, wire(t, f, []string{"run", "list", "--job", axi.Strings(p["queueJobIds"])[0], "--all-branches", "--state", "queued", "--result", "unknown", "--format", format}, 0), format)
		equal(t, ids(objects(queued.Data["runs"]), "id"), []string{axi.Strings(p["queuedRunIds"])[0]})
		equal(t, objects(queued.Data["runs"])[0]["state"], "queued")
		equal(t, objects(queued.Data["runs"])[0]["result"], "unknown")
		equal(t, at(queued.Data, "selection", "timestampBasis"), nil)
		_, present := axi.Obj(queued.Data["selection"])["window"]
		require(t, !present, "queued list fabricated finish window")
		equal(t, at(queued.Data, "selection", "resultBasis"), "normalized_candidates")
		equal(t, at(queued.Data, "aggregates", "unknown"), 1)
		finished := decode(t, wire(t, f, []string{"run", "list", "--job", axi.Str(precision, "jobId"), "--all-branches", "--result", "unknown", "--since", axi.Str(precision, "lowerExclusive"), "--until", axi.Str(precision, "upperExclusive"), "--format", format}, 0), format)
		equal(t, objects(finished.Data["runs"]), []axi.Object{})
		equal(t, at(finished.Data, "selection", "providerReturned"), 1)
		equal(t, at(finished.Data, "page", "total"), 0)
		equal(t, at(finished.Data, "page", "hasMore"), false)
		equal(t, finished.Status, "ok")
	}
}
func TestLiveFinishedEmptyExactZeroAndLookupCapContinuation(t *testing.T) {
	f := fixture(t)
	for _, name := range []string{"exhaustion-cap-empty", "exhaustion-cap-positive"} {
		result := native(t, f, name)
		equal(t, result.Code, 0)
		body := api(t, result)
		count := 0
		if strings.HasSuffix(name, "positive") {
			count = 1
		}
		equal(t, body["count"], count)
		require(t, strings.Contains(axi.Str(body, "nextHref"), "lookupLimit:2"), "provider lookup cap not visible")
	}
	p := axi.Obj(f.Contract.Fixture["exhaustion"])
	for _, format := range []string{"json", "toon"} {
		value := decode(t, wire(t, f, []string{"run", "list", "--job", axi.Str(p, "emptyFinishedJobId"), "--all-branches", "--since", axi.Str(p, "since"), "--until", axi.Str(p, "until"), "--require-complete", "--format", format}, 0), format)
		equal(t, value.Status, "ok")
		equal(t, value.Meta["complete"], true)
		equal(t, objects(value.Data["runs"]), []axi.Object{})
		equal(t, value.Data["page"], axi.Object{"returned": 0, "total": 0, "totalKind": "exact", "hasMore": false, "cursor": nil})
		equal(t, value.Context["job"], p["emptyFinishedJobId"])
		equal(t, value.Context["project"], f.Contract.Fixture["projectId"])
		equal(t, at(value.Data, "selection", "exhaustionBasis"), "verified_server_pagination")
		equal(t, at(value.Data, "selection", "consistency"), "best_effort_offset")
		equal(t, at(value.Data, "selection", "window", "since"), p["since"])
		equal(t, at(value.Data, "selection", "window", "until"), p["until"])
		equal(t, at(value.Meta, "counts", "childProcesses"), 4)
		require(t, value.Next == nil, "empty exhausted list emitted next")
	}
}
