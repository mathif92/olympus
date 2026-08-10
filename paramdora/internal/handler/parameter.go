package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mathif92/olympus/paramdora/pkg"
	"github.com/mathif92/olympus/paramdora/pkg/database"
)

// ParamdoraHandler handles HTTP requests for multi-tenant parameter operations.
type ParamdoraHandler struct {
	Store *pkg.ParamStore
}

// NewParamdoraHandler creates a handler wired to the given store.
func NewParamdoraHandler(store *pkg.ParamStore) *ParamdoraHandler {
	return &ParamdoraHandler{Store: store}
}

// Router returns the mux with all Paramdora routes registered.
func (h *ParamdoraHandler) Router() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/projects", h.handleProjects)
	mux.HandleFunc("/projects/", h.handleProjects)
	mux.HandleFunc("/parameter/", h.handleParameter)
	mux.HandleFunc("/parameters/", h.handleParameterList)
	return mux
}

// accountName returns the tenant identified by the X-Account-Id header.
func accountName(r *http.Request) string {
	if id := r.Header.Get("X-Account-Id"); id != "" {
		return id
	}
	if id := r.Header.Get("X-Account-ID"); id != "" {
		return id
	}
	return "default"
}

// ensureAccount registers the caller tenant so operations always resolve.
func (h *ParamdoraHandler) ensureAccount(r *http.Request) {
	_ = h.Store.EnsureAccount(r.Context(), database.Account{
		ID:             accountName(r),
		DisplayName:    accountName(r),
		Email:          accountName(r) + "@paramdora.internal",
		Plan:           "pro",
		ParameterLimit: 1000,
	})
}

// handleProjects routes project create/list.
func (h *ParamdoraHandler) handleProjects(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()

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
		if err := h.Store.CreateProject(ctx, accountName(r), database.Project{
			Name:        in.Name,
			Description: in.Description,
		}); err != nil {
			http.Error(w, "create project: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), "", in.Name, pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, map[string]string{"project": in.Name})

	case http.MethodGet:
		projects, err := h.Store.ListProjects(ctx, accountName(r))
		if err != nil {
			http.Error(w, "list projects: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), "", "", pkg.OpList, "success")
		writeJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleParameter routes operations on a single /parameter/{project}/{name}.
func (h *ParamdoraHandler) handleParameter(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()

	project, name, ok := splitParameterPath(r.URL.Path)
	if !ok {
		http.Error(w, "Invalid URL format. Expected /parameter/{project}/{name}", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var in struct {
			Value       string            `json:"value"`
			Type        string            `json:"type"`
			Description string            `json:"description"`
			Tier        string            `json:"tier"`
			Tags        map[string]string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		param, err := h.Store.PutParameter(ctx, accountName(r), project, pkg.PutParameterInput{
			Name:           name,
			Value:          in.Value,
			Type:           in.Type,
			Description:    in.Description,
			Tier:           in.Tier,
			Tags:           in.Tags,
			LastModifiedBy: accountName(r),
		})
		if err != nil {
			http.Error(w, "put parameter: "+err.Error(), http.StatusBadRequest)
			return
		}
		op := pkg.OpUpdate
		if param.Version == 1 {
			op = pkg.OpCreate
		}
		_ = h.Store.Audit(ctx, accountName(r), param.ProjectID, param.Name, op, "success")
		writeJSON(w, http.StatusCreated, param)

	case http.MethodGet:
		if r.URL.Query().Get("history") == "true" {
			versions, err := h.Store.GetParameterHistory(ctx, accountName(r), project, name)
			if err != nil {
				http.Error(w, "history: "+err.Error(), http.StatusNotFound)
				return
			}
			_ = h.Store.Audit(ctx, accountName(r), "", name, pkg.OpHistory, "success")
			writeJSON(w, http.StatusOK, map[string]interface{}{"versions": versions})
			return
		}
		decrypt := r.URL.Query().Get("decrypt") == "true"
		param, err := h.Store.GetParameter(ctx, accountName(r), project, name, decrypt)
		if err != nil {
			http.Error(w, "parameter not found", http.StatusNotFound)
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), param.ProjectID, param.Name, pkg.OpGet, "success")
		writeJSON(w, http.StatusOK, param)

	case http.MethodDelete:
		if err := h.Store.DeleteParameter(ctx, accountName(r), project, name); err != nil {
			http.Error(w, "parameter not found", http.StatusNotFound)
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), "", name, pkg.OpDelete, "success")
		writeJSON(w, http.StatusOK, map[string]string{"deleted": name})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleParameterList routes GET /parameters/{project}?prefix=...
func (h *ParamdoraHandler) handleParameterList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.ensureAccount(r)
	ctx := r.Context()

	project := strings.TrimPrefix(r.URL.Path, "/parameters/")
	if project == "" {
		http.Error(w, "Invalid URL format. Expected /parameters/{project}", http.StatusBadRequest)
		return
	}

	params, err := h.Store.ListParameters(ctx, accountName(r), project, r.URL.Query().Get("prefix"))
	if err != nil {
		http.Error(w, "list parameters: "+err.Error(), http.StatusNotFound)
		return
	}
	_ = h.Store.Audit(ctx, accountName(r), "", project, pkg.OpList, "success")
	writeJSON(w, http.StatusOK, map[string]interface{}{"parameters": params})
}

// splitParameterPath splits /parameter/{project}/{name} into project and the
// (possibly hierarchical) parameter name.
func splitParameterPath(path string) (project, name string, ok bool) {
	tail := strings.TrimPrefix(path, "/parameter/")
	if tail == "" {
		return "", "", false
	}
	seg := strings.SplitN(tail, "/", 2)
	if len(seg) < 2 || seg[0] == "" || seg[1] == "" {
		return "", "", false
	}
	return seg[0], seg[1], true
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
