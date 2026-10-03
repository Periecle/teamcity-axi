package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Periecle/teamcity-axi/internal/nativefixture"
)

func main() {
	value, err := nativefixture.Capture(context.Background(), os.Getenv("TEAMCITY_AXI_TEST_BINARY"), "docs/compatibility.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	destination := "test-results/native-contract.json"
	if len(os.Args) == 2 {
		destination = os.Args[1]
	} else if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "expected optional output path")
		os.Exit(2)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		err = os.MkdirAll("test-results", 0700)
	}
	if err == nil {
		err = os.WriteFile(destination, append(data, '\n'), 0600)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
