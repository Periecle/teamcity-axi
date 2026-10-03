package main

import (
	"fmt"
	"os"

	"github.com/Periecle/teamcity-axi/internal/livefixture"
)

func main() {
	if e := record(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func record() error {
	output := os.Getenv("TEAMCITY_AXI_LIVE_OUTPUT")
	if output == "" {
		return fmt.Errorf("Set TEAMCITY_AXI_LIVE_OUTPUT to an explicit destination for sanitized test captures")
	}
	f, e := livefixture.NewFromEnvironment()
	if e != nil {
		return e
	}
	defer f.Close()
	if e = f.Record(output); e != nil {
		return e
	}
	fmt.Printf("Recorded %d sanitized live native observations\n", len(f.Contract.Records))
	return nil
}
