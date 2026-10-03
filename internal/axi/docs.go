package axi

import (
	"fmt"
	"strings"
)

// GenerateCommandDocs uses the same registry and parser as the executable.
func GenerateCommandDocs() (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# Command reference\n\nGenerated for teamcity-axi %s from the executable command registry.\nRun `make docs` after changing commands; `make test` rejects drift.\n\nUse a registered server alias. Example IDs are placeholders, not discovered resources.\nThese commands only read TeamCity. Status checks the local committed checkout;\nwatch checks the terminal outcome of one fixed execution. A normal red observation\nexits zero; `--check` fails its assertion with exit one.\n\n", Version)
	for _, d := range Registry {
		args := strings.Split(d.Name, ".")
		switch {
		case d.Positional == "runId":
			args = append(args, "482193")
		case d.Name == "job.view":
			args = append(args, "Payments_Build")
		case d.Name == "agent.view":
			args = append(args, "7")
		case d.Name == "schema":
			args = append(args, "run.view")
		}
		if contains([]string{"status", "run.list", "queue.list", "agent.list"}, d.Name) {
			args = append(args, "--job=Payments_Build")
		}
		if d.Name == "job.list" {
			args = append(args, "--project=Payments")
		}
		if !contains([]string{"schema", "context.show"}, d.Name) {
			args = append(args, "--server=work")
		}
		if _, err := Parse(args); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "## %s\n\n%s.\n\n```sh\nteamcity-axi %s\n```\n\n```text\n%s\n```\n\n", d.Name, d.Summary, strings.Join(args, " "), strings.TrimRight(Help(&d), "\n"))
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}
