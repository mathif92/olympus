package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClaims() Claims {
	return Claims{
		Subject:       "alice",
		PrincipalType: "user",
		AccountID:     "default",
		ProjectID:     "proj-1",
		ExpiresAt:     time.Now().Add(time.Hour),
	}
}

func TestMiddlewareAllow(t *testing.T) {
	var got map[string]string
	themis := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Account-Id") != "default" {
			t.Errorf("missing account header")
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"allowed":true}`))
	}))
	defer themis.Close()

	c := NewClient(themis.URL, testSecret)
	handler := c.Middleware(func(r *http.Request) (string, string) { return "iris:POST", "/queues" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cl := ContextClaims(r.Context())
			if cl == nil || cl.Subject != "alice" {
				t.Errorf("claims not injected: %+v", cl)
			}
			if r.Header.Get("X-Account-Id") != "default" {
				t.Errorf("tenant header not bound")
			}
			_, _ = w.Write([]byte("ok"))
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "/queues", nil)
	req.Header.Set("Authorization", "Bearer "+sign(t, testSecret, newTestClaims()))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got["action"] != "iris:POST" || got["resource"] != "/queues" {
		t.Fatalf("authorize body mismatch: %+v", got)
	}
	if got["project"] != "proj-1" {
		t.Fatalf("expected token project id, got %q", got["project"])
	}
}

func TestMiddlewareDeny(t *testing.T) {
	themis := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"allowed":false}`))
	}))
	defer themis.Close()

	c := NewClient(themis.URL, testSecret)
	handler := c.Middleware(func(r *http.Request) (string, string) { return "iris:POST", "/queues" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }),
	)
	req := httptest.NewRequest(http.MethodPost, "/queues", nil)
	req.Header.Set("Authorization", "Bearer "+sign(t, testSecret, newTestClaims()))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestMiddlewareMissingToken(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", testSecret)
	handler := c.Middleware(func(r *http.Request) (string, string) { return "a", "/b" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }),
	)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/b", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMiddlewareInvalidToken(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", testSecret)
	handler := c.Middleware(func(r *http.Request) (string, string) { return "a", "/b" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }),
	)
	req := httptest.NewRequest(http.MethodGet, "/b", nil)
	req.Header.Set("Authorization", "Bearer garbage")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMiddlewareThemisUnreachable(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", testSecret) // nothing listening
	handler := c.Middleware(func(r *http.Request) (string, string) { return "a", "/b" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }),
	)
	req := httptest.NewRequest(http.MethodGet, "/b", nil)
	req.Header.Set("Authorization", "Bearer "+sign(t, testSecret, newTestClaims()))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestAuthorizeSendsAccount(t *testing.T) {
	var account string
	themis := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account = r.Header.Get("X-Account-Id")
		_, _ = w.Write([]byte(`{"allowed":true}`))
	}))
	defer themis.Close()

	c := NewClient(themis.URL, testSecret)
	ok, err := c.Authorize(context.Background(), "acme", "proj-1", "user", "alice", "a", "/b")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if !ok {
		t.Fatal("expected allowed")
	}
	if account != "acme" {
		t.Fatalf("expected account header acme, got %q", account)
	}
}
