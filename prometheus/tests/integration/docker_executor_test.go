package integration

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/mathif92/olympus/prometheus/pkg"
	"github.com/mathif92/olympus/prometheus/pkg/database"
)

// The real Docker-backed executor builds a per-runtime image from the uploaded
// code (on the fly) and runs the handler in a constrained container. These
// tests exercise the subset of runtimes verified end-to-end; they need a
// Docker daemon and network to pull base images on the first build.
func dockerExecutor(t *testing.T) *pkg.DockerExecutor {
	t.Helper()
	if os.Getenv("RUN_DOCKER_TESTS") == "" {
		t.Skip("set RUN_DOCKER_TESTS=1 to exercise the real docker executor (needs a Docker daemon)")
	}
	return pkg.NewDockerExecutor(pkg.DockerExecutorConfig{})
}

func fn(runtime string) database.Function {
	return database.Function{Runtime: runtime, TimeoutMS: 30000, MemoryMB: 128, CPUs: 0.5}
}

func TestDockerExecutorRunsPython(t *testing.T) {
	e := dockerExecutor(t)
	ctx := context.Background()

	v := database.FunctionVersion{ID: "it-python", Code: zipOf(t, map[string]string{
		"handler.py": "def handler(event):\n    return {\"echo\": event, \"lang\": \"python\"}\n",
	})}
	res, err := e.Invoke(ctx, fn("python3.12"), v, json.RawMessage(`{"x":1}`))
	if err != nil {
		t.Fatalf("invoke python: %v", err)
	}
	if res.Status != pkg.StatusSuccess {
		t.Fatalf("python status=%s error=%s", res.Status, res.Error)
	}
	if !strings.Contains(res.Response, `"lang": "python"`) || !strings.Contains(res.Response, `"x": 1`) {
		t.Fatalf("unexpected python response: %q", res.Response)
	}
}

func TestDockerExecutorRunsNode(t *testing.T) {
	e := dockerExecutor(t)
	ctx := context.Background()

	v := database.FunctionVersion{ID: "it-node", Code: zipOf(t, map[string]string{
		"handler.js": "exports.handler = async (event) => ({ echo: event, lang: 'node' })\n",
	})}
	res, err := e.Invoke(ctx, fn("nodejs20"), v, json.RawMessage(`{"x":2}`))
	if err != nil {
		t.Fatalf("invoke node: %v", err)
	}
	if res.Status != pkg.StatusSuccess {
		t.Fatalf("node status=%s error=%s", res.Status, res.Error)
	}
	if !strings.Contains(res.Response, `"lang":"node"`) || !strings.Contains(res.Response, `"x":2`) {
		t.Fatalf("unexpected node response: %q", res.Response)
	}
}

func TestDockerExecutorRunsGo(t *testing.T) {
	e := dockerExecutor(t)
	ctx := context.Background()

	v := database.FunctionVersion{ID: "it-go", Code: zipOf(t, map[string]string{
		"handler.go": `package main

import "encoding/json"

func Handler(event string) (string, error) {
	var e map[string]any
	_ = json.Unmarshal([]byte(event), &e)
	out, _ := json.Marshal(map[string]any{"echo": e, "lang": "go"})
	return string(out), nil
}
`,
	})}
	res, err := e.Invoke(ctx, fn("go1.25"), v, json.RawMessage(`{"x":3}`))
	if err != nil {
		t.Fatalf("invoke go: %v", err)
	}
	if res.Status != pkg.StatusSuccess {
		t.Fatalf("go status=%s error=%s", res.Status, res.Error)
	}
	if !strings.Contains(res.Response, `"lang":"go"`) || !strings.Contains(res.Response, `"x":3`) {
		t.Fatalf("unexpected go response: %q", res.Response)
	}
}

func TestDockerExecutorTimeout(t *testing.T) {
	e := dockerExecutor(t)
	ctx := context.Background()

	v := database.FunctionVersion{ID: "it-slow", Code: zipOf(t, map[string]string{
		"handler.py": "import time\n\ndef handler(event):\n    time.sleep(30)\n    return {}\n",
	})}
	f := fn("python3.12")
	f.TimeoutMS = 1500
	res, err := e.Invoke(ctx, f, v, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("invoke slow python: %v", err)
	}
	if res.Status != pkg.StatusTimeout {
		t.Fatalf("expected timeout status, got %s (error=%s)", res.Status, res.Error)
	}
}
