package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mathif92/olympus/hephaestus/pkg"
	"github.com/mathif92/olympus/hephaestus/pkg/database"
)

// ComputeHandler handles HTTP requests for multi-tenant compute operations.
type ComputeHandler struct {
	Compute *pkg.Hephaestus
}

// NewComputeHandler creates a handler wired to the given control plane.
func NewComputeHandler(c *pkg.Hephaestus) *ComputeHandler {
	return &ComputeHandler{Compute: c}
}

// Router returns the mux with all Hephaestus routes registered.
func (h *ComputeHandler) Router() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/projects", h.handleProjects)
	mux.HandleFunc("/types", h.handleTypes)
	mux.HandleFunc("/instances", h.handleInstances)
	mux.HandleFunc("/instance/", h.handleInstance)
	mux.HandleFunc("/keypairs", h.handleKeyPairs)
	mux.HandleFunc("/keypairs/", h.handleKeyPairs)
	mux.HandleFunc("/security-groups", h.handleSecurityGroups)
	mux.HandleFunc("/security-groups/", h.handleSecurityGroups)
	mux.HandleFunc("/volumes", h.handleVolumes)
	mux.HandleFunc("/snapshots", h.handleSnapshots)
	mux.HandleFunc("/volume/", h.handleVolume)
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

func (h *ComputeHandler) ensureAccount(r *http.Request) {
	_ = h.Compute.EnsureAccount(r.Context(), database.Account{
		ID:            accountName(r),
		DisplayName:   accountName(r),
		Email:         accountName(r) + "@hephaestus.internal",
		Plan:          "pro",
		InstanceLimit: 50,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *ComputeHandler) handleProjects(w http.ResponseWriter, r *http.Request) {
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
		if err := h.Compute.CreateProject(ctx, account, database.Project{Name: in.Name, Description: in.Description}); err != nil {
			http.Error(w, "create project: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Compute.Audit(ctx, account, "", "project", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, map[string]string{"project": in.Name})

	case http.MethodGet:
		projects, err := h.Compute.ListProjects(ctx, account)
		if err != nil {
			http.Error(w, "list projects: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = h.Compute.Audit(ctx, account, "", "project", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *ComputeHandler) handleTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	types, err := h.Compute.ListInstanceTypes(r.Context())
	if err != nil {
		http.Error(w, "list types: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"instance_types": types})
}

// handleInstances routes launch/list against the /instances collection.
func (h *ComputeHandler) handleInstances(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project        string   `json:"project"`
			Name           string   `json:"name"`
			Type           string   `json:"type"`
			ImageID        string   `json:"image_id"`
			KeyPair        string   `json:"key_pair"`
			VolumeSizeGB   int      `json:"volume_gb"`
			SecurityGroups []string `json:"security_groups"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		inst, err := h.Compute.LaunchInstance(ctx, account, in.Project, pkg.InstanceSpec{
			Name:          in.Name,
			Type:          in.Type,
			ImageID:       in.ImageID,
			KeyPairName:   in.KeyPair,
			SecurityGroup: in.SecurityGroups,
			VolumeSizeGB:  in.VolumeSizeGB,
			LaunchedBy:    account,
		})
		if err != nil {
			http.Error(w, "launch: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Compute.Audit(ctx, account, inst.ProjectID, "instance", pkg.OpLaunch, "success")
		writeJSON(w, http.StatusCreated, inst)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			http.Error(w, "missing project query param", http.StatusBadRequest)
			return
		}
		instances, err := h.Compute.ListInstances(ctx, account, project)
		if err != nil {
			http.Error(w, "list instances: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Compute.Audit(ctx, account, "", "instance", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"instances": instances})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleInstance routes GET/start/stop/terminate/DELETE on /instance/{project}/{name}[/action].
func (h *ComputeHandler) handleInstance(w http.ResponseWriter, r *http.Request) {
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
		inst, err := h.Compute.GetInstance(ctx, account, project, name)
		if err != nil {
			http.Error(w, "instance not found", http.StatusNotFound)
			return
		}
		_ = h.Compute.Audit(ctx, account, inst.ProjectID, "instance", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, inst)

	case http.MethodPost:
		var inst *database.Instance
		var err error
		switch action {
		case "start":
			inst, err = h.Compute.StartInstance(ctx, account, project, name)
		case "stop":
			inst, err = h.Compute.StopInstance(ctx, account, project, name)
		case "terminate":
			err = h.Compute.TerminateInstance(ctx, account, project, name)
			if err == nil {
				writeJSON(w, http.StatusOK, map[string]string{"instance": name, "state": "terminated"})
				_ = h.Compute.Audit(ctx, account, "", "instance", pkg.OpTerminate, "success")
				return
			}
		default:
			http.Error(w, "unknown action (want start|stop|terminate)", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, action+" failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		op := pkg.OpStart
		if action == "stop" {
			op = pkg.OpStop
		}
		_ = h.Compute.Audit(ctx, account, "", "instance", op, "success")
		writeJSON(w, http.StatusOK, inst)

	case http.MethodDelete:
		if err := h.Compute.TerminateInstance(ctx, account, project, name); err != nil {
			http.Error(w, "terminate: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Compute.Audit(ctx, account, "", "instance", pkg.OpTerminate, "success")
		writeJSON(w, http.StatusOK, map[string]string{"instance": name, "state": "terminated"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *ComputeHandler) handleKeyPairs(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	// /keypairs/{project}
	if strings.Contains(strings.TrimPrefix(r.URL.Path, "/keypairs"), "/") {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		project := strings.TrimPrefix(r.URL.Path, "/keypairs/")
		keys, err := h.Compute.ListKeyPairs(ctx, account, project)
		if err != nil {
			http.Error(w, "list key pairs: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Compute.Audit(ctx, account, "", "key_pair", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"key_pairs": keys})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Project string `json:"project"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	kp, privateKey, err := h.Compute.CreateKeyPair(ctx, account, in.Project, in.Name)
	if err != nil {
		http.Error(w, "create key pair: "+err.Error(), http.StatusBadRequest)
		return
	}
	_ = h.Compute.Audit(ctx, account, "", "key_pair", pkg.OpCreate, "success")
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"key_pair":    kp,
		"private_key": privateKey, // returned exactly once
	})
}

func (h *ComputeHandler) handleSecurityGroups(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	if strings.Contains(strings.TrimPrefix(r.URL.Path, "/security-groups"), "/") {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		project := strings.TrimPrefix(r.URL.Path, "/security-groups/")
		groups, err := h.Compute.ListSecurityGroups(ctx, account, project)
		if err != nil {
			http.Error(w, "list security groups: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Compute.Audit(ctx, account, "", "security_group", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"security_groups": groups})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Project     string                  `json:"project"`
		Name        string                  `json:"name"`
		Description string                  `json:"description"`
		Rules       []database.SecurityRule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	sg, err := h.Compute.CreateSecurityGroup(ctx, account, in.Project, database.SecurityGroup{
		Name: in.Name, Description: in.Description, Rules: in.Rules,
	})
	if err != nil {
		http.Error(w, "create security group: "+err.Error(), http.StatusBadRequest)
		return
	}
	_ = h.Compute.Audit(ctx, account, "", "security_group", pkg.OpCreate, "success")
	writeJSON(w, http.StatusCreated, sg)
}

func (h *ComputeHandler) handleVolumes(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project string `json:"project"`
			Name    string `json:"name"`
			SizeGB  int    `json:"size_gb"`
			Type    string `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		vol, err := h.Compute.CreateVolume(ctx, account, in.Project, database.Volume{
			Name: in.Name, SizeGB: in.SizeGB, Type: in.Type,
		})
		if err != nil {
			http.Error(w, "create volume: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Compute.Audit(ctx, account, "", "volume", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, vol)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			http.Error(w, "missing project query param", http.StatusBadRequest)
			return
		}
		volumes, err := h.Compute.ListVolumes(ctx, account, project)
		if err != nil {
			http.Error(w, "list volumes: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Compute.Audit(ctx, account, "", "volume", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"volumes": volumes})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *ComputeHandler) handleVolume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	project, name, ok := splitPath(r.URL.Path, "/volume/")
	if !ok {
		http.Error(w, "Invalid URL format. Expected /volume/{project}/{name}", http.StatusBadRequest)
		return
	}
	if err := h.Compute.DeleteVolume(ctx, account, project, name); err != nil {
		http.Error(w, "delete volume: "+err.Error(), http.StatusNotFound)
		return
	}
	_ = h.Compute.Audit(ctx, account, "", "volume", pkg.OpDelete, "success")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": name})
}

func (h *ComputeHandler) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	account := accountName(r)

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project string `json:"project"`
			Name    string `json:"name"`
			Volume  string `json:"volume"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Resolve the volume name to an id within the project.
		volID := ""
		snap, err := func() (*database.Snapshot, error) {
			vols, err := h.Compute.ListVolumes(ctx, account, in.Project)
			if err != nil {
				return nil, err
			}
			for _, v := range vols {
				if v.Name == in.Volume {
					volID = v.ID
					break
				}
			}
			if volID == "" {
				return nil, pkg.ErrNotFound
			}
			return h.Compute.CreateSnapshot(ctx, account, in.Project, database.Snapshot{
				Name: in.Name, VolumeID: volID,
			})
		}()
		if err != nil {
			http.Error(w, "create snapshot: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Compute.Audit(ctx, account, "", "snapshot", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, snap)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			http.Error(w, "missing project query param", http.StatusBadRequest)
			return
		}
		snapshots, err := h.Compute.ListSnapshots(ctx, account, project)
		if err != nil {
			http.Error(w, "list snapshots: "+err.Error(), http.StatusNotFound)
			return
		}
		_ = h.Compute.Audit(ctx, account, "", "snapshot", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"snapshots": snapshots})

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

// splitNameAction separates an optional trailing /action from a resource name.
func splitNameAction(rest string) (name, action string) {
	if i := strings.LastIndex(rest, "/"); i > 0 {
		return rest[:i], rest[i+1:]
	}
	return rest, ""
}
