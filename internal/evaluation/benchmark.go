package evaluation

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Periecle/teamcity-axi/internal/axi"
	"github.com/Periecle/teamcity-axi/internal/testfixture"
	toon "github.com/toon-format/toon-go"
)

const detailFields = "id,buildTypeId,state,status,failedToStart,canceledInfo(timestamp),branchName,personal,composite,buildType(id,projectId),revisions(revision(version,vcs-root-instance(id,vcs-root-id)))"
const problemFields = "id,type,identity,details,build(id)"
const testFields = "id,name,status,muted,ignored,details,build(id),test(id)"

type BenchmarkOptions struct {
	Repository, Binary, Wrapper, StartupProbe string
	StartupProbeArgs                          []string
	Repetitions                               int
}

type corpus struct {
	ID    string   `json:"id"`
	Tasks []Object `json:"tasks"`
}

func fileDigest(path string) (string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(bytes)
	return hex.EncodeToString(digest[:]), nil
}

func verifiedNative(repository, binary string) (Object, string, error) {
	if binary == "" {
		return nil, "", fmt.Errorf("TEAMCITY_AXI_TEST_BINARY must name the pinned native executable; no skips")
	}
	bytes, err := os.ReadFile(filepath.Join(repository, "docs", "compatibility.json"))
	if err != nil {
		return nil, "", err
	}
	var manifest Object
	if err := json.Unmarshal(bytes, &manifest); err != nil {
		return nil, "", err
	}
	digest, err := fileDigest(binary)
	if err != nil {
		return nil, "", err
	}
	for _, artifact := range axi.Objects(manifest["artifacts"]) {
		if axi.Str(artifact, "binarySha256") == digest && axi.Bool(artifact, "executionTested") {
			return manifest, digest, nil
		}
	}
	return nil, "", fmt.Errorf("Evaluation requires the checksum-verified, execution-tested native artifact")
}

func api(path string) []string {
	return []string{"api", path, "-X", "GET", "--raw", "-H", "Accept: application/json", "--no-input"}
}

func pagePath(resource, locator, fields string) string {
	return "/app/rest/" + resource + "?" + url.Values{"locator": {locator}, "fields": {fields}}.Encode()
}

func valueString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func hasCycle(edges []string) bool {
	active, complete := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if active[id] {
			return true
		}
		if complete[id] {
			return false
		}
		active[id] = true
		for _, edge := range edges {
			if strings.HasPrefix(edge, id+">") && visit(strings.SplitN(edge, ">", 2)[1]) {
				return true
			}
		}
		delete(active, id)
		complete[id] = true
		return false
	}
	for _, edge := range edges {
		if visit(strings.SplitN(edge, ">", 2)[0]) {
			return true
		}
	}
	return false
}

func WrapperEvidence(document axi.Response) Object {
	data := document.Data
	findings, sources := axi.Objects(data["findings"]), axi.Objects(data["sources"])
	graph, run := axi.Obj(data["graph"]), axi.Obj(data["run"])
	nodeIDs, edges, tests, problems, unavailable, empty, boundary := []string{}, []string{}, []string{}, []string{}, []string{}, []string{}, []string{}
	jobs := Object{}
	for _, node := range axi.Objects(graph["nodes"]) {
		execution := axi.Obj(node["run"])
		id := axi.Str(execution, "id")
		nodeIDs, jobs[id] = append(nodeIDs, id), execution["jobId"]
		if axi.Str(node, "expansion") == "depth_limit" {
			boundary = append(boundary, id)
		}
	}
	for _, edge := range axi.Objects(graph["edges"]) {
		edges = append(edges, axi.Str(edge, "fromRunId")+">"+axi.Str(edge, "toRunId"))
	}
	for _, finding := range findings {
		if axi.Str(finding, "kind") == "build_problem" {
			problems = append(problems, axi.Str(finding, "runId"))
		}
		for _, evidence := range axi.Objects(finding["evidence"]) {
			if axi.Str(evidence, "kind") == "test" {
				tests = append(tests, axi.Str(evidence, "itemId"))
			}
		}
	}
	for _, source := range sources {
		id := axi.Str(source, "kind") + ":" + axi.Str(source, "runId")
		if axi.Str(source, "state") == "unavailable" {
			unavailable = append(unavailable, id)
		}
		if axi.Str(source, "state") == "complete" && axi.Int(source, "returned") == 0 {
			empty = append(empty, id)
		}
	}
	return Object{"runId": run["id"], "state": run["state"], "jobId": run["jobId"], "projectId": document.Context["project"], "nodeJobs": jobs, "identityIssues": EvidenceIdentityIssues(findings, sources), "complete": document.Meta["complete"], "result": run["result"], "nodeIds": nodeIDs, "edges": edges, "cycle": len(axi.Objects(graph["cycles"])) > 0, "testIds": tests, "problemRunIds": problems, "unavailableSources": unavailable, "emptySources": empty, "boundaryRunIds": boundary, "findings": len(findings)}
}

type nativeGraph struct {
	nodes    map[string]Object
	order    []string
	edges    []string
	boundary []string
	result   string
}

func selectedNative(task Object, call func(string, string, []string) (Call, error)) (nativeGraph, error) {
	root, err := call("run", "482193", api("/app/rest/builds/id:482193?fields="+detailFields))
	if err != nil {
		return nativeGraph{}, err
	}
	graph := nativeGraph{nodes: map[string]Object{"482193": root.Body}, order: []string{"482193"}, edges: []string{}, boundary: []string{}, result: "unknown"}
	if axi.Str(root.Body, "status") == "SUCCESS" {
		graph.result = "success"
	} else if axi.Str(root.Body, "status") == "FAILURE" {
		graph.result = "failure"
	}
	if graph.result != "failure" {
		return graph, nil
	}
	batch := func(requests []func() error) error {
		for offset := 0; offset < len(requests); offset += 3 {
			var wait sync.WaitGroup
			errors := make([]error, min(3, len(requests)-offset))
			for index, request := range requests[offset:min(offset+3, len(requests))] {
				wait.Add(1)
				go func() { defer wait.Done(); errors[index] = request() }()
			}
			wait.Wait()
			for _, err := range errors {
				if err != nil {
					return err
				}
			}
		}
		return nil
	}
	frontier := []string{"482193"}
	var graphMu sync.Mutex
	for depth := 0; len(frontier) != 0 && depth <= axi.Int(task, "depth"); depth++ {
		if depth == axi.Int(task, "depth") {
			graph.boundary = append(graph.boundary, frontier...)
			break
		}
		next, requests := []string{}, []func() error{}
		for _, id := range frontier {
			requests = append(requests, func() error {
				count, err := call("dependency-count", id, api("/app/rest/builds/id:"+id+"?fields=id,snapshot-dependencies(count)"))
				if err != nil || axi.Int(axi.Obj(count.Body["snapshot-dependencies"]), "count") == 0 {
					return err
				}
				page, err := call("dependencies", id, api(pagePath("builds", "snapshotDependency:(to:(id:"+id+"),recursive:false),defaultFilter:false,count:100,start:0,lookupLimit:5000", "count,nextHref,build("+detailFields+")")))
				if err != nil {
					return err
				}
				graphMu.Lock()
				defer graphMu.Unlock()
				for _, node := range axi.Objects(page.Body["build"]) {
					child := valueString(node["id"])
					graph.edges = append(graph.edges, id+">"+child)
					if _, known := graph.nodes[child]; !known {
						graph.nodes[child] = node
						graph.order, next = append(graph.order, child), append(next, child)
					}
				}
				return nil
			})
		}
		if err := batch(requests); err != nil {
			return graph, err
		}
		sort.Slice(next, func(i, j int) bool { a, _ := strconv.Atoi(next[i]); b, _ := strconv.Atoi(next[j]); return a > b })
		frontier = next
	}
	requests := []func() error{}
	for _, id := range graph.order[:min(len(graph.order), axi.Int(task, "maxDiagnosedRuns"))] {
		requests = append(requests, func() error {
			_, err := call("problems", id, api(pagePath("problemOccurrences", "build:(id:"+id+"),count:20,start:0,lookupLimit:5000", "count,nextHref,problemOccurrence("+problemFields+")")))
			return err
		})
		for _, muted := range []bool{false, true} {
			requests = append(requests, func() error {
				_, err := call("tests", id, api(pagePath("testOccurrences", "build:(id:"+id+"),status:FAILURE,muted:"+strconv.FormatBool(muted)+",count:20,start:0,lookupLimit:5000", "count,nextHref,testOccurrence("+testFields+")")))
				return err
			})
		}
	}
	if err := batch(requests); err != nil {
		return graph, err
	}
	if _, err := call("changes", "482193", api(pagePath("changes", "build:(id:482193),count:10,start:0,lookupLimit:5000", "count,nextHref,change(id,version,comment,date,vcsRootInstance(vcs-root-id))"))); err != nil {
		return graph, err
	}
	if axi.Str(task, "mode") == "logs-needed" {
		if _, err := call("log", "482193", []string{"run", "log", "482193", "--tail", "80", "--json", "--no-input"}); err != nil {
			return graph, err
		}
	}
	return graph, nil
}

func nativeEvidence(calls []Call, graph nativeGraph) Object {
	tests, problems, unavailable, empty, issues := []string{}, []string{}, []string{}, []string{}, []string{}
	jobs := Object{}
	for id, node := range graph.nodes {
		jobs[id] = node["buildTypeId"]
	}
	for _, call := range calls {
		if call.Code != 0 {
			unavailable = append(unavailable, call.Kind+":"+call.RunID)
		}
		if call.Code == 0 && call.Body["count"] != nil && axi.Int(call.Body, "count") == 0 {
			empty = append(empty, call.Kind+":"+call.RunID)
		}
		if call.Kind == "tests" {
			for _, test := range axi.Objects(call.Body["testOccurrence"]) {
				tests = append(tests, axi.Str(test, "id"))
			}
		}
		if call.Kind == "problems" && len(axi.Objects(call.Body["problemOccurrence"])) != 0 {
			problems = append(problems, call.RunID)
		}
		rows := axi.Objects(call.Body["testOccurrence"])
		if call.Body["testOccurrence"] == nil {
			rows = axi.Objects(call.Body["problemOccurrence"])
		}
		for _, row := range rows {
			if valueString(axi.Obj(row["build"])["id"]) != call.RunID || !strings.HasPrefix(axi.Str(row, "id"), "build:(id:"+call.RunID+"),") {
				issues = append(issues, "inconsistent_occurrence_identity")
			}
		}
	}
	root := graph.nodes["482193"]
	return Object{"runId": valueString(root["id"]), "state": root["state"], "jobId": root["buildTypeId"], "projectId": axi.Obj(root["buildType"])["projectId"], "nodeJobs": jobs, "identityIssues": issues, "result": graph.result, "nodeIds": graph.order, "edges": graph.edges, "cycle": hasCycle(graph.edges), "testIds": tests, "problemRunIds": problems, "unavailableSources": unavailable, "emptySources": empty, "boundaryRunIds": graph.boundary, "findings": len(tests) + len(problems)}
}

func nativeDiagnosticsEvidence(body Object) Object {
	result := "unknown"
	if axi.Str(body, "status") == "SUCCESS" {
		result = "success"
	} else if axi.Str(body, "status") == "FAILURE" {
		result = "failure"
	}
	tests, problems := []string{}, []string{}
	for _, test := range axi.Objects(axi.Obj(body["failed_tests"])["testOccurrence"]) {
		tests = append(tests, axi.Str(test, "id"))
	}
	if len(axi.Objects(body["problems"])) != 0 {
		problems = append(problems, "482193")
	}
	return Object{"runId": body["run_id"], "result": result, "nodeIds": []string{valueString(body["run_id"])}, "testIds": tests, "problemRunIds": problems, "unavailableSources": []string{}, "emptySources": []string{}, "findings": len(problems) + len(tests)}
}

func decodeWrapper(stdout, format string) (axi.Response, error) {
	var value any
	var err error
	if format == "json" {
		err = json.Unmarshal([]byte(stdout), &value)
	} else {
		err = toon.Unmarshal([]byte(stdout), &value)
	}
	if err != nil {
		return axi.Response{}, err
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		return axi.Response{}, err
	}
	var document axi.Response
	if err := json.Unmarshal(bytes, &document); err != nil {
		return document, err
	}
	return document, axi.ValidateResponse(document)
}

// Evaluate preserves the four fixed-corpus workflows and checks actual subprocess outputs.
func Evaluate(ctx context.Context, options BenchmarkOptions) (Object, error) {
	if options.Repetitions < 1 || options.Repetitions > 10 {
		return nil, fmt.Errorf("Repetitions must be an integer from 1 to 10")
	}
	repository, err := filepath.Abs(options.Repository)
	if err != nil {
		return nil, err
	}
	binary, err := filepath.Abs(options.Binary)
	if err != nil {
		return nil, err
	}
	manifest, digest, err := verifiedNative(repository, options.Binary)
	if err != nil {
		return nil, err
	}
	corpusBytes, err := os.ReadFile(filepath.Join(repository, "evaluations", "corpus.json"))
	if err != nil {
		return nil, err
	}
	var corpus corpus
	if err := json.Unmarshal(corpusBytes, &corpus); err != nil {
		return nil, err
	}
	canaryBytes := make([]byte, 24)
	if _, err := rand.Read(canaryBytes); err != nil {
		return nil, err
	}
	canary := "evaluation-secret-" + hex.EncodeToString(canaryBytes)
	directory, err := os.MkdirTemp("", "axi-evaluation-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	server := testfixture.NewTree(testfixture.Options{Token: canary, RespectFields: true})
	defer server.Close()
	envMap := map[string]string{"PATH": os.Getenv("PATH"), "HOME": directory, "XDG_CONFIG_HOME": directory, "TEAMCITY_URL": server.BaseURL, "TEAMCITY_TOKEN": canary, "TEAMCITY_RO": "1", "TEAMCITY_NO_UPDATE": "1", "DO_NOT_TRACK": "1", "NO_COLOR": "1", "TERM": "dumb"}
	env := []string{}
	for key, value := range envMap {
		env = append(env, key+"="+value)
	}
	if err := os.Mkdir(filepath.Join(directory, "teamcity-axi"), 0700); err != nil {
		return nil, err
	}
	config := Object{"schemaVersion": "1.0", "readOnly": true, "defaultServer": "work", "binaryPath": binary, "servers": Object{"work": Object{"url": server.BaseURL, "allowHttpLoopback": true, "allowedProjects": []string{"Payments"}}}}
	bytes, _ := json.Marshal(config)
	if err := os.WriteFile(filepath.Join(directory, "teamcity-axi", "config.json"), bytes, 0600); err != nil {
		return nil, err
	}
	wrapper := options.Wrapper
	if wrapper == "" {
		wrapper = filepath.Join(directory, "teamcity-axi-bin")
		build, err := Execute(ctx, "go", []string{"build", "-o", wrapper, "./cmd/teamcity-axi"}, os.Environ(), repository, 2097152)
		if err != nil || build.Code != 0 {
			return nil, fmt.Errorf("Cannot build Go wrapper for evaluation: %v (%s)", err, build.Stderr)
		}
	}
	wrapper, err = filepath.Abs(wrapper)
	if err != nil {
		return nil, err
	}
	wrapperDigest, err := fileDigest(wrapper)
	if err != nil {
		return nil, err
	}
	execute := func(executable string, args []string) (Call, error) {
		readCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		return Execute(readCtx, executable, args, env, directory, 2097152)
	}
	version, err := execute(binary, []string{"--version"})
	if err != nil || version.Code != 0 || version.Stdout != "teamcity version "+axi.Str(manifest, "nativeVersion")+"\n" {
		return nil, fmt.Errorf("Native version mismatch: %v", err)
	}
	startupProbe, startupArgs := options.StartupProbe, options.StartupProbeArgs
	if startupProbe == "" {
		startupProbe, startupArgs = "/bin/true", nil
	}
	startupSamples := []Object{}
	for sample := 0; sample < 9; sample++ {
		order := []string{"bare", "wrapper"}
		if sample%2 != 0 {
			order = []string{"wrapper", "bare"}
		}
		durations := map[string]float64{}
		for _, kind := range order {
			executable, args := startupProbe, startupArgs
			if kind == "wrapper" {
				executable, args = wrapper, []string{"--version"}
			}
			result, err := execute(executable, args)
			if err != nil || result.Code != 0 || result.Signal != nil || result.Stderr != "" {
				return nil, fmt.Errorf("Startup probe failed: %v", err)
			}
			durations[kind] = result.WallTimeMS
		}
		startupSamples = append(startupSamples, Object{"bareProcessMs": durations["bare"], "wrapperVersionMs": durations["wrapper"], "addedMs": durations["wrapper"] - durations["bare"]})
	}
	if len(server.Requests()) != 0 {
		return nil, fmt.Errorf("Version probes performed unexpected HTTP requests")
	}
	observations := []Object{}
	for repetition := 0; repetition < options.Repetitions; repetition++ {
		for _, task := range corpus.Tasks {
			conditions := []string{"wrapper-toon", "wrapper-json", "native-selected-json", "native-failure-diagnostics"}
			if repetition%2 != 0 {
				for left, right := 0, len(conditions)-1; left < right; left, right = left+1, right-1 {
					conditions[left], conditions[right] = conditions[right], conditions[left]
				}
			}
			for _, condition := range conditions {
				server.SetMode(axi.Str(task, "mode"))
				calls := []Call{}
				var callsMu sync.Mutex
				start := time.Now()
				var evidence Object
				var subprocessCount, byteBudget any
				call := func(kind, id string, args []string) (Call, error) {
					result, err := execute(binary, args)
					result.Kind, result.RunID = kind, id
					_ = json.Unmarshal([]byte(result.Stdout), &result.Body)
					callsMu.Lock()
					calls = append(calls, result)
					callsMu.Unlock()
					return result, err
				}
				if strings.HasPrefix(condition, "wrapper") {
					argv := []string{"run", "failure", "482193", "--server=work", "--job=Payments_Build", "--project=Payments", "--depth", strconv.Itoa(axi.Int(task, "depth")), "--max-diagnosed-runs", strconv.Itoa(axi.Int(task, "maxDiagnosedRuns"))}
					format := "toon"
					if condition == "wrapper-json" {
						argv, format = append(argv, "--json"), "json"
					}
					result, err := execute(wrapper, argv)
					if err != nil {
						return nil, err
					}
					document, err := decodeWrapper(result.Stdout, format)
					if err != nil {
						return nil, fmt.Errorf("Wrapper contract failed for %s: %w", axi.Str(task, "id"), err)
					}
					if result.Code != 0 || result.Signal != nil {
						return nil, fmt.Errorf("Wrapper observation failed for %s: %v", axi.Str(task, "id"), document.Error)
					}
					calls = append(calls, result)
					evidence, subprocessCount, byteBudget = WrapperEvidence(document), axi.Obj(document.Meta["counts"])["childProcesses"], 24576
					if declared := axi.Obj(document.Meta["limits"])["maxBytes"]; declared != nil {
						byteBudget = declared
					}
				} else if condition == "native-selected-json" {
					graph, err := selectedNative(task, call)
					if err != nil {
						return nil, err
					}
					evidence, subprocessCount = nativeEvidence(calls, graph), len(calls)
				} else {
					result, err := call("diagnostics", "482193", []string{"run", "log", "482193", "--failed", "--json", "--no-input"})
					if err != nil {
						return nil, err
					}
					evidence, subprocessCount = nativeDiagnosticsEvidence(result.Body), len(calls)
				}
				wallTime := float64(time.Since(start)) / float64(time.Millisecond)
				metrics, err := OutputMetrics(calls, canary)
				if err != nil {
					return nil, err
				}
				var withinBudget any
				if byteBudget != nil {
					withinBudget = len(calls[0].Stdout) <= axi.Int(Object{"value": byteBudget}, "value")
				}
				row := Object{"taskId": task["id"], "repetition": repetition, "condition": condition, "wallTimeMs": wallTime, "nativeSubprocessCount": subprocessCount, "httpRequestCount": len(server.Requests()), "agentFacingToolTurns": nil, "byteBudget": byteBudget, "withinByteBudget": withinBudget, "score": ScoreEvidence(axi.Obj(task["expected"]), evidence, axi.Int(metrics, "secretExposures")), "evidence": evidence, "calls": calls}
				for key, value := range metrics {
					row[key] = value
				}
				observations = append(observations, row)
				for _, request := range server.Requests() {
					if request.Method != "GET" {
						return nil, fmt.Errorf("Evaluation performed an unexpected non-read request")
					}
				}
			}
		}
	}
	corpusDigest := sha256.Sum256(corpusBytes)
	startup := Object{"samples": startupSamples, "baseline": "bare process startup; no JavaScript runtime", "httpRequestCount": 0}
	for field, target := range map[string]string{"bareProcessMs": "medianBareProcessMs", "wrapperVersionMs": "medianWrapperVersionMs", "addedMs": "medianAddedMs"} {
		values := []float64{}
		for _, sample := range startupSamples {
			values = append(values, sample[field].(float64))
		}
		startup[target] = Median(values)
	}
	report := Object{"schemaVersion": "1.0", "corpusId": corpus.ID, "corpusSha256": hex.EncodeToString(corpusDigest[:]), "kind": "scripted-evidence-benchmark", "agentEvaluation": "not-performed", "environment": Object{"platform": runtime.GOOS, "architecture": runtime.GOARCH, "goVersion": runtime.Version(), "wrapperSha256": wrapperDigest, "nativeVersion": manifest["nativeVersion"], "nativeSha256": digest}, "tokenizer": TokenizerIdentity, "startup": startup, "repetitions": options.Repetitions, "conditions": SummarizeObservations(observations, []string{"wrapper-toon", "wrapper-json", "native-selected-json", "native-failure-diagnostics"}), "observations": observations}
	return PortableReport(report, map[string]string{canary: "<secret-canary>", server.BaseURL: "http://127.0.0.1:PORT/teamcity", directory: "<isolated-config>", repository: "<repository>"})
}

func SummarizeObservations(observations []Object, conditions []string) []Object {
	result := []Object{}
	for _, condition := range conditions {
		rows := []Object{}
		for _, row := range observations {
			if axi.Str(row, "condition") == condition {
				rows = append(rows, row)
			}
		}
		retained, exposures, identity, completeness := 0, 0, 0, 0
		for _, row := range rows {
			if axi.Bool(axi.Obj(row["score"]), "evidenceRetained") {
				retained++
			}
			exposures += axi.Int(row, "secretExposures")
			identity += axi.Int(axi.Obj(row["score"]), "identityMistakes")
			completeness += axi.Int(axi.Obj(row["score"]), "completenessMistakes")
		}
		entry := Object{"condition": condition, "observations": len(rows), "evidenceRetained": retained, "secretExposures": exposures, "identityMistakes": identity, "completenessMistakes": completeness, "agentTaskSuccess": nil, "medianAgentFacingToolTurns": nil, "unjustifiedCausalClaims": nil}
		for field, target := range map[string]string{"outputTokens": "medianOutputTokens", "wallTimeMs": "medianWallTimeMs", "nativeSubprocessCount": "medianNativeSubprocessCount"} {
			values := []float64{}
			for _, row := range rows {
				if value, ok := row[field].(float64); ok {
					values = append(values, value)
				} else if value, ok := row[field].(int); ok {
					values = append(values, float64(value))
				}
			}
			entry[target] = Median(values)
		}
		result = append(result, entry)
	}
	return result
}

// PortableReport redacts after channel metrics are computed, including before JSON escaping.
func PortableReport(report Object, replacements map[string]string) (Object, error) {
	bytes, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(bytes, &value); err != nil {
		return nil, err
	}
	keys := []string{}
	for key := range replacements {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	var scrub func(any) any
	scrub = func(value any) any {
		switch typed := value.(type) {
		case string:
			for _, key := range keys {
				typed = strings.ReplaceAll(typed, key, replacements[key])
			}
			return typed
		case []any:
			for index, item := range typed {
				typed[index] = scrub(item)
			}
		case map[string]any:
			for key, item := range typed {
				typed[key] = scrub(item)
			}
		}
		return value
	}
	return axi.Obj(scrub(value)), nil
}

func WriteReport(path string, report Object) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return err
	}
	return os.WriteFile(path, data.Bytes(), 0600)
}
