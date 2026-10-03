//go:build realcli

package nativecontract_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Periecle/teamcity-axi/internal/nativefixture"
)

func obj(v any) map[string]any { o, _ := v.(map[string]any); return o }
func TestReleasedNativeProtocolAndSummaryOmission(t *testing.T) {
	result, err := nativefixture.Capture(context.Background(), os.Getenv("TEAMCITY_AXI_TEST_BINARY"), "../../docs/compatibility.json")
	if err != nil {
		t.Fatal(err)
	}
	probes := obj(result["probes"])
	if obj(probes["version"])["stdout"] != "teamcity version 1.5.0\n" {
		t.Fatal("Pinned native version response changed")
	}
	for _, name := range []string{"version", "help"} {
		if len(obj(probes[name])["requests"].([]any)) != 0 {
			t.Fatal("local probe performed HTTP")
		}
	}
	records := obj(result["records"])
	operations, _, err := nativefixture.Operations()
	if err != nil {
		t.Fatal(err)
	}
	operationPaths := map[string]*url.URL{}
	for _, op := range operations {
		if len(op.Argv) > 1 && op.Argv[0] == "api" {
			u, e := url.Parse(op.Argv[1])
			if e != nil {
				t.Fatal(e)
			}
			operationPaths[op.Name] = u
		}
	}
	for name, raw := range records {
		record := obj(raw)
		for _, channel := range []string{"stdout", "stderr"} {
			if strings.Contains(record[channel].(string), "fixture-only-token") {
				t.Fatalf("Fixture credential exposed in %s %s", name, channel)
			}
		}
		for _, request := range record["requests"].([]any) {
			q := obj(request)
			if q["method"] != "GET" || !strings.HasPrefix(q["path"].(string), "/teamcity/") || q["authenticated"] != true {
				t.Fatalf("native transport changed for %s", name)
			}
		}
	}
	for _, name := range []string{"run-view", "run-list", "problems", "tests", "dependencies", "changes", "jobs", "queue", "agents"} {
		record := obj(records[name])
		if record["code"] != float64(0) || !strings.HasPrefix(record["stdout"].(string), "HTTP/1.1 200 OK\n") || len(record["requests"].([]any)) != 1 {
			t.Fatalf("native contract failed for %s", name)
		}
		request := obj(record["requests"].([]any)[0])
		path := operationPaths[name]
		if request["path"] != "/teamcity"+path.Path {
			t.Fatal("context prefix or native path changed")
		}
		expected := map[string]any{}
		for key, values := range path.Query() {
			if len(values) != 1 {
				t.Fatal("ambiguous fixture query")
			}
			expected[key] = values[0]
		}
		if !reflect.DeepEqual(obj(request["query"]), expected) {
			t.Fatalf("native query changed for %s", name)
		}
	}
	for mode, status := range map[string]string{"denied": "403", "missing": "404", "expired": "401"} {
		r := obj(records["error-"+mode])
		if r["code"] == float64(0) || !strings.HasPrefix(r["stdout"].(string), "HTTP/1.1 "+status+" ") {
			t.Fatal("independent error hidden", mode)
		}
	}
	omitted := obj(records["summary-denied"])
	var body map[string]any
	if json.Unmarshal([]byte(omitted["stdout"].(string)), &body) != nil || omitted["code"] != float64(0) {
		t.Fatal("native summary behavior changed")
	}
	if _, ok := body["failed_tests"]; ok {
		t.Fatal("summary no longer omits denied test source")
	}
	readTests := false
	for _, raw := range omitted["requests"].([]any) {
		if strings.HasSuffix(obj(raw)["path"].(string), "/testOccurrences") {
			readTests = true
		}
	}
	if !readTests {
		t.Fatal("native summary did not attempt independent test read")
	}
	unsupported := obj(records["logs-unsupported"])
	if unsupported["code"] == float64(0) {
		t.Fatal("unsupported structured log became success")
	}
	for _, raw := range unsupported["requests"].([]any) {
		if strings.Contains(obj(raw)["path"].(string), "downloadBuildLog") {
			t.Fatal("unbounded full-log fallback")
		}
	}
}
