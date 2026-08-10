package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mathif92/olympus/iris/pkg"
	"github.com/mathif92/olympus/iris/pkg/database"
)

// IrisHandler handles HTTP requests for multi-tenant messaging (queues + topics).
type IrisHandler struct {
	Iris *pkg.Iris
}

// NewIrisHandler creates a handler wired to the given control plane.
func NewIrisHandler(ir *pkg.Iris) *IrisHandler {
	return &IrisHandler{Iris: ir}
}

// Router returns the mux with all Iris routes registered.
func (h *IrisHandler) Router() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/projects", h.handleProjects)
	mux.HandleFunc("/queues", h.handleQueues)
	mux.HandleFunc("/queue/", h.handleQueue)
	mux.HandleFunc("/topics", h.handleTopics)
	mux.HandleFunc("/topic/", h.handleTopic)
	return mux
}

func accountName(r *http.Request) string {
	if id := r.Header.Get("X-Account-Id"); id != "" {
		return id
	}
	if id := r.Header.Get("X-Account-ID"); id != "" {
		return id
	}
	return "default"
}

func (h *IrisHandler) ensureAccount(r *http.Request) {
	_ = h.Iris.EnsureAccount(r.Context(), database.Account{
		ID:          accountName(r),
		DisplayName: accountName(r),
		Email:       accountName(r) + "@iris.internal",
		Plan:        "pro",
		QueueLimit:  100,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *IrisHandler) handleProjects(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.Iris.CreateProject(ctx, account, database.Project{Name: in.Name, Description: in.Description}); err != nil {
			http.Error(w, "create project: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Iris.Audit(ctx, account, "", "project", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, map[string]string{"project": in.Name})

	case http.MethodGet:
		projects, err := h.Iris.ListProjects(ctx, account)
		if err != nil {
			http.Error(w, "list projects: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = h.Iris.Audit(ctx, account, "", "project", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleQueues routes create/list against the /queues collection.
func (h *IrisHandler) handleQueues(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project              string `json:"project"`
			Name                 string `json:"name"`
			VisibilityTimeoutSec int    `json:"visibility_timeout_sec"`
			MessageRetentionSec  int    `json:"message_retention_sec"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		q, err := h.Iris.CreateQueue(ctx, account, in.Project, database.Queue{
			Name:                 in.Name,
			VisibilityTimeoutSec: in.VisibilityTimeoutSec,
			MessageRetentionSec:  in.MessageRetentionSec,
		})
		if err != nil {
			http.Error(w, "create queue: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Iris.Audit(ctx, account, q.ProjectID, "queue", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, q)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			http.Error(w, "missing project query param", http.StatusBadRequest)
			return
		}
		queues, err := h.Iris.ListQueues(ctx, account, project)
		if err != nil {
			http.Error(w, "list queues: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Iris.Audit(ctx, account, "", "queue", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"queues": queues})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleQueue routes GET/DELETE and message actions on /queue/{project}/{name}[/action].
func (h *IrisHandler) handleQueue(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	project, rest, ok := splitPath(r.URL.Path, "/queue/")
	if !ok || rest == "" {
		http.Error(w, "Invalid URL format. Expected /queue/{project}/{name}[/action]", http.StatusBadRequest)
		return
	}
	name, action := splitNameAction(rest)

	// Message actions on /queue/{project}/{name}/send|pull|ack
	if action != "" {
		h.handleQueueAction(w, r, account, project, name, action)
		return
	}

	switch r.Method {
	case http.MethodGet:
		q, err := h.Iris.GetQueue(ctx, account, project, name)
		if err != nil {
			http.Error(w, "queue not found", http.StatusNotFound)
			return
		}
		_ = h.Iris.Audit(ctx, account, q.ProjectID, "queue", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, q)

	case http.MethodDelete:
		if err := h.Iris.DeleteQueue(ctx, account, project, name); err != nil {
			http.Error(w, "delete queue: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Iris.Audit(ctx, account, "", "queue", pkg.OpDelete, "success")
		writeJSON(w, http.StatusOK, map[string]string{"queue": name, "state": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleQueueAction routes to send/poll/ack sub-actions of a queue.
func (h *IrisHandler) handleQueueAction(w http.ResponseWriter, r *http.Request, account, project, name, action string) {
	ctx := r.Context()

	switch action {
	case "send":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in struct {
			Body       string            `json:"body"`
			Attributes map[string]string `json:"attributes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		msg, err := h.Iris.SendMessage(ctx, account, project, name, in.Body, in.Attributes)
		if err != nil {
			http.Error(w, "send message: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Iris.Audit(ctx, account, "", "message", pkg.OpSend, "success")
		writeJSON(w, http.StatusCreated, msg)

	case "poll":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		msgs, err := h.Iris.PollMessages(ctx, account, project, name, 10)
		if err != nil {
			http.Error(w, "poll messages: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Iris.Audit(ctx, account, "", "message", pkg.OpPoll, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"messages": msgs})

	case "ack":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in struct {
			MessageID string `json:"message_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.Iris.AckMessage(ctx, account, project, name, in.MessageID); err != nil {
			http.Error(w, "ack message: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Iris.Audit(ctx, account, "", "message", pkg.OpAck, "success")
		writeJSON(w, http.StatusOK, map[string]string{"message_id": in.MessageID, "state": "delivered"})

	default:
		http.Error(w, "unknown queue action "+action, http.StatusNotFound)
	}
}

// handleTopics routes create/list against the /topics collection.
func (h *IrisHandler) handleTopics(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project string `json:"project"`
			Name    string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		t, err := h.Iris.CreateTopic(ctx, account, in.Project, database.Topic{Name: in.Name})
		if err != nil {
			http.Error(w, "create topic: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Iris.Audit(ctx, account, t.ProjectID, "topic", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, t)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			http.Error(w, "missing project query param", http.StatusBadRequest)
			return
		}
		topics, err := h.Iris.ListTopics(ctx, account, project)
		if err != nil {
			http.Error(w, "list topics: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Iris.Audit(ctx, account, "", "topic", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"topics": topics})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTopic routes topic actions on /topic/{project}/{name}[/action].
func (h *IrisHandler) handleTopic(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	project, rest, ok := splitPath(r.URL.Path, "/topic/")
	if !ok || rest == "" {
		http.Error(w, "Invalid URL format. Expected /topic/{project}/{name}[/action]", http.StatusBadRequest)
		return
	}
	name, action := splitNameAction(rest)

	switch action {
	case "publish":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		queueCopies, webhookDeliveries, err := h.Iris.PublishMessage(ctx, account, project, name, in.Body)
		if err != nil {
			http.Error(w, "publish: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Iris.Audit(ctx, account, "", "topic", pkg.OpPublish, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"topic":              name,
			"queue_copies":       queueCopies,
			"webhook_deliveries": webhookDeliveries,
		})

	case "subscribe":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in struct {
			QueueName  string `json:"queue_name"`
			WebhookURL string `json:"webhook_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		var sub *database.Subscriber
		var err error
		switch {
		case in.QueueName != "":
			sub, err = h.Iris.SubscribeQueue(ctx, account, project, name, in.QueueName)
		case in.WebhookURL != "":
			sub, err = h.Iris.SubscribeWebhook(ctx, account, project, name, in.WebhookURL)
		default:
			http.Error(w, "provide queue_name or webhook_url", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "subscribe: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Iris.Audit(ctx, account, "", "topic", pkg.OpSubscribe, "success")
		writeJSON(w, http.StatusCreated, sub)

	case "subscribers":
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		subs, err := h.Iris.ListSubscribers(ctx, account, project, name)
		if err != nil {
			http.Error(w, "list subscribers: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Iris.Audit(ctx, account, "", "topic", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"subscribers": subs})

	case "unsubscribe":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in struct {
			SubscriberID string `json:"subscriber_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.Iris.Unsubscribe(ctx, account, project, name, in.SubscriberID); err != nil {
			http.Error(w, "unsubscribe: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Iris.Audit(ctx, account, "", "topic", pkg.OpUnsubscribe, "success")
		writeJSON(w, http.StatusOK, map[string]string{"subscriber_id": in.SubscriberID, "state": "unsubscribed"})

	case "":
		switch r.Method {
		case http.MethodGet:
			t, err := h.Iris.GetTopic(ctx, account, project, name)
			if err != nil {
				http.Error(w, "topic not found", http.StatusNotFound)
				return
			}
			_ = h.Iris.Audit(ctx, account, t.ProjectID, "topic", pkg.OpList, "success")
			writeJSON(w, http.StatusOK, t)

		case http.MethodDelete:
			if err := h.Iris.DeleteTopic(ctx, account, project, name); err != nil {
				http.Error(w, "delete topic: "+err.Error(), http.StatusBadRequest)
				return
			}
			_ = h.Iris.Audit(ctx, account, "", "topic", pkg.OpDelete, "success")
			writeJSON(w, http.StatusOK, map[string]string{"topic": name, "state": "deleted"})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}

	default:
		http.Error(w, "unknown topic action "+action, http.StatusNotFound)
	}
}

// splitPath splits /prefix/{project}/{rest} fetching project and remainder.
func splitPath(path, prefix string) (project, rest string, ok bool) {
	tail := strings.TrimPrefix(path, prefix)
	if tail == "" {
		return "", "", false
	}
	seg := strings.SplitN(tail, "/", 2)
	if len(seg) < 2 || seg[0] == "" || seg[1] == "" {
		return "", "", false
	}
	return seg[0], seg[1], true
}

// splitNameAction splits a /{name}[/action] remainder into name and optional action.
func splitNameAction(rest string) (name, action string) {
	seg := strings.SplitN(rest, "/", 2)
	name = seg[0]
	if len(seg) > 1 {
		action = seg[1]
	}
	return name, action
}
