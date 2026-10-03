package axi

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// These wire snapshots were captured before the v0.1.1 optimization. They check
// both formats and byte reductions, rather than reproducing the renderer.
func renderRegressionInputs(t testing.TB) map[string]Response {
	t.Helper()
	cases := map[string]Response{}
	for _, name := range []string{"status.exact", "run.list", "failure.partial", "failure.not-failed", "error"} {
		b, err := os.ReadFile("../../examples/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var r Response
		if err = json.Unmarshal(b, &r); err != nil {
			t.Fatal(err)
		}
		cases[name] = r
	}
	r := cases["error"]
	r.Error = &DomainError{Code: "INTERNAL_ERROR", Message: "synthetic\x1b[31m-credential\x00\u202e 日本語", Retryable: false, ExitCode: 99, HTTPStatus: 401, RetryAfter: "private", Details: Object{"nil": nil, "empty": []Object{}, "items": []Object{{"token": "synthetic-credential", "message": "Bearer abc"}}}}
	r.Context = Object{"server": "work", "branch": strings.Repeat("日本語", 3000)}
	r.Meta = Object{"observedAt": "2026-10-03T00:00:00.000Z", "complete": false, "truncated": false, "counts": Object{"childProcesses": 3}}
	r.Next = []Object{{"reason": "Inspect context", "argv": []string{"teamcity-axi", "context", "show"}}}
	cases["controls-and-budget"] = r
	r = cases["error"]
	r.Meta = Object{"observedAt": "2026-10-03T00:00:00.000Z", "complete": false, "truncated": false}
	notes := []Limitation{}
	for i := 0; i < 100; i++ {
		notes = append(notes, Limitation{Code: "SOURCE_UNAVAILABLE", Message: "Unavailable", Source: "tests", RunID: "123"})
	}
	r.Meta["limitations"] = notes
	cases["compacted-notes"] = r
	return cases
}

func TestRenderRetainsPreOptimizationWire(t *testing.T) {
	b, err := os.ReadFile("testdata/render-v0.1.0.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]string
	if err = json.Unmarshal(b, &expected); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for name, input := range renderRegressionInputs(t) {
		for _, format := range []string{"json", "toon"} {
			for _, limit := range []int{2048, 65536} {
				key := name + "/" + format + "/" + strconv.Itoa(limit)
				before, _ := json.Marshal(input)
				got, err := Render(input, format, limit, []string{"synthetic-credential"}, nil, Object{"maxChildProcesses": 20})
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if want, ok := expected[key]; !ok || got.Document != want {
					t.Errorf("%s/%s/%d: wire changed", name, format, limit)
				}
				after, _ := json.Marshal(input)
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("%s: input changed", name)
				}
				seen++
			}
		}
	}
	if seen != len(expected) {
		t.Fatal("snapshot inventory changed", seen, len(expected))
	}
}

func BenchmarkRender(b *testing.B) {
	cases := renderRegressionInputs(b)
	for _, name := range []string{"status.exact", "failure.partial", "compacted-notes"} {
		for _, format := range []string{"json", "toon"} {
			b.Run(name+"/"+format, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := Render(cases[name], format, 65536, nil, nil, nil); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkSanitizeText(b *testing.B) {
	for name, s := range map[string]string{"ascii": strings.Repeat("Build failed with connection refused. ", 50), "unicode": strings.Repeat("日本語 café ", 50), "controls": strings.Repeat("synthetic\x1b[31m-credential\x00\u202e ", 50)} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				SanitizeText(s, []string{"synthetic-credential"})
			}
		})
	}
}

func TestBoundedTextPreservesRuneBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text  string
		limit int
		want  string
		cut   bool
	}{
		{"", 0, "", false}, {"a", 0, "", true},
		{"日本語", 3, "日本語", false}, {"日本語", 2, "日本", true},
		{"🙂x", 1, "🙂", true}, {"a\xffb", 2, "a\ufffd", true},
		{"a\xff", 2, "a\xff", false},
	} {
		got, cut := boundedText(tc.text, tc.limit)
		if got != tc.want || cut != tc.cut {
			t.Fatalf("%q/%d: got %q/%v", tc.text, tc.limit, got, cut)
		}
	}
}

func TestRenderRejectsNonJSONAndInvalidPayload(t *testing.T) {
	cycle := Object{}
	cycle["cycle"] = cycle
	for _, data := range []Object{{"value": math.NaN()}, {"value": make(chan int)}, cycle} {
		for _, format := range []string{"json", "toon"} {
			if _, err := Render(NewResponse("schema", data), format, 65536, nil, nil, nil); err == nil {
				t.Fatal("non-JSON input accepted")
			}
		}
	}
	// A valid envelope must still fail its command-specific payload schema.
	_, err := Render(NewResponse("run.view", Object{"bogus": true}), "json", 65536, nil, nil, nil)
	if err == nil || AsDomainError(err).Message != "Normalized payload violated its public contract" {
		t.Fatal("payload gate bypassed", err)
	}
}
