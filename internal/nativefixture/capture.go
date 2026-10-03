// Package nativefixture records the immutable released CLI protocol against a
// synthetic projecting HTTP fixture. It is verification tooling, not live evidence.
package nativefixture

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Periecle/teamcity-axi/internal/testfixture"
)

//go:embed operations.json
var operationData []byte

type Operation struct {
	Name string
	Argv []string
}

func Operations() ([]Operation, []string, error) {
	var value struct {
		Operations []json.RawMessage `json:"operations"`
		ErrorModes []string          `json:"errorModes"`
	}
	if err := json.Unmarshal(operationData, &value); err != nil {
		return nil, nil, err
	}
	result := []Operation{}
	for _, raw := range value.Operations {
		var tuple []json.RawMessage
		if err := json.Unmarshal(raw, &tuple); err != nil || len(tuple) != 2 {
			return nil, nil, fmt.Errorf("invalid recorded operation")
		}
		var op Operation
		if err := json.Unmarshal(tuple[0], &op.Name); err != nil {
			return nil, nil, err
		}
		if err := json.Unmarshal(tuple[1], &op.Argv); err != nil {
			return nil, nil, err
		}
		result = append(result, op)
	}
	return result, value.ErrorModes, nil
}
func Verify(binary, manifestPath string) (string, error) {
	if binary == "" {
		return "", fmt.Errorf("set TEAMCITY_AXI_TEST_BINARY to the checksum-verified v1.5.0 binary")
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	actual := hex.EncodeToString(hash[:])
	data, err = os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}
	var m struct {
		Artifacts []struct{ Archive, BinarySha256 string }
	}
	if err = json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	for _, a := range m.Artifacts {
		if strings.HasSuffix(a.Archive, runtime.GOOS+"_"+arch+".tar.gz") && a.BinarySha256 == actual {
			return actual, nil
		}
	}
	return "", fmt.Errorf("native executable checksum differs from pinned platform release")
}
func Capture(ctx context.Context, binary, manifestPath string) (map[string]any, error) {
	hash, err := Verify(binary, manifestPath)
	if err != nil {
		return nil, err
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "teamcity-axi-native-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	server := testfixture.NewServer(testfixture.Options{Token: "fixture-only-token"})
	defer server.Close()
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "XDG_CONFIG_HOME=" + dir, "TEAMCITY_URL=" + server.BaseURL, "TEAMCITY_TOKEN=fixture-only-token", "TEAMCITY_RO=1", "TEAMCITY_NO_UPDATE=1", "DO_NOT_TRACK=1", "NO_COLOR=1", "TERM=dumb"}
	call := func(argv []string) (map[string]any, error) {
		child, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(child, binary, argv...)
		cmd.Env = env
		cmd.Dir = dir
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			if child.Err() != nil {
				return nil, fmt.Errorf("native fixture request exceeded deadline")
			}
			if exitErr, ok := err.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				return nil, fmt.Errorf("native fixture executable failed to start")
			}
		}
		requests := server.Requests()
		server.ClearRequests()
		entries := []map[string]any{}
		for _, q := range requests {
			query := map[string]string{}
			for k, v := range q.Query {
				if len(v) != 1 {
					return nil, fmt.Errorf("fixture returned ambiguous query")
				}
				query[k] = v[0]
			}
			entries = append(entries, map[string]any{"method": q.Method, "path": q.Path, "query": query, "authenticated": q.Authenticated})
		}
		return map[string]any{"argv": argv, "code": code, "signal": nil, "stdout": out.String(), "stderr": stderr.String(), "requests": entries}, nil
	}
	version, err := call([]string{"--version"})
	if err != nil {
		return nil, err
	}
	if version["code"] != 0 || version["stdout"] != "teamcity version 1.5.0\n" {
		return nil, fmt.Errorf("native version differs from manifest")
	}
	help, err := call([]string{"api", "--help"})
	if err != nil {
		return nil, err
	}
	operations, modes, err := Operations()
	if err != nil {
		return nil, err
	}
	records := map[string]any{}
	for _, op := range operations {
		r, e := call(op.Argv)
		if e != nil {
			return nil, e
		}
		records[op.Name] = r
	}
	for _, mode := range modes {
		server.SetMode(mode)
		r, e := call(operations[1].Argv)
		if e != nil {
			return nil, e
		}
		records["error-"+mode] = r
	}
	server.SetMode("summary-denied")
	r, err := call(operations[len(operations)-1].Argv)
	if err != nil {
		return nil, err
	}
	records["summary-denied"] = r
	server.SetMode("logs-unsupported")
	for _, op := range operations {
		if op.Name == "log-tail" {
			r, e := call(op.Argv)
			if e != nil {
				return nil, e
			}
			records["logs-unsupported"] = r
		}
	}
	value := map[string]any{"nativeVersion": "1.5.0", "binarySha256": hash, "sourceRevision": "ac6c1fd83300b2e42df48fcfff365ba03f73bb92", "serverKind": "synthetic-mock-not-live-TeamCity", "platform": runtime.GOOS, "architecture": runtime.GOARCH, "probes": map[string]any{"version": version, "help": help}, "records": records}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(data), server.BaseURL, "http://127.0.0.1:PORT/teamcity")
	text = strings.ReplaceAll(text, dir, "<isolated-config>")
	if strings.Contains(text, "fixture-only-token") {
		return nil, fmt.Errorf("native capture disclosed credential canary")
	}
	if err = json.Unmarshal([]byte(text), &value); err != nil {
		return nil, err
	}
	return value, nil
}
