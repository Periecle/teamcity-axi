package testfixture

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Object = map[string]any
type Options struct {
	Token, ProjectID, VCSRootID string
	RespectFields, Tree         bool
}
type Request struct {
	Method, Path  string
	Query         url.Values
	Authenticated bool
}
type Server struct {
	mu                                           sync.Mutex
	server                                       *httptest.Server
	mode, token, project, root, revision, branch string
	watchReads, detailReads                      int
	requests                                     []Request
	tree, respectFields                          bool
	BaseURL                                      string
}

const nativeCanary = "fixture-secret-"

func Str(o Object, k string) string { s, _ := o[k].(string); return s }
func Int(o Object, k string) int {
	switch v := o[k].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}
func Obj(v any) Object { o, _ := v.(map[string]any); return o }
func Objects(v any) []Object {
	switch a := v.(type) {
	case []Object:
		return a
	case []any:
		r := []Object{}
		for _, v := range a {
			if o := Obj(v); o != nil {
				r = append(r, o)
			}
		}
		return r
	}
	return nil
}
func containsString(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func cloneMock(o Object) Object {
	data, _ := json.Marshal(o)
	var result Object
	json.Unmarshal(data, &result)
	return result
}
func mockRun(id int) Object {
	return Object{"id": id, "buildTypeId": "Payments_Build", "number": "42", "state": "finished", "status": "FAILURE", "failedToStart": false, "branchName": "feature/refund", "statusText": "Tests failed", "personal": false, "composite": false, "buildType": Object{"id": "Payments_Build", "name": "Build", "projectId": "Payments"}, "revisions": Object{"count": 1, "revision": []Object{{"version": strings.Repeat("a", 40), "vcs-root-instance": Object{"id": "17", "vcs-root-id": "Payments_Git"}}}}, "startDate": "20261001T140000+0000", "finishDate": "20261001T140100+0000"}
}
func mockJob() Object {
	return Object{"id": "Payments_Build", "name": "Build", "projectId": "Payments", "paused": false}
}
func mockAgent() Object {
	return Object{"id": 7, "name": "linux-1", "connected": true, "enabled": true, "authorized": true, "pool": Object{"id": 1, "name": "Default"}}
}
func mockQueue() Object {
	return Object{"id": 482194, "buildTypeId": "Payments_Build", "state": "queued", "branchName": "feature/refund", "waitReason": "Waiting for compatible agent", "queuedDate": "20261001T140000+0000", "buildType": Object{"id": "Payments_Build", "projectId": "Payments"}}
}
func mockMessage() Object {
	return Object{"id": 12, "text": "Connection refused", "level": 0, "status": 4, "timestamp": "2026-10-01T14:00:01Z"}
}
func mockChange(index int, files bool) Object {
	o := Object{"id": strconv.Itoa(101 + index), "version": strings.Repeat("a", 40), "comment": fmt.Sprintf("Synthetic change %d\n\nContextual fixture evidence only", index), "date": "20261001T135900+0000", "vcsRootInstance": Object{"vcs-root-id": "Payments_Git"}}
	if files {
		o["files"] = Object{"count": 1, "file": []Object{{"file": "src/fixture.go", "changeType": "edited"}}}
	}
	return o
}
func longNativeCanary() string { return nativeCanary + strings.Repeat("q", 1600) }

func NewServer(options Options) *Server {
	tree := options.Tree
	m := &Server{mode: "ok", token: "fixture-only-token", project: "Payments", root: "Payments_Git", revision: strings.Repeat("a", 40), branch: "feature/refund", tree: tree}
	if tree {
		m.mode = "dag"
	}
	m.server = httptest.NewServer(http.HandlerFunc(m.serve))
	if options.Token != "" {
		m.token = options.Token
	}
	if options.ProjectID != "" {
		m.project = options.ProjectID
	}
	if options.VCSRootID != "" {
		m.root = options.VCSRootID
	}
	m.respectFields = options.RespectFields
	m.BaseURL = m.server.URL + "/teamcity"
	return m
}
func NewTree(options Options) *Server            { options.Tree = true; return NewServer(options) }
func (m *Server) Close()                         { m.server.Close() }
func (m *Server) SetStatusRevision(value string) { m.mu.Lock(); m.revision = value; m.mu.Unlock() }
func (m *Server) SetStatusBranch(value string)   { m.mu.Lock(); m.branch = value; m.mu.Unlock() }
func (m *Server) SetMode(mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mode = mode
	m.watchReads = 0
	m.detailReads = 0
}
func (m *Server) Requests() []Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Request{}, m.requests...)
}
func (m *Server) ClearRequests() { m.mu.Lock(); defer m.mu.Unlock(); m.requests = nil }
func mockBound(locator, name string, fallback int) int {
	match := regexp.MustCompile(`(?:^|,)` + name + `:(\d+)`).FindStringSubmatch(locator)
	if match == nil {
		return fallback
	}
	n, _ := strconv.Atoi(match[1])
	return n
}
func mockNext(path string, q url.Values, count, start int) string {
	next := url.Values{}
	next.Set("locator", regexp.MustCompile(`start:\d+`).ReplaceAllString(q.Get("locator"), "start:"+strconv.Itoa(start+count)))
	next.Set("fields", q.Get("fields"))
	return "/teamcity" + path + "?" + next.Encode()
}
func mockID(path string) string {
	match := regexp.MustCompile(`\(\$base64:([A-Za-z0-9_-]+)\)`).FindStringSubmatch(path)
	if match != nil {
		data, _ := base64.RawURLEncoding.DecodeString(match[1])
		return string(data)
	}
	_, id, _ := strings.Cut(path, "id:")
	return id
}

func (m *Server) serve(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	mode := m.mode
	m.requests = append(m.requests, Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Authenticated: r.Header.Get("Authorization") == "Bearer "+m.token})
	m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Date", "Thu, 01 Oct 2026 14:00:00 GMT")
	send := func(value any) {
		if m.respectFields && w.Header().Get("Content-Type") == "application/json" {
			value = SelectFields(value, r.URL.Query().Get("fields"))
		}
		json.NewEncoder(w).Encode(value)
	}
	deny := func(status int) {
		w.WriteHeader(status)
		send(Object{"message": "Independent fixture source unavailable"})
	}
	if r.Method != "GET" {
		deny(405)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/teamcity/") {
		deny(404)
		return
	}
	switch mode {
	case "denied":
		if m.tree {
			break
		}
		deny(403)
		return
	case "root-denied":
		deny(403)
		return
	case "missing":
		deny(404)
		return
	case "expired":
		deny(401)
		return
	case "malformed":
		fmt.Fprint(w, "{broken")
		return
	case "html":
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>Login</html>")
		return
	case "hang":
		<-r.Context().Done()
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/teamcity")
	q := r.URL.Query()
	locator, fields := q.Get("locator"), q.Get("fields")
	count, start := mockBound(locator, "count", 20), mockBound(locator, "start", 0)
	if m.tree {
		if m.treeServe(w, r, path, q, mode) {
			return
		}
	}
	if (mode == "summary-denied" && path == "/app/rest/testOccurrences") || (mode == "problems-denied" && strings.HasPrefix(path, "/app/rest/problemOccurrences")) || (mode == "tests-denied" && strings.HasPrefix(path, "/app/rest/testOccurrences")) {
		deny(403)
		return
	}
	if mode == "logs-unsupported" && path == "/app/messages" {
		deny(404)
		return
	}
	if mode == "logs-hang" && path == "/app/messages" {
		<-r.Context().Done()
		return
	}
	var value Object
	switch {
	case path == "/app/rest/server":
		if mode == "list-exhaustion-probe-denied" {
			deny(403)
			return
		}
		value = Object{"version": "mock-contract", "buildNumber": "not-a-TeamCity-server"}
		if strings.HasPrefix(mode, "list-verified") {
			value = Object{"version": "2026.2 (build 238924)", "buildNumber": "238924"}
		}
	case path == "/app/rest/users/current":
		value = Object{"id": 2, "username": "fixture-reader"}
	case strings.HasPrefix(path, "/app/rest/projects/id:"):
		id := mockID(path)
		if mode == "agent-policy-hang" && id == "Payments_Child" {
			<-r.Context().Done()
			return
		}
		value = Object{"id": id, "name": "Fixture project", "archived": false}
		if id != "_Root" {
			value["parentProjectId"] = "_Root"
		}
		if id == "Payments_Child" {
			value["parentProjectId"] = "Payments"
		}
		if mode == "project-cycle" {
			value["parentProjectId"] = id
		}
		if mode == "project-wrong-id" {
			value["id"] = "Other"
		}
	case strings.HasPrefix(path, "/app/rest/buildTypes/id:"):
		if mode == "jobs-detail-denied" {
			deny(403)
			return
		}
		if mode == "jobs-detail-missing" {
			deny(404)
			return
		}
		value = mockJob()
		switch mode {
		case "jobs-wrong-id":
			value["id"] = "Unrelated"
		case "jobs-leading-id":
			value["id"] = "--job"
		case "jobs-foreign":
			value["projectId"] = "Forbidden"
		case "jobs-unknown-paused":
			delete(value, "paused")
		case "jobs-secret":
			value["name"] = "fixture-only-token\x1b[31m"
			value["parameters"] = Object{"secret": "server-private-parameter"}
		}
	case strings.HasPrefix(path, "/app/rest/builds/id:"):
		id, _ := strconv.Atoi(strings.TrimPrefix(path, "/app/rest/builds/id:"))
		if fields == "id,snapshot-dependencies(count)" {
			if mode == "dependency-count-wrong-id" {
				id = 482100
			}
			value = Object{"id": id, "snapshot-dependencies": Object{"count": 1}}
			break
		}
		value = mockRun(id)
		if id == 482194 {
			value["state"] = "queued"
			delete(value, "status")
		}
		switch mode {
		case "long-secret":
			value["status"] = longNativeCanary()
			value["statusText"] = longNativeCanary()
		case "decorated-secret":
			secret := longNativeCanary()
			value["status"] = secret[:600] + "\x1b[31m" + secret[600:]
			value["statusText"] = value["status"]
		case "missing-revisions":
			delete(value, "revisions")
		case "wrong-id":
			value["id"] = 482100
		case "invalid-identity":
			value["id"] = uint64(9007199254740992)
		case "unknown-enum":
			value["state"] = "new_lifecycle"
			value["status"] = "FUTURE_RESULT"
		case "huge-text":
			value["statusText"] = strings.Repeat("🦊", 100000)
		case "huge":
			value["statusText"] = strings.Repeat("x", 3000000)
		}
		if strings.HasPrefix(mode, "watch-") {
			m.mu.Lock()
			m.watchReads++
			n := m.watchReads
			m.mu.Unlock()
			if n > 1 && mode == "watch-vanish" {
				deny(404)
				return
			}
			if n > 1 && mode == "watch-inaccessible" {
				deny(403)
				return
			}
			value["state"] = "running"
			value["status"] = "SUCCESS"
			if mode == "watch-success" || (mode == "watch-transition" && n > 1) {
				value["state"] = "finished"
			}
			if mode == "watch-queued" {
				value["state"] = "queued"
				delete(value, "status")
			}
			if mode == "watch-missing-revisions" {
				delete(value, "revisions")
			}
		}
	case path == "/app/rest/builds":
		row := mockRun(482193)
		value = Object{"count": 1, "build": []Object{row}}
		if strings.Contains(locator, "snapshotDependency:") {
			row = mockRun(482188)
			row["buildTypeId"] = "Payments_IntegrationTests"
			row["buildType"] = Object{"id": "Payments_IntegrationTests", "projectId": "Payments"}
			value = Object{"count": 1, "build": []Object{row}}
			switch mode {
			case "dependencies-denied":
				deny(403)
				return
			case "dependencies-unsupported":
				deny(404)
				return
			case "dependencies-huge":
				row["statusText"] = strings.Repeat("x", 3000000)
			case "dependencies-malformed":
				value["count"] = 0
			}
		}
		if strings.HasPrefix(mode, "list-") {
			value["nextHref"] = mockNext(path, q, count, start)
			switch mode {
			case "list-empty":
				value["count"] = 0
				value["build"] = []Object{}
			case "list-verified-empty", "list-unverified-empty", "list-exhaustion-probe-denied":
				value = Object{"count": 0, "build": []Object{}}
			case "list-verified-malformed-next":
				value = Object{"count": 0, "build": []Object{}, "nextHref": nil}
			case "list-verified-cap-empty":
				value = Object{"count": 0, "build": []Object{}, "nextHref": strings.Replace(mockNext(path, q, count, start), "lookupLimit%3A5000", "lookupLimit%3A10000", 1)}
			case "list-unsafe":
				value["nextHref"] = "https://attacker.invalid/app/rest/builds"
			case "list-escalating":
				value["nextHref"] = strings.Replace(mockNext(path, q, count, start), "lookupLimit%3A5000", "lookupLimit%3A10000", 1)
			case "list-wrong-branch":
				row["branchName"] = "another-branch"
			case "list-unknown-result":
				row["status"] = "FUTURE_RESULT"
			case "list-duplicate":
				value = Object{"count": 2, "build": []Object{row, row}}
			case "list-verified-missing-revision":
				delete(row, "revisions")
				delete(value, "nextHref")
			case "list-unknown-candidates":
				a, b := mockRun(482193), mockRun(482194)
				b["status"] = "FUTURE_RESULT"
				if start > 0 {
					a["id"] = 482195
					a["status"] = "SUCCESS"
					b["id"] = 482196
					b["status"] = "UNKNOWN"
					b["canceledInfo"] = Object{"timestamp": "20261001T110000+0000"}
				}
				value["build"] = []Object{a, b}
				value["count"] = 2
			case "list-fractional":
				a, b := mockRun(482193), mockRun(482194)
				a["finishDate"] = "20261001T110000+0000"
				b["finishDate"] = "20261001T110001+0000"
				rows := []Object{a, b}
				matches := regexp.MustCompile(`finishDate:\(date:(\d{8}T\d{6}(?:\.\d{3})?\+0000),condition:(before|after)\)`).FindAllStringSubmatch(locator, -1)
				for _, match := range matches {
					threshold, _ := time.Parse("20060102T150405.999-0700", match[1])
					filtered := []Object{}
					for i, row := range rows {
						actual := time.Date(2026, 10, 1, 11, 0, 0, 500000000, time.UTC)
						if Int(row, "id") == 482194 {
							actual = time.Date(2026, 10, 1, 11, 0, 1, 0, time.UTC)
						}
						_ = i
						if (match[2] == "before" && actual.Before(threshold)) || (match[2] == "after" && actual.After(threshold)) {
							filtered = append(filtered, row)
						}
					}
					rows = filtered
				}
				value["build"] = rows
				value["count"] = len(rows)
			}
		}
	case strings.HasPrefix(path, "/app/rest/problemOccurrences") || strings.HasPrefix(path, "/app/rest/testOccurrences"):
		problem := strings.Contains(path, "problemOccurrences")
		key := "testOccurrence"
		items := []Object{}
		if problem {
			key = "problemOccurrence"
			for i := 1; i <= 2; i++ {
				items = append(items, Object{"id": fmt.Sprintf("build:(id:482193),problem:(id:%d)", i), "type": "TC_TESTS_FAILED", "identity": fmt.Sprintf("problem-%d", i), "details": "One test failed", "build": Object{"id": 482193}})
			}
		} else {
			for i := 0; i < 3; i++ {
				status := "FAILURE"
				if i == 2 {
					status = "IGNORED"
				}
				items = append(items, Object{"id": fmt.Sprintf("build:(id:482193),id:%d", 2000000000+i), "name": "PaymentServiceIT.shouldRefund", "status": status, "duration": 100, "muted": i == 1, "ignored": i == 2, "details": "Connection refused", "build": Object{"id": 482193}, "test": Object{"id": strconv.FormatInt(517450581327024597+int64(i), 10)}})
			}
		}
		switch mode {
		case "evidence-wrong-run":
			for _, item := range items {
				item["build"] = Object{"id": 482100}
			}
		case "evidence-duplicate-id":
			items[1]["id"] = items[0]["id"]
		case "evidence-unknown-test":
			items[0]["status"] = "FUTURE_RESULT"
		case "evidence-no-flags":
			delete(items[0], "muted")
			delete(items[0], "ignored")
		case "evidence-preview":
			items[1]["details"] = strings.Repeat("🦊", 2500)
		case "evidence-secret":
			items[0]["details"] = longNativeCanary()
		case "evidence-huge":
			items[0]["details"] = strings.Repeat("x", 3000000)
		}
		filtered := []Object{}
		for _, item := range items {
			if strings.Contains(locator, "status:FAILURE") && item["status"] != "FAILURE" {
				continue
			}
			if strings.Contains(locator, "muted:false") && item["muted"] == true {
				continue
			}
			if strings.Contains(locator, "muted:true") && item["muted"] != true {
				continue
			}
			filtered = append(filtered, item)
		}
		items = filtered
		if strings.HasSuffix(path, "Occurrences") {
			rows := items[min(start, len(items)):min(start+count, len(items))]
			if mode == "evidence-empty" {
				rows = []Object{}
			}
			value = Object{"count": len(rows), key: rows}
			if start+count <= len(items) || mode == "evidence-empty" {
				value["nextHref"] = mockNext(path, q, count, start)
			}
			if mode == "evidence-unsafe" {
				value["nextHref"] = "https://attacker.invalid/app/rest/testOccurrences"
			}
		} else {
			id := path[strings.LastIndex(path, "/")+1:]
			for _, item := range items {
				if item["id"] == id {
					value = item
					break
				}
			}
			if value == nil {
				deny(404)
				return
			}
			if mode == "evidence-wrong-id" {
				value["id"] = items[1]["id"]
			}
		}
	case path == "/app/rest/changes":
		if mode == "changes-denied" {
			deny(403)
			return
		}
		if mode == "changes-unsupported" {
			deny(404)
			return
		}
		items := []Object{mockChange(0, strings.Contains(fields, "files(")), mockChange(1, strings.Contains(fields, "files(")), mockChange(2, strings.Contains(fields, "files("))}
		switch mode {
		case "changes-wrong-root":
			items[0]["vcsRootInstance"] = Object{"vcs-root-id": "Foreign_Git"}
		case "changes-no-root":
			items[0]["vcsRootInstance"] = nil
		case "changes-secret":
			items[0]["comment"] = longNativeCanary()
		case "changes-preview":
			items[0]["comment"] = strings.Repeat("🦊", 2500) + "\nOther line"
		case "changes-huge":
			items[0]["comment"] = strings.Repeat("x", 3000000)
		case "changes-file-limit":
			files := []Object{}
			for i := 0; i < 101; i++ {
				files = append(files, Object{"file": "src/fixture.go"})
			}
			items[0]["files"] = Object{"count": 101, "file": files}
		}
		rows := items[min(start, len(items)):min(start+count, len(items))]
		value = Object{"count": len(rows), "change": rows}
		if start+count < len(items) {
			value["nextHref"] = mockNext(path, q, count, start)
		}
		if mode == "changes-unsafe" {
			value["nextHref"] = 42
		}
	case path == "/app/rest/buildTypes":
		if strings.Contains(fields, "builds($locator:") {
			value = m.statusWire(locator, mode)
			break
		}
		if mode == "jobs-denied" {
			deny(403)
			return
		}
		if mode == "jobs-unsupported" {
			deny(404)
			return
		}
		job := mockJob()
		value = Object{"count": 1, "buildType": []Object{job}}
		if strings.HasPrefix(mode, "jobs-") {
			value["nextHref"] = mockNext(path, q, count, start)
		}
		switch mode {
		case "jobs-empty":
			value = Object{"count": 0, "buildType": []Object{}}
		case "jobs-empty-next":
			value["count"] = 0
			value["buildType"] = []Object{}
		case "jobs-unsafe":
			value["nextHref"] = "https://attacker.invalid/app/rest/buildTypes"
		case "jobs-escalating":
			value["nextHref"] = strings.Replace(mockNext(path, q, count, start), "lookupLimit%3A5000", "lookupLimit%3A10000", 1)
		case "jobs-scope-change":
			value["nextHref"] = strings.Replace(mockNext(path, q, count, start), "UGF5bWVudHM", "Rm9yYmlkZGVu", 1)
		case "jobs-duplicate":
			value = Object{"count": 2, "buildType": []Object{job, job}}
		case "jobs-foreign":
			job["projectId"] = "Forbidden"
		case "jobs-leading-id":
			job["id"] = "--job"
			delete(value, "nextHref")
		case "jobs-malformed":
			value["count"] = 0
		case "jobs-unknown-paused":
			delete(job, "paused")
		case "jobs-secret":
			job["name"] = "fixture-only-token\x1b[31m"
		case "jobs-huge":
			job["name"] = strings.Repeat("x", 3000000)
		case "jobs-all-unknown", "jobs-oversized":
			rows := []Object{}
			for i := 0; i < count; i++ {
				item := mockJob()
				item["id"] = fmt.Sprintf("Payments_Job%d", i)
				if mode == "jobs-all-unknown" {
					delete(item, "paused")
				} else {
					item["name"] = strings.Repeat("🦊", 100)
				}
				rows = append(rows, item)
			}
			value = Object{"count": count, "buildType": rows}
		}
	case path == "/app/rest/buildQueue":
		if mode == "queue-denied" {
			deny(403)
			return
		}
		if mode == "queue-unsupported" {
			deny(404)
			return
		}
		row := mockQueue()
		value = Object{"count": 1, "build": []Object{row}}
		if mode == "queue-page" || mode == "queue-empty-next" || strings.HasPrefix(mode, "queue-unsafe") || mode == "queue-escalating" {
			value["nextHref"] = mockNext(path, q, count, start)
		}
		switch mode {
		case "queue-empty", "queue-empty-next":
			value["count"] = 0
			value["build"] = []Object{}
		case "queue-unsafe":
			value["nextHref"] = "https://attacker.invalid/app/rest/buildQueue"
		case "queue-unsafe-scope":
			value["nextHref"] = strings.Replace(mockNext(path, q, count, start), "UGF5bWVudHM", "Rm9yYmlkZGVu", 1)
		case "queue-escalating":
			value["nextHref"] = strings.Replace(mockNext(path, q, count, start), "lookupLimit%3A5000", "lookupLimit%3A10000", 1)
		case "queue-foreign":
			Obj(row["buildType"])["projectId"] = "Forbidden"
		case "queue-wrong-job":
			row["buildTypeId"] = "Other"
			Obj(row["buildType"])["id"] = "Other"
		case "queue-conflict":
			Obj(row["buildType"])["id"] = "Other"
		case "queue-control-id", "queue-bidi-id":
			id := "bad\u0085job"
			if mode == "queue-bidi-id" {
				id = "bad\u202ejob"
			}
			row["buildTypeId"] = id
			Obj(row["buildType"])["id"] = id
		case "queue-surrogate-id":
			row["buildTypeId"] = "bad-surrogate-job"
			Obj(row["buildType"])["id"] = "bad-surrogate-job"
		case "queue-duplicate":
			value = Object{"count": 2, "build": []Object{row, row}}
		case "queue-malformed":
			value["count"] = 0
		case "queue-unknown":
			row["state"] = "future-state"
		case "queue-running":
			row["state"] = "running"
		case "queue-finished":
			row["state"] = "finished"
		case "queue-no-reason":
			delete(row, "waitReason")
		case "queue-secret":
			row["branchName"] = "fixture-only-token"
			row["waitReason"] = "fixture-only-token\x1b[31m"
		case "queue-bad-date":
			row["queuedDate"] = "20260230T140000+0000"
		case "queue-huge":
			row["waitReason"] = strings.Repeat("x", 3000000)
		case "queue-oversized":
			row["waitReason"] = strings.Repeat("🦊", 1000)
		case "queue-many-unknown":
			rows := []Object{}
			for i := 0; i < count; i++ {
				item := cloneMock(row)
				item["id"] = 482194 + i
				item["state"] = "future-state"
				item["queuedDate"] = "invalid"
				rows = append(rows, item)
			}
			value = Object{"count": count, "build": rows}
		}
	case strings.HasPrefix(path, "/app/rest/agents"):
		if mode == "agent-denied" {
			deny(403)
			return
		}
		if mode == "agent-missing" || mode == "agent-unsupported" {
			deny(404)
			return
		}
		agent := mockAgent()
		switch mode {
		case "agent-idle":
			agent["build"] = nil
		case "agent-state-mix":
			agent["enabled"] = false
			agent["authorized"] = false
			agent["build"] = nil
		case "agent-zero-pool":
			Obj(agent["pool"])["id"] = 0
		case "agent-wrong-pool":
			Obj(agent["pool"])["id"] = 2
		case "agent-wrong-id":
			agent["id"] = 8
		case "agent-malformed-id":
			agent["id"] = 0
		case "agent-unknown":
			delete(agent, "connected")
			agent["enabled"] = nil
			delete(agent, "authorized")
			delete(agent, "pool")
		case "agent-bad-state":
			agent["enabled"] = "true"
		case "agent-secret":
			agent["name"] = "fixture-only-token\x1b[31m"
			Obj(agent["pool"])["name"] = "fixture-only-token"
		case "agent-huge":
			agent["name"] = strings.Repeat("x", 3000000)
		case "agent-oversized":
			agent["name"] = strings.Repeat("🦊", 1000)
		}
		if containsString([]string{"agent-active", "agent-foreign-active", "agent-conflict-active", "agent-policy-hang"}, mode) {
			project := "Payments"
			if mode == "agent-foreign-active" {
				project = "Forbidden"
			}
			if mode == "agent-policy-hang" {
				project = "Payments_Child"
			}
			agent["build"] = Object{"id": 482193, "buildTypeId": "Payments_Build", "buildType": Object{"id": "Payments_Build", "projectId": project}}
			if mode == "agent-conflict-active" {
				Obj(Obj(agent["build"])["buildType"])["id"] = "Other"
			}
		}
		if strings.Contains(path, "/id:") {
			value = agent
		} else {
			value = Object{"count": 1, "agent": []Object{agent}}
			if mode == "agent-page" || mode == "agent-slow-page" || mode == "agent-empty-next" || strings.HasPrefix(mode, "agent-unsafe") {
				value["nextHref"] = mockNext(path, q, count, start)
			}
			switch mode {
			case "agent-empty", "agent-empty-next":
				value["count"] = 0
				value["agent"] = []Object{}
			case "agent-unsafe":
				value["nextHref"] = "https://attacker.invalid/app/rest/agents"
			case "agent-unsafe-scope":
				value["nextHref"] = strings.Replace(mockNext(path, q, count, start), "UGF5bWVudHM", "Rm9yYmlkZGVu", 1)
			case "agent-unsafe-filter":
				value["nextHref"] = strings.Replace(mockNext(path, q, count, start), "defaultFilter%3Afalse", "defaultFilter%3Atrue", 1)
			case "agent-duplicate":
				value = Object{"count": 2, "agent": []Object{agent, agent}}
			case "agent-malformed-page":
				value["count"] = 0
			case "agent-many-unknown":
				rows := []Object{}
				for i := 0; i < count; i++ {
					rows = append(rows, Object{"id": i + 1, "name": fmt.Sprintf("Synthetic agent %d", i)})
				}
				value = Object{"count": count, "agent": rows}
			}
		}
		if mode == "agent-slow-page" && path == "/app/rest/agents" {
			select {
			case <-time.After(time.Second):
			case <-r.Context().Done():
				return
			}
		}
	case path == "/app/messages":
		messages := []Object{mockMessage()}
		if mode == "log-window" {
			messages[0]["text"] = strings.Repeat("x", 2500) + "literal[needle]"
			messages = append(messages, Object{"id": 13, "text": "plain", "level": 0, "status": 4, "timestamp": "2026-10-01T14:00:01Z"})
		}
		if mode == "log-overdelivery" {
			messages = []Object{}
			for i := 0; i < 81; i++ {
				item := mockMessage()
				item["id"] = i
				item["text"] = "plain"
				messages = append(messages, item)
			}
		}
		last := 12
		if len(messages) > 1 {
			last = Int(messages[len(messages)-1], "id")
		}
		value = Object{"messages": messages, "lastMessageIncluded": true, "lastMessageIndex": last, "focusIndex": last}
	default:
		deny(404)
		return
	}
	if strings.HasPrefix(mode, "outcome-") {
		exceptional := mockRun(482193)
		switch mode {
		case "outcome-canceled":
			exceptional["status"] = "UNKNOWN"
			exceptional["canceledInfo"] = Object{"timestamp": "20261001T110000+0000", "text": "Private cancellation comment", "user": Object{"username": "Private actor"}}
		case "outcome-failed-to-start":
			exceptional["failedToStart"] = true
		case "outcome-composite":
			exceptional["composite"] = true
			exceptional["status"] = "SUCCESS"
		case "outcome-missing", "outcome-many-missing":
			delete(exceptional, "failedToStart")
		}
		if path == "/app/rest/builds/id:482193" && fields != "id,snapshot-dependencies(count)" {
			value = exceptional
		}
		if path == "/app/rest/builds" && !strings.Contains(locator, "snapshotDependency:") {
			value = Object{"count": 1, "build": []Object{exceptional}}
		}
		if mode == "outcome-many-missing" && path == "/app/rest/builds" {
			rows := []Object{}
			for i := 0; i < 100; i++ {
				item := cloneMock(exceptional)
				item["id"] = 482193 + i
				rows = append(rows, item)
			}
			value = Object{"count": 100, "build": rows}
		}
	}
	if mode == "queue-surrogate-id" && path == "/app/rest/buildQueue" {
		encoded, _ := json.Marshal(value)
		fmt.Fprint(w, strings.ReplaceAll(string(encoded), "bad-surrogate-job", `bad\ud800job`))
		return
	}
	send(value)
}

func (m *Server) statusWire(locator, mode string) Object {
	m.mu.Lock()
	revision, branch, project, root := m.revision, m.branch, m.project, m.root
	m.mu.Unlock()
	matches := regexp.MustCompile(`item:\(id:\(\$base64:([^)]*)\)\)`).FindAllStringSubmatch(locator, -1)
	jobs := []Object{}
	for index, match := range matches {
		data, _ := base64.RawURLEncoding.DecodeString(match[1])
		id := string(data)
		run := mockRun(482193 + index)
		run["buildTypeId"] = id
		run["buildType"] = Object{"id": id, "projectId": project}
		run["branchName"] = branch
		run["status"] = "SUCCESS"
		run["revisions"] = Object{"revision": []Object{{"version": revision, "vcs-root-instance": Object{"id": "17", "vcs-root-id": root}}}}
		jobs = append(jobs, Object{"id": id, "name": id, "projectId": project, "paused": false, "builds": Object{"count": 1, "build": []Object{run}}})
	}
	if len(jobs) > 0 {
		first := jobs[0]
		page := Obj(first["builds"])
		selected := Objects(page["build"])[0]
		switch mode {
		case "status-red":
			selected["status"] = "FAILURE"
		case "status-canceled":
			selected["status"] = "UNKNOWN"
			selected["canceledInfo"] = Object{"timestamp": "20261001T110000+0000"}
		case "status-failed-to-start":
			selected["status"] = "FAILURE"
			selected["failedToStart"] = true
		case "status-missing-outcome":
			delete(selected, "failedToStart")
		case "status-composite":
			selected["composite"] = true
		case "status-stale":
			Obj(Objects(Obj(selected["revisions"])["revision"])[0])["version"] = strings.Repeat("b", 40)
		case "status-unknown":
			delete(selected, "revisions")
		case "status-personal":
			selected["personal"] = true
		case "status-newer-unknown":
			newer := cloneMock(selected)
			newer["id"] = Int(selected, "id") + 100
			newer["revisions"] = Object{"revision": []Object{}}
			page["build"] = []Object{newer, selected}
			page["count"] = 2
		case "status-running":
			selected["state"] = "running"
		case "status-queued":
			selected["state"] = "queued"
			delete(selected, "status")
		case "status-multi-root":
			revisions := Obj(selected["revisions"])
			revisions["revision"] = append(Objects(revisions["revision"]), Object{"version": strings.Repeat("c", 40), "vcs-root-instance": Object{"id": "18", "vcs-root-id": "Other_Git"}})
		case "status-missing-job":
			jobs = jobs[:len(jobs)-1]
		case "status-foreign":
			first["projectId"] = "Forbidden"
			Obj(selected["buildType"])["projectId"] = "Forbidden"
		case "status-wrong-run":
			selected["buildTypeId"] = "Foreign_Job"
		case "status-unsafe-continuation":
			page["nextHref"] = "https://attacker.invalid/steal"
		case "status-huge":
			revisions := []Object{}
			for i := 0; i < 100; i++ {
				revisions = append(revisions, Object{"version": strings.Repeat("v", 256), "vcs-root-instance": Object{"id": strconv.Itoa(i + 1), "vcs-root-id": strings.Repeat("r", 245) + strconv.Itoa(i)}})
			}
			selected["revisions"] = Object{"revision": revisions}
		}
	}
	return Object{"count": len(jobs), "buildType": jobs}
}
