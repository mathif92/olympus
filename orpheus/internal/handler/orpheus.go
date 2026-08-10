package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mathif92/olympus/orpheus/pkg"
	"github.com/mathif92/olympus/orpheus/pkg/database"
)

// OrpheusHandler handles HTTP requests for multi-tenant managed-Kubernetes operations.
type OrpheusHandler struct {
	Orpheus *pkg.Orpheus
}

// NewOrpheusHandler creates a handler wired to the given control plane.
func NewOrpheusHandler(o *pkg.Orpheus) *OrpheusHandler {
	return &OrpheusHandler{Orpheus: o}
}

// Router returns the mux with all Orpheus routes registered.
func (h *OrpheusHandler) Router() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/projects", h.handleProjects)
	mux.HandleFunc("/versions", h.handleVersions)
	mux.HandleFunc("/node-sizes", h.handleNodeSizes)
	mux.HandleFunc("/clusters", h.handleClusters)
	mux.HandleFunc("/cluster/", h.handleCluster)
	mux.HandleFunc("/nodegroups", h.handleNodeGroups)
	mux.HandleFunc("/nodegroup/", h.handleNodeGroup)
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

func (h *OrpheusHandler) ensureAccount(r *http.Request) {
	_ = h.Orpheus.EnsureAccount(r.Context(), database.Account{
		ID:           accountName(r),
		DisplayName:  accountName(r),
		Email:        accountName(r) + "@orpheus.internal",
		Plan:         "pro",
		ClusterLimit: 50,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *OrpheusHandler) handleProjects(w http.ResponseWriter, r *http.Request) {
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
		if err := h.Orpheus.CreateProject(ctx, account, database.Project{Name: in.Name, Description: in.Description}); err != nil {
			http.Error(w, "create project: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Orpheus.Audit(ctx, account, "", "project", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, map[string]string{"project": in.Name})

	case http.MethodGet:
		projects, err := h.Orpheus.ListProjects(ctx, account)
		if err != nil {
			http.Error(w, "list projects: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = h.Orpheus.Audit(ctx, account, "", "project", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *OrpheusHandler) handleVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	versions, err := h.Orpheus.ListKubernetesVersions(r.Context())
	if err != nil {
		http.Error(w, "list versions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"kubernetes_versions": versions})
}

func (h *OrpheusHandler) handleNodeSizes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sizes, err := h.Orpheus.ListNodeSizes(r.Context())
	if err != nil {
		http.Error(w, "list node sizes: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"node_sizes": sizes})
}

// handleClusters routes create/list against the /clusters collection.
func (h *OrpheusHandler) handleClusters(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project           string `json:"project"`
			Name              string `json:"name"`
			KubernetesVersion string `json:"kubernetes_version"`
			Region            string `json:"region"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		cluster, err := h.Orpheus.CreateCluster(ctx, account, in.Project, pkg.ClusterSpec{
			Name:              in.Name,
			KubernetesVersion: in.KubernetesVersion,
			Region:            in.Region,
		})
		if err != nil {
			http.Error(w, "create cluster: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Orpheus.Audit(ctx, account, cluster.ProjectID, "cluster", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, cluster)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			http.Error(w, "missing project query param", http.StatusBadRequest)
			return
		}
		clusters, err := h.Orpheus.ListClusters(ctx, account, project)
		if err != nil {
			http.Error(w, "list clusters: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Orpheus.Audit(ctx, account, "", "cluster", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"clusters": clusters})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCluster routes GET/DELETE/kubeconfig on /cluster/{project}/{name}[/action].
func (h *OrpheusHandler) handleCluster(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	project, rest, ok := splitPath(r.URL.Path, "/cluster/")
	if !ok {
		http.Error(w, "Invalid URL format. Expected /cluster/{project}/{name}", http.StatusBadRequest)
		return
	}
	name, action := splitNameAction(rest)

	switch r.Method {
	case http.MethodGet:
		if action == "kubeconfig" {
			kubeconfig, err := h.Orpheus.ClusterKubeconfig(ctx, account, project, name)
			if err != nil {
				http.Error(w, "kubeconfig not available", http.StatusNotFound)
				return
			}
			_ = h.Orpheus.Audit(ctx, account, "", "cluster", pkg.OpKubeconfig, "success")
			w.Header().Set("Content-Type", "application/yaml")
			_, _ = w.Write([]byte(kubeconfig))
			return
		}
		cluster, err := h.Orpheus.GetCluster(ctx, account, project, name)
		if err != nil {
			http.Error(w, "cluster not found", http.StatusNotFound)
			return
		}
		_ = h.Orpheus.Audit(ctx, account, cluster.ProjectID, "cluster", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, cluster)

	case http.MethodDelete:
		if err := h.Orpheus.DeleteCluster(ctx, account, project, name); err != nil {
			http.Error(w, "delete cluster: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Orpheus.Audit(ctx, account, "", "cluster", pkg.OpDelete, "success")
		writeJSON(w, http.StatusOK, map[string]string{"cluster": name, "state": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleNodeGroups routes create/list against the /nodegroups collection.
func (h *OrpheusHandler) handleNodeGroups(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project     string `json:"project"`
			Cluster     string `json:"cluster"`
			Name        string `json:"name"`
			NodeSize    string `json:"node_size"`
			MinSize     int    `json:"min_size"`
			DesiredSize int    `json:"desired_size"`
			MaxSize     int    `json:"max_size"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		ng, err := h.Orpheus.CreateNodeGroup(ctx, account, in.Project, in.Cluster, pkg.NodeGroupSpec{
			Name:        in.Name,
			NodeSize:    in.NodeSize,
			MinSize:     in.MinSize,
			DesiredSize: in.DesiredSize,
			MaxSize:     in.MaxSize,
		})
		if err != nil {
			http.Error(w, "create node group: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Orpheus.Audit(ctx, account, "", "node_group", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, ng)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		cluster := r.URL.Query().Get("cluster")
		if project == "" || cluster == "" {
			http.Error(w, "missing project or cluster query param", http.StatusBadRequest)
			return
		}
		groups, err := h.Orpheus.ListNodeGroups(ctx, account, project, cluster)
		if err != nil {
			http.Error(w, "list node groups: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Orpheus.Audit(ctx, account, "", "node_group", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"node_groups": groups})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleNodeGroup routes scale/DELETE on /nodegroup/{project}/{cluster}/{name}[/scale].
func (h *OrpheusHandler) handleNodeGroup(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	project, rest, ok := splitPath(r.URL.Path, "/nodegroup/")
	if !ok {
		http.Error(w, "Invalid URL format. Expected /nodegroup/{project}/{cluster}/{name}", http.StatusBadRequest)
		return
	}
	clusterName, ngName, action := splitNodeGroup(rest)

	switch r.Method {
	case http.MethodPost:
		if action != "scale" {
			http.Error(w, "unknown action (want scale)", http.StatusBadRequest)
			return
		}
		var in struct {
			DesiredSize int `json:"desired_size"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		ng, err := h.Orpheus.ScaleNodeGroup(ctx, account, project, clusterName, ngName, in.DesiredSize)
		if err != nil {
			http.Error(w, "scale: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Orpheus.Audit(ctx, account, "", "node_group", pkg.OpScale, "success")
		writeJSON(w, http.StatusOK, ng)

	case http.MethodDelete:
		if err := h.Orpheus.DeleteNodeGroup(ctx, account, project, clusterName, ngName); err != nil {
			http.Error(w, "delete node group: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Orpheus.Audit(ctx, account, "", "node_group", pkg.OpDelete, "success")
		writeJSON(w, http.StatusOK, map[string]string{"node_group": ngName, "state": "deleted"})

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

// splitNodeGroup splits a /{cluster}/{name}[/action] remainder into parts.
func splitNodeGroup(rest string) (cluster, name, action string) {
	seg := strings.Split(rest, "/")
	if len(seg) == 1 {
		return "", "", ""
	}
	cluster = seg[0]
	name = seg[1]
	if len(seg) >= 3 {
		action = seg[2]
	}
	return cluster, name, action
}

// splitNameAction separates an optional trailing /action from a resource name.
func splitNameAction(rest string) (name, action string) {
	if i := strings.LastIndex(rest, "/"); i > 0 {
		return rest[:i], rest[i+1:]
	}
	return rest, ""
}
