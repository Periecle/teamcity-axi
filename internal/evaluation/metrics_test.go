package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Periecle/teamcity-axi/internal/axi"
	"github.com/Periecle/teamcity-axi/internal/testfixture"
)

func TestPublishedHistoricalEvidenceReportBindsCorpusWithoutClaimingGoOrAgentResults(t *testing.T) {
	corpus, err := os.ReadFile(filepath.Join("..", "..", "evaluations", "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(filepath.Join("..", "..", "evaluations", "results", "linux-x64-node24-native1.5.0.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report Object
	if err := json.Unmarshal(bytes, &report); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(corpus)
	if axi.Str(report, "corpusSha256") != hex.EncodeToString(digest[:]) || len(axi.Objects(report["observations"])) != 96 || axi.Str(report, "agentEvaluation") != "not-performed" || axi.Int(axi.Obj(report["startup"]), "httpRequestCount") != 0 || strings.Contains(string(bytes), "evaluation-secret-") {
		t.Fatal("historical report lost its evidence boundaries")
	}
	for _, condition := range axi.Objects(report["conditions"]) {
		if strings.HasPrefix(axi.Str(condition, "condition"), "wrapper") && (axi.Int(condition, "evidenceRetained") != 24 || axi.Int(condition, "secretExposures") != 0 || condition["medianAgentFacingToolTurns"] != nil || condition["agentTaskSuccess"] != nil) {
			t.Fatalf("scripted evidence mislabeled as agent success: %#v", condition)
		}
	}
}

func TestEvaluationNestedFixtureProjection(t *testing.T) {
	input := Object{"count": 1, "build": []Object{{"id": 7, "statusText": "Excluded", "buildType": Object{"id": "Job", "name": "Excluded", "projectId": "Project"}}}, "unknown": true}
	got, err := testfixture.SelectFieldsChecked(input, "count,build(id,buildType(id,projectId))")
	if err != nil {
		t.Fatal(err)
	}
	want := Object{"count": 1, "build": []Object{{"id": 7, "buildType": Object{"id": "Job", "projectId": "Project"}}}}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		t.Fatalf("selected fields differ: %s", a)
	}
	if _, err := testfixture.SelectFieldsChecked(input, "build(id"); err == nil {
		t.Fatal("unbalanced field projection accepted")
	}
}

func TestEvaluationTokenizerMatchesVerifiedJSTiktokenCounts(t *testing.T) {
	// Counts were executed against js-tiktoken@1.0.21 with specials treated as ordinary text.
	for _, sample := range []struct {
		text  string
		count int
	}{
		{"hello world", 2}, {"<|endoftext|>🦊secret-canary", 13}, {"Zażółć gęślą jaźń", 11}, {"東京のCIが失敗しました", 7}, {"\n{\"status\":\"partial\",\"runId\":\"482193\"}\n", 12}, {"Bearer fixture-secret\x1b[31m", 7},
	} {
		result, err := OutputMetrics([]Call{{Stdout: sample.text}}, "absent")
		if err != nil || axi.Int(result, "outputTokens") != sample.count || axi.Int(result, "outputBytes") != len(sample.text) {
			t.Fatalf("tokenizer mismatch for %q: %#v, %v", sample.text, result, err)
		}
	}
	result, err := OutputMetrics([]Call{{Stdout: "hello world", Stderr: "<|endoftext|>🦊secret-canary"}}, "secret-canary")
	if err != nil || axi.Int(result, "outputTokens") != 15 || axi.Int(result, "outputBytes") != len("hello world<|endoftext|>🦊secret-canary") || axi.Int(result, "secretExposures") != 1 || Median([]float64{9, 1, 5, 3}) != float64(4) || Median(nil) != nil {
		t.Fatalf("per-channel metrics: %#v, %v", result, err)
	}
}

func TestEvaluationScoringRejectsIdentityOmissionsFalseZerosAndSecrets(t *testing.T) {
	expected := Object{"result": "failure", "nodeIds": []string{"482193", "482190"}, "unavailableSources": []string{"tests:482193"}, "noSecretExposure": true}
	result := ScoreEvidence(expected, Object{"runId": "482999", "result": "failure", "nodeIds": []string{"482193"}, "emptySources": []string{"tests:482193"}}, 1)
	if axi.Bool(result, "evidenceRetained") || axi.Int(result, "identityMistakes") != 1 || axi.Int(result, "completenessMistakes") != 1 || !has(axi.Strings(result["missing"]), "nodeIds:482190") || !has(axi.Strings(result["missing"]), "secret_exposure") || result["agentTaskSuccess"] != nil || result["unjustifiedCausalClaims"] != nil {
		t.Fatalf("contradictions accepted: %#v", result)
	}
}

func TestEvaluationScoringRejectsForeignSiblingsAndCoverageClaims(t *testing.T) {
	result := ScoreEvidence(Object{"result": "failure", "unavailableSources": []string{"problems:482193"}}, Object{"runId": "482193", "jobId": "Foreign_Job", "projectId": "Foreign", "result": "failure", "nodeIds": []string{"482193", "999"}, "nodeJobs": Object{"482193": "Foreign_Job"}, "unavailableSources": []string{"problems:482193"}, "complete": true}, 0)
	if axi.Bool(result, "evidenceRetained") || axi.Int(result, "identityMistakes") != 4 || axi.Int(result, "completenessMistakes") != 1 {
		t.Fatalf("foreign/unsupported evidence accepted: %#v", result)
	}
}

func TestEvaluationCompoundIdentityBindsFindingAndSource(t *testing.T) {
	sources := []Object{{"id": "tests:482193", "runId": "482193"}}
	evidence := Object{"kind": "test", "runId": "482193", "sourceRef": "tests:482193", "itemId": "build:(id:482190),id:1"}
	findings := []Object{{"runId": "482193", "evidence": []Object{evidence}}}
	if !reflect.DeepEqual(EvidenceIdentityIssues(findings, sources), []string{"inconsistent_evidence_identity"}) {
		t.Fatal("foreign occurrence attributed to root")
	}
	evidence["itemId"] = "build:(id:482193),id:1"
	if len(EvidenceIdentityIssues(findings, sources)) != 0 {
		t.Fatal("exact compound identity rejected")
	}
}
