package livefixture

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Periecle/teamcity-axi/internal/axi"
)

func contractForTest(t *testing.T) Contract {
	t.Helper()
	c, e := ReadContract()
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func require(t *testing.T, ok bool, message string) {
	t.Helper()
	if !ok {
		t.Fatal(message)
	}
}
func captureBody(t *testing.T, c Contract, name string) axi.Object {
	t.Helper()
	v, e := body(c.Records, name)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func cloneRecords(c Contract) map[string]Result {
	r := map[string]Result{}
	for k, v := range c.Records {
		r[k] = v
	}
	return r
}
func modifyRecord(t *testing.T, records map[string]Result, name string, replace func(axi.Object) axi.Object) {
	t.Helper()
	record := records[name]
	split := strings.Index(record.Stdout, "\n\n")
	value, e := axi.DecodeJSON([]byte(record.Stdout[split+2:]))
	if e != nil {
		t.Fatal(e)
	}
	raw, e := json.Marshal(replace(axi.Obj(value)))
	if e != nil {
		t.Fatal(e)
	}
	record.Stdout = record.Stdout[:split+2] + string(raw)
	records[name] = record
}
func patched(o, patch axi.Object) axi.Object {
	r := axi.Object{}
	for k, v := range o {
		r[k] = v
	}
	for k, v := range patch {
		r[k] = v
	}
	return r
}
func TestLiveRecorderDecodedCredentialsAndJSONGoEscapes(t *testing.T) {
	secret := "fixture-canary\"slash\\end<&"
	raw, _ := json.Marshal(axi.Object{"statusText": secret})
	wire := string(raw)
	for _, payload := range []string{wire, strings.ReplaceAll(strings.ReplaceAll(wire, "<", `\u003c`), "&", `\u0026`)} {
		clean, e := SanitizeStdout("HTTP/1.1 200 OK\nContent-Type: application/json\nSet-Cookie: another-private-session\n\n"+payload, []string{secret})
		if e != nil {
			t.Fatal(e)
		}
		var dto axi.Object
		if e = json.Unmarshal([]byte(strings.SplitN(clean, "\n\n", 2)[1]), &dto); e != nil {
			t.Fatal(e)
		}
		require(t, dto["statusText"] == "[REDACTED]", "decoded secret not redacted")
		require(t, !strings.Contains(clean, "another-private-session"), "cookie leaked")
	}
	raw, _ = json.Marshal(axi.Object{"text": secret[:10] + "\x1b[31m" + secret[10:]})
	clean, e := SanitizeStdout(string(raw), []string{secret})
	if e != nil {
		t.Fatal(e)
	}
	var dto axi.Object
	if e = json.Unmarshal([]byte(clean), &dto); e != nil {
		t.Fatal(e)
	}
	require(t, dto["text"] == "[REDACTED]", "decorated secret not redacted")
	clean, e = SanitizeStdout("<html>credential-encoded</html>", nil)
	if e != nil {
		t.Fatal(e)
	}
	require(t, !strings.Contains(clean, "credential-encoded"), "unparseable sensitive payload leaked")
}
func TestLiveSessionRedactionPreservesJSONAndContinuation(t *testing.T) {
	input := `{"nextHref":"/app/rest/builds;TCSESSIONID=fixture-session","name":"original"}`
	clean, e := SanitizeStdout(input, nil)
	if e != nil {
		t.Fatal(e)
	}
	var dto axi.Object
	if e = json.Unmarshal([]byte(clean), &dto); e != nil {
		t.Fatal(e)
	}
	require(t, dto["nextHref"] == "/app/rest/builds;TCSESSIONID=[REDACTED]", "wrong session redaction")
	require(t, dto["name"] == "original", "unrelated text changed")
}
func TestLiveRecorderRejectsRunIdentityProjectResultReuse(t *testing.T) {
	c := contractForTest(t)
	if e := VerifyCapture(c.Records, c); e != nil {
		t.Fatal(e)
	}
	for _, entry := range []struct {
		name  string
		patch axi.Object
	}{{"run-detail", axi.Object{"buildTypeId": "OtherJob"}}, {"green", axi.Object{"buildType": axi.Object{"id": "OtherJob", "projectId": "AxiContract"}}}, {"run-detail", axi.Object{"buildType": axi.Object{"id": "AxiContract_Fail", "projectId": "OtherProject"}}}, {"green", axi.Object{"status": "FAILURE"}}, {"run-detail", axi.Object{"state": "running"}}} {
		records := cloneRecords(c)
		modifyRecord(t, records, entry.name, func(v axi.Object) axi.Object { return patched(v, entry.patch) })
		e := VerifyCapture(records, c)
		require(t, e != nil && strings.Contains(e.Error(), "identity or outcome"), "run reattribution accepted")
	}
}
func TestLiveFIFOCredentialsFailPromptlyBeforeNativeLaunch(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "credentials")
	if e := syscall.Mkfifo(fifo, 0600); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	_, e := New(fifo, filepath.Join(t.TempDir(), "not-an-executable"), "")
	require(t, e != nil && e.Error() == "Live credential configuration must be an owned private regular file", "FIFO credential accepted")
	require(t, time.Since(start) < 2*time.Second, "FIFO credential blocked")
}
func TestLiveRecorderRejectsJobIdentityProjectPausedChanges(t *testing.T) {
	c := contractForTest(t)
	for _, patch := range []axi.Object{{"id": "OtherJob"}, {"projectId": "OtherProject"}, {"paused": true}} {
		records := cloneRecords(c)
		modifyRecord(t, records, "bounded-jobs", func(v axi.Object) axi.Object {
			jobs := axi.Objects(v["buildType"])
			jobs[0] = patched(jobs[0], patch)
			v["buildType"] = jobs
			return v
		})
		e := VerifyCapture(records, c)
		require(t, e != nil && strings.Contains(e.Error(), "job page identity or scope"), "job reattribution accepted")
	}
}
func TestLiveRecorderRejectsQueueIDScopeAndStateReuse(t *testing.T) {
	c := contractForTest(t)
	for _, patch := range []axi.Object{{"id": 999}, {"buildTypeId": "OtherJob"}, {"state": "finished"}, {"buildType": axi.Object{"id": "AxiContract_QueueA", "projectId": "Forbidden"}}} {
		records := cloneRecords(c)
		modifyRecord(t, records, "queue-project-positive", func(v axi.Object) axi.Object {
			builds := axi.Objects(v["build"])
			builds[0] = patched(builds[0], patch)
			v["build"] = builds
			return v
		})
		e := VerifyCapture(records, c)
		require(t, e != nil && strings.Contains(e.Error(), "queue identity, scope or lifecycle"), "queue reattribution accepted")
	}
}
func TestLiveRecorderRejectsAgentReusePoolAvailabilityAndFalseEmpty(t *testing.T) {
	c := contractForTest(t)
	for _, entry := range []struct {
		name    string
		replace func(axi.Object) axi.Object
	}{{"agent-detail", func(v axi.Object) axi.Object { return patched(v, axi.Object{"id": 99}) }}, {"agents-project", func(v axi.Object) axi.Object {
		return patched(v, axi.Object{"agent": []axi.Object{patched(axi.Objects(v["agent"])[0], axi.Object{"pool": axi.Object{"id": 99}})}})
	}}, {"agent-detail-job", func(v axi.Object) axi.Object { return patched(v, axi.Object{"authorized": false}) }}, {"agents-impossible", func(v axi.Object) axi.Object { return patched(v, axi.Object{"agent": []axi.Object{{"id": 1}}}) }}} {
		records := cloneRecords(c)
		modifyRecord(t, records, entry.name, entry.replace)
		require(t, VerifyCapture(records, c) != nil, "agent invalid capture accepted")
	}
}
func TestLiveLifecyclePermissionInventoryCannotBroaden(t *testing.T) {
	c := contractForTest(t)
	for _, patch := range []axi.Object{{"isGlobalScope": true}, {"permission": axi.Object{"id": "run_build"}}, {"project": axi.Object{"id": "AxiDenied"}}} {
		records := cloneRecords(c)
		modifyRecord(t, records, "outcome-permissions", func(v axi.Object) axi.Object {
			rows := axi.Objects(v["permissionAssignment"])
			for i, row := range rows {
				if axi.Str(axi.Obj(row["project"]), "id") == axi.Str(axi.Obj(c.Fixture["lifecycle"]), "projectId") {
					rows[i] = patched(row, patch)
				}
			}
			v["permissionAssignment"] = rows
			return v
		})
		e := VerifyCapture(records, c)
		require(t, e != nil && strings.Contains(e.Error(), "restricted fixture permission inventory"), "permission inventory broadened")
	}
}
func TestLiveCapturedRestrictedIdentityAndFailedGreenDTOs(t *testing.T) {
	c := contractForTest(t)
	server := captureBody(t, c, "server")
	require(t, identity(server["buildNumber"]) == identity(c.Server["buildNumber"]), "server build changed")
	permissions := axi.Objects(captureBody(t, c, "permissions")["permissionAssignment"])
	ids := []string{}
	for _, row := range permissions {
		id := axi.Str(axi.Obj(row["permission"]), "id")
		ids = append(ids, id)
		if id == "view_project" {
			project := axi.Str(axi.Obj(row["project"]), "id")
			require(t, row["isGlobalScope"] == false && (project == "AxiContract" || project == "_Root"), "permission outside recorded scope")
		}
	}
	require(t, sameStrings(ids, []string{"change_own_profile", "view_project", "view_project"}), "permission inventory changed")
	for _, entry := range []struct{ name, id, result string }{{"outcome-normal-failed", "1", "failure"}, {"outcome-normal-green", "2", "success"}} {
		run, project, notes, e := axi.NormalizeRun(captureBody(t, c, entry.name), "http://127.0.0.1:32768", nil)
		if e != nil {
			t.Fatal(e)
		}
		require(t, run["id"] == entry.id && run["result"] == entry.result && project == "AxiContract", "run normalization changed")
		require(t, len(axi.Objects(run["revisions"])) == 0 && len(notes) == 0, "unexpected missing metadata")
	}
	for _, entry := range []struct{ name, code string }{{"denied", "PERMISSION_DENIED"}, {"missing", "NOT_FOUND"}, {"invalid-auth", "AUTH_REQUIRED"}, {"dependencies-unsupported", "UPSTREAM_FAILURE"}} {
		_, e := body(c.Records, entry.name)
		require(t, e != nil && axi.AsDomainError(e).Code == entry.code, "wrong error classification")
	}
}
func TestLiveCapturedEncodedContinuationAndFinishFilters(t *testing.T) {
	c := contractForTest(t)
	for _, name := range []string{"pages", "encoded-page"} {
		record := c.Records[name]
		u, e := urlParse(record.Args[1])
		if e != nil {
			t.Fatal(e)
		}
		filters := []string{"buildType:(id:AxiContract_Fail)", "defaultFilter:false"}
		if name == "encoded-page" {
			filters = []string{"buildType:(id:($base64:QXhpQ29udHJhY3RfRmFpbA))", "defaultFilter:false", "state:finished", "finishDate:(date:20261001T000000+0000,condition:after)", "finishDate:(date:20261003T000000+0000,condition:before)"}
		}
		position, e := axi.NextPosition(axi.Str(captureBody(t, c, name), "nextHref"), axi.ContinuationRequest{ServerURL: "http://127.0.0.1:32768", Resource: "builds", Filters: filters, Fields: u, Count: 1, ScanLimit: 5000})
		if e != nil {
			t.Fatal(e)
		}
		require(t, position == 1, "wrong next position")
	}
	empty := captureBody(t, c, "empty-page")
	require(t, len(empty) == 2 && integer(empty["count"]) == 0 && len(axi.Objects(empty["build"])) == 0, "empty capture changed")
}
func TestLiveCapturedEvidenceAndDistinctStructuredLogShape(t *testing.T) {
	c := contractForTest(t)
	require(t, integer(captureBody(t, c, "problems")["count"]) == 3, "problem count changed")
	require(t, axi.Objects(captureBody(t, c, "tests")["testOccurrence"])[0]["id"] == "build:(id:1),id:2000000000", "occurrence identity changed")
	for _, entry := range []struct{ name, key string }{{"dependencies", "build"}, {"changes", "change"}, {"queue", "build"}} {
		value := captureBody(t, c, entry.name)
		require(t, len(value) == 2 && integer(value["count"]) == 0 && len(axi.Objects(value[entry.key])) == 0, "empty evidence changed")
	}
	require(t, identity(axi.Obj(axi.Objects(captureBody(t, c, "agents")["agent"])[0]["pool"])["id"]) == "0", "default pool changed")
	log, e := axi.DecodeJSON([]byte(c.Records["log"].Stdout))
	if e != nil {
		t.Fatal(e)
	}
	dto := axi.Obj(log)
	rows := axi.Objects(dto["messages"])
	require(t, dto["run_id"] == "1" && len(rows) == 21 && integer(rows[0]["id"]) == 35, "native log window changed")
	require(t, strings.HasSuffix(axi.Str(rows[0], "timestamp"), "+0000"), "log timestamp changed")
}

func urlParse(path string) (string, error) {
	u, e := url.Parse(path)
	if e != nil {
		return "", e
	}
	return u.Query().Get("fields"), nil
}

type pipeReader struct{ reader io.Reader }

func (r pipeReader) Read(p []byte) (int, error) { return r.reader.Read(p) }
func TestLiveCaptureBoundAppliesToIOCopy(t *testing.T) {
	canceled := false
	buffer := &captureBuffer{limit: 32, overflow: func() { canceled = true }}
	_, e := io.Copy(buffer, pipeReader{strings.NewReader(strings.Repeat("sensitive", 100))})
	require(t, errors.Is(e, io.ErrShortBuffer), "capture overflow bypassed Write through ReaderFrom")
	require(t, canceled && buffer.exceeded && buffer.Len() <= buffer.limit, "capture ceiling did not stop output")
}
