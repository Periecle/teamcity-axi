package axi

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

type NativeTransport interface {
	Execute(context.Context, Operation, int) (Captured, error)
}
type NativeReader struct {
	transport NativeTransport
	serverURL string
	secrets   []string
}

func NewNativeReader(transport NativeTransport, serverURL string, secrets []string) Reader {
	return &NativeReader{transport, serverURL, append([]string{}, secrets...)}
}
func (r *NativeReader) Read(ctx context.Context, q ReadRequest, b Budget) (result ReadResult) {
	result.Provenance = Provenance{ObservedAt: ObservedAt(), Operation: q.Kind, ProjectID: q.ProjectID, Limitations: []Limitation{}}
	result.State = "unavailable"
	var err error
	defer func() {
		if value := recover(); value != nil {
			if e, ok := value.(*DomainError); ok {
				result.Error = e
			} else {
				panic(value)
			}
		}
	}()
	var path string
	op := Operation{Kind: "api"}
	normalize := func(body any) (Object, error) {
		return nil, NewError("DEPENDENCY_UNSUPPORTED", "Unsupported read operation", 1)
	}
	switch q.Kind {
	case "run.detail":
		id := adapterIdentity(q.ID, true)
		path = "/app/rest/builds/id:" + id + "?fields=" + RunDetailFields
		normalize = func(body any) (Object, error) {
			run, project, notes, e := NormalizeRun(body, r.serverURL, r.secrets)
			if e != nil {
				return nil, e
			}
			if Str(run, "id") != id {
				return nil, NewError("CONTEXT_MISMATCH", "Server returned a different execution than the requested ID", 1)
			}
			result.Provenance.ProjectID = project
			result.Provenance.Limitations = notes
			return run, nil
		}
	case "run.page":
		filters, e := RunFilters(q)
		if e != nil {
			panic(AsDomainError(e))
		}
		fields := "count,nextHref,build(" + RunDetailFields + ")"
		request := pageRequest("builds", filters, fields, q)
		path = request.Path
		normalize = func(body any) (Object, error) {
			return NormalizeRunPage(body, q, continuationRequest(request, q, r.serverURL), r.secrets)
		}
	case "job.detail":
		id := adapterIdentity(q.ID, false)
		path = "/app/rest/buildTypes/id:" + literal(id) + "?fields=" + JobFields
		normalize = func(body any) (Object, error) {
			job, e := NormalizeJob(body, r.secrets)
			if e != nil {
				return nil, e
			}
			if Str(job, "id") != id {
				return nil, NewError("CONTEXT_MISMATCH", "Server returned a different job", 1)
			}
			result.Provenance.ProjectID = Str(job, "projectId")
			result.Provenance.Limitations = JobLimitations(job)
			return job, nil
		}
	case "job.page":
		request, e := JobRequest(q)
		if e != nil {
			panic(AsDomainError(e))
		}
		path = request.Path
		normalize = func(body any) (Object, error) { return NormalizeJobPage(body, q, r.serverURL, r.secrets) }
	case "queue.page":
		request, e := QueueRequest(q)
		if e != nil {
			panic(AsDomainError(e))
		}
		path = request.Path
		normalize = func(body any) (Object, error) { return NormalizeQueuePage(body, q, r.serverURL, r.secrets) }
	case "agents.page":
		request, e := AgentRequest(q)
		if e != nil {
			panic(AsDomainError(e))
		}
		path = request.Path
		normalize = func(body any) (Object, error) { return NormalizeAgentPage(body, q, r.serverURL, r.secrets) }
	case "agent.detail":
		if e := ValidateAgentID(q.ID, false); e != nil {
			panic(AsDomainError(e))
		}
		filters, e := AgentFilters(q)
		if e != nil {
			panic(AsDomainError(e))
		}
		parts := append([]string{"id:" + q.ID}, filters...)
		path = "/app/rest/agents/" + strings.Join(parts, ",") + "?fields=" + AgentFields
		normalize = func(body any) (Object, error) {
			agent, notes, e := NormalizeAgent(body, q, r.secrets)
			if e != nil {
				return nil, e
			}
			if Str(agent, "id") != q.ID {
				return nil, NewError("CONTEXT_MISMATCH", "Server returned a different agent", 1)
			}
			result.Provenance.Limitations = notes
			return agent, nil
		}
	case "status.snapshot":
		path, err = StatusRequest(q)
		if err != nil {
			panic(AsDomainError(err))
		}
		normalize = func(body any) (Object, error) {
			snapshots, e := NormalizeStatus(body, q, r.serverURL, r.secrets)
			return Object{"snapshots": snapshots}, e
		}
	case "server.detail":
		path = "/app/rest/server?fields=version,buildNumber"
		normalize = func(body any) (Object, error) { return NormalizeServer(body, r.secrets) }
	case "identity.current":
		path = "/app/rest/users/current?fields=id,username"
		normalize = func(body any) (Object, error) { return NormalizeIdentity(body, r.serverURL) }
	case "project.detail":
		id := adapterIdentity(q.ID, false)
		path = "/app/rest/projects/id:" + literal(id) + "?fields=id,name,parentProjectId,archived"
		normalize = func(body any) (Object, error) {
			project, e := NormalizeProject(body, id, r.secrets)
			if e == nil {
				result.Provenance.ProjectID = Str(project, "id")
			}
			return project, e
		}
	case "log.tail":
		id := q.ID
		if id == "" {
			id = q.RunID
		}
		id = adapterIdentity(id, true)
		op = Operation{Kind: "log", RunID: id, Tail: q.Tail}
		normalize = func(body any) (Object, error) { return NormalizeLogTail(body, id, q.Tail, r.secrets) }
	case "problems.page", "tests.page":
		kind := "tests"
		if q.Kind == "problems.page" {
			kind = "problems"
		}
		request, e := EvidenceRequest(kind, q)
		if e != nil {
			panic(AsDomainError(e))
		}
		path = request.Path
		normalize = func(body any) (Object, error) { return NormalizeEvidencePage(kind, body, q, r.serverURL, r.secrets) }
	case "problem.detail", "test.detail":
		kind, resource, fields := "tests", "testOccurrences", TestFields
		if q.Kind == "problem.detail" {
			kind, resource, fields = "problems", "problemOccurrences", ProblemFields
		}
		locator, e := OccurrenceLocator(kind, q.ID, q.RunID)
		if e != nil {
			panic(AsDomainError(e))
		}
		path = "/app/rest/" + resource + "/" + locator + "?fields=" + fields
		normalize = func(body any) (Object, error) {
			var item Object
			var e error
			if kind == "problems" {
				item, e = NormalizeProblem(body, q.RunID, r.secrets)
			} else {
				item, e = NormalizeTest(body, q.RunID, r.secrets, &result.Provenance.Limitations)
			}
			if e != nil {
				return nil, e
			}
			if Str(item, "id") != q.ID {
				return nil, NewError("CONTEXT_MISMATCH", "Server returned another "+strings.TrimSuffix(q.Kind, ".detail")+" occurrence", 1)
			}
			return item, nil
		}
	case "changes.page", "dependencies.page":
		kind := "changes"
		if q.Kind == "dependencies.page" {
			kind = "dependencies"
		}
		request, e := RelatedRequest(kind, q, RunDetailFields)
		if e != nil {
			panic(AsDomainError(e))
		}
		path = request.Path
		normalize = func(body any) (Object, error) {
			if kind == "changes" {
				return NormalizeChangePage(body, q, r.serverURL, r.secrets)
			}
			return NormalizeDependencyPage(body, q, r.serverURL, RunDetailFields, r.secrets)
		}
	case "dependencies.count":
		id := q.ID
		if id == "" {
			id = q.RunID
		}
		id = adapterIdentity(id, true)
		path = "/app/rest/builds/id:" + id + "?fields=id,snapshot-dependencies(count)"
		normalize = func(body any) (Object, error) {
			n, e := NormalizeDependencyCount(body, id)
			return Object{"count": n}, e
		}
	default:
		result.Error = NewError("DEPENDENCY_UNSUPPORTED", "Unsupported read operation", 1)
		return
	}
	if time.Now().UnixMilli() >= b.Deadline {
		result.Error = NewError("DEADLINE_EXCEEDED", "Overall deadline exceeded", 1)
		result.Error.Retryable = true
		return
	}
	if op.Kind == "api" {
		op.Path = path
	}
	capture, e := r.transport.Execute(ctx, op, b.MaxChildProcesses)
	if e != nil {
		result.Error = AsDomainError(e)
		return
	}
	var body any
	if op.Kind == "log" {
		if capture.ExitCode != 0 || capture.Signal != "" {
			result.Error = NewError("UPSTREAM_FAILURE", "Structured log capability could not be verified", 1)
			return
		}
		if !utf8.Valid(capture.Stdout) {
			result.Error = NewError("UPSTREAM_SCHEMA_MISMATCH", "Invalid structured log encoding", 1)
			return
		}
		if body, e = DecodeJSON(capture.Stdout); e != nil {
			result.Error = NewError("UPSTREAM_SCHEMA_MISMATCH", "Invalid structured log document", 1)
			return
		}
	} else {
		raw, e := ParseRaw(capture)
		if e != nil {
			result.Error = AsDomainError(e)
			return
		}
		body = raw.Body
	}
	value, e := normalize(body)
	if e != nil {
		result.Error = AsDomainError(e)
		return
	}
	if notes, ok := value["limitations"].([]Limitation); ok {
		result.Provenance.Limitations = notes
	}
	if q.Kind == "run.page" && Int(value, "providerReturned") < q.Count && hasLimitation(result.Provenance.Limitations, "SCAN_COVERAGE_UNKNOWN") {
		server := r.Read(ctx, ReadRequest{Kind: "server.detail"}, b)
		if server.State == "unavailable" && server.Error.Code == "INTERRUPTED" {
			result.Error = server.Error
			return
		}
		if server.State == "available" && Str(server.Value, "version") == "2026.2 (build 238924)" && Str(server.Value, "buildNumber") == "238924" {
			notes := []Limitation{}
			for _, note := range result.Provenance.Limitations {
				if note.Code != "SCAN_COVERAGE_UNKNOWN" {
					notes = append(notes, note)
				}
			}
			value["limitations"] = notes
			value["hasMore"] = false
			result.Provenance.Limitations = notes
		}
	}
	result.State = "available"
	result.Value = value
	result.Provenance.ObservedAt = ObservedAt()
	return
}
func hasLimitation(notes []Limitation, code string) bool {
	for _, note := range notes {
		if note.Code == code {
			return true
		}
	}
	return false
}
