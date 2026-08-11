package pkg

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mathif92/olympus/themis/pkg/database"
)

// Operations recorded in the audit_logs table.
const (
	OpCreate     = "create"
	OpGet        = "get"
	OpUpdate     = "update"
	OpDelete     = "delete"
	OpList       = "list"
	OpAttach     = "attach"
	OpDetach     = "detach"
	OpAuthorize  = "authorize"
	OpToken      = "token"
)

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrInvalidCredentials is returned when an access key lookup fails.
var ErrInvalidCredentials = errors.New("invalid access key credentials")

// ThemisStore is the business-logic layer for multi-tenant identity.
type ThemisStore struct {
	DB  *database.Client
	JWT *JWT
}

// NewThemisStore creates a store backed by Postgres and a JWT signer.
func NewThemisStore(db *database.Client, jwt *JWT) *ThemisStore {
	return &ThemisStore{DB: db, JWT: jwt}
}

// newID returns a 32-char hex identifier.
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// keyIDAlphabet avoids ambiguous characters (AWS-style).
const keyIDAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

// secretAlphabet is the alphabet for generated secret access keys.
const secretAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomString(alphabet string, n int) string {
	out := make([]byte, n)
	for i := range out {
		b := make([]byte, 1)
		if _, err := rand.Read(b); err != nil {
			return ""
		}
		out[i] = alphabet[int(b[0])%len(alphabet)]
	}
	return string(out)
}

// newAccessKeyID returns an AWS-style access key id (AKIA + 16 chars).
func newAccessKeyID() string {
	return "AKIA" + randomString(keyIDAlphabet, 16)
}

// newSecretAccessKey returns a 40-char random secret.
func newSecretAccessKey() string {
	return randomString(secretAlphabet, 40)
}

// secretHash returns the hex SHA-256 of a secret access key.
func secretHash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// EnsureAccount upserts the tenant so requests referencing it always resolve.
func (s *ThemisStore) EnsureAccount(ctx context.Context, a database.Account) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO accounts (id, display_name, email, plan)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET display_name = EXCLUDED.display_name`,
		a.ID, a.DisplayName, a.Email, a.Plan)
	return err
}

// CreateProject provisions a project namespace inside the account.
func (s *ThemisStore) CreateProject(ctx context.Context, accountID string, p database.Project) error {
	if p.Name == "" {
		return errors.New("project name is required")
	}
	p.ID = newID()
	p.AccountID = accountID
	err := s.DB.QueryRow(ctx, `
		INSERT INTO projects (id, account_id, name, description)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`,
		p.ID, p.AccountID, p.Name, p.Description).Scan(&p.CreatedAt)
	return err
}

// ListProjects returns all projects for an account.
func (s *ThemisStore) ListProjects(ctx context.Context, accountID string) ([]database.Project, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), resource_count, created_at, status
		FROM projects
		WHERE account_id = $1
		ORDER BY name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Project
	for rows.Next() {
		var p database.Project
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &p.Description, &p.ResourceCount, &p.CreatedAt, &p.Status); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// resolveProject loads a project owned by the account or returns ErrNotFound.
func (s *ThemisStore) resolveProject(ctx context.Context, accountID, projectName string) (*database.Project, error) {
	var p database.Project
	err := s.DB.QueryRow(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), resource_count, created_at, status
		FROM projects
		WHERE account_id = $1 AND name = $2 AND status = 'active'`,
		accountID, projectName).Scan(
		&p.ID, &p.AccountID, &p.Name, &p.Description, &p.ResourceCount, &p.CreatedAt, &p.Status)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// resolveUser loads an active user in a project by name.
func (s *ThemisStore) resolveUser(ctx context.Context, projectID, name string) (*database.User, error) {
	var u database.User
	var tags []byte
	err := s.DB.QueryRow(ctx, `
		SELECT id, project_id, name, COALESCE(description,''), COALESCE(path,'/'), tags, created_at, updated_at, status
		FROM users
		WHERE project_id = $1 AND name = $2 AND status = 'active'`,
		projectID, name).Scan(
		&u.ID, &u.ProjectID, &u.Name, &u.Description, &u.Path, &tags, &u.CreatedAt, &u.UpdatedAt, &u.Status)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(tags, &u.Tags)
	return &u, nil
}

// resolveGroup loads an active group in a project by name.
func (s *ThemisStore) resolveGroup(ctx context.Context, projectID, name string) (*database.Group, error) {
	var g database.Group
	var tags []byte
	err := s.DB.QueryRow(ctx, `
		SELECT id, project_id, name, COALESCE(description,''), tags, created_at, updated_at, status
		FROM groups
		WHERE project_id = $1 AND name = $2 AND status = 'active'`,
		projectID, name).Scan(
		&g.ID, &g.ProjectID, &g.Name, &g.Description, &tags, &g.CreatedAt, &g.UpdatedAt, &g.Status)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(tags, &g.Tags)
	return &g, nil
}

// resolveRole loads an active role in a project by name.
func (s *ThemisStore) resolveRole(ctx context.Context, projectID, name string) (*database.Role, error) {
	var r database.Role
	var tags []byte
	err := s.DB.QueryRow(ctx, `
		SELECT id, project_id, name, COALESCE(description,''), tags, created_at, updated_at, status
		FROM roles
		WHERE project_id = $1 AND name = $2 AND status = 'active'`,
		projectID, name).Scan(
		&r.ID, &r.ProjectID, &r.Name, &r.Description, &tags, &r.CreatedAt, &r.UpdatedAt, &r.Status)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(tags, &r.Tags)
	return &r, nil
}

// resolvePolicy loads an active policy in a project by name.
func (s *ThemisStore) resolvePolicy(ctx context.Context, projectID, name string) (*database.Policy, error) {
	p, err := scanPolicy(s.DB.QueryRow(ctx, `
		SELECT id, project_id, name, COALESCE(description,''), document, version, created_at, updated_at, status
		FROM policies
		WHERE project_id = $1 AND name = $2 AND status = 'active'`,
		projectID, name))
	if err != nil {
		return nil, err
	}
	return p, nil
}

// refreshProjectCount keeps the cached resource_count in sync.
func (s *ThemisStore) refreshProjectCount(ctx context.Context, projectID string) error {
	_, err := s.DB.Exec(ctx, `
		UPDATE projects SET resource_count = (
			(SELECT COUNT(*) FROM users WHERE project_id = $1) +
			(SELECT COUNT(*) FROM groups WHERE project_id = $1) +
			(SELECT COUNT(*) FROM roles WHERE project_id = $1) +
			(SELECT COUNT(*) FROM policies WHERE project_id = $1)
		) WHERE id = $1`, projectID)
	return err
}

// ---- Users ------------------------------------------------------------------

// UserInput is the payload for creating a user.
type UserInput struct {
	Name        string
	Description string
	Path        string
	Tags        map[string]string
}

// CreateUser provisions an IAM user.
func (s *ThemisStore) CreateUser(ctx context.Context, accountID, projectName string, in UserInput) (*database.User, error) {
	if in.Name == "" {
		return nil, errors.New("user name is required")
	}
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	path := in.Path
	if path == "" {
		path = "/"
	}
	u := &database.User{
		ID:          newID(),
		ProjectID:   proj.ID,
		Name:        in.Name,
		Description: in.Description,
		Path:        path,
		Tags:        in.Tags,
		Status:      "active",
	}
	if err := s.DB.QueryRow(ctx, `
		INSERT INTO users (id, project_id, name, description, path, tags)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at`,
		u.ID, proj.ID, u.Name, u.Description, u.Path, wrapTags(in.Tags)).Scan(&u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	_ = s.refreshProjectCount(ctx, proj.ID)
	return u, nil
}

// GetUser returns a single user by name.
func (s *ThemisStore) GetUser(ctx context.Context, accountID, projectName, userName string) (*database.User, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	return s.resolveUser(ctx, proj.ID, userName)
}

// ListUsers returns all users in a project.
func (s *ThemisStore) ListUsers(ctx context.Context, accountID, projectName string) ([]database.User, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `
		SELECT id, project_id, name, COALESCE(description,''), COALESCE(path,'/'), tags, created_at, updated_at, status
		FROM users
		WHERE project_id = $1 AND status = 'active'
		ORDER BY name`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.User
	for rows.Next() {
		var u database.User
		var tags []byte
		if err := rows.Scan(&u.ID, &u.ProjectID, &u.Name, &u.Description, &u.Path, &tags, &u.CreatedAt, &u.UpdatedAt, &u.Status); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(tags, &u.Tags)
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeleteUser removes a user and all of their access keys.
func (s *ThemisStore) DeleteUser(ctx context.Context, accountID, projectName, userName string) error {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(ctx, `DELETE FROM users WHERE project_id = $1 AND name = $2`, proj.ID, userName)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return s.refreshProjectCount(ctx, proj.ID)
}

// ---- Groups -----------------------------------------------------------------

// GroupInput is the payload for creating a group.
type GroupInput struct {
	Name        string
	Description string
	Tags        map[string]string
}

// CreateGroup provisions an IAM group.
func (s *ThemisStore) CreateGroup(ctx context.Context, accountID, projectName string, in GroupInput) (*database.Group, error) {
	if in.Name == "" {
		return nil, errors.New("group name is required")
	}
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	g := &database.Group{
		ID:          newID(),
		ProjectID:   proj.ID,
		Name:        in.Name,
		Description: in.Description,
		Tags:        in.Tags,
		Status:      "active",
	}
	if err := s.DB.QueryRow(ctx, `
		INSERT INTO groups (id, project_id, name, description, tags)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`,
		g.ID, proj.ID, g.Name, g.Description, wrapTags(in.Tags)).Scan(&g.CreatedAt, &g.UpdatedAt); err != nil {
		return nil, err
	}
	_ = s.refreshProjectCount(ctx, proj.ID)
	return g, nil
}

// GetGroup returns a single group by name.
func (s *ThemisStore) GetGroup(ctx context.Context, accountID, projectName, groupName string) (*database.Group, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	return s.resolveGroup(ctx, proj.ID, groupName)
}

// ListGroups returns all groups in a project.
func (s *ThemisStore) ListGroups(ctx context.Context, accountID, projectName string) ([]database.Group, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `
		SELECT id, project_id, name, COALESCE(description,''), tags, created_at, updated_at, status
		FROM groups
		WHERE project_id = $1 AND status = 'active'
		ORDER BY name`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.Group
	for rows.Next() {
		var g database.Group
		var tags []byte
		if err := rows.Scan(&g.ID, &g.ProjectID, &g.Name, &g.Description, &tags, &g.CreatedAt, &g.UpdatedAt, &g.Status); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(tags, &g.Tags)
		out = append(out, g)
	}
	return out, rows.Err()
}

// DeleteGroup removes a group and its memberships.
func (s *ThemisStore) DeleteGroup(ctx context.Context, accountID, projectName, groupName string) error {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(ctx, `DELETE FROM groups WHERE project_id = $1 AND name = $2`, proj.ID, groupName)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return s.refreshProjectCount(ctx, proj.ID)
}

// GroupAddMember adds a user to a group.
func (s *ThemisStore) GroupAddMember(ctx context.Context, accountID, projectName, groupName, userName string) error {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	g, err := s.resolveGroup(ctx, proj.ID, groupName)
	if err != nil {
		return err
	}
	u, err := s.resolveUser(ctx, proj.ID, userName)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `
		INSERT INTO group_memberships (group_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (group_id, user_id) DO NOTHING`, g.ID, u.ID)
	return err
}

// GroupRemoveMember removes a user from a group.
func (s *ThemisStore) GroupRemoveMember(ctx context.Context, accountID, projectName, groupName, userName string) error {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	g, err := s.resolveGroup(ctx, proj.ID, groupName)
	if err != nil {
		return err
	}
	u, err := s.resolveUser(ctx, proj.ID, userName)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(ctx, `
		DELETE FROM group_memberships WHERE group_id = $1 AND user_id = $2`, g.ID, u.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GroupMembers lists the users in a group with their names.
func (s *ThemisStore) GroupMembers(ctx context.Context, accountID, projectName, groupName string) ([]database.GroupMembership, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	g, err := s.resolveGroup(ctx, proj.ID, groupName)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `
		SELECT m.group_id, m.user_id, u.name, m.created_at
		FROM group_memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.group_id = $1
		ORDER BY u.name`, g.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.GroupMembership
	for rows.Next() {
		var m database.GroupMembership
		if err := rows.Scan(&m.GroupID, &m.UserID, &m.UserName, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---- Roles ------------------------------------------------------------------

// RoleInput is the payload for creating a role.
type RoleInput struct {
	Name        string
	Description string
	Tags        map[string]string
}

// CreateRole provisions an IAM role.
func (s *ThemisStore) CreateRole(ctx context.Context, accountID, projectName string, in RoleInput) (*database.Role, error) {
	if in.Name == "" {
		return nil, errors.New("role name is required")
	}
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	r := &database.Role{
		ID:          newID(),
		ProjectID:   proj.ID,
		Name:        in.Name,
		Description: in.Description,
		Tags:        in.Tags,
		Status:      "active",
	}
	if err := s.DB.QueryRow(ctx, `
		INSERT INTO roles (id, project_id, name, description, tags)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`,
		r.ID, proj.ID, r.Name, r.Description, wrapTags(in.Tags)).Scan(&r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	_ = s.refreshProjectCount(ctx, proj.ID)
	return r, nil
}

// GetRole returns a single role by name.
func (s *ThemisStore) GetRole(ctx context.Context, accountID, projectName, roleName string) (*database.Role, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	return s.resolveRole(ctx, proj.ID, roleName)
}

// ListRoles returns all roles in a project.
func (s *ThemisStore) ListRoles(ctx context.Context, accountID, projectName string) ([]database.Role, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `
		SELECT id, project_id, name, COALESCE(description,''), tags, created_at, updated_at, status
		FROM roles
		WHERE project_id = $1 AND status = 'active'
		ORDER BY name`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.Role
	for rows.Next() {
		var r database.Role
		var tags []byte
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Name, &r.Description, &tags, &r.CreatedAt, &r.UpdatedAt, &r.Status); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(tags, &r.Tags)
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRole removes a role and its attachments.
func (s *ThemisStore) DeleteRole(ctx context.Context, accountID, projectName, roleName string) error {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(ctx, `DELETE FROM roles WHERE project_id = $1 AND name = $2`, proj.ID, roleName)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return s.refreshProjectCount(ctx, proj.ID)
}

// ---- Policies ---------------------------------------------------------------

// PolicyInput is the payload for creating a policy.
type PolicyInput struct {
	Name        string
	Description string
	Document    string
}

// CreatePolicy provisions an IAM policy from a JSON document.
func (s *ThemisStore) CreatePolicy(ctx context.Context, accountID, projectName string, in PolicyInput) (*database.Policy, error) {
	if in.Name == "" {
		return nil, errors.New("policy name is required")
	}
	if _, err := ParsePolicyDocument(in.Document); err != nil {
		return nil, err
	}
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(in.Document), &doc); err != nil {
		return nil, fmt.Errorf("invalid policy document: %w", err)
	}
	p := &database.Policy{
		ID:          newID(),
		ProjectID:   proj.ID,
		Name:        in.Name,
		Description: in.Description,
		Document:    doc,
		Version:     1,
		Status:      "active",
	}
	docJSON, _ := json.Marshal(doc)
	if err := s.DB.QueryRow(ctx, `
		INSERT INTO policies (id, project_id, name, description, document)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`,
		p.ID, proj.ID, p.Name, p.Description, string(docJSON)).Scan(&p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	_ = s.refreshProjectCount(ctx, proj.ID)
	return p, nil
}

// GetPolicy returns a single policy by name.
func (s *ThemisStore) GetPolicy(ctx context.Context, accountID, projectName, policyName string) (*database.Policy, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	return s.resolvePolicy(ctx, proj.ID, policyName)
}

// ListPolicies returns all policies in a project.
func (s *ThemisStore) ListPolicies(ctx context.Context, accountID, projectName string) ([]database.Policy, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `
		SELECT id, project_id, name, COALESCE(description,''), document, version, created_at, updated_at, status
		FROM policies
		WHERE project_id = $1 AND status = 'active'
		ORDER BY name`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.Policy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// DeletePolicy removes a policy and all its attachments.
func (s *ThemisStore) DeletePolicy(ctx context.Context, accountID, projectName, policyName string) error {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(ctx, `DELETE FROM policies WHERE project_id = $1 AND name = $2`, proj.ID, policyName)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return s.refreshProjectCount(ctx, proj.ID)
}

// ---- Attachments ------------------------------------------------------------

// resolvePrincipal returns the id of a user/group/role in a project.
func (s *ThemisStore) resolvePrincipal(ctx context.Context, projectID, principalType, name string) (string, error) {
	switch principalType {
	case database.PrincipalUser:
		u, err := s.resolveUser(ctx, projectID, name)
		if err != nil {
			return "", err
		}
		return u.ID, nil
	case database.PrincipalGroup:
		g, err := s.resolveGroup(ctx, projectID, name)
		if err != nil {
			return "", err
		}
		return g.ID, nil
	case database.PrincipalRole:
		r, err := s.resolveRole(ctx, projectID, name)
		if err != nil {
			return "", err
		}
		return r.ID, nil
	default:
		return "", fmt.Errorf("unsupported principal type %q", principalType)
	}
}

// AttachPolicy attaches a policy to a user, group or role.
func (s *ThemisStore) AttachPolicy(ctx context.Context, accountID, projectName, principalType, principalName, policyName string) (*database.PolicyAttachment, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	principalID, err := s.resolvePrincipal(ctx, proj.ID, principalType, principalName)
	if err != nil {
		return nil, err
	}
	pol, err := s.resolvePolicy(ctx, proj.ID, policyName)
	if err != nil {
		return nil, err
	}
	a := &database.PolicyAttachment{
		ID:            newID(),
		ProjectID:     proj.ID,
		PrincipalType: principalType,
		PrincipalID:   principalID,
		PolicyID:      pol.ID,
	}
	_, err = s.DB.Exec(ctx, `
		INSERT INTO policy_attachments (id, project_id, principal_type, principal_id, policy_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (principal_type, principal_id, policy_id) DO NOTHING`,
		a.ID, proj.ID, principalType, principalID, pol.ID)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// DetachPolicy removes a policy from a principal.
func (s *ThemisStore) DetachPolicy(ctx context.Context, accountID, projectName, principalType, principalName, policyName string) error {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	principalID, err := s.resolvePrincipal(ctx, proj.ID, principalType, principalName)
	if err != nil {
		return err
	}
	pol, err := s.resolvePolicy(ctx, proj.ID, policyName)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(ctx, `
		DELETE FROM policy_attachments
		WHERE principal_type = $1 AND principal_id = $2 AND policy_id = $3`,
		principalType, principalID, pol.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAttachments returns all attachments, optionally filtered by principal.
func (s *ThemisStore) ListAttachments(ctx context.Context, accountID, projectName, principalType, principalName string) ([]database.PolicyAttachment, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT a.id, a.project_id, a.principal_type, a.principal_id,
		       COALESCE(u.name, g.name, r.name), a.policy_id, COALESCE(po.name,''), a.created_at
		FROM policy_attachments a
		JOIN policies po ON po.id = a.policy_id
		LEFT JOIN users u ON u.id = a.principal_id
		LEFT JOIN groups g ON g.id = a.principal_id
		LEFT JOIN roles r ON r.id = a.principal_id
		WHERE a.project_id = $1`
	args := []interface{}{proj.ID}
	if principalType != "" {
		query += ` AND a.principal_type = $2`
		args = append(args, principalType)
		if principalName != "" {
			query += ` AND $3 = COALESCE(u.name, g.name, r.name)`
			args = append(args, principalName)
		}
	}
	query += ` ORDER BY a.principal_type, COALESCE(u.name, g.name, r.name), po.name`

	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.PolicyAttachment
	for rows.Next() {
		var a database.PolicyAttachment
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.PrincipalType, &a.PrincipalID,
			&a.PrincipalName, &a.PolicyID, &a.PolicyName, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- Access keys ------------------------------------------------------------

// CreateAccessKey generates an access key pair for a user. The secret is only
// ever returned here; only its hash is persisted.
func (s *ThemisStore) CreateAccessKey(ctx context.Context, accountID, projectName, userName string) (*database.AccessKey, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	u, err := s.resolveUser(ctx, proj.ID, userName)
	if err != nil {
		return nil, err
	}
	id := newAccessKeyID()
	secret := newSecretAccessKey()
	k := &database.AccessKey{
		ID:         id,
		ProjectID:  proj.ID,
		UserID:     u.ID,
		UserName:   u.Name,
		SecretHash: secretHash(secret),
		Secret:     secret,
		Status:     "active",
	}
	if err := s.DB.QueryRow(ctx, `
		INSERT INTO access_keys (id, project_id, user_id, secret_hash, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`,
		k.ID, proj.ID, u.ID, k.SecretHash, k.Status).Scan(&k.CreatedAt, &k.UpdatedAt); err != nil {
		return nil, err
	}
	return k, nil
}

// ListAccessKeys returns the keys of a user (never the secrets).
func (s *ThemisStore) ListAccessKeys(ctx context.Context, accountID, projectName, userName string) ([]database.AccessKey, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	u, err := s.resolveUser(ctx, proj.ID, userName)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `
		SELECT k.id, k.project_id, k.user_id, COALESCE(u.name,''), k.status, k.last_used_at, k.created_at, k.updated_at
		FROM access_keys k
		JOIN users u ON u.id = k.user_id
		WHERE k.user_id = $1
		ORDER BY k.created_at DESC`, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []database.AccessKey
	for rows.Next() {
		var k database.AccessKey
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.UserID, &k.UserName, &k.Status, &k.LastUsedAt, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// SetAccessKeyStatus activates or deactivates a key.
func (s *ThemisStore) SetAccessKeyStatus(ctx context.Context, accountID, projectName, userName, keyID, status string) (*database.AccessKey, error) {
	if status != "active" && status != "inactive" {
		return nil, errors.New("status must be active or inactive")
	}
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	u, err := s.resolveUser(ctx, proj.ID, userName)
	if err != nil {
		return nil, err
	}
	var k database.AccessKey
	err = s.DB.QueryRow(ctx, `
		UPDATE access_keys
		SET status = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $2
		RETURNING id, project_id, user_id, status, last_used_at, created_at, updated_at`,
		keyID, u.ID, status).Scan(
		&k.ID, &k.ProjectID, &k.UserID, &k.Status, &k.LastUsedAt, &k.CreatedAt, &k.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	k.UserName = u.Name
	return &k, nil
}

// DeleteAccessKey permanently removes a key.
func (s *ThemisStore) DeleteAccessKey(ctx context.Context, accountID, projectName, userName, keyID string) error {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	u, err := s.resolveUser(ctx, proj.ID, userName)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(ctx, `DELETE FROM access_keys WHERE id = $1 AND user_id = $2`, keyID, u.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Authenticate validates an access key id + secret and returns its user.
func (s *ThemisStore) Authenticate(ctx context.Context, accessKeyID, secret string) (*database.User, error) {
	if accessKeyID == "" || secret == "" {
		return nil, ErrInvalidCredentials
	}
	var (
		userID string
		hash   string
		status string
	)
	err := s.DB.QueryRow(ctx, `
		SELECT user_id, secret_hash, status FROM access_keys WHERE id = $1`, accessKeyID).
		Scan(&userID, &hash, &status)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if status != "active" || secretHash(secret) != hash {
		return nil, ErrInvalidCredentials
	}
	var (
		u    database.User
		tags []byte
	)
	if err := s.DB.QueryRow(ctx, `
		SELECT id, project_id, name, COALESCE(description,''), COALESCE(path,'/'), tags, created_at, updated_at, status
		FROM users WHERE id = $1 AND status = 'active'`, userID).
		Scan(&u.ID, &u.ProjectID, &u.Name, &u.Description, &u.Path, &tags, &u.CreatedAt, &u.UpdatedAt, &u.Status); err != nil {
		return nil, ErrInvalidCredentials
	}
	_ = json.Unmarshal(tags, &u.Tags)
	_, _ = s.DB.Exec(ctx, `UPDATE access_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = $1`, accessKeyID)
	return &u, nil
}

// MintToken validates an access key and returns a signed JWT for the user.
func (s *ThemisStore) MintToken(ctx context.Context, accessKeyID, secret string) (string, *TokenClaims, error) {
	if s.JWT == nil {
		return "", nil, errors.New("token minting is not configured")
	}
	u, err := s.Authenticate(ctx, accessKeyID, secret)
	if err != nil {
		return "", nil, err
	}
	proj, err := s.resolveProjectByID(ctx, u.ProjectID)
	if err != nil {
		return "", nil, err
	}
	claims := TokenClaims{
		Issuer:        "themis",
		Subject:       u.Name,
		PrincipalType: database.PrincipalUser,
		AccountID:     proj.AccountID,
		ProjectID:     proj.ID,
	}
	tok, err := s.JWT.Sign(claims)
	if err != nil {
		return "", nil, err
	}
	claims.IssuedAt = s.JWT.now().UTC()
	claims.ExpiresAt = claims.IssuedAt.Add(TokenTTL)
	return tok, &claims, nil
}

// resolveProjectByID loads a project row regardless of account scoping.
func (s *ThemisStore) resolveProjectByID(ctx context.Context, projectID string) (*database.Project, error) {
	var p database.Project
	err := s.DB.QueryRow(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), resource_count, created_at, status
		FROM projects WHERE id = $1`, projectID).
		Scan(&p.ID, &p.AccountID, &p.Name, &p.Description, &p.ResourceCount, &p.CreatedAt, &p.Status)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ---- Policy evaluation ------------------------------------------------------

// principalPolicyIDs returns the policy ids attached to a principal, including
// policies inherited from groups a user belongs to.
func (s *ThemisStore) principalPolicyIDs(ctx context.Context, projectID, principalType, principalID string) ([]string, error) {
	query := `
		SELECT a.policy_id FROM policy_attachments a
		WHERE a.project_id = $1 AND a.principal_type = $2 AND a.principal_id = $3`
	args := []interface{}{projectID, principalType, principalID}

	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Users additionally inherit policies from the groups they belong to.
	if principalType == database.PrincipalUser {
		grows, err := s.DB.Query(ctx, `
			SELECT a.policy_id
			FROM policy_attachments a
			JOIN group_memberships m ON m.group_id = a.principal_id
			WHERE a.project_id = $1 AND a.principal_type = 'group' AND m.user_id = $2`,
			projectID, principalID)
		if err != nil {
			return nil, err
		}
		defer grows.Close()
		for grows.Next() {
			var id string
			if err := grows.Scan(&id); err != nil {
				return nil, err
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if err := grows.Err(); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// Authorize evaluates an action/resource for a principal against the effective
// policies (direct attachments plus group-inherited ones for users).
func (s *ThemisStore) Authorize(ctx context.Context, accountID, projectName, principalType, principalName, action, resource string) (*EvaluationDecision, error) {
	if action == "" || resource == "" {
		return nil, errors.New("action and resource are required")
	}
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	principalID, err := s.resolvePrincipal(ctx, proj.ID, principalType, principalName)
	if err != nil {
		return nil, err
	}
	ids, err := s.principalPolicyIDs(ctx, proj.ID, principalType, principalID)
	if err != nil {
		return nil, err
	}
	docs := make([]*PolicyDocument, 0, len(ids))
	for _, id := range ids {
		var raw []byte
		if err := s.DB.QueryRow(ctx, `SELECT document FROM policies WHERE id = $1`, id).Scan(&raw); err != nil {
			return nil, err
		}
		doc, derr := ParsePolicyDocument(string(raw))
		if derr != nil {
			continue
		}
		docs = append(docs, doc)
	}
	decision := EvaluatePolicies(docs, action, resource)
	decision.Principal = fmt.Sprintf("%s:%s", principalType, principalName)
	decision.Action = action
	decision.Resource = resource
	return &decision, nil
}

// Audit records an operation in the audit trail.
func (s *ThemisStore) Audit(ctx context.Context, accountID, projectID, principal, resource, operation, status string) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO audit_logs (account_id, project_id, principal, resource, operation, status)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5, $6)`,
		accountID, projectID, principal, resource, operation, status)
	return err
}

// rowScanner abstracts *sql.Row and *sql.Rows for shared scanning.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanPolicy scans a policy row.
func scanPolicy(s rowScanner) (*database.Policy, error) {
	var p database.Policy
	var doc []byte
	err := s.Scan(&p.ID, &p.ProjectID, &p.Name, &p.Description, &doc, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.Status)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(doc) > 0 {
		_ = json.Unmarshal(doc, &p.Document)
	}
	return &p, nil
}

// wrapTags converts a tags map into a JSON string for storage (nil when empty).
func wrapTags(tags map[string]string) interface{} {
	if len(tags) == 0 {
		return nil
	}
	data, err := json.Marshal(tags)
	if err != nil {
		return nil
	}
	return string(data)
}
