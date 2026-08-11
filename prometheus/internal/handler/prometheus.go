// Package handler exposes the Prometheus control plane over HTTP.
package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/mathif92/olympus/prometheus/pkg"
	"github.com/mathif92/olympus/prometheus/pkg/database"
)

// PrometheusHandler handles HTTP requests for multi-tenant serverless functions.
type PrometheusHandler struct {
	Prometheus *pkg.Prometheus
}

// NewPrometheusHandler creates a handler wired to the given control plane.
func NewPrometheusHandler(p *pkg.Prometheus) *PrometheusHandler {
	return &PrometheusHandler{Prometheus: p}
}

// Router returns the mux with all Prometheus routes registered.
func (h *PrometheusHandler) Router() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/projects", h.handleProjects)
	mux.HandleFunc("/runtimes", h.handleRuntimes)
	mux.HandleFunc("/functions", h.handleFunctions)
	mux.HandleFunc("/function/", h.handleFunction)
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

func (h *PrometheusHandler) ensureAccount(r *http.Request) {
	_ = h.Prometheus.EnsureAccount(r.Context(), database.Account{
		ID:            accountName(r),
		DisplayName:   accountName(r),
		Email:         accountName(r) + "@prometheus.internal",
		Plan:          "pro",
		FunctionLimit: 64,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *PrometheusHandler) handleProjects(w http.ResponseWriter, r *http.Request) {
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
		if err := h.Prometheus.CreateProject(ctx, account, database.Project{Name: in.Name, Description: in.Description}); err != nil {
			http.Error(w, "create project: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Prometheus.Audit(ctx, account, "", "project", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, map[string]string{"project": in.Name})

	case http.MethodGet:
		projects, err := h.Prometheus.ListProjects(ctx, account)
		if err != nil {
			http.Error(w, "list projects: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = h.Prometheus.Audit(ctx, account, "", "project", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRuntimes lists the supported language runtimes and their handler
// restrictions, so clients can validate their code before uploading.
func (h *PrometheusHandler) handleRuntimes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"runtimes": pkg.ListRuntimes()})
}

// handleFunctions routes create/list against the /functions collection.
func (h *PrometheusHandler) handleFunctions(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project     string  `json:"project"`
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Runtime     string  `json:"runtime"`
			Handler     string  `json:"handler"`
			TimeoutMS   int     `json:"timeout_ms"`
			MemoryMB    int     `json:"memory_mb"`
			CPUs        float64 `json:"cpus"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		fn, err := h.Prometheus.CreateFunction(ctx, account, in.Project, database.Function{
			Name:        in.Name,
			Description: in.Description,
			Runtime:     in.Runtime,
			Handler:     in.Handler,
			TimeoutMS:   in.TimeoutMS,
			MemoryMB:    in.MemoryMB,
			CPUs:        in.CPUs,
		})
		if err != nil {
			http.Error(w, "create function: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Prometheus.Audit(ctx, account, fn.ProjectID, "function", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, fn)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			http.Error(w, "missing project query param", http.StatusBadRequest)
			return
		}
		functions, err := h.Prometheus.ListFunctions(ctx, account, project)
		if err != nil {
			http.Error(w, "list functions: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Prometheus.Audit(ctx, account, "", "function", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"functions": functions})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleFunction routes /function/{project}/{name}[/action] where action is
// versions | invoke | invocations.
func (h *PrometheusHandler) handleFunction(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	account := accountName(r)

	project, rest, ok := splitPath(r.URL.Path, "/function/")
	if !ok || rest == "" {
		http.Error(w, "Invalid URL format. Expected /function/{project}/{name}[/action]", http.StatusBadRequest)
		return
	}
	name, action := splitNameAction(rest)

	switch action {
	case "versions":
		h.handleVersions(w, r, account, project, name)
	case "invoke":
		h.handleInvoke(w, r, account, project, name)
	case "invocations":
		h.handleInvocations(w, r, account, project, name)
	case "":
		h.handleFunctionResource(w, r, account, project, name)
	default:
		http.Error(w, "unknown function action "+action, http.StatusNotFound)
	}
}

func (h *PrometheusHandler) handleFunctionResource(w http.ResponseWriter, r *http.Request, account, project, name string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		fn, err := h.Prometheus.GetFunction(ctx, account, project, name)
		if err != nil {
			http.Error(w, "function not found", http.StatusNotFound)
			return
		}
		_ = h.Prometheus.Audit(ctx, account, fn.ProjectID, "function", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, fn)

	case http.MethodDelete:
		if err := h.Prometheus.DeleteFunction(ctx, account, project, name); err != nil {
			http.Error(w, "delete function: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Prometheus.Audit(ctx, account, "", "function", pkg.OpDelete, "success")
		writeJSON(w, http.StatusOK, map[string]string{"function": name, "state": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleVersions lists versions (GET) or deploys a new one (POST, multipart
// with a `code` file part).
func (h *PrometheusHandler) handleVersions(w http.ResponseWriter, r *http.Request, account, project, name string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		versions, err := h.Prometheus.ListVersions(ctx, account, project, name)
		if err != nil {
			http.Error(w, "list versions: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Prometheus.Audit(ctx, account, "", "version", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"versions": versions})

	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, pkg.MaxCodeBytes+1<<20)
		if err := r.ParseMultipartForm(pkg.MaxCodeBytes + 1<<20); err != nil {
			http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("code")
		if err != nil {
			http.Error(w, "missing `code` file part (a .zip archive)", http.StatusBadRequest)
			return
		}
		defer file.Close()
		code, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "read code archive: "+err.Error(), http.StatusBadRequest)
			return
		}
		ver, err := h.Prometheus.UploadVersion(ctx, account, project, name, code)
		if err != nil {
			http.Error(w, "deploy version: "+err.Error(), http.StatusBadRequest)
			return
		}
		fn, _ := h.Prometheus.GetFunction(ctx, account, project, name)
		projID := ""
		if fn != nil {
			projID = fn.ProjectID
		}
		_ = h.Prometheus.Audit(ctx, account, projID, "version", pkg.OpDeploy, "success")
		writeJSON(w, http.StatusCreated, ver)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleInvoke runs the function's active version with the request body as the
// event. Any valid JSON payload is passed through to the handler.
func (h *PrometheusHandler) handleInvoke(w http.ResponseWriter, r *http.Request, account, project, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	var event json.RawMessage
	if strings.TrimSpace(string(body)) == "" {
		event = json.RawMessage("{}")
	} else {
		if !json.Valid(body) {
			http.Error(w, "invoke payload must be valid JSON", http.StatusBadRequest)
			return
		}
		event = body
	}

	inv, err := h.Prometheus.InvokeFunction(ctx, account, project, name, event)
	if err != nil {
		// Recorded with status=error, but the backend could not run the code.
		http.Error(w, "invoke failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	_ = h.Prometheus.Audit(ctx, account, inv.FunctionID, "function", pkg.OpInvoke, "success")
	writeJSON(w, http.StatusOK, inv)
}

// handleInvocations lists the recent invocation records of a function.
func (h *PrometheusHandler) handleInvocations(w http.ResponseWriter, r *http.Request, account, project, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()

	limit := 0
	// limit is an optional query param; defaults handled in the service.
	invs, err := h.Prometheus.ListInvocations(ctx, account, project, name, limit)
	if err != nil {
		http.Error(w, "list invocations: "+err.Error(), http.StatusNotFound)
		return
	}
	_ = h.Prometheus.Audit(ctx, account, "", "invocation", pkg.OpList, "success")
	writeJSON(w, http.StatusOK, map[string]interface{}{"invocations": invs})
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
