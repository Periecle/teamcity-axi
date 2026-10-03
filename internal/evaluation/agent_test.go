package evaluation

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Periecle/teamcity-axi/internal/axi"
)

func TestEvaluationRuntimeDigestDeterministicAndRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "b"), []byte("B"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a"), []byte("A"), 0600); err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256([]byte("a\x00A\x00b\x00B\x00"))
	digest, err := RuntimeDigest(root)
	if err != nil || digest != hex.EncodeToString(expected[:]) {
		t.Fatalf("runtime digest did not preserve exact sorted files: %s, %v", digest, err)
	}
	if err := os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := RuntimeDigest(root); err == nil {
		t.Fatal("runtime symlink admitted")
	}
}

func TestEvaluationShellBoundaryAndBudget(t *testing.T) {
	for _, command := range []string{"cat /metrics/native-launches", "/tools/native --version", "cat /proc/self/environ", "bwrap true", "kill 7", "pkill codex", strings.Repeat("x", 65537)} {
		if _, err := ValidateShellRequest(Object{"tool": "evaluation_shell", "arguments": Object{"command": command}}, 0); err == nil {
			t.Fatalf("instrumentation bypass admitted: %q", command[:min(80, len(command))])
		}
	}
	for _, params := range []Object{{"tool": "shell", "arguments": Object{"command": "true"}}, {"tool": "evaluation_shell", "arguments": Object{"command": 42}}} {
		if _, err := ValidateShellRequest(params, 0); err == nil {
			t.Fatal("unexpected tool request admitted")
		}
	}
	params := Object{"tool": "evaluation_shell", "arguments": Object{"command": "teamcity-axi run failure 482193 & teamcity --version; wait"}}
	if _, err := ValidateShellRequest(params, 23); err != nil {
		t.Fatal("batched ordinary shell request rejected:", err)
	}
	if _, err := ValidateShellRequest(params, 24); err == nil {
		t.Fatal("twenty fifth tool request admitted")
	}
}

func TestEvaluationBubblewrapMountsOnlyExecutableRuntime(t *testing.T) {
	args := bubblewrapArgs("/private/work", "/private/metrics", "/private/tools", "/private/native", "/private/wrapper", "wrapper", map[string]string{"HOME": "/work", "TEAMCITY_TOKEN": "fixture-canary"})
	joined := strings.Join(args, "\x00")
	if !strings.Contains(joined, "--ro-bind\x00/private/wrapper\x00/tools/axi") || strings.Contains(joined, "node") || strings.Contains(joined, "/runtime") || !strings.Contains(joined, "--clearenv") || !strings.Contains(joined, "--tmpfs\x00"+filepath.Join(string(filepath.Separator), "tmp")) {
		t.Fatalf("runtime/isolation mount regression: %v", args)
	}
	native := bubblewrapArgs("/work", "/metrics", "/tools", "/native", "/wrapper", "native", nil)
	if has(native, "/wrapper") || has(native, "/tools/axi") {
		t.Fatal("native condition can inspect wrapper")
	}
}

func TestEvaluationPortableReportRedactsBeforeJSONEscaping(t *testing.T) {
	secret := "canary-with-quote\"and\\slash"
	result, err := PortableReport(Object{"calls": []Object{{"stdout": secret}}, "events": []Object{{"secret": secret}}}, map[string]string{secret: "<secret-canary>"})
	if err != nil {
		t.Fatal(err)
	}
	bytes, _ := json.Marshal(result)
	if strings.Contains(string(bytes), "canary-with-quote") || !strings.Contains(string(bytes), "secret-canary") {
		t.Fatal("JSON escaping hid a canary from report scrubbing")
	}
}

func TestEvaluationHelperProcess(t *testing.T) {
	mode := os.Getenv("TEAMCITY_AXI_EVALUATION_HELPER")
	if mode == "" {
		return
	}
	if mode == "capture" {
		fmt.Print(strings.Repeat("x", 65536))
		os.Exit(0)
	}
	if mode == "sleep" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	scanner := bufio.NewScanner(os.Stdin)
	emit := func(message Object) { bytes, _ := json.Marshal(message); fmt.Println(string(bytes)) }
	for scanner.Scan() {
		var message Object
		_ = json.Unmarshal(scanner.Bytes(), &message)
		switch axi.Str(message, "method") {
		case "initialize":
			emit(Object{"id": message["id"], "result": Object{"userAgent": "evaluation-test"}})
		case "thread/start":
			emit(Object{"id": message["id"], "result": Object{"thread": Object{"id": "thread-1"}, "model": "fixture-model", "modelProvider": "fixture", "reasoningEffort": "xhigh"}})
		case "turn/start":
			emit(Object{"id": message["id"], "result": Object{"turn": Object{"id": "turn-1"}}})
			if mode == "unexpected" {
				emit(Object{"method": "item/started", "params": Object{"item": Object{"type": "commandExecution"}}})
			} else if mode == "rpc" {
				emit(Object{"id": 99, "method": "item/tool/call", "params": Object{"tool": "evaluation_shell", "callId": "call-1", "arguments": Object{"command": "teamcity-axi --version"}}})
			} else if mode == "invalid" {
				fmt.Println("not-json")
			}
		case "":
			if valueString(message["id"]) == "99" {
				if !axi.Bool(axi.Obj(message["result"]), "success") {
					os.Exit(2)
				}
				emit(Object{"method": "item/completed", "params": Object{"item": Object{"type": "agentMessage", "text": "Observed exact fixture"}}})
				emit(Object{"method": "turn/completed", "params": Object{"turn": Object{"id": "turn-1", "status": "completed"}}})
			}
		}
	}
	os.Exit(0)
}

func testRPCClient(t *testing.T, ctx context.Context, mode string, tool func(context.Context, Object) (Object, error)) *rpcClient {
	t.Helper()
	env := append(os.Environ(), "TEAMCITY_AXI_EVALUATION_HELPER="+mode)
	client, err := startRPC(ctx, os.Args[0], []string{"-test.run=TestEvaluationHelperProcess"}, env, t.TempDir(), tool)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func TestEvaluationRPCHandshakeToolRoundtripAndFinalEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	calls := 0
	client := testRPCClient(t, ctx, "rpc", func(_ context.Context, message Object) (Object, error) {
		params := axi.Obj(message["params"])
		command, err := ValidateShellRequest(params, calls)
		if err != nil || command != "teamcity-axi --version" {
			return nil, fmt.Errorf("invalid tool roundtrip")
		}
		calls++
		return Object{"success": true, "contentItems": []Object{{"type": "inputText", "text": "0.1.0"}}}, nil
	})
	if _, err := client.request("initialize", Object{}); err != nil {
		t.Fatal(err)
	}
	if err := client.send(Object{"method": "initialized"}); err != nil {
		t.Fatal(err)
	}
	metadata, err := client.request("thread/start", Object{})
	if err != nil || axi.Str(metadata, "model") != "fixture-model" {
		t.Fatalf("thread metadata: %#v, %v", metadata, err)
	}
	if _, err := client.request("turn/start", Object{}); err != nil {
		t.Fatal(err)
	}
	turn, err := client.awaitTurn()
	client.tools.Wait()
	if err != nil || axi.Str(turn, "status") != "completed" || calls != 1 || len(client.Events()) != 2 {
		t.Fatalf("turn evidence: %#v, %v", turn, err)
	}
}

func TestEvaluationRPCRejectsUnexpectedToolsAndMalformedDocuments(t *testing.T) {
	for _, mode := range []string{"unexpected", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			client := testRPCClient(t, ctx, mode, func(context.Context, Object) (Object, error) { return nil, errors.New("unexpected call") })
			if _, err := client.request("turn/start", Object{}); err != nil {
				t.Fatal(err)
			}
			if _, err := client.awaitTurn(); err == nil {
				t.Fatal("invalid protocol accepted")
			}
		})
	}
}

func TestEvaluationRPCTimeoutUnblocksPendingRequestsAndReapsProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	client := testRPCClient(t, ctx, "silent", nil)
	start := time.Now()
	_, err := client.request("unanswered", Object{})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("pending request survived timeout: %v", err)
	}
}

func TestEvaluationProcessCaptureAndCancellation(t *testing.T) {
	for _, mode := range []string{"capture", "sleep"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			result, err := Execute(ctx, os.Args[0], []string{"-test.run=TestEvaluationHelperProcess"}, append(os.Environ(), "TEAMCITY_AXI_EVALUATION_HELPER="+mode), t.TempDir(), 1024)
			if err == nil || len(result.Stdout)+len(result.Stderr) > 1024 {
				t.Fatalf("unbounded capture or immortal child: %#v, %v", result, err)
			}
			if mode == "capture" && !result.CaptureLimitHit {
				t.Fatal("capture ceiling not recorded")
			}
			if mode == "sleep" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("deadline not propagated")
			}
		})
	}
}

func TestEvaluationBenchmarkEvidenceDoesNotUpgradeUnknownSourceOrOutcome(t *testing.T) {
	body := Object{"run_id": "482193", "status": "FAILURE", "problems": []Object{}, "failed_tests": Object{"testOccurrence": []Object{}}}
	value := nativeDiagnosticsEvidence(body)
	if len(axi.Strings(value["unavailableSources"])) != 0 || len(axi.Strings(value["emptySources"])) != 0 || value["state"] != nil {
		t.Fatal("native combined summary manufactured source availability")
	}
	if !hasCycle([]string{"1>2", "2>3", "3>1"}) || hasCycle([]string{"1>2", "1>3", "2>4", "3>4"}) {
		t.Fatal("evaluation confuses shared descendants and cycles")
	}
	if !reflect.DeepEqual(axi.Strings(value["nodeIds"]), []string{"482193"}) {
		t.Fatal("native exact run identity changed")
	}
}
