//go:build packageaudit

package distribution

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Periecle/teamcity-axi/internal/axi"
	toon "github.com/toon-format/toon-go"
)

func TestStandalonePackageContentsAndOfflineCommands(t *testing.T) {
	archive := os.Getenv("TEAMCITY_AXI_PACKAGE")
	if archive == "" {
		t.Fatal("Package audit requires TEAMCITY_AXI_PACKAGE; missing artifacts never skip")
	}
	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	zipped, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer zipped.Close()
	reader := tar.NewReader(zipped)
	allowed := map[string]bool{"teamcity-axi": false, "LICENSE": false, "README.md": false, "CHANGELOG.md": false, "THIRD_PARTY_LICENSES.txt": false, "docs/commands.md": false, "docs/dependencies.md": false}
	dir := t.TempDir()
	for {
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		seen, ok := allowed[h.Name]
		if !ok || seen || h.Typeflag != tar.TypeReg || h.Size <= 0 || h.Size > 32<<20 {
			t.Fatalf("Unexpected, repeated or unsafe archive entry %q", h.Name)
		}
		allowed[h.Name] = true
		contents, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "teamcity-axi" {
			if h.Mode&0111 == 0 {
				t.Fatal("Product executable lacks execute permission")
			}
			if err := os.WriteFile(filepath.Join(dir, h.Name), contents, 0700); err != nil {
				t.Fatal(err)
			}
		} else if h.Name == "THIRD_PARTY_LICENSES.txt" {
			for _, text := range []string{"BurntSushi/toml", "toon-format/toon-go", "santhosh-tekuri/jsonschema", "golang.org/x/text", "Apache License", "Permission is hereby granted"} {
				if !bytes.Contains(contents, []byte(text)) {
					t.Fatalf("Missing dependency notice %s", text)
				}
			}
		}
	}
	for name, seen := range allowed {
		if !seen {
			t.Fatalf("Missing archive entry %s", name)
		}
	}
	binary := filepath.Join(dir, "teamcity-axi")
	invoke := func(args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir = dir
		cmd.Env = []string{"HOME=" + dir, "XDG_CONFIG_HOME=" + dir, "PATH=/nonexistent", "TEAMCITY_TOKEN=package-secret-canary", "TEAMCITY_URL=https://untrusted.invalid"}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil || stderr.Len() != 0 || bytes.Contains(stdout.Bytes(), []byte("package-secret-canary")) {
			t.Fatalf("Offline package command %v failed: %v, %s", args, err, stderr.String())
		}
		return stdout.Bytes()
	}
	t.Run("version", func(t *testing.T) {
		if strings.TrimSpace(string(invoke("--version"))) != axi.Version {
			t.Fatal("Unexpected product version")
		}
	})
	t.Run("root-help", func(t *testing.T) {
		if !bytes.Contains(invoke("--help"), []byte("run failure")) {
			t.Fatal("Packaged help lost investigation commands")
		}
	})
	for _, descriptor := range axi.Registry {
		t.Run(descriptor.Name, func(t *testing.T) {
			args := append(strings.Split(descriptor.Name, "."), "--help")
			if !bytes.Contains(invoke(args...), []byte(descriptor.Summary)) {
				t.Fatal("Packaged command help differs from registry")
			}
			for _, format := range []string{"toon", "json"} {
				t.Run(format, func(t *testing.T) {
					out := invoke("schema", descriptor.Name, "--format="+format, "--max-bytes=262144")
					var value any
					var err error
					if format == "json" {
						err = json.Unmarshal(out, &value)
					} else {
						value, err = toon.Decode(out)
					}
					if err != nil {
						t.Fatal(err)
					}
					body := axi.Obj(value)
					if axi.Str(body, "command") != "schema" || axi.Str(body, "status") != "ok" || axi.Str(axi.Obj(axi.Obj(body["data"])["descriptor"]), "name") != descriptor.Name {
						t.Fatal("Embedded schema or command identity lost")
					}
				})
			}
		})
	}
}
