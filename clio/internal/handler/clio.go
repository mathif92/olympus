package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mathif92/olympus/clio/pkg"
	"github.com/mathif92/olympus/clio/pkg/database"
)

// ClioHandler handles HTTP requests for multi-tenant managed relational databases.
type ClioHandler struct {
	Clio *pkg.Clio
}

// NewClioHandler creates a handler wired to the given control plane.
func NewClioHandler(c *pkg.Clio) *ClioHandler {
	return &ClioHandler{Clio: c}
}

// Router returns the mux with all Clio routes registered.
func (h *ClioHandler) Router() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/projects", h.handleProjects)
	mux.HandleFunc("/engines", h.handleEngines)
	mux.HandleFunc("/instance-sizes", h.handleInstanceSizes)
	mux.HandleFunc("/instances", h.handleInstances)
	mux.HandleFunc("/instance/", h.handleInstance)
	mux.HandleFunc("/snapshots", h.handleSnapshots)
	mux.HandleFunc("/snapshot/", h.handleSnapshot)
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

func (h *ClioHandler) ensureAccount(r *http.Request) {
	_ = h.Clio.EnsureAccount(r.Context(), database.Account{
		ID:            accountName(r),
		DisplayName:   accountName(r),
		Email:         accountName(r) + "@clio.internal",
		Plan:          "pro",
		InstanceLimit: 50,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *ClioHandler) handleProjects(w http.ResponseWriter, r *http.Request) {
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
		if err := h.Clio.CreateProject(ctx, account, database.Project{Name: in.Name, Description: in.Description}); err != nil {
			http.Error(w, "create project: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Clio.Audit(ctx, account, "", "project", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, map[string]string{"project": in.Name})

	case http.MethodGet:
		projects, err := h.Clio.ListProjects(ctx, account)
		if err != nil {
			http.Error(w, "list projects: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = h.Clio.Audit(ctx, account, "", "project", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *ClioHandler) handleEngines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engines, err := h.Clio.ListEngines(r.Context())
	if err != nil {
		http.Error(w, "list engines: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"database_engines": engines})
}

func (h *ClioHandler) handleInstanceSizes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sizes, err := h.Clio.ListInstanceSizes(r.Context())
	if err != nil {
		http.Error(w, "list instance sizes: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"instance_sizes": sizes})
}

// handleInstances routes create/list against the /instances collection.
func (h *ClioHandler) handleInstances(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project       string `json:"project"`
			Name          string `json:"name"`
			Engine        string `json:"engine"`
			EngineVersion string `json:"engine_version"`
			Size          string `json:"size"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		inst, err := h.Clio.CreateInstance(ctx, account, in.Project, pkg.InstanceSpec{
			Name:          in.Name,
			Engine:        in.Engine,
			EngineVersion: in.EngineVersion,
			Size:          in.Size,
		})
		if err != nil {
			http.Error(w, "create instance: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Clio.Audit(ctx, account, inst.ProjectID, "instance", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, inst)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			http.Error(w, "missing project query param", http.StatusBadRequest)
			return
		}
		instances, err := h.Clio.ListInstances(ctx, account, project)
		if err != nil {
			http.Error(w, "list instances: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Clio.Audit(ctx, account, "", "instance", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"instances": instances})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleInstance routes GET/DELETE/start/stop on /instance/{project}/{name}[/action].
func (h *ClioHandler) handleInstance(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	project, rest, ok := splitPath(r.URL.Path, "/instance/")
	if !ok {
		http.Error(w, "Invalid URL format. Expected /instance/{project}/{name}", http.StatusBadRequest)
		return
	}
	name, action := splitNameAction(rest)

	switch r.Method {
	case http.MethodGet:
		inst, err := h.Clio.GetInstance(ctx, account, project, name)
		if err != nil {
			http.Error(w, "instance not found", http.StatusNotFound)
			return
		}
		_ = h.Clio.Audit(ctx, account, inst.ProjectID, "instance", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, inst)

	case http.MethodPost:
		var inst *database.DBInstance
		var err error
		switch action {
		case "start":
			inst, err = h.Clio.StartInstance(ctx, account, project, name)
		case "stop":
			inst, err = h.Clio.StopInstance(ctx, account, project, name)
		default:
			http.Error(w, "unknown action (want start|stop)", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "instance action: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Clio.Audit(ctx, account, "", "instance", action, "success")
		writeJSON(w, http.StatusOK, inst)

	case http.MethodDelete:
		if err := h.Clio.DeleteInstance(ctx, account, project, name); err != nil {
			http.Error(w, "delete instance: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Clio.Audit(ctx, account, "", "instance", pkg.OpDelete, "success")
		writeJSON(w, http.StatusOK, map[string]string{"instance": name, "state": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSnapshots routes create/list against the /snapshots collection.
func (h *ClioHandler) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project  string `json:"project"`
			Instance string `json:"instance"`
			Name     string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		snap, err := h.Clio.CreateSnapshot(ctx, account, in.Project, in.Instance, in.Name)
		if err != nil {
			http.Error(w, "create snapshot: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Clio.Audit(ctx, account, snap.ProjectID, "snapshot", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, snap)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		instance := r.URL.Query().Get("instance")
		if project == "" || instance == "" {
			http.Error(w, "missing project or instance query param", http.StatusBadRequest)
			return
		}
		snapshots, err := h.Clio.ListSnapshots(ctx, account, project, instance)
		if err != nil {
			http.Error(w, "list snapshots: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Clio.Audit(ctx, account, "", "snapshot", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"snapshots": snapshots})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSnapshot routes DELETE on /snapshot/{project}/{instance}/{name}.
func (h *ClioHandler) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	project, rest, ok := splitPath(r.URL.Path, "/snapshot/")
	if !ok {
		http.Error(w, "Invalid URL format. Expected /snapshot/{project}/{instance}/{name}", http.StatusBadRequest)
		return
	}
	instance, snapshot, ok2 := splitTwo(rest)

	switch r.Method {
	case http.MethodDelete:
		if !ok2 {
			http.Error(w, "Invalid URL format. Expected /snapshot/{project}/{instance}/{name}", http.StatusBadRequest)
			return
		}
		if err := h.Clio.DeleteSnapshot(ctx, account, project, instance, snapshot); err != nil {
			http.Error(w, "delete snapshot: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Clio.Audit(ctx, account, "", "snapshot", pkg.OpDelete, "success")
		writeJSON(w, http.StatusOK, map[string]string{"snapshot": snapshot, "state": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// splitPath splits /prefix/{project}/{name} fetching project and remainder.
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

// splitTwo splits a /{instance}/{name} remainder into parts.
func splitTwo(rest string) (instance, name string, ok bool) {
	seg := strings.Split(rest, "/")
	if len(seg) != 2 || seg[0] == "" || seg[1] == "" {
		return "", "", false
	}
	return seg[0], seg[1], true
}

// splitNameAction separates an optional trailing /action from a resource name.
func splitNameAction(rest string) (name, action string) {
	if i := strings.LastIndex(rest, "/"); i > 0 {
		return rest[:i], rest[i+1:]
	}
	return rest, ""
}
