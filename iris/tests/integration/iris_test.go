// Package integration contains end-to-end tests that exercise Iris against a
// real PostgreSQL started with testcontainers. Iris is the broker itself, so
// the full send/poll/fan-out/webhook flow runs against real Postgres.
package integration

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mathif92/olympus/iris/internal/handler"
	"github.com/mathif92/olympus/iris/pkg"
	"github.com/mathif92/olympus/iris/pkg/database"
)

// startPostgres boots a real Postgres, applies the goose migrations, and
// returns a ready database.Client plus a cleanup func.
func startPostgres(t *testing.T) (*database.Client, func()) {
	t.Helper()

	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("olympus_messaging"),
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

func newIris(t *testing.T) (*pkg.Iris, func()) {
	t.Helper()
	client, stop := startPostgres(t)
	return pkg.NewIris(client), stop
}

func ensureTenant(t *testing.T, ir *pkg.Iris, id string) {
	t.Helper()
	if err := ir.EnsureAccount(context.Background(), database.Account{
		ID: id, DisplayName: id, Email: id + "@i.dev", Plan: "pro", QueueLimit: 100,
	}); err != nil {
		t.Fatalf("ensure account %s: %v", id, err)
	}
}

func TestQueueLifecycle(t *testing.T) {
	ir, stop := newIris(t)
	defer stop()
	ctx := context.Background()

	ensureTenant(t, ir, "acme")
	if err := ir.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	q, err := ir.CreateQueue(ctx, "acme", "prod", database.Queue{Name: "jobs", VisibilityTimeoutSec: 2})
	if err != nil {
		t.Fatalf("create queue: %v", err)
	}
	if q.State != "active" {
		t.Fatalf("expected active state, got %q", q.State)
	}

	msg, err := ir.SendMessage(ctx, "acme", "prod", "jobs", "hello", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if msg.State != pkg.MsgPending {
		t.Fatalf("expected pending state, got %q", msg.State)
	}

	// First poll takes the message in_flight and hides it from the next poll.
	polled, err := ir.PollMessages(ctx, "acme", "prod", "jobs", 10)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(polled) != 1 || polled[0].ID != msg.ID {
		t.Fatalf("expected the sent message, got %+v", polled)
	}
	if polled[0].State != pkg.MsgInFlight {
		t.Fatalf("expected in_flight state, got %q", polled[0].State)
	}
	again, err := ir.PollMessages(ctx, "acme", "prod", "jobs", 10)
	if err != nil {
		t.Fatalf("second poll: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected no visible messages while in_flight, got %+v", again)
	}

	// Ack removes it.
	if err := ir.AckMessage(ctx, "acme", "prod", "jobs", msg.ID); err != nil {
		t.Fatalf("ack: %v", err)
	}
	after, err := ir.PollMessages(ctx, "acme", "prod", "jobs", 10)
	if err != nil {
		t.Fatalf("poll after ack: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected empty queue after ack, got %+v", after)
	}

	// Delete the queue.
	if err := ir.DeleteQueue(ctx, "acme", "prod", "jobs"); err != nil {
		t.Fatalf("delete queue: %v", err)
	}
	if _, err := ir.GetQueue(ctx, "acme", "prod", "jobs"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestVisibilityTimeoutReexposes(t *testing.T) {
	ir, stop := newIris(t)
	defer stop()
	ctx := context.Background()

	ensureTenant(t, ir, "acme")
	if err := ir.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := ir.CreateQueue(ctx, "acme", "prod", database.Queue{Name: "q", VisibilityTimeoutSec: 1}); err != nil {
		t.Fatalf("create queue: %v", err)
	}
	if _, err := ir.SendMessage(ctx, "acme", "prod", "q", "unacked", nil); err != nil {
		t.Fatalf("send message: %v", err)
	}

	polled, err := ir.PollMessages(ctx, "acme", "prod", "q", 10)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(polled) != 1 {
		t.Fatalf("expected 1 polled, got %d", len(polled))
	}

	// Sleep past the 1s visibility window: the unacked message becomes visible again.
	time.Sleep(1500 * time.Millisecond)
	polled, err = ir.PollMessages(ctx, "acme", "prod", "q", 10)
	if err != nil {
		t.Fatalf("poll after timeout: %v", err)
	}
	if len(polled) != 1 {
		t.Fatalf("expected the message re-exposed after visibility timeout, got %d", len(polled))
	}
}

func TestTopicFanOut(t *testing.T) {
	ir, stop := newIris(t)
	defer stop()
	ctx := context.Background()

	ensureTenant(t, ir, "acme")
	if err := ir.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	for _, name := range []string{"q1", "q2"} {
		if _, err := ir.CreateQueue(ctx, "acme", "prod", database.Queue{Name: name}); err != nil {
			t.Fatalf("create queue %s: %v", name, err)
		}
	}
	if _, err := ir.CreateTopic(ctx, "acme", "prod", database.Topic{Name: "events"}); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	if _, err := ir.SubscribeQueue(ctx, "acme", "prod", "events", "q1"); err != nil {
		t.Fatalf("subscribe q1: %v", err)
	}
	if _, err := ir.SubscribeQueue(ctx, "acme", "prod", "events", "q2"); err != nil {
		t.Fatalf("subscribe q2: %v", err)
	}

	queueCopies, webhookDeliveries, err := ir.PublishMessage(ctx, "acme", "prod", "events", "a broadcast")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if queueCopies != 2 {
		t.Fatalf("expected 2 queue copies, got %d", queueCopies)
	}
	if webhookDeliveries != 0 {
		t.Fatalf("expected 0 webhook deliveries, got %d", webhookDeliveries)
	}

	for _, name := range []string{"q1", "q2"} {
		msgs, err := ir.PollMessages(ctx, "acme", "prod", name, 10)
		if err != nil {
			t.Fatalf("poll %s: %v", name, err)
		}
		if len(msgs) != 1 || msgs[0].Body != "a broadcast" {
			t.Fatalf("expected the published message in %s, got %+v", name, msgs)
		}
	}
}

func TestWebhookDelivery(t *testing.T) {
	ir, stop := newIris(t)
	defer stop()
	ctx := context.Background()

	// A tiny HTTP receiver that records POSTs and replies 200.
	received := make(chan string, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	ensureTenant(t, ir, "acme")
	if err := ir.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := ir.CreateTopic(ctx, "acme", "prod", database.Topic{Name: "events"}); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	sub, err := ir.SubscribeWebhook(ctx, "acme", "prod", "events", receiver.URL+"/hook")
	if err != nil {
		t.Fatalf("subscribe webhook: %v", err)
	}
	if sub.Status != "active" {
		t.Fatalf("expected active subscriber, got %q", sub.Status)
	}

	_, deliveries, err := ir.PublishMessage(ctx, "acme", "prod", "events", `{"event":"signup"}`)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if deliveries != 1 {
		t.Fatalf("expected 1 successful webhook delivery, got %d", deliveries)
	}

	select {
	case body := <-received:
		if body != `{"event":"signup"}` {
			t.Fatalf("webhook received unexpected body %q", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("webhook never received the published message")
	}

	var status, lastError string
	var attempts int
	if err := ir.DB.QueryRow(ctx,
		`SELECT status, attempts, COALESCE(last_error, '') FROM webhook_deliveries WHERE subscriber_id = $1`,
		sub.ID).Scan(&status, &attempts, &lastError); err != nil {
		t.Fatalf("read delivery row: %v", err)
	}
	if status != "delivered" || attempts != 1 || lastError != "" {
		t.Fatalf("expected delivered/1 attempts, got %q/%d/%q", status, attempts, lastError)
	}
}

func TestWebhookRetriesOnFailure(t *testing.T) {
	ir, stop := newIris(t)
	defer stop()
	ctx := context.Background()

	// A receiver that fails the first two attempts then accepts.
	var calls int
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	ensureTenant(t, ir, "acme")
	if err := ir.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := ir.CreateTopic(ctx, "acme", "prod", database.Topic{Name: "events"}); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	sub, err := ir.SubscribeWebhook(ctx, "acme", "prod", "events", receiver.URL+"/hook")
	if err != nil {
		t.Fatalf("subscribe webhook: %v", err)
	}

	began := time.Now()
	_, deliveries, err := ir.PublishMessage(ctx, "acme", "prod", "events", "flaky")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if deliveries != 1 {
		t.Fatalf("expected delivery recovered after retries, got %d", deliveries)
	}
	if calls < 3 {
		t.Fatalf("expected retries to hit the endpoint 3 times, got %d", calls)
	}
	if time.Since(began) < 300*time.Millisecond {
		t.Fatalf("expected backoff between attempts, entire delivery took %v", time.Since(began))
	}

	var status string
	var attempts int
	if err := ir.DB.QueryRow(ctx,
		`SELECT status, attempts FROM webhook_deliveries WHERE subscriber_id = $1`, sub.ID).
		Scan(&status, &attempts); err != nil {
		t.Fatalf("read delivery row: %v", err)
	}
	if status != "delivered" || attempts != 3 {
		t.Fatalf("expected delivered/3 attempts, got %q/%d", status, attempts)
	}
}

func TestTenantIsolation(t *testing.T) {
	ir, stop := newIris(t)
	defer stop()
	ctx := context.Background()

	ensureTenant(t, ir, "tenant-a")
	ensureTenant(t, ir, "tenant-b")

	if err := ir.CreateProject(ctx, "tenant-a", database.Project{Name: "lab"}); err != nil {
		t.Fatalf("create project for a: %v", err)
	}
	if _, err := ir.CreateQueue(ctx, "tenant-a", "lab", database.Queue{Name: "q"}); err != nil {
		t.Fatalf("create queue in a: %v", err)
	}
	if _, err := ir.SendMessage(ctx, "tenant-a", "lab", "q", "secret", nil); err != nil {
		t.Fatalf("send message in a: %v", err)
	}

	for _, probe := range []func() error{
		func() error { _, err := ir.ListQueues(ctx, "tenant-b", "lab"); return err },
		func() error { _, err := ir.GetQueue(ctx, "tenant-b", "lab", "q"); return err },
		func() error { _, err := ir.PollMessages(ctx, "tenant-b", "lab", "q", 10); return err },
		func() error { return ir.AckMessage(ctx, "tenant-b", "lab", "q", "whatever") },
		func() error { return ir.DeleteQueue(ctx, "tenant-b", "lab", "q") },
	} {
		if err := probe(); err != pkg.ErrNotFound {
			t.Fatalf("expected ErrNotFound for tenant-b on tenant-a resource, got %v", err)
		}
	}

	msgs, err := ir.PollMessages(ctx, "tenant-a", "lab", "q", 10)
	if err != nil {
		t.Fatalf("tenant-a poll: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("tenant-a should still see its message, got %d", len(msgs))
	}
}

// TestHTTPEndpoints drives the real mux and verifies audit trail entries are
// written for each operation.
func TestHTTPEndpoints(t *testing.T) {
	ir, stopFn := newIris(t)
	defer stopFn()
	ctx := context.Background()

	mux := handler.NewIrisHandler(ir).Router()
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
	if resp, body := do(http.MethodPost, "/queues", `{"project":"prod","name":"jobs"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create queue: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/queue/prod/jobs/send", `{"body":"hello"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("send message: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/queue/prod/jobs/poll", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("poll: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/topics", `{"project":"prod","name":"events"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create topic: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/topic/prod/events/subscribe", `{"queue_name":"jobs"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("subscribe: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/topic/prod/events/publish", `{"body":"broadcast"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("publish: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodGet, "/topic/prod/events/subscribers", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("list subscribers: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodGet, "/queues?project=prod", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("list queues: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodDelete, "/queue/prod/jobs", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("delete queue: %d %s", resp.StatusCode, body)
	}

	var auditCount int
	if err := ir.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = 'acme'`).Scan(&auditCount); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("expected audit trail rows from HTTP operations")
	}
}
