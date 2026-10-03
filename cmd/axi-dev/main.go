// axi-dev provides offline release and documentation checks. It is not a product command.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Periecle/teamcity-axi/internal/axi"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("expected docs [--check], verify-go-tree or verify-native PATH")
	}
	switch args[0] {
	case "verify-go-tree":
		if len(args) != 1 {
			return fmt.Errorf("verify-go-tree accepts no arguments")
		}
		err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			if entry.Name() == "node_modules" || entry.Name() == "package.json" || entry.Name() == "package-lock.json" || entry.Name() == "tsconfig.json" {
				return fmt.Errorf("Go-only worktree contains obsolete runtime/configuration: %s", path)
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".mts", ".cts":
				return fmt.Errorf("Go-only worktree contains JavaScript/TypeScript source: %s", path)
			}
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Println("Verified Go-only worktree; no JS/TS or Node dependency/configuration files")
		return nil
	case "docs":
		content, err := axi.GenerateCommandDocs()
		if err != nil {
			return err
		}
		if len(args) == 2 && args[1] == "--check" {
			actual, err := os.ReadFile("docs/commands.md")
			if err != nil {
				return err
			}
			if string(actual) != content {
				return fmt.Errorf("generated command documentation has drifted; run make docs")
			}
			fmt.Printf("Verified help and parseable examples for %d commands\n", len(axi.Registry))
			return nil
		}
		if len(args) != 1 {
			return fmt.Errorf("unexpected docs arguments")
		}
		return os.WriteFile("docs/commands.md", []byte(content), 0644)
	case "verify-native":
		if len(args) != 2 {
			return fmt.Errorf("verify-native requires binary path")
		}
		b, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		hash := sha256.Sum256(b)
		actual := hex.EncodeToString(hash[:])
		b, err = os.ReadFile("docs/compatibility.json")
		if err != nil {
			return err
		}
		var manifest struct {
			Artifacts []struct{ Archive, BinarySha256 string }
		}
		if err = json.Unmarshal(b, &manifest); err != nil {
			return err
		}
		arch := runtime.GOARCH
		if arch == "amd64" {
			arch = "x86_64"
		}
		platform := runtime.GOOS + "_" + arch + ".tar.gz"
		for _, a := range manifest.Artifacts {
			if strings.HasSuffix(a.Archive, platform) && a.BinarySha256 == actual {
				fmt.Println("Verified pinned official native binary for " + platform)
				return nil
			}
		}
		return fmt.Errorf("native binary checksum does not match pinned platform release")
	default:
		return fmt.Errorf("unknown development command")
	}
}
