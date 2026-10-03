package axi

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewedContextRejectsExplicitlyEmptyNativeIdentities(t *testing.T) {
	for _, input := range []string{
		"[[server]]\nurl='https://ci.example'\nproject=''\n",
		"[[server]]\nurl='https://ci.example'\njob=''\n",
		"[[server]]\nurl='https://ci.example'\n[server.paths.sub]\nproject=''\n",
		"[[server]]\nurl='https://ci.example'\n[server.paths.sub]\njob=''\n",
		"[[server]]\nurl='https://ci.example'\nproject='Allowed'\n[[server]]\nurl='https://other.example'\nproject=''\n",
	} {
		if _, err := nativeBindings([]byte(input)); err == nil || AsDomainError(err).Code != "USAGE_ERROR" {
			t.Fatalf("empty bound identity accepted: %s", input)
		}
	}
	if _, err := nativeBindings([]byte("[[server]]\nurl='https://ci.example'\n")); err != nil {
		t.Fatal("absent optional identity rejected:", err)
	}
}

func TestReviewedContextRejectsExplicitlyEmptyCLIIdentities(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "teamcity-axi")
	if err := os.Mkdir(config, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "config.json"), []byte(`{"schemaVersion":"1.0","readOnly":true,"servers":{"work":{"url":"https://ci.example"}},"defaultServer":"work"}`), 0600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"XDG_CONFIG_HOME": root, "HOME": root, "PATH": os.Getenv("PATH")}
	for _, selector := range []string{"project", "job", "revision", "vcs-root", "server"} {
		t.Run(selector, func(t *testing.T) {
			parsed, err := Parse([]string{"run", "view", "1", "--cwd", root, "--" + selector + "="})
			if err != nil {
				if AsDomainError(err).Code != "USAGE_ERROR" {
					t.Fatal(err)
				}
				return
			}
			_, err = ResolveContext(context.Background(), parsed, env)
			if err == nil || (AsDomainError(err).Code != "USAGE_ERROR" && AsDomainError(err).Code != "UNTRUSTED_SERVER") {
				t.Fatalf("explicit empty %s fell back to another identity: %v", selector, err)
			}
		})
	}
	parsed, err := Parse([]string{"run", "view", "1", "--cwd", root})
	if err != nil {
		t.Fatal(err)
	}
	env["TEAMCITY_AXI_SERVER"] = ""
	if _, err := ResolveContext(context.Background(), parsed, env); err == nil || AsDomainError(err).Code != "UNTRUSTED_SERVER" {
		t.Fatal("present empty server environment selector fell back to default")
	}
}
