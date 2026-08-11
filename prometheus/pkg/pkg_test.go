package pkg

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/mathif92/olympus/prometheus/pkg/database"
)

// zipOf builds an in-memory zip archive from a name -> content map.
func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func TestRuntimesRegistry(t *testing.T) {
	rts := ListRuntimes()
	if len(rts) != 8 {
		t.Fatalf("expected 8 runtimes, got %d", len(rts))
	}
	for _, rt := range rts {
		if rt.ID == "" || rt.Name == "" || rt.Image == "" || len(rt.RequiredFiles) == 0 {
			t.Fatalf("runtime %+v is missing metadata", rt)
		}
		// Every runtime must have an embedded Dockerfile scaffold so the docker
		// executor can materialise a build context on the fly.
		sub, err := fs.Sub(runtimesFS, filepath.Join("runtimes", rt.ID))
		if err != nil {
			t.Fatalf("runtime %s has no embedded scaffold: %v", rt.ID, err)
		}
		if _, err := fs.Stat(sub, "Dockerfile"); err != nil {
			t.Fatalf("runtime %s is missing an embedded Dockerfile: %v", rt.ID, err)
		}
		if _, ok := GetRuntime(rt.ID); !ok {
			t.Fatalf("GetRuntime(%q) failed", rt.ID)
		}
	}
	if _, ok := GetRuntime("pascal"); ok {
		t.Fatal("GetRuntime accepted an unknown runtime")
	}
}

func TestValidateFunctionCode(t *testing.T) {
	py, _ := GetRuntime("python3.12")
	goRT, _ := GetRuntime("go1.25")

	if err := ValidateFunctionCode(zipOf(t, map[string]string{"handler.py": "def handler(event):\n    return {}\n"}), py); err != nil {
		t.Fatalf("valid python zip rejected: %v", err)
	}
	if err := ValidateFunctionCode(zipOf(t, map[string]string{"nope.py": "x = 1\n"}), py); err == nil {
		t.Fatal("expected error for missing handler.py")
	}
	if err := ValidateFunctionCode(zipOf(t, map[string]string{"handler.go": "package main\n"}), goRT); err != nil {
		t.Fatalf("valid go zip rejected: %v", err)
	}
	// Empty / corrupt archives are rejected.
	if err := ValidateFunctionCode([]byte("not a zip"), py); err == nil {
		t.Fatal("expected error for corrupt archive")
	}
	if err := ValidateFunctionCode([]byte{}, py); err == nil {
		t.Fatal("expected error for empty archive")
	}
	// Zip-slip entries are rejected regardless of separator.
	if err := ValidateFunctionCode(zipOf(t, map[string]string{"handler.py": "x", "../evil": "boom"}), py); err == nil {
		t.Fatal("expected error for ../ traversal")
	}
	if err := ValidateFunctionCode(zipOf(t, map[string]string{"/abs/handler.py": "x"}), py); err == nil {
		t.Fatal("expected error for absolute path entry")
	}
}

func TestExtractZip(t *testing.T) {
	dir := t.TempDir()
	code := zipOf(t, map[string]string{
		"handler.py":      "def handler(event):\n    return event\n",
		"libs/util.py":    "def helper():\n    return 1\n",
		"data/sample.txt": "hello",
	})
	if err := extractZip(code, dir); err != nil {
		t.Fatalf("extract: %v", err)
	}
	for _, want := range []string{"handler.py", "libs/util.py", "data/sample.txt"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Fatalf("expected extracted file %s: %v", want, err)
		}
	}
}

func TestExtractZipRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := extractZip(zipOf(t, map[string]string{"../escape.txt": "x"}), dir); err == nil {
		t.Fatal("expected traversal extraction to fail")
	}
}

func TestMockExecutorEchoesEvent(t *testing.T) {
	m := NewMockExecutor()
	event := json.RawMessage(`{"name":"alice"}`)
	res, err := m.Invoke(context.Background(), database.Function{}, database.FunctionVersion{}, event)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.Status != StatusSuccess || res.Response != string(event) {
		t.Fatalf("unexpected mock result: %+v", res)
	}
	if err := m.Healthy(context.Background()); err != nil {
		t.Fatalf("mock unhealthy: %v", err)
	}
	if got := len(m.Invocations()); got != 1 {
		t.Fatalf("expected 1 recorded invocation, got %d", got)
	}
}
