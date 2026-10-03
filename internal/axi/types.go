package axi

import (
	"context"
	"fmt"
	"time"
)

type Object = map[string]any

type DomainError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	ExitCode   int    `json:"-"`
	Retryable  bool   `json:"retryable"`
	Details    Object `json:"details,omitempty"`
	HTTPStatus int    `json:"-"`
	RetryAfter string `json:"-"`
}

func (e *DomainError) Error() string { return e.Message }
func NewError(code, message string, exit int) *DomainError {
	return &DomainError{Code: code, Message: message, ExitCode: exit}
}
func Usage(message string) *DomainError { return NewError("USAGE_ERROR", message, 2) }
func AsDomainError(err error) *DomainError {
	if e, ok := err.(*DomainError); ok {
		return e
	}
	return NewError("INTERNAL_ERROR", "The operation failed unexpectedly", 1)
}

type Limitation struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	RunID   string `json:"runId,omitempty"`
	Source  string `json:"source,omitempty"`
}
type Response struct {
	SchemaVersion string       `json:"schemaVersion"`
	Command       string       `json:"command"`
	Status        string       `json:"status"`
	Context       Object       `json:"context,omitempty"`
	Data          Object       `json:"data,omitempty"`
	Error         *DomainError `json:"error,omitempty"`
	Meta          Object       `json:"meta"`
	Next          []Object     `json:"next,omitempty"`
}

func ObservedAt() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }
func NewResponse(command string, data Object) Response {
	return Response{SchemaVersion: "1.0", Command: command, Status: "ok", Data: data, Meta: Object{"observedAt": ObservedAt(), "complete": true, "truncated": false}}
}

type Budget struct {
	Deadline          int64
	MaxChildProcesses int
}
type Provenance struct {
	ObservedAt  string
	Operation   string
	ProjectID   string
	Limitations []Limitation
}
type ReadResult struct {
	State      string
	Value      Object
	Error      *DomainError
	Provenance Provenance
}
type TimeWindow struct {
	Since string `json:"since"`
	Until string `json:"until"`
}
type ReadRequest struct {
	Kind                                                                            string
	ID, RunID, JobID, ProjectID, PoolID, Branch, State, Result, Revision, VCSRootID string
	Count, Start, ScanLimit, Tail                                                   int
	Failed, Muted, Files                                                            bool
	Window                                                                          *TimeWindow
	AllowedProjects                                                                 []string
	JobIDs                                                                          []string
	MutedSet, FailedSet, BranchSet                                                  bool
}
type Reader interface {
	Read(context.Context, ReadRequest, Budget) ReadResult
}

func Str(o Object, key string) string { s, _ := o[key].(string); return s }
func Bool(o Object, key string) bool  { v, _ := o[key].(bool); return v }
func Int(o Object, key string) int {
	switch v := o[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}
func Objects(v any) []Object {
	switch a := v.(type) {
	case []Object:
		return a
	case []any:
		r := make([]Object, 0, len(a))
		for _, x := range a {
			if o, ok := x.(map[string]any); ok {
				r = append(r, o)
			}
		}
		return r
	}
	return nil
}
func Obj(v any) Object { o, _ := v.(map[string]any); return o }
func Strings(v any) []string {
	switch a := v.(type) {
	case []string:
		return a
	case []any:
		r := []string{}
		for _, x := range a {
			if s, ok := x.(string); ok {
				r = append(r, s)
			}
		}
		return r
	}
	return nil
}
func Text(v any) string { return fmt.Sprint(v) }
