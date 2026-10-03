package axi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
)

var openCommandSession = OpenReadSession

func Dispatch(ctx context.Context, parsed Parsed, ec ExecutionContext) (Response, int, error) {
	var output Response
	var err error
	switch parsed.Descriptor.Name {
	case "context.show":
		if !parsed.Bool("verify") {
			output = LocalContext(ec)
		} else {
			output, err = Diagnose(ctx, parsed, ec)
		}
	case "doctor":
		output, err = Diagnose(ctx, parsed, ec)
	case "run.view":
		output, err = ViewRun(ctx, parsed, ec)
	case "run.list":
		output, err = ListRuns(ctx, parsed, ec)
	case "run.problems", "run.tests", "run.log":
		output, err = ReadEvidence(ctx, parsed, ec)
	case "run.changes":
		output, err = ReadChanges(ctx, parsed, ec)
	case "run.tree":
		output, err = ReadTree(ctx, parsed, ec)
	case "run.failure":
		output, err = ReadFailure(ctx, parsed, ec)
	case "run.watch":
		output, err = WatchRun(ctx, parsed, ec)
	case "job.view":
		output, err = ViewJob(ctx, parsed, ec)
	case "job.list":
		output, err = ListJobs(ctx, parsed, ec)
	case "queue.list":
		output, err = ListQueue(ctx, parsed, ec)
	case "agent.list", "agent.view":
		output, err = ReadAgents(ctx, parsed, ec)
	case "status":
		output, err = ReadStatus(ctx, parsed, ec)
	default:
		err = NewError("COMMAND_UNAVAILABLE", "Command is outside the implemented read-only scope", 2)
	}
	if err != nil {
		return Response{}, AsDomainError(err).ExitCode, err
	}
	code := 0
	if parsed.Bool("require-complete") && output.Status == "partial" {
		code = 1
	}
	if parsed.Bool("check") && !Bool(Obj(output.Data["check"]), "passed") {
		code = 1
	}
	return output, code, nil
}

func commandAllowed(ec ExecutionContext) []string {
	if ec.Config == nil {
		return nil
	}
	return ec.Config.Servers[ec.Server].AllowedProjects
}

func commandPolicy(session *ReadSession, ec ExecutionContext, budget Budget) *ProjectPolicy {
	return NewProjectPolicy(session.Reader, commandAllowed(ec), budget)
}

func commandRead(ctx context.Context, session *ReadSession, ec ExecutionContext, request ReadRequest) (ReadResult, error) {
	r := session.Reader.Read(ctx, request, Budget{Deadline: ec.Deadline, MaxChildProcesses: -1})
	if r.State == "unavailable" {
		return r, r.Error
	}
	return r, nil
}

func commandRun(ctx context.Context, parsed Parsed, ec ExecutionContext, session *ReadSession) (ReadResult, error) {
	r, err := commandRead(ctx, session, ec, ReadRequest{Kind: "run.detail", ID: parsed.Positional})
	if err != nil {
		return r, err
	}
	if value, ok := parsed.Flags["job"]; ok && Text(value) != Str(r.Value, "jobId") {
		return r, NewError("CONTEXT_MISMATCH", "Requested run belongs to another job", 1)
	}
	if value, ok := parsed.Flags["project"]; ok && Text(value) != r.Provenance.ProjectID {
		return r, NewError("CONTEXT_MISMATCH", "Requested run belongs to another project", 1)
	}
	if err := commandPolicy(session, ec, Budget{Deadline: ec.Deadline, MaxChildProcesses: -1}).Assert(ctx, r.Provenance.ProjectID); err != nil {
		return r, err
	}
	return r, nil
}

func commandRunContext(ec ExecutionContext, r ReadResult) Object {
	o := Object{"server": ec.Server, "job": r.Value["jobId"], "branch": valueOrNull(r.Value, "branch")}
	if r.Provenance.ProjectID != "" {
		o["project"] = r.Provenance.ProjectID
	}
	return o
}

func commandScopeArgs(ec ExecutionContext, r ReadResult) []string {
	a := []string{"--server", ec.Server, "--job", Str(r.Value, "jobId")}
	if r.Provenance.ProjectID != "" {
		a = append(a, "--project", r.Provenance.ProjectID)
	}
	return a
}

func commandLimitations(output *Response, limits []Limitation, allowed ...string) {
	if len(limits) > 0 {
		output.Meta["limitations"] = limits
	}
	for _, l := range limits {
		if !containsString(allowed, l.Code) {
			output.Status = "partial"
			output.Meta["complete"] = false
			break
		}
	}
}

func commandVersion(limits []Limitation, session *ReadSession) []Limitation {
	if session.NativeVersion != "1.5.0" {
		limits = append(limits, Limitation{Code: "UNVERIFIED_VERSION", Message: "Native version has not been release-certified", Source: "context"})
	}
	return limits
}

func commandCounts(output *Response, session *ReadSession) {
	count := 0
	if session.Transport != nil {
		count = session.Transport.ChildProcesses()
	}
	output.Meta["counts"] = Object{"childProcesses": count}
}

func commandFinish(output Response, parsed Parsed, ec ExecutionContext, session *ReadSession, maxBytes int, limits []Limitation, tolerated ...string) Response {
	limits = commandVersion(limits, session)
	commandLimitations(&output, limits, tolerated...)
	commandCounts(&output, session)
	if maxBytes > 0 {
		output.Meta["limits"] = PublicLimits(ReadLimits(ec, "simple"), commandMaxBytes(parsed, ec, maxBytes))
	}
	if parsed.Bool("no-hints") {
		output.Next = nil
	}
	return output
}

func commandMaxBytes(parsed Parsed, ec ExecutionContext, fallback int) int {
	ceiling := 262144
	if ec.Config != nil && ec.Config.Limits.MaxBytes > 0 {
		ceiling = ec.Config.Limits.MaxBytes
	}
	return min(parsed.Int("max-bytes", fallback), ceiling)
}

func commandHash(value any) string {
	encoded, _ := json.Marshal(value)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func sortedAllowed(ec ExecutionContext) any {
	allowed := commandAllowed(ec)
	if allowed == nil {
		return nil
	}
	a := append([]string{}, allowed...)
	sort.Strings(a)
	return a
}

func commandCursor(parsed Parsed, now int64) (*Cursor, error) {
	if !flagPresent(parsed, "cursor") {
		return nil, nil
	}
	c, err := DecodeCursor(parsed.String("cursor"), now)
	return &c, err
}

func continuation(binding CursorBinding, cursor *Cursor, page Object, now int64) (any, *Limitation, error) {
	if page["position"] == nil {
		return nil, nil, nil
	}
	expires := now + 1800000
	if cursor != nil {
		expires = cursor.ExpiresAt
	}
	current := time.Now().UnixMilli()
	if expires <= current {
		return nil, &Limitation{Code: "CURSOR_EXPIRED", Message: "The query cursor expired during acquisition; continuation is unavailable"}, nil
	}
	token, err := EncodeCursor(Cursor{CursorBinding: binding, Version: 1, Position: Int(page, "position"), ExpiresAt: expires}, current)
	return token, nil, err
}

func commandPage(items []Object, page Object, token any) Object {
	return Object{"returned": len(items), "total": nil, "totalKind": "unknown", "hasMore": valueOrNull(page, "hasMore"), "cursor": token}
}

func commandPrimaryRun(run Object) Object {
	o := projectObject(run, []string{"id", "jobId", "state", "result"})
	o["branch"] = valueOrNull(run, "branch")
	if revisions, ok := run["revisions"]; ok {
		o["revisions"] = revisions
	}
	if Str(run, "result") == "unknown" {
		o["rawStatus"] = valueOrNull(run, "rawStatus")
	}
	return o
}

func pageLimitations(page Object) []Limitation { return commandNotes(page["limitations"]) }

func commandNotes(value any) []Limitation {
	if limits, ok := value.([]Limitation); ok {
		return append([]Limitation{}, limits...)
	}
	limits := []Limitation{}
	for _, l := range Objects(value) {
		limits = append(limits, Limitation{Code: Str(l, "code"), Message: Str(l, "message"), Source: Str(l, "source"), RunID: Str(l, "runId")})
	}
	return limits
}

func flagPresent(parsed Parsed, key string) bool { _, ok := parsed.Flags[key]; return ok }
func valueOrNull(o Object, key string) any {
	if v, ok := o[key]; ok {
		return v
	}
	return nil
}
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func containsString(values []string, value string) bool {
	for _, v := range values {
		if value == v {
			return true
		}
	}
	return false
}
func projectObject(o Object, fields []string) Object {
	r := Object{}
	for k, v := range o {
		if containsString(fields, k) {
			r[k] = v
		}
	}
	return r
}
func fieldProjection(o Object, fields string, mandatory ...string) Object {
	return projectObject(o, append(mandatory, strings.Split(fields, ",")...))
}
func commandHint(reason string, argv []string) Object { return Object{"reason": reason, "argv": argv} }
func numberArg(value int) string                      { return strconv.Itoa(value) }
func commandSecrets(ec ExecutionContext) []string {
	additional := []string{}
	if ec.Config != nil {
		additional = ec.Config.Servers[ec.Server].ForwardHeaderEnvNames
	}
	return KnownSecrets(Environment(), secretPatterns(ec), additional)
}
