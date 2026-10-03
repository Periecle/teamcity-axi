package axi

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const RunDetailFields = "id,buildTypeId,number,state,status,failedToStart,canceledInfo(timestamp),branchName,statusText,personal,composite,buildType(id,name,projectId),revisions(revision(version,vcs-root-instance(id,vcs-root-id))),startDate,finishDate"

var providerTimestamp = regexp.MustCompile(`^([0-9]{4})([0-9]{2})([0-9]{2})T([0-9]{2})([0-9]{2})([0-9]{2})([+-])([0-9]{2})([0-9]{2})$`)

func adapterTimestamp(value any, field string, notes *[]Limitation) any {
	if value == nil || value == "" {
		return nil
	}
	s := adapterString(value)
	match := providerTimestamp.FindStringSubmatch(s)
	if match != nil {
		year, _ := strconv.Atoi(match[1])
		zh, _ := strconv.Atoi(match[8])
		zm, _ := strconv.Atoi(match[9])
		t, e := time.Parse("20060102T150405-0700", s)
		if e == nil && year >= 100 && zh <= 23 && zm < 60 {
			return t.UTC().Format("2006-01-02T15:04:05.000Z")
		}
	}
	adapterNote(notes, "INVALID_TIMESTAMP", "The server supplied an invalid "+field+" timestamp", "run", "")
	return nil
}
func Timestamp(value any, field string, notes *[]Limitation) (result any, err error) {
	defer adapterRecover(&err)
	return adapterTimestamp(value, field, notes), nil
}
func normalizeRun(input any, server string, secrets []string) (Object, string, []Limitation) {
	dto := adapterObject(input)
	notes := []Limitation{}
	id, jobID := adapterIdentity(dto["id"], true), adapterIdentity(dto["buildTypeId"], false)
	project := ""
	if v, present := dto["buildType"]; present {
		bt := adapterObject(v)
		if adapterIdentity(bt["id"], false) != jobID {
			adapterInvalid("Conflicting run job identities")
		}
		if p, present := bt["projectId"]; present {
			project = adapterIdentity(p, false)
		}
	}
	rawState, ok := adapterText(dto["state"])
	if !ok {
		adapterInvalid("Run lifecycle and result metadata are required")
	}
	status, statusOK := adapterText(dto["status"])
	_, statusPresent := dto["status"]
	if !statusOK && !(rawState == "queued" && !statusPresent) {
		adapterInvalid("Run lifecycle and result metadata are required")
	}
	state := rawState
	if state != "queued" && state != "running" && state != "finished" {
		state = "unknown"
	}
	results := map[string]string{"SUCCESS": "success", "FAILURE": "failure", "ERROR": "error"}
	result := results[status]
	if result == "" {
		result = "unknown"
	}
	failed := adapterFlag(dto["failedToStart"])
	canceled := dto["canceledInfo"] != nil
	var cancellationTime any
	if canceled {
		cancellationTime = adapterTimestamp(adapterObject(dto["canceledInfo"])["timestamp"], "cancellation", &notes)
	}
	if failed == nil || (canceled && cancellationTime == nil) {
		result = "unknown"
		adapterNote(&notes, "OUTCOME_METADATA_UNAVAILABLE", "Explicit execution outcome metadata is unavailable", "run", id)
	} else if (canceled && failed == true) || ((canceled || failed == true) && status == "SUCCESS") || (failed == true && state != "finished") {
		result = "unknown"
		adapterNote(&notes, "CONFLICTING_OUTCOME_METADATA", "Execution outcome metadata conflicts with the reported state or status", "run", id)
	} else if canceled {
		result = "canceled"
	} else if failed == true {
		result = "failed_to_start"
	}
	if state == "unknown" {
		adapterNote(&notes, "UNKNOWN_LIFECYCLE", "The upstream lifecycle is unknown to this adapter", "run", id)
	}
	if result == "unknown" && results[status] == "" {
		code, message := "UNKNOWN_RESULT", "The upstream result is unknown to this adapter"
		if !statusPresent {
			code, message = "RESULT_UNAVAILABLE", "The queued execution has no reported result"
		}
		adapterNote(&notes, code, message, "run", id)
	}
	revisions := []Object{}
	if value, present := dto["revisions"]; present {
		rows := adapterArray(adapterObject(value)["revision"])
		if len(rows) > 100 {
			adapterInvalid("Invalid revision collection")
		}
		for _, value := range rows {
			revision := adapterObject(value)
			root := adapterObject(revision["vcs-root-instance"])
			revisions = append(revisions, Object{"vcsRootId": adapterIdentity(root["vcs-root-id"], false), "revision": adapterIdentity(revision["version"], false)})
		}
	} else {
		adapterNote(&notes, "MISSING_REVISION_METADATA", "The server omitted revision metadata; revision coverage is unknown", "run", id)
	}
	started, finished, queued := adapterTimestamp(dto["startDate"], "start", &notes), adapterTimestamp(dto["finishDate"], "finish", &notes), adapterTimestamp(dto["queuedDate"], "queue", &notes)
	var web any
	if value, present := dto["webUrl"]; present {
		s := adapterString(value)
		u, e := url.Parse(s)
		base, be := url.Parse(server)
		if e == nil && be == nil && u.IsAbs() && u.Scheme == base.Scheme && u.Host == base.Host && u.User == nil && strings.HasPrefix(u.Path, strings.TrimSuffix(base.Path, "/")+"/") {
			web = u.String()
		} else {
			message := "Untrusted run web link omitted"
			if e != nil || u == nil || !u.IsAbs() {
				message = "Invalid run web link omitted"
			}
			adapterNote(&notes, "UNSAFE_WEB_URL", message, "run", id)
		}
	}
	run := Object{"id": id, "jobId": jobID, "state": state, "result": result, "branch": adapterOptionalString(dto, "branchName"), "personal": adapterFlag(dto["personal"]), "composite": adapterFlag(dto["composite"]), "startedAt": started, "finishedAt": finished, "queuedAt": queued, "webUrl": web, "rawStatus": nil}
	if _, present := dto["revisions"]; present {
		run["revisions"] = revisions
	}
	if result == "unknown" && statusOK {
		run["rawStatus"] = adapterPreview(SanitizeText(status, secrets), 80)
	}
	for _, key := range []string{"number", "statusText"} {
		if value, present := dto[key]; present {
			run[key] = SanitizeText(adapterString(value), secrets)
		}
	}
	if started != nil && finished != nil {
		a, _ := time.Parse(time.RFC3339Nano, started.(string))
		b, _ := time.Parse(time.RFC3339Nano, finished.(string))
		duration := b.Sub(a).Milliseconds()
		if duration >= 0 {
			run["durationMs"] = duration
		} else {
			adapterNote(&notes, "INVALID_DURATION", "Finish precedes start; duration omitted", "run", id)
		}
	}
	return run, project, notes
}
func adapterPreview(value string, limit int) string {
	r := []rune(value)
	if len(r) > limit {
		return string(r[:limit])
	}
	return value
}
func NormalizeRun(input any, server string, secrets []string) (run Object, projectID string, notes []Limitation, err error) {
	defer adapterRecover(&err)
	run, projectID, notes = normalizeRun(input, server, secrets)
	return
}
func RunFilters(q ReadRequest) (result []string, err error) {
	defer adapterRecover(&err)
	return runFilters(q), nil
}
func runFilters(q ReadRequest) []string {
	if q.JobID == "" && q.ProjectID == "" {
		adapterFail("CONTEXT_REQUIRED", "Run list requires a job or project", 2)
	}
	boundedQuery(q, "Invalid bounded page request")
	if q.Revision != "" && q.VCSRootID == "" {
		adapterFail("CONTEXT_REQUIRED", "Exact revision filtering requires a VCS root", 2)
	}
	filters := []string{"defaultFilter:false"}
	if q.JobID != "" {
		filters = append(filters, "buildType:"+idCondition(q.JobID))
	}
	if q.ProjectID != "" {
		filters = append(filters, "project:"+idCondition(q.ProjectID))
	}
	if q.Branch != "" || q.BranchSet {
		filters = append(filters, "branch:"+branchCondition(q.Branch))
	} else {
		filters = append(filters, "branch:(default:any)")
	}
	if q.State != "" {
		if q.State != "queued" && q.State != "running" && q.State != "finished" {
			adapterFail("USAGE_ERROR", "Invalid lifecycle filter", 2)
		}
		filters = append(filters, "state:"+q.State)
	}
	if q.Result != "" {
		switch q.Result {
		case "canceled":
			filters = append(filters, "canceled:true", "failedToStart:false")
		case "failed_to_start":
			filters = append(filters, "canceled:false", "failedToStart:true")
		case "success", "failure", "error":
			filters = append(filters, "status:"+strings.ToUpper(q.Result), "canceled:false", "failedToStart:false")
		case "unknown":
		default:
			adapterFail("DEPENDENCY_UNSUPPORTED", "Result filter requires separately verified explicit outcome metadata", 1)
		}
	}
	if q.Window != nil {
		comparison, e := CompareTimestamps(q.Window.Since, q.Window.Until)
		if e != nil {
			panic(AsDomainError(e))
		}
		if q.State != "finished" || comparison > 0 {
			adapterFail("USAGE_ERROR", "Finish-time window requires ordered finished-run semantics", 2)
		}
		for _, entry := range []struct{ condition, value string }{{"after", q.Window.Since}, {"before", q.Window.Until}} {
			date, e := ProviderDate(entry.value, entry.condition)
			if e != nil {
				panic(AsDomainError(e))
			}
			if date != "" {
				filters = append(filters, "finishDate:(date:"+date+",condition:"+entry.condition+")")
			}
		}
	}
	return filters
}
func NormalizeRunPage(input any, q ReadRequest, r ContinuationRequest, secrets []string) (result Object, err error) {
	defer adapterRecover(&err)
	return normalizeRunPage(input, q, r, secrets), nil
}
func normalizeRunPage(input any, q ReadRequest, r ContinuationRequest, secrets []string) Object {
	dto := adapterObject(input)
	rows := adapterRows(dto, "build", q.Count)
	notes := []Limitation{}
	runs := []Object{}
	ids := map[string]bool{}
	for _, value := range rows {
		run, project, ls := normalizeRun(value, r.ServerURL, secrets)
		id := Str(run, "id")
		if ids[id] {
			adapterInvalid("Provider repeated an execution in one page")
		}
		ids[id] = true
		if q.JobID != "" && Str(run, "jobId") != q.JobID || q.ProjectID != "" && project != q.ProjectID || (q.Branch != "" || q.BranchSet) && run["branch"] != q.Branch || q.State != "" && Str(run, "state") != q.State || q.Result != "" && q.Result != "unknown" && Str(run, "result") != q.Result {
			adapterFail("CONTEXT_MISMATCH", "Provider returned an execution outside the declared list scope", 1)
		}
		if q.AllowedProjects != nil && !contains(q.AllowedProjects, project) {
			adapterFail("POLICY_DENIED", "Run project is outside the verified trusted scope", 1)
		}
		notes = append(notes, ls...)
		if q.Result == "unknown" && Str(run, "result") != "unknown" {
			continue
		}
		if q.Window != nil {
			finish := Str(run, "finishedAt")
			if finish == "" {
				adapterNote(&notes, "FINISH_TIME_UNVERIFIED", "Candidate omitted a valid finish timestamp", "run", id)
				continue
			}
			t, _ := time.Parse(time.RFC3339Nano, finish)
			lower := t.Add(999 * time.Millisecond).Format(time.RFC3339Nano)
			a, e := CompareTimestamps(lower, q.Window.Since)
			if e != nil {
				panic(AsDomainError(e))
			}
			b, e := CompareTimestamps(finish, q.Window.Until)
			if e != nil {
				panic(AsDomainError(e))
			}
			if a <= 0 || b >= 0 {
				adapterNote(&notes, "FINISH_TIME_OUTSIDE_WINDOW", "Reported finish-time interval is disjoint from the requested window", "run", id)
				continue
			}
		}
		if q.Revision != "" {
			matches := []Object{}
			for _, revision := range Objects(run["revisions"]) {
				if Str(revision, "vcsRootId") == q.VCSRootID {
					matches = append(matches, revision)
				}
			}
			if len(matches) == 0 {
				adapterNote(&notes, "REVISION_UNVERIFIED", "Candidate lacks the requested VCS root revision", "run", id)
				continue
			}
			match := true
			for _, revision := range matches {
				if Str(revision, "revision") != q.Revision {
					match = false
				}
			}
			if !match {
				continue
			}
		}
		runs = append(runs, run)
	}
	page := Object{"runs": runs, "providerReturned": len(rows), "position": nil, "hasMore": nil, "limitations": notes}
	adapterContinuation(dto, page, AdapterRequest{Resource: r.Resource, Fields: r.Fields, Filters: r.Filters}, q, r.ServerURL, "run", "Provider continuation could not be safely reconstructed; search stopped", "No continuation was supplied; bounded scan exhaustion is unverified")
	return page
}
