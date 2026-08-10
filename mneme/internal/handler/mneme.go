package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mathif92/olympus/mneme/pkg"
	"github.com/mathif92/olympus/mneme/pkg/database"
)

// MnemeHandler handles HTTP requests for multi-tenant managed caches.
type MnemeHandler struct {
	Mneme *pkg.Mneme
}

// NewMnemeHandler creates a handler wired to the given control plane.
func NewMnemeHandler(m *pkg.Mneme) *MnemeHandler {
	return &MnemeHandler{Mneme: m}
}

// Router returns the mux with all Mneme routes registered.
func (h *MnemeHandler) Router() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/projects", h.handleProjects)
	mux.HandleFunc("/engines", h.handleEngines)
	mux.HandleFunc("/node-types", h.handleNodeTypes)
	mux.HandleFunc("/clusters", h.handleClusters)
	mux.HandleFunc("/cluster/", h.handleCluster)
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

func (h *MnemeHandler) ensureAccount(r *http.Request) {
	_ = h.Mneme.EnsureAccount(r.Context(), database.Account{
		ID:           accountName(r),
		DisplayName:  accountName(r),
		Email:        accountName(r) + "@mneme.internal",
		Plan:         "pro",
		ClusterLimit: 50,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *MnemeHandler) handleProjects(w http.ResponseWriter, r *http.Request) {
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
		if err := h.Mneme.CreateProject(ctx, account, database.Project{Name: in.Name, Description: in.Description}); err != nil {
			http.Error(w, "create project: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Mneme.Audit(ctx, account, "", "project", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, map[string]string{"project": in.Name})

	case http.MethodGet:
		projects, err := h.Mneme.ListProjects(ctx, account)
		if err != nil {
			http.Error(w, "list projects: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = h.Mneme.Audit(ctx, account, "", "project", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *MnemeHandler) handleEngines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engines, err := h.Mneme.ListEngines(r.Context())
	if err != nil {
		http.Error(w, "list engines: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"cache_engines": engines})
}

func (h *MnemeHandler) handleNodeTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	types, err := h.Mneme.ListNodeTypes(r.Context())
	if err != nil {
		http.Error(w, "list node types: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"node_types": types})
}

// handleClusters routes create/list against the /clusters collection.
func (h *MnemeHandler) handleClusters(w http.ResponseWriter, r *http.Request) {
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
			NodeType      string `json:"node_type"`
			NumNodes      int    `json:"num_nodes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		cl, err := h.Mneme.CreateCluster(ctx, account, in.Project, pkg.ClusterSpec{
			Name:          in.Name,
			Engine:        in.Engine,
			EngineVersion: in.EngineVersion,
			NodeType:      in.NodeType,
			NumNodes:      in.NumNodes,
		})
		if err != nil {
			http.Error(w, "create cluster: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Mneme.Audit(ctx, account, cl.ProjectID, "cache_cluster", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, cl)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			http.Error(w, "missing project query param", http.StatusBadRequest)
			return
		}
		clusters, err := h.Mneme.ListClusters(ctx, account, project)
		if err != nil {
			http.Error(w, "list clusters: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Mneme.Audit(ctx, account, "", "cache_cluster", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"clusters": clusters})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCluster routes GET/DELETE on /cluster/{project}/{name}.
func (h *MnemeHandler) handleCluster(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	project, name, ok := splitPath(r.URL.Path, "/cluster/")
	if !ok {
		http.Error(w, "Invalid URL format. Expected /cluster/{project}/{name}", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		cl, err := h.Mneme.GetCluster(ctx, account, project, name)
		if err != nil {
			http.Error(w, "cluster not found", http.StatusNotFound)
			return
		}
		_ = h.Mneme.Audit(ctx, account, cl.ProjectID, "cache_cluster", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, cl)

	case http.MethodDelete:
		if err := h.Mneme.DeleteCluster(ctx, account, project, name); err != nil {
			http.Error(w, "delete cluster: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Mneme.Audit(ctx, account, "", "cache_cluster", pkg.OpDelete, "success")
		writeJSON(w, http.StatusOK, map[string]string{"cluster": name, "state": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSnapshots routes create/list against the /snapshots collection.
func (h *MnemeHandler) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project string `json:"project"`
			Cluster string `json:"cluster"`
			Name    string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		snap, err := h.Mneme.CreateSnapshot(ctx, account, in.Project, in.Cluster, in.Name)
		if err != nil {
			http.Error(w, "create snapshot: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Mneme.Audit(ctx, account, snap.ProjectID, "snapshot", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, snap)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		cluster := r.URL.Query().Get("cluster")
		if project == "" || cluster == "" {
			http.Error(w, "missing project or cluster query param", http.StatusBadRequest)
			return
		}
		snapshots, err := h.Mneme.ListSnapshots(ctx, account, project, cluster)
		if err != nil {
			http.Error(w, "list snapshots: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Mneme.Audit(ctx, account, "", "snapshot", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"snapshots": snapshots})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSnapshot routes DELETE on /snapshot/{project}/{cluster}/{name}.
func (h *MnemeHandler) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	project, rest, ok := splitPath(r.URL.Path, "/snapshot/")
	if !ok {
		http.Error(w, "Invalid URL format. Expected /snapshot/{project}/{cluster}/{name}", http.StatusBadRequest)
		return
	}
	cluster, snapshot, ok2 := splitTwo(rest)

	switch r.Method {
	case http.MethodDelete:
		if !ok2 {
			http.Error(w, "Invalid URL format. Expected /snapshot/{project}/{cluster}/{name}", http.StatusBadRequest)
			return
		}
		if err := h.Mneme.DeleteSnapshot(ctx, account, project, cluster, snapshot); err != nil {
			http.Error(w, "delete snapshot: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Mneme.Audit(ctx, account, "", "snapshot", pkg.OpDelete, "success")
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

// splitTwo splits a /{cluster}/{name} remainder into parts.
func splitTwo(rest string) (cluster, name string, ok bool) {
	seg := strings.Split(rest, "/")
	if len(seg) != 2 || seg[0] == "" || seg[1] == "" {
		return "", "", false
	}
	return seg[0], seg[1], true
}
