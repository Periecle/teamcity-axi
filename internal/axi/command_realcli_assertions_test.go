//go:build realcli

package axi

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func requireNativeValue(t *testing.T, actual, expected any) {
	t.Helper()
	a, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("value mismatch\nactual: %s\nexpected: %s", a, b)
	}
}

func requireNativeKeys(t *testing.T, object Object, expected ...string) {
	t.Helper()
	keys := keysOf(object)
	sort.Strings(keys)
	sort.Strings(expected)
	if !reflect.DeepEqual(keys, expected) {
		t.Fatalf("keys %v want %v", keys, expected)
	}
}

func requireNativePartial(t *testing.T, r nativeCall) {
	t.Helper()
	requireNativeValue(t, r.value["status"], "partial")
	requireNativeValue(t, nativeMeta(r)["complete"], false)
}

func nativeNoteCount(r nativeCall, code string) int {
	n := 0
	for _, note := range Objects(nativeMeta(r)["limitations"]) {
		if Str(note, "code") == code {
			n++
		}
	}
	return n
}

func nativeNoteMessage(r nativeCall, contains string) bool {
	for _, note := range Objects(nativeMeta(r)["limitations"]) {
		if strings.Contains(Str(note, "message"), contains) {
			return true
		}
	}
	return false
}

func nativeCapability(r nativeCall, name string) Object {
	for _, capability := range Objects(nativeData(r)["capabilities"]) {
		if Str(capability, "name") == name {
			return capability
		}
	}
	return nil
}

func nativeSource(r nativeCall, id string) Object {
	for _, source := range Objects(nativeData(r)["sources"]) {
		if Str(source, "id") == id {
			return source
		}
	}
	return nil
}

func stableNativeFindings(value any) []Object {
	encoded, _ := json.Marshal(value)
	var decoded []Object
	json.Unmarshal(encoded, &decoded)
	for _, finding := range decoded {
		for _, evidence := range Objects(finding["evidence"]) {
			delete(evidence, "observedAt")
		}
	}
	return decoded
}

func requireNativeAbsent(t *testing.T, object Object, key string) {
	t.Helper()
	if _, exists := object[key]; exists {
		t.Fatalf("optional field %s must be absent", key)
	}
}
