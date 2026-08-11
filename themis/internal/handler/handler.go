// Package handler wires the Themis store to the HTTP surface: identity CRUD,
// credential issuance, policy attachment, authorization and token minting.
package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mathif92/olympus/themis/pkg"
	"github.com/mathif92/olympus/themis/pkg/database"
)

// ThemisHandler handles HTTP requests for multi-tenant IAM operations.
type ThemisHandler struct {
	Store *pkg.ThemisStore
}

// NewThemisHandler creates a handler wired to the given store.
func NewThemisHandler(store *pkg.ThemisStore) *ThemisHandler {
	return &ThemisHandler{Store: store}
}

// Router returns the mux with all Themis routes registered.
func (h *ThemisHandler) Router() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/projects", h.handleProjects)
	mux.HandleFunc("/users", h.handleUsers)
	mux.HandleFunc("/user/", h.handleUser)
	mux.HandleFunc("/groups", h.handleGroups)
	mux.HandleFunc("/group/", h.handleGroup)
	mux.HandleFunc("/roles", h.handleRoles)
	mux.HandleFunc("/role/", h.handleRole)
	mux.HandleFunc("/policies", h.handlePolicies)
	mux.HandleFunc("/policy/", h.handlePolicy)
	mux.HandleFunc("/attachments", h.handleAttachments)
	mux.HandleFunc("/authorize", h.handleAuthorize)
	mux.HandleFunc("/tokens", h.handleTokens)
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
func (h *ThemisHandler) ensureAccount(r *http.Request) {
	_ = h.Store.EnsureAccount(r.Context(), database.Account{
		ID:          accountName(r),
		DisplayName: accountName(r),
		Email:       accountName(r) + "@themis.internal",
		Plan:        "pro",
	})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	http.Error(w, msg, status)
}

func decodeBody(r *http.Request, dst interface{}) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

// tail splits "/a/{name}/..." into the segment after /a and the remainder.
func tail(prefix, path string) (string, string) {
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" {
		return "", ""
	}
	rest = strings.TrimPrefix(rest, "/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i], strings.TrimPrefix(rest[i:], "/")
	}
	return rest, ""
}

// handleProjects routes project create/list.
func (h *ThemisHandler) handleProjects(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := decodeBody(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		if err := h.Store.CreateProject(ctx, accountName(r), database.Project{
			Name:        in.Name,
			Description: in.Description,
		}); err != nil {
			writeErr(w, http.StatusBadRequest, "create project: "+err.Error())
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), "", "", in.Name, pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, map[string]string{"project": in.Name})

	case http.MethodGet:
		projects, err := h.Store.ListProjects(ctx, accountName(r))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "list projects: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleUsers routes user create/list.
func (h *ThemisHandler) handleUsers(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project     string            `json:"project"`
			Name        string            `json:"name"`
			Description string            `json:"description"`
			Path        string            `json:"path"`
			Tags        map[string]string `json:"tags"`
		}
		if err := decodeBody(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		u, err := h.Store.CreateUser(ctx, accountName(r), in.Project, pkg.UserInput{
			Name:        in.Name,
			Description: in.Description,
			Path:        in.Path,
			Tags:        in.Tags,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, "create user: "+err.Error())
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), u.ProjectID, in.Name, "", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, u)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			writeErr(w, http.StatusBadRequest, "missing query param project")
			return
		}
		users, err := h.Store.ListUsers(ctx, accountName(r), project)
		if err != nil {
			writeErr(w, http.StatusNotFound, "list users: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"users": users})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleUser routes /user/{name}[/keys[/{keyID}[/status]]].
func (h *ThemisHandler) handleUser(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	project := r.URL.Query().Get("project")
	if project == "" {
		writeErr(w, http.StatusBadRequest, "missing query param project")
		return
	}

	userName, rest := tail("/user/", r.URL.Path)
	if userName == "" {
		writeErr(w, http.StatusBadRequest, "Invalid URL format. Expected /user/{name}")
		return
	}

	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			u, err := h.Store.GetUser(ctx, accountName(r), project, userName)
			if err != nil {
				writeErr(w, http.StatusNotFound, "user not found")
				return
			}
			writeJSON(w, http.StatusOK, u)
		case http.MethodDelete:
			if err := h.Store.DeleteUser(ctx, accountName(r), project, userName); err != nil {
				writeErr(w, http.StatusNotFound, "user not found")
				return
			}
			_ = h.Store.Audit(ctx, accountName(r), "", userName, "user", pkg.OpDelete, "success")
			writeJSON(w, http.StatusOK, map[string]string{"deleted": userName})
		default:
			writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	if rest == "keys" {
		h.handleUserKeys(w, r, project, userName)
		return
	}
	keyID, sub := tail("/user/"+userName+"/keys/", r.URL.Path)
	if sub == "status" {
		h.handleKeyStatus(w, r, project, userName, keyID)
		return
	}
	if sub == "" {
		h.handleKeyDelete(w, r, project, userName, keyID)
		return
	}
	writeErr(w, http.StatusNotFound, "not found")
}

func (h *ThemisHandler) handleUserKeys(w http.ResponseWriter, r *http.Request, project, userName string) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodPost:
		key, err := h.Store.CreateAccessKey(ctx, accountName(r), project, userName)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "create access key: "+err.Error())
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), key.ProjectID, userName, "access_key", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, key)
	case http.MethodGet:
		keys, err := h.Store.ListAccessKeys(ctx, accountName(r), project, userName)
		if err != nil {
			writeErr(w, http.StatusNotFound, "list access keys: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"access_keys": keys})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (h *ThemisHandler) handleKeyStatus(w http.ResponseWriter, r *http.Request, project, userName, keyID string) {
	if r.Method != http.MethodPatch {
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	key, err := h.Store.SetAccessKeyStatus(r.Context(), accountName(r), project, userName, keyID, in.Status)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "update access key: "+err.Error())
		return
	}
	_ = h.Store.Audit(r.Context(), accountName(r), "", userName, keyID, pkg.OpUpdate, "success")
	writeJSON(w, http.StatusOK, key)
}

func (h *ThemisHandler) handleKeyDelete(w http.ResponseWriter, r *http.Request, project, userName, keyID string) {
	if r.Method != http.MethodDelete {
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if err := h.Store.DeleteAccessKey(r.Context(), accountName(r), project, userName, keyID); err != nil {
		writeErr(w, http.StatusNotFound, "access key not found")
		return
	}
	_ = h.Store.Audit(r.Context(), accountName(r), "", userName, keyID, pkg.OpDelete, "success")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": keyID})
}

// handleGroups routes group create/list.
func (h *ThemisHandler) handleGroups(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project     string            `json:"project"`
			Name        string            `json:"name"`
			Description string            `json:"description"`
			Tags        map[string]string `json:"tags"`
		}
		if err := decodeBody(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		g, err := h.Store.CreateGroup(ctx, accountName(r), in.Project, pkg.GroupInput{
			Name:        in.Name,
			Description: in.Description,
			Tags:        in.Tags,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, "create group: "+err.Error())
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), g.ProjectID, in.Name, "", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, g)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			writeErr(w, http.StatusBadRequest, "missing query param project")
			return
		}
		groups, err := h.Store.ListGroups(ctx, accountName(r), project)
		if err != nil {
			writeErr(w, http.StatusNotFound, "list groups: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"groups": groups})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleGroup routes /group/{name} and /group/{name}/members.
func (h *ThemisHandler) handleGroup(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	project := r.URL.Query().Get("project")
	if project == "" {
		writeErr(w, http.StatusBadRequest, "missing query param project")
		return
	}

	groupName, rest := tail("/group/", r.URL.Path)
	if groupName == "" {
		writeErr(w, http.StatusBadRequest, "Invalid URL format. Expected /group/{name}")
		return
	}

	if rest == "members" {
		h.handleGroupMembers(w, r, project, groupName)
		return
	}
	if rest != "" {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		g, err := h.Store.GetGroup(ctx, accountName(r), project, groupName)
		if err != nil {
			writeErr(w, http.StatusNotFound, "group not found")
			return
		}
		writeJSON(w, http.StatusOK, g)
	case http.MethodDelete:
		if err := h.Store.DeleteGroup(ctx, accountName(r), project, groupName); err != nil {
			writeErr(w, http.StatusNotFound, "group not found")
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), "", groupName, "group", pkg.OpDelete, "success")
		writeJSON(w, http.StatusOK, map[string]string{"deleted": groupName})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (h *ThemisHandler) handleGroupMembers(w http.ResponseWriter, r *http.Request, project, groupName string) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		members, err := h.Store.GroupMembers(ctx, accountName(r), project, groupName)
		if err != nil {
			writeErr(w, http.StatusNotFound, "list members: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"members": members})
	case http.MethodPost:
		var in struct {
			User string `json:"user"`
		}
		if err := decodeBody(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		if err := h.Store.GroupAddMember(ctx, accountName(r), project, groupName, in.User); err != nil {
			writeErr(w, http.StatusBadRequest, "add member: "+err.Error())
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), "", groupName, in.User, pkg.OpUpdate, "success")
		writeJSON(w, http.StatusOK, map[string]string{"member": in.User})
	case http.MethodDelete:
		user := r.URL.Query().Get("user")
		if user == "" {
			writeErr(w, http.StatusBadRequest, "missing query param user")
			return
		}
		if err := h.Store.GroupRemoveMember(ctx, accountName(r), project, groupName, user); err != nil {
			writeErr(w, http.StatusNotFound, "remove member: "+err.Error())
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), "", groupName, user, pkg.OpUpdate, "success")
		writeJSON(w, http.StatusOK, map[string]string{"removed": user})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleRoles routes role create/list.
func (h *ThemisHandler) handleRoles(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project     string            `json:"project"`
			Name        string            `json:"name"`
			Description string            `json:"description"`
			Tags        map[string]string `json:"tags"`
		}
		if err := decodeBody(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		role, err := h.Store.CreateRole(ctx, accountName(r), in.Project, pkg.RoleInput{
			Name:        in.Name,
			Description: in.Description,
			Tags:        in.Tags,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, "create role: "+err.Error())
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), role.ProjectID, in.Name, "", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, role)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			writeErr(w, http.StatusBadRequest, "missing query param project")
			return
		}
		roles, err := h.Store.ListRoles(ctx, accountName(r), project)
		if err != nil {
			writeErr(w, http.StatusNotFound, "list roles: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"roles": roles})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleRole routes /role/{name}.
func (h *ThemisHandler) handleRole(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	project := r.URL.Query().Get("project")
	if project == "" {
		writeErr(w, http.StatusBadRequest, "missing query param project")
		return
	}

	roleName, _ := tail("/role/", r.URL.Path)
	if roleName == "" {
		writeErr(w, http.StatusBadRequest, "Invalid URL format. Expected /role/{name}")
		return
	}

	switch r.Method {
	case http.MethodGet:
		role, err := h.Store.GetRole(ctx, accountName(r), project, roleName)
		if err != nil {
			writeErr(w, http.StatusNotFound, "role not found")
			return
		}
		writeJSON(w, http.StatusOK, role)
	case http.MethodDelete:
		if err := h.Store.DeleteRole(ctx, accountName(r), project, roleName); err != nil {
			writeErr(w, http.StatusNotFound, "role not found")
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), "", roleName, "role", pkg.OpDelete, "success")
		writeJSON(w, http.StatusOK, map[string]string{"deleted": roleName})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handlePolicies routes policy create/list.
func (h *ThemisHandler) handlePolicies(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()

	switch r.Method {
	case http.MethodPost:
		var in struct {
			Project     string `json:"project"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Document    string `json:"document"`
		}
		if err := decodeBody(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		p, err := h.Store.CreatePolicy(ctx, accountName(r), in.Project, pkg.PolicyInput{
			Name:        in.Name,
			Description: in.Description,
			Document:    in.Document,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, "create policy: "+err.Error())
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), p.ProjectID, in.Name, "", pkg.OpCreate, "success")
		writeJSON(w, http.StatusCreated, p)

	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			writeErr(w, http.StatusBadRequest, "missing query param project")
			return
		}
		policies, err := h.Store.ListPolicies(ctx, accountName(r), project)
		if err != nil {
			writeErr(w, http.StatusNotFound, "list policies: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"policies": policies})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handlePolicy routes /policy/{name}.
func (h *ThemisHandler) handlePolicy(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()
	project := r.URL.Query().Get("project")
	if project == "" {
		writeErr(w, http.StatusBadRequest, "missing query param project")
		return
	}

	policyName, _ := tail("/policy/", r.URL.Path)
	if policyName == "" {
		writeErr(w, http.StatusBadRequest, "Invalid URL format. Expected /policy/{name}")
		return
	}

	switch r.Method {
	case http.MethodGet:
		p, err := h.Store.GetPolicy(ctx, accountName(r), project, policyName)
		if err != nil {
			writeErr(w, http.StatusNotFound, "policy not found")
			return
		}
		writeJSON(w, http.StatusOK, p)
	case http.MethodDelete:
		if err := h.Store.DeletePolicy(ctx, accountName(r), project, policyName); err != nil {
			writeErr(w, http.StatusNotFound, "policy not found")
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), "", policyName, "policy", pkg.OpDelete, "success")
		writeJSON(w, http.StatusOK, map[string]string{"deleted": policyName})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleAttachments routes attach/list/detach of policies on principals.
func (h *ThemisHandler) handleAttachments(w http.ResponseWriter, r *http.Request) {
	h.ensureAccount(r)
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		project := r.URL.Query().Get("project")
		if project == "" {
			writeErr(w, http.StatusBadRequest, "missing query param project")
			return
		}
		atts, err := h.Store.ListAttachments(ctx, accountName(r), project,
			r.URL.Query().Get("principal_type"), r.URL.Query().Get("principal_name"))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "list attachments: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"attachments": atts})

	case http.MethodPost:
		var in struct {
			Project       string `json:"project"`
			PrincipalType string `json:"principal_type"`
			PrincipalName string `json:"principal_name"`
			PolicyName    string `json:"policy_name"`
		}
		if err := decodeBody(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		a, err := h.Store.AttachPolicy(ctx, accountName(r), in.Project, in.PrincipalType, in.PrincipalName, in.PolicyName)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "attach policy: "+err.Error())
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), a.ProjectID, in.PrincipalName, in.PolicyName, pkg.OpAttach, "success")
		writeJSON(w, http.StatusCreated, a)

	case http.MethodDelete:
		var in struct {
			Project       string `json:"project"`
			PrincipalType string `json:"principal_type"`
			PrincipalName string `json:"principal_name"`
			PolicyName    string `json:"policy_name"`
		}
		if err := decodeBody(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		if err := h.Store.DetachPolicy(ctx, accountName(r), in.Project, in.PrincipalType, in.PrincipalName, in.PolicyName); err != nil {
			writeErr(w, http.StatusBadRequest, "detach policy: "+err.Error())
			return
		}
		_ = h.Store.Audit(ctx, accountName(r), "", in.PrincipalName, in.PolicyName, pkg.OpDetach, "success")
		writeJSON(w, http.StatusOK, map[string]string{"detached": in.PolicyName})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleAuthorize evaluates an action/resource for a principal.
func (h *ThemisHandler) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	h.ensureAccount(r)
	var in struct {
		Project       string `json:"project"`
		PrincipalType string `json:"principal_type"`
		PrincipalName string `json:"principal_name"`
		Action        string `json:"action"`
		Resource      string `json:"resource"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	decision, err := h.Store.Authorize(r.Context(), accountName(r), in.Project, in.PrincipalType, in.PrincipalName, in.Action, in.Resource)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "authorize: "+err.Error())
		return
	}
	status := "denied"
	if decision.Allowed {
		status = "allowed"
	}
	_ = h.Store.Audit(r.Context(), accountName(r), "", decision.Principal, in.Action, pkg.OpAuthorize, status)
	writeJSON(w, http.StatusOK, decision)
}

// handleTokens exchanges an access key for a signed JWT.
func (h *ThemisHandler) handleTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	h.ensureAccount(r)
	var in struct {
		AccessKeyID     string `json:"access_key_id"`
		SecretAccessKey string `json:"secret_access_key"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	token, claims, err := h.Store.MintToken(r.Context(), in.AccessKeyID, in.SecretAccessKey)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "token minting failed: "+err.Error())
		return
	}
	_ = h.Store.Audit(r.Context(), accountName(r), claims.ProjectID, claims.Subject, "token", pkg.OpToken, "success")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":      token,
		"claims":     claims,
		"expires_at": claims.ExpiresAt,
	})
}
