// Package integration contains end-to-end tests that exercise Prometheus
// against a real PostgreSQL started with testcontainers. Function execution
// uses the in-process mock executor; the real Docker-backed executor is tested
// separately in docker_executor_test.go behind RUN_DOCKER_TESTS.
package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mathif92/olympus/prometheus/internal/handler"
	"github.com/mathif92/olympus/prometheus/pkg"
	"github.com/mathif92/olympus/prometheus/pkg/database"
)

// startPostgres boots a real Postgres, applies the goose migrations, and
// returns a ready database.Client plus a cleanup func.
func startPostgres(t *testing.T) (*database.Client, func()) {
	t.Helper()

	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("olympus_functions"),
		postgres.WithUsername("olympus"),
		postgres.WithPassword("olympus_secret"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	url, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}

	client, err := database.NewClient(database.Config{
		PostgresURL: url,
		PoolMax:     10,
		PoolMin:     2,
		PoolTimeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("new database client: %v", err)
	}

	dir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	if err := database.Migrate(client.DB, dir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	stop := func() {
		client.Close()
		_ = pg.Terminate(context.Background())
	}
	return client, stop
}

func newPrometheus(t *testing.T) (*pkg.Prometheus, func()) {
	t.Helper()
	client, stop := startPostgres(t)
	return pkg.NewPrometheus(client, pkg.NewMockExecutor()), stop
}

func ensureTenant(t *testing.T, p *pkg.Prometheus, id string) {
	t.Helper()
	if err := p.EnsureAccount(context.Background(), database.Account{
		ID: id, DisplayName: id, Email: id + "@p.dev", Plan: "pro", FunctionLimit: 64,
	}); err != nil {
		t.Fatalf("ensure account %s: %v", id, err)
	}
}

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

func TestFunctionLifecycle(t *testing.T) {
	p, stop := newPrometheus(t)
	defer stop()
	ctx := context.Background()

	ensureTenant(t, p, "acme")
	if err := p.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	fn, err := p.CreateFunction(ctx, "acme", "prod", database.Function{Name: "greet", Runtime: "python3.12"})
	if err != nil {
		t.Fatalf("create function: %v", err)
	}
	if fn.TimeoutMS != 30000 || fn.MemoryMB != 128 || fn.Handler != "handler.handler" || fn.CurrentVersion != 0 {
		t.Fatalf("unexpected defaults: %+v", fn)
	}

	// Deploy v1 (handler.py echoes the event back) and invoke it.
	code := zipOf(t, map[string]string{"handler.py": "def handler(event):\n    return event\n"})
	ver, err := p.UploadVersion(ctx, "acme", "prod", "greet", code)
	if err != nil {
		t.Fatalf("upload version: %v", err)
	}
	if ver.Version != 1 || !ver.IsActive || ver.CodeSize != len(code) {
		t.Fatalf("unexpected version: %+v", ver)
	}

	inv, err := p.InvokeFunction(ctx, "acme", "prod", "greet", json.RawMessage(`{"msg":"hello"}`))
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if inv.Status != pkg.StatusSuccess || inv.Response != `{"msg":"hello"}` {
		t.Fatalf("unexpected invocation: %+v", inv)
	}

	// Redeploy bumps the version and the new code becomes active.
	code2 := zipOf(t, map[string]string{"handler.py": "def handler(event):\n    return {\"v\": 2}\n"})
	ver2, err := p.UploadVersion(ctx, "acme", "prod", "greet", code2)
	if err != nil {
		t.Fatalf("upload v2: %v", err)
	}
	if ver2.Version != 2 || !ver2.IsActive {
		t.Fatalf("expected v2 active, got %+v", ver2)
	}

	versions, err := p.ListVersions(ctx, "acme", "prod", "greet")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 2 || versions[0].Version != 2 {
		t.Fatalf("expected 2 versions newest-first, got %+v", versions)
	}
	if versions[1].IsActive {
		t.Fatalf("v1 must be inactive after redeploy: %+v", versions[1])
	}

	inv2, err := p.InvokeFunction(ctx, "acme", "prod", "greet", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("invoke after redeploy: %v", err)
	}
	if inv2.Version != 2 {
		t.Fatalf("expected invocation on v2, got version %d", inv2.Version)
	}

	invs, err := p.ListInvocations(ctx, "acme", "prod", "greet", 10)
	if err != nil {
		t.Fatalf("list invocations: %v", err)
	}
	if len(invs) != 2 {
		t.Fatalf("expected 2 invocations, got %d", len(invs))
	}

	if err := p.DeleteFunction(ctx, "acme", "prod", "greet"); err != nil {
		t.Fatalf("delete function: %v", err)
	}
	if _, err := p.GetFunction(ctx, "acme", "prod", "greet"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestInvokeWithoutCodeFails(t *testing.T) {
	p, stop := newPrometheus(t)
	defer stop()
	ctx := context.Background()

	ensureTenant(t, p, "acme")
	if err := p.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := p.CreateFunction(ctx, "acme", "prod", database.Function{Name: "empty", Runtime: "nodejs20"}); err != nil {
		t.Fatalf("create function: %v", err)
	}
	if _, err := p.InvokeFunction(ctx, "acme", "prod", "empty", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error invoking a function with no deployed code")
	}
}

func TestUploadValidation(t *testing.T) {
	p, stop := newPrometheus(t)
	defer stop()
	ctx := context.Background()

	ensureTenant(t, p, "acme")
	if err := p.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := p.CreateFunction(ctx, "acme", "prod", database.Function{Name: "svc", Runtime: "bogus"}); err == nil {
		t.Fatal("expected error for unknown runtime")
	}
	if _, err := p.CreateFunction(ctx, "acme", "prod", database.Function{Name: "svc", Runtime: "nodejs20"}); err != nil {
		t.Fatalf("create function: %v", err)
	}

	cases := []struct {
		name string
		zip  map[string]string
	}{
		{"missing handler.js for nodejs20", map[string]string{"main.py": "x"}},
		{"wrong entrypoint file for nodejs20", map[string]string{"handler.ts": "x"}},
		{"path traversal entry", map[string]string{"handler.js": "x", "../evil": "boom"}},
		{"absolute path entry", map[string]string{"/etc/passwd": "x"}},
	}
	for _, c := range cases {
		if _, err := p.UploadVersion(ctx, "acme", "prod", "svc", zipOf(t, c.zip)); err == nil {
			t.Fatalf("expected upload error for case %q", c.name)
		}
	}

	if _, err := p.UploadVersion(ctx, "acme", "prod", "svc", []byte("not a zip")); err == nil {
		t.Fatal("expected error for corrupt archive")
	}
}

func TestHTTPEndpoints(t *testing.T) {
	p, stopFn := newPrometheus(t)
	defer stopFn()
	ctx := context.Background()

	mux := handler.NewPrometheusHandler(p).Router()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	do := func(method, path, body string) (*http.Response, string) {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+path, bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Account-Id", "acme")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return resp, string(data)
	}

	if resp, body := do(http.MethodPost, "/projects", `{"name":"prod"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/functions", `{"project":"prod","name":"greet","runtime":"python3.12"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create function: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodGet, "/runtimes", ""); resp.StatusCode != http.StatusOK || !bytes.Contains([]byte(body), []byte("python3.12")) {
		t.Fatalf("list runtimes: %d %s", resp.StatusCode, body)
	}

	// Deploy the code via a multipart upload.
	var zipBuf bytes.Buffer
	mw := multipart.NewWriter(&zipBuf)
	fw, err := mw.CreateFormFile("code", "greet.zip")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := fw.Write(zipOf(t, map[string]string{"handler.py": "def handler(event):\n    return event\n"})); err != nil {
		t.Fatalf("write zip part: %v", err)
	}
	mw.Close()
	uploadReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/function/prod/greet/versions", &zipBuf)
	uploadReq.Header.Set("Content-Type", mw.FormDataContentType())
	uploadReq.Header.Set("X-Account-Id", "acme")
	uploadResp, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	uploadBody, _ := io.ReadAll(uploadResp.Body)
	uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("upload version: %d %s", uploadResp.StatusCode, string(uploadBody))
	}

	// Invoke: the mock executor echoes the event back.
	if resp, body := do(http.MethodPost, "/function/prod/greet/invoke", `{"msg":"hi"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("invoke: %d %s", resp.StatusCode, body)
	} else if !bytes.Contains([]byte(body), []byte(`"status":"success"`)) || !bytes.Contains([]byte(body), []byte(`"msg":"hi"`)) {
		t.Fatalf("unexpected invoke body: %s", body)
	}

	// Invalid invoke payload is rejected.
	if resp, body := do(http.MethodPost, "/function/prod/greet/invoke", `{not json`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d %s", resp.StatusCode, body)
	}

	if resp, body := do(http.MethodGet, "/function/prod/greet/invocations", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("list invocations: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodGet, "/functions?project=prod", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("list functions: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodGet, "/function/prod/greet/versions", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("list versions: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodDelete, "/function/prod/greet", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("delete function: %d %s", resp.StatusCode, body)
	}

	var auditCount int
	if err := p.DB.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE account_id = 'acme'`).Scan(&auditCount); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("expected audit trail rows from HTTP operations")
	}
}
