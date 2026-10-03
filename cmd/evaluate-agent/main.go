package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/Periecle/teamcity-axi/internal/axi"
	"github.com/Periecle/teamcity-axi/internal/evaluation"
)

func fallback(name, value string) string {
	if actual := os.Getenv(name); actual != "" {
		return actual
	}
	return value
}

func main() {
	timeout, err := strconv.Atoi(fallback("TEAMCITY_AXI_AGENT_TIMEOUT_MS", "300000"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid model session timeout")
		os.Exit(1)
	}
	root, _ := os.Getwd()
	options := evaluation.AgentOptions{Repository: root, Binary: os.Getenv("TEAMCITY_AXI_TEST_BINARY"), Runtime: os.Getenv("TEAMCITY_AXI_AGENT_RUNTIME"), AuthPath: os.Getenv("TEAMCITY_AXI_AGENT_AUTH_PATH"), Output: fallback("TEAMCITY_AXI_AGENT_OUTPUT", filepath.Join(root, "test-results", "agent-evaluation.json")), Model: fallback("TEAMCITY_AXI_AGENT_MODEL", "gpt-6.1-sol"), Effort: fallback("TEAMCITY_AXI_AGENT_EFFORT", "xhigh"), SessionTimeout: time.Duration(timeout) * time.Millisecond, WrapperPackage: os.Getenv("TEAMCITY_AXI_AGENT_PACKAGE"), SourceCheckpoint: os.Getenv("TEAMCITY_AXI_AGENT_WRAPPER_CHECKPOINT"), Task: os.Getenv("TEAMCITY_AXI_AGENT_TASK")}
	flag.StringVar(&options.Repository, "repository", options.Repository, "source repository root")
	flag.StringVar(&options.Binary, "binary", options.Binary, "checksum-pinned native executable")
	flag.StringVar(&options.Runtime, "runtime", options.Runtime, "production directory containing teamcity-axi Go executable")
	flag.StringVar(&options.AuthPath, "auth", options.AuthPath, "private local model-auth file")
	flag.StringVar(&options.Output, "output", options.Output, "report destination")
	flag.StringVar(&options.Task, "task", options.Task, "single corpus task, or all when omitted")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	report, err := evaluation.EvaluateAgents(ctx, options)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bytes, _ := json.MarshalIndent(axi.Object{"report": options.Output, "kind": report["kind"], "conditions": report["conditions"], "independentGrading": report["independentGrading"]}, "", "  ")
	fmt.Println(string(bytes))
}
