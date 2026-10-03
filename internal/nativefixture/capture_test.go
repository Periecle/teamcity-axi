package nativefixture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingAndMismatchedBinaryFail(t *testing.T) {
	if _, err := Verify("", "../../docs/compatibility.json"); err == nil {
		t.Fatal("missing binary became success")
	}
	path := filepath.Join(t.TempDir(), "teamcity")
	if err := os.WriteFile(path, []byte("unverified"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(path, "../../docs/compatibility.json"); err == nil {
		t.Fatal("unverified binary became success")
	}
}
func TestFrozenNativeOperations(t *testing.T) {
	operations, modes, err := Operations()
	if err != nil || len(operations) < 20 || len(modes) != 5 {
		t.Fatal("required protocol fixture missing", err)
	}
	for _, op := range operations {
		if op.Name == "" || len(op.Argv) == 0 {
			t.Fatal("invalid fixture")
		}
		if op.Argv[0] == "api" {
			method := ""
			for i, s := range op.Argv {
				if s == "-X" && i+1 < len(op.Argv) {
					method = op.Argv[i+1]
				}
			}
			if method != "GET" {
				t.Fatal("native fixture mutation")
			}
		} else if op.Argv[0] != "run" {
			t.Fatal("unsupported native fixture family")
		}
	}
}
