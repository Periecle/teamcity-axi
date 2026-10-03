// Package evaluation contains release-only benchmarks; its reports do not certify live support.
package evaluation

import (
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Periecle/teamcity-axi/internal/axi"
	"github.com/tiktoken-go/tokenizer"
)

type Object = axi.Object

var tokenOnce sync.Once
var tokenCodec tokenizer.Codec
var tokenError error
var tokenMu sync.Mutex

var TokenizerIdentity = Object{"package": "github.com/tiktoken-go/tokenizer", "version": "v0.8.1", "encoding": "o200k_base", "modelVocabulary": "gpt-4o", "scope": "stdout and stderr text separately; no chat framing, prompts, or model reasoning"}

func OutputMetrics(calls []Call, canary string) (Object, error) {
	tokenOnce.Do(func() { tokenCodec, tokenError = tokenizer.Get(tokenizer.O200kBase) })
	if tokenError != nil {
		return nil, tokenError
	}
	bytes, tokens, exposures := 0, 0, 0
	for _, call := range calls {
		bytes += len(call.Stdout) + len(call.Stderr)
		tokenMu.Lock()
		stdout, err := tokenCodec.Count(call.Stdout)
		if err != nil {
			tokenMu.Unlock()
			return nil, err
		}
		stderr, err := tokenCodec.Count(call.Stderr)
		tokenMu.Unlock()
		if err != nil {
			return nil, err
		}
		tokens += stdout + stderr
		if strings.Contains(call.Stdout, canary) || strings.Contains(call.Stderr, canary) {
			exposures++
		}
	}
	return Object{"outputBytes": bytes, "outputTokens": tokens, "secretExposures": exposures}, nil
}

func Median(values []float64) any {
	if len(values) == 0 {
		return nil
	}
	ordered := append([]float64{}, values...)
	sort.Float64s(ordered)
	middle := len(ordered) / 2
	if len(ordered)%2 != 0 {
		return ordered[middle]
	}
	return (ordered[middle-1] + ordered[middle]) / 2
}

var itemExecution = regexp.MustCompile(`^build:\(id:(\d+)\),`)
var exactTestID = regexp.MustCompile(`^build:\(id:(\d+)\),id:\d+$`)

func EvidenceIdentityIssues(findings, sources []Object) []string {
	issues := []string{}
	for _, finding := range findings {
		for _, evidence := range axi.Objects(finding["evidence"]) {
			var source Object
			for _, candidate := range sources {
				if axi.Str(candidate, "id") == axi.Str(evidence, "sourceRef") {
					source = candidate
					break
				}
			}
			itemRunID := ""
			if match := itemExecution.FindStringSubmatch(axi.Str(evidence, "itemId")); len(match) == 2 {
				itemRunID = match[1]
			}
			kind, id := axi.Str(evidence, "kind"), axi.Str(evidence, "runId")
			if id != axi.Str(finding, "runId") || axi.Str(source, "runId") != id || ((kind == "test" || kind == "problem") && itemRunID != id) {
				issues = append(issues, "inconsistent_evidence_identity")
			}
		}
	}
	return issues
}

func has(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

// ScoreEvidence grades retained observations. Model answers need independent blinded grading.
func ScoreEvidence(expected, evidence Object, secretExposures int) Object {
	missing := []string{}
	for _, field := range []string{"nodeIds", "edges", "testIds", "problemRunIds", "unavailableSources", "boundaryRunIds"} {
		for _, value := range axi.Strings(expected[field]) {
			if !has(axi.Strings(evidence[field]), value) {
				missing = append(missing, field+":"+value)
			}
		}
	}
	if axi.Str(expected, "result") != axi.Str(evidence, "result") {
		missing = append(missing, "result")
	}
	if axi.Str(evidence, "state") != "finished" {
		missing = append(missing, "lifecycle")
	}
	if axi.Bool(expected, "cycle") && !axi.Bool(evidence, "cycle") {
		missing = append(missing, "cycle")
	}
	if axi.Bool(expected, "noFindings") && axi.Int(evidence, "findings") > 0 {
		missing = append(missing, "unexpected_findings")
	}
	if axi.Bool(expected, "noSecretExposure") && secretExposures != 0 {
		missing = append(missing, "secret_exposure")
	}
	allowedNodes := axi.Strings(expected["nodeIds"])
	if expected["nodeIds"] == nil {
		allowedNodes = []string{"482193"}
	}
	nodeIDs := axi.Strings(evidence["nodeIds"])
	identityMistakes := 0
	if axi.Str(evidence, "runId") != "482193" {
		identityMistakes++
	}
	for _, id := range nodeIDs {
		if !has(allowedNodes, id) {
			identityMistakes++
		}
	}
	for field, identity := range map[string]string{"jobId": "Payments_Build", "projectId": "Payments"} {
		if value, exists := evidence[field]; exists && value != nil && value != identity {
			identityMistakes++
		}
	}
	for id, job := range axi.Obj(evidence["nodeJobs"]) {
		expectedJob := "Payments_Job_" + id
		if id == "482193" {
			expectedJob = "Payments_Build"
		}
		if job != expectedJob {
			identityMistakes++
		}
	}
	for _, id := range axi.Strings(evidence["testIds"]) {
		match := exactTestID.FindStringSubmatch(id)
		if len(match) != 2 || !has(nodeIDs, match[1]) {
			identityMistakes++
		}
	}
	for _, edge := range axi.Strings(evidence["edges"]) {
		for _, id := range strings.Split(edge, ">") {
			if !has(nodeIDs, id) {
				identityMistakes++
				break
			}
		}
	}
	identityMistakes += len(axi.Strings(evidence["identityIssues"]))
	completenessMistakes := 0
	for _, id := range axi.Strings(expected["unavailableSources"]) {
		if has(axi.Strings(evidence["emptySources"]), id) {
			completenessMistakes++
		}
	}
	if axi.Bool(evidence, "complete") && (len(axi.Strings(expected["unavailableSources"])) > 0 || len(axi.Strings(expected["boundaryRunIds"])) > 0) {
		completenessMistakes++
	}
	return Object{"evidenceRetained": len(missing) == 0 && identityMistakes == 0 && completenessMistakes == 0, "missing": missing, "identityMistakes": identityMistakes, "completenessMistakes": completenessMistakes, "agentTaskSuccess": nil, "unjustifiedCausalClaims": nil}
}
