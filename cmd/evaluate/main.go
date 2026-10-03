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
	"strings"
	"syscall"

	"github.com/Periecle/teamcity-axi/internal/axi"
	"github.com/Periecle/teamcity-axi/internal/evaluation"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--startup-probe" {
		return
	}
	repetitions := 3
	if value := os.Getenv("TEAMCITY_AXI_EVAL_REPETITIONS"); value != "" {
		number, err := strconv.Atoi(value)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Repetitions must be an integer from 1 to 10")
			os.Exit(1)
		}
		repetitions = number
	}
	repository, _ := os.Getwd()
	binary := flag.String("binary", os.Getenv("TEAMCITY_AXI_TEST_BINARY"), "checksum-pinned native executable")
	wrapper := flag.String("wrapper", os.Getenv("TEAMCITY_AXI_EVAL_WRAPPER"), "Go wrapper executable (built when omitted)")
	root := flag.String("repository", repository, "source repository root")
	repeats := flag.Int("repetitions", repetitions, "repetitions from 1 to 10")
	destination := os.Getenv("TEAMCITY_AXI_EVAL_OUTPUT")
	if destination == "" {
		destination = filepath.Join(repository, "test-results", "evaluation.json")
	}
	output := flag.String("output", destination, "report destination")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	report, err := evaluation.Evaluate(ctx, evaluation.BenchmarkOptions{Repository: *root, Binary: *binary, Wrapper: *wrapper, Repetitions: *repeats, StartupProbe: self, StartupProbeArgs: []string{"--startup-probe"}})
	if err == nil {
		err = evaluation.WriteReport(*output, report)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	summary, _ := json.MarshalIndent(axi.Object{"report": *output, "kind": report["kind"], "conditions": report["conditions"]}, "", "  ")
	fmt.Println(string(summary))
	for _, row := range axi.Objects(report["observations"]) {
		if strings.HasPrefix(axi.Str(row, "condition"), "wrapper") && (!axi.Bool(axi.Obj(row["score"]), "evidenceRetained") || axi.Int(row, "secretExposures") > 0 || !axi.Bool(row, "withinByteBudget")) {
			os.Exit(1)
		}
	}
}
