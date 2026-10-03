package axi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func NormalizeProject(value any, expected string, secrets []string) (result Object, err error) {
	defer adapterRecover(&err)
	dto := adapterObject(value)
	id := adapterIdentity(dto["id"], false)
	if id != expected {
		adapterFail("CONTEXT_MISMATCH", "Server returned a different project", 1)
	}
	var archived any
	if v, present := dto["archived"]; present {
		b, ok := v.(bool)
		if !ok {
			adapterInvalid("Invalid scoped metadata response")
		}
		archived = b
	}
	var parent any
	v, present := dto["parentProjectId"]
	if present || id != "_Root" {
		parent = adapterIdentity(v, false)
	}
	if parent == id {
		adapterInvalid("Invalid scoped metadata response")
	}
	name := adapterString(dto["name"])
	if name == "" {
		adapterInvalid("Invalid scoped metadata response")
	}
	return Object{"id": id, "name": adapterPreview(SanitizeText(name, secrets), 200), "parentProjectId": parent, "archived": archived}, nil
}
func NormalizeServer(value any, secrets []string) (result Object, err error) {
	defer adapterRecover(&err)
	dto := adapterObject(value)
	version := adapterString(dto["version"])
	if version == "" {
		adapterInvalid("Invalid scoped metadata response")
	}
	return Object{"version": adapterPreview(SanitizeText(version, secrets), 100), "buildNumber": adapterIdentity(dto["buildNumber"], false)}, nil
}
func NormalizeIdentity(value any, server string) (result Object, err error) {
	defer adapterRecover(&err)
	dto := adapterObject(value)
	id := adapterIdentity(dto["id"], true)
	adapterIdentity(dto["username"], false)
	encoded, _ := json.Marshal([]string{server, id})
	hash := sha256.Sum256(encoded)
	return Object{"fingerprint": "sha256:" + hex.EncodeToString(hash[:])[:32]}, nil
}

var logTimestampPattern = regexp.MustCompile(`^([0-9]{4})-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,3})?(Z|[+-][0-9]{2}:?[0-9]{2})$`)

func logTimestamp(value any, notes *[]Limitation) any {
	s := adapterString(value)
	m := logTimestampPattern.FindStringSubmatch(s)
	if m != nil {
		year, _ := strconv.Atoi(m[1])
		zoneValid := true
		if m[3] != "Z" {
			digits := strings.ReplaceAll(m[3][1:], ":", "")
			hours, _ := strconv.Atoi(digits[:2])
			minutes, _ := strconv.Atoi(digits[2:])
			zoneValid = hours <= 23 && minutes < 60
		}
		if year >= 100 && zoneValid {
			if len(m[3]) == 5 {
				n := len(s)
				s = s[:n-2] + ":" + s[n-2:]
			}
			if t, e := time.Parse(time.RFC3339Nano, s); e == nil {
				return t.UTC().Format("2006-01-02T15:04:05.000Z")
			}
		}
	}
	adapterNote(notes, "INVALID_TIMESTAMP", "The server supplied an invalid log timestamp", "log", "")
	return nil
}
func NormalizeLogTail(value any, expected string, tail int, secrets []string) (result Object, err error) {
	defer adapterRecover(&err)
	dto := adapterObject(value)
	if tail < 1 || tail > 1000 {
		adapterFail("USAGE_ERROR", "Invalid bounded log tail", 2)
	}
	if adapterIdentity(dto["run_id"], true) != expected {
		adapterFail("CONTEXT_MISMATCH", "Log belongs to another execution", 1)
	}
	rows := adapterArray(dto["messages"])
	if len(rows) > 1001 {
		adapterInvalid("Invalid scoped metadata response")
	}
	notes := []Limitation{}
	messages := []Object{}
	previous := int64(-1)
	for _, row := range rows {
		m := adapterObject(row)
		id, ok := adapterNumber(m["id"])
		if !ok || id < 0 || id <= previous {
			adapterInvalid("Invalid scoped metadata response")
		}
		previous = id
		level, lok := adapterNumber(m["level"])
		status, sok := adapterNumber(m["status"])
		if !lok || !sok {
			adapterInvalid("Invalid scoped metadata response")
		}
		message := Object{"id": strconv.FormatInt(id, 10), "text": SanitizeText(adapterString(m["text"]), secrets), "level": level, "status": status}
		if t, present := m["timestamp"]; present {
			message["timestamp"] = logTimestamp(t, &notes)
		}
		messages = append(messages, message)
	}
	returned := len(messages)
	if returned > tail {
		messages = messages[returned-tail:]
	}
	return Object{"runId": expected, "messages": messages, "providerReturned": returned, "truncated": returned > tail, "limitations": notes}, nil
}
func scopeIdentities(q ReadRequest, message string) {
	for _, id := range []string{q.JobID, q.ProjectID} {
		if id != "" && !safeIdentityText(id, 256, true) {
			adapterFail("USAGE_ERROR", message, 2)
		}
	}
}
func JobRequest(q ReadRequest) (result AdapterRequest, err error) {
	defer adapterRecover(&err)
	if !safeIdentityText(q.ProjectID, 256, false) {
		adapterFail("USAGE_ERROR", "Invalid bounded job query", 2)
	}
	boundedQuery(q, "Invalid bounded job query")
	return pageRequest("buildTypes", []string{"project:(id:" + literal(q.ProjectID) + ")"}, "count,nextHref,buildType("+JobFields+")", q), nil
}

const JobFields = "id,name,projectId,paused"

func normalizeJob(value any, secrets []string) Object {
	dto := adapterObject(value)
	id, project := adapterIdentity(dto["id"], false), adapterIdentity(dto["projectId"], false)
	var paused any
	if p, present := dto["paused"]; present {
		b, ok := p.(bool)
		if !ok {
			adapterInvalid("Invalid safe job metadata")
		}
		paused = b
	}
	return Object{"id": id, "name": SanitizeText(adapterString(dto["name"]), secrets), "projectId": project, "paused": paused}
}
func NormalizeJob(value any, secrets []string) (result Object, err error) {
	defer adapterRecover(&err)
	return normalizeJob(value, secrets), nil
}
func JobLimitations(job Object) []Limitation {
	if job["paused"] == nil {
		return []Limitation{{Code: "JOB_PAUSED_UNAVAILABLE", Message: "The selected job did not supply its paused state", Source: "job"}}
	}
	return []Limitation{}
}
func NormalizeJobPage(value any, q ReadRequest, server string, secrets []string) (result Object, err error) {
	defer adapterRecover(&err)
	dto := adapterObject(value)
	rows := adapterRows(dto, "buildType", q.Count)
	items := []Object{}
	notes := []Limitation{}
	ids := map[string]bool{}
	for _, row := range rows {
		job := normalizeJob(row, secrets)
		if Str(job, "projectId") != q.ProjectID {
			adapterFail("CONTEXT_MISMATCH", "Job page contains another project", 1)
		}
		if ids[Str(job, "id")] {
			adapterInvalid("Invalid safe job metadata")
		}
		ids[Str(job, "id")] = true
		for _, note := range JobLimitations(job) {
			uniqueNote(&notes, note)
		}
		items = append(items, job)
	}
	page := adapterPage(items, len(rows), notes)
	r, e := JobRequest(q)
	if e != nil {
		panic(AsDomainError(e))
	}
	adapterContinuation(dto, page, r, q, server, "job", "Useful jobs retained; unsafe continuation was not followed", "No continuation returned; bounded lookup does not prove job collection exhaustion")
	return page, nil
}

const QueueFields = "id,buildTypeId,state,branchName,queuedDate,waitReason,buildType(id,projectId)"

func QueueRequest(q ReadRequest) (result AdapterRequest, err error) {
	defer adapterRecover(&err)
	if q.JobID == "" && q.ProjectID == "" {
		adapterFail("CONTEXT_REQUIRED", "Queue list requires a job or project", 2)
	}
	scopeIdentities(q, "Invalid queue scope identity")
	boundedQuery(q, "Invalid bounded queue query")
	filters := []string{}
	if q.JobID != "" {
		filters = append(filters, "buildType:"+idCondition(q.JobID))
	}
	if q.ProjectID != "" {
		filters = append(filters, "project:"+idCondition(q.ProjectID))
	}
	return pageRequest("buildQueue", filters, "count,nextHref,build("+QueueFields+")", q), nil
}
func NormalizeQueuePage(value any, q ReadRequest, server string, secrets []string) (result Object, err error) {
	defer adapterRecover(&err)
	dto := adapterObject(value)
	rows := adapterRows(dto, "build", q.Count)
	notes := []Limitation{}
	items := []Object{}
	ids := map[string]bool{}
	note := func(code, message string) {
		uniqueNote(&notes, Limitation{Code: code, Message: message, Source: "queue"})
	}
	for _, row := range rows {
		build := adapterObject(row)
		id, jobID := adapterIdentity(build["id"], true), adapterIdentity(build["buildTypeId"], false)
		bt := adapterObject(build["buildType"])
		project := adapterIdentity(bt["projectId"], false)
		if adapterIdentity(bt["id"], false) != jobID {
			adapterInvalid("Invalid scoped queue metadata")
		}
		if q.JobID != "" && jobID != q.JobID || q.ProjectID != "" && project != q.ProjectID {
			adapterFail("CONTEXT_MISMATCH", "Queued execution belongs to another selected scope", 1)
		}
		if ids[id] {
			adapterInvalid("Invalid scoped queue metadata")
		}
		ids[id] = true
		rawState := adapterString(build["state"])
		state := rawState
		known := state == "queued" || state == "running" || state == "finished"
		if !known {
			state = "unknown"
			note("UNKNOWN_QUEUE_STATE", "The server supplied an unknown queue lifecycle")
		} else if state != "queued" {
			note("QUEUE_STATE_CHANGED", "An observed queue item is no longer queued; this page is provisional")
		}
		var branch, wait any
		for _, key := range []string{"branchName", "waitReason"} {
			if v := build[key]; v != nil {
				s := SanitizeText(adapterString(v), secrets)
				if key == "branchName" {
					branch = s
				} else if s != "" {
					wait = s
				}
			}
		}
		times := []Limitation{}
		queued := adapterTimestamp(build["queuedDate"], "queued", &times)
		for _, n := range times {
			note(n.Code, n.Message)
		}
		item := Object{"id": id, "jobId": jobID, "state": state, "branch": branch, "queuedAt": queued, "waitReason": wait}
		if !known {
			item["rawState"] = SanitizeText(rawState, secrets)
		}
		items = append(items, item)
	}
	page := adapterPage(items, len(rows), notes)
	r, e := QueueRequest(q)
	if e != nil {
		panic(AsDomainError(e))
	}
	adapterContinuation(dto, page, r, q, server, "queue", "Useful queue items retained; unsafe continuation was not followed", "No continuation returned; bounded lookup does not prove queue exhaustion")
	return page, nil
}
func jsonStringLiteral(value string) string {
	b, _ := json.Marshal(value)
	return strings.TrimSpace(string(b))
}
