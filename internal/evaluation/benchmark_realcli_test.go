//go:build realcli

package evaluation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Periecle/teamcity-axi/internal/axi"
)

func TestRealCLIEvaluationRetainsCriticalEvidenceAndHonestNativeRegressions(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Evaluate(context.Background(), BenchmarkOptions{Repository: repository, Binary: os.Getenv("TEAMCITY_AXI_TEST_BINARY"), Repetitions: 1})
	if err != nil {
		t.Fatal(err)
	}
	rows := axi.Objects(report["observations"])
	if len(rows) != 32 || axi.Str(report, "agentEvaluation") != "not-performed" {
		t.Fatal("scripted report lost its exact corpus/agent boundary")
	}
	diagnosticMissing, nativeExposure := false, false
	workflowRequests := map[string]int{
		"root-failure": 5, "shared-dependency": 18, "denied-problems": 5,
		"missing-tests-and-logs": 6, "dependency-cycle": 17, "depth-boundary": 13,
		"exact-green": 1, "secret-in-tests": 5,
	}
	for _, row := range rows {
		condition, task := axi.Str(row, "condition"), axi.Str(row, "taskId")
		wantRequests, known := workflowRequests[task]
		if !known {
			t.Fatalf("unexpected task %q", task)
		}
		if condition == "native-failure-diagnostics" {
			wantRequests = 4
			if task == "missing-tests-and-logs" {
				wantRequests = 3
			} else if task == "exact-green" {
				wantRequests = 2
			}
		}
		if got := axi.Int(row, "httpRequestCount"); got != wantRequests {
			t.Fatalf("%s/%s counted %d HTTP requests, want %d for this workflow", condition, task, got, wantRequests)
		}
		score := axi.Obj(row["score"])
		if strings.HasPrefix(condition, "wrapper") {
			if axi.Int(row, "nativeSubprocessCount") != wantRequests+1 {
				t.Fatalf("%s/%s native launches do not match requests plus the no-network version probe", condition, task)
			}
			if !axi.Bool(score, "evidenceRetained") || axi.Int(row, "secretExposures") != 0 || !axi.Bool(row, "withinByteBudget") || row["agentFacingToolTurns"] != nil || axi.Int(row, "nativeSubprocessCount") > 24 {
				t.Fatalf("%s/%s: %#v", condition, task, score)
			}
		}
		if condition == "native-selected-json" {
			if len(axi.Objects(row["calls"])) != wantRequests {
				t.Fatalf("native selected calls do not match independent HTTP requests for %s", task)
			}
			nativeExposure = nativeExposure || axi.Int(row, "secretExposures") > 0
			if task != "secret-in-tests" && !axi.Bool(score, "evidenceRetained") {
				t.Fatalf("native selected-field regression %s: %#v", task, score)
			}
			for _, call := range axi.Objects(row["calls"]) {
				if axi.Str(call, "kind") == "run" {
					var dto Object
					if err := json.Unmarshal([]byte(axi.Str(call, "stdout")), &dto); err != nil {
						t.Fatal(err)
					}
					if dto["number"] != nil || dto["statusText"] != nil || dto["startDate"] != nil || axi.Obj(dto["buildType"])["name"] != nil {
						t.Fatal("native baseline padded with unrequested fields")
					}
				}
			}
		}
		if condition == "native-failure-diagnostics" && task == "denied-problems" {
			diagnosticMissing = !axi.Bool(score, "evidenceRetained") && has(axi.Strings(score["missing"]), "unavailableSources:problems:482193")
		}
	}
	reportPath := filepath.Join(t.TempDir(), "report.json")
	if err := WriteReport(reportPath, report); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !diagnosticMissing || !nativeExposure || !strings.Contains(string(bytes), "<secret-canary>") || strings.Contains(string(bytes), "evaluation-secret-") {
		t.Fatal("baseline omissions/exposures hidden or canary not scrubbed")
	}
}
