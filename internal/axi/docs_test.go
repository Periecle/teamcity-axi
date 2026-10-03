package axi

import (
	"os"
	"testing"
)

func TestGeneratedDocsAndExamples(t *testing.T) {
	content, err := GenerateCommandDocs()
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile("../../docs/commands.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != content {
		t.Fatal("generated docs drift: run make docs")
	}
}
