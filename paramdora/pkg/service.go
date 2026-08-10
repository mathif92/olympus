// Package pkg contains the Paramdora business-logic layer: multi-tenant
// parameter storage, versioning, and optional encryption of secure values.
package pkg

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mathif92/olympus/paramdora/pkg/database"
)

// Audit operations recorded in the audit_logs table.
const (
	OpCreate  = "create"
	OpGet     = "get"
	OpUpdate  = "update"
	OpDelete  = "delete"
	OpList    = "list"
	OpHistory = "history"
)

// ErrNotFound is returned when a requested project or parameter does not exist.
var ErrNotFound = errors.New("not found")

// paramCols is the canonical column projection for scanning parameters.
// Keep it in sync with scanParameter.
const paramCols = `id, project_id, name, value, data_type,
       COALESCE(description,''), is_encrypted, tier, version, COALESCE(key_id,''),
       created_at, updated_at, COALESCE(last_modified_by,''), status, tags`

// ParamStore is the business-logic layer for multi-tenant parameter storage.
type ParamStore struct {
	DB     *database.Client
	Cipher *Cipher
}

// NewParamStore creates a store backed by Postgres and an AES-GCM cipher.
func NewParamStore(db *database.Client, cipher *Cipher) *ParamStore {
	return &ParamStore{DB: db, Cipher: cipher}
}

// CipherKeyID returns the cipher key identifier, or "" when unset.
func (s *ParamStore) CipherKeyID() string {
	if s.Cipher == nil {
		return ""
	}
	return s.Cipher.KeyID
}

// newID returns a 32-char hex identifier.
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// EnsureAccount upserts the tenant so requests referencing it always resolve.
func (s *ParamStore) EnsureAccount(ctx context.Context, a database.Account) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO accounts (id, display_name, email, plan, parameter_limit)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET display_name = EXCLUDED.display_name`,
		a.ID, a.DisplayName, a.Email, a.Plan, a.ParameterLimit)
	return err
}

// CreateProject provisions a project namespace inside the account.
func (s *ParamStore) CreateProject(ctx context.Context, accountID string, p database.Project) error {
	if p.Name == "" {
		return errors.New("project name is required")
	}
	p.ID = newID()
	p.AccountID = accountID
	_, err := s.DB.Exec(ctx, `
		INSERT INTO projects (id, account_id, name, description)
		VALUES ($1, $2, $3, $4)`,
		p.ID, p.AccountID, p.Name, p.Description)
	return err
}

// ListProjects returns all projects for an account.
func (s *ParamStore) ListProjects(ctx context.Context, accountID string) ([]database.Project, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), parameter_count, created_at, status
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
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &p.Description, &p.ParameterCount, &p.CreatedAt, &p.Status); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// resolveProject loads a project owned by the account or returns ErrNotFound.
func (s *ParamStore) resolveProject(ctx context.Context, accountID, projectName string) (*database.Project, error) {
	var p database.Project
	err := s.DB.QueryRow(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), parameter_count, created_at, status
		FROM projects
		WHERE account_id = $1 AND name = $2 AND status = 'active'`,
		accountID, projectName).Scan(
		&p.ID, &p.AccountID, &p.Name, &p.Description, &p.ParameterCount, &p.CreatedAt, &p.Status)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// PutParameterInput is the payload for creating or updating a parameter.
type PutParameterInput struct {
	Name           string
	Value          string
	Type           string
	Description    string
	Tier           string
	Tags           map[string]string
	LastModifiedBy string
}

// PutParameter upserts a parameter, bumping its version and appending to the
// version history. SecureString values are encrypted at rest.
func (s *ParamStore) PutParameter(ctx context.Context, accountID, projectName string, in PutParameterInput) (*database.Parameter, error) {
	if projectName == "" || in.Value == "" {
		return nil, errors.New("project name and parameter value are required")
	}

	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}

	typ := in.Type
	if typ == "" {
		typ = database.TypeSecureString
	}
	if typ != database.TypeString && typ != database.TypeStringList && typ != database.TypeSecureString {
		return nil, fmt.Errorf("unsupported parameter type %q", typ)
	}

	encrypted := false
	value := in.Value
	if typ == database.TypeSecureString {
		if s.Cipher == nil {
			return nil, errors.New("secure_string requires an encryption cipher")
		}
		value, err = s.Cipher.Encrypt(value)
		if err != nil {
			return nil, fmt.Errorf("encrypt value: %w", err)
		}
		encrypted = true
	}

	tier := in.Tier
	if tier == "" {
		tier = "standard"
	}

	p := &database.Parameter{
		ID:             newID(),
		ProjectID:      proj.ID,
		Name:           strings.TrimPrefix(in.Name, "/"),
		Value:          value,
		Type:           typ,
		Description:    in.Description,
		Encrypted:      encrypted,
		Tier:           tier,
		KeyID:          s.CipherKeyID(),
		LastModifiedBy: in.LastModifiedBy,
	}

	// 1. Upsert the current parameter, bumping the version on conflict.
	var (
		paramID string
		version int
	)
	err = s.DB.QueryRow(ctx, `
		INSERT INTO parameters (id, project_id, name, value, data_type, description,
		                        is_encrypted, tier, key_id, tags, version, last_modified_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 1, $11)
		ON CONFLICT (project_id, name) DO UPDATE SET
			value            = EXCLUDED.value,
			data_type        = EXCLUDED.data_type,
			description      = EXCLUDED.description,
			is_encrypted     = EXCLUDED.is_encrypted,
			tier             = EXCLUDED.tier,
			key_id           = EXCLUDED.key_id,
			tags             = EXCLUDED.tags,
			version          = parameters.version + 1,
			updated_at       = CURRENT_TIMESTAMP,
			last_modified_by = EXCLUDED.last_modified_by
		RETURNING id, version`,
		p.ID, proj.ID, p.Name, value, typ, in.Description, encrypted, tier, p.KeyID, wrapTags(in.Tags), in.LastModifiedBy).
		Scan(&paramID, &version)
	if err != nil {
		return nil, err
	}

	// 2. Append an immutable version to the history.
	if _, err := s.DB.Exec(ctx, `
		INSERT INTO parameter_versions (parameter_id, version, value, data_type, description, is_encrypted, key_id, tags, last_modified_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		paramID, version, value, typ, in.Description, encrypted, p.KeyID, wrapTags(in.Tags), in.LastModifiedBy); err != nil {
		return nil, err
	}

	// 3. Re-fetch the stored row for a full, consistent projection.
	stored, err := scanParameter(s.DB.QueryRow(ctx,
		`SELECT `+paramCols+` FROM parameters WHERE id = $1`, paramID))
	if err != nil {
		return nil, err
	}
	stored.Version = version

	// Keep the cached parameter_count in sync.
	if _, err := s.DB.Exec(ctx,
		`UPDATE projects SET parameter_count = (SELECT COUNT(*) FROM parameters WHERE project_id = $1) WHERE id = $1`,
		proj.ID); err != nil {
		return nil, err
	}

	// Expose the plaintext value back to the caller.
	if stored.Encrypted {
		if plain, derr := s.Cipher.Decrypt(stored.Value); derr == nil {
			stored.Value = plain
		}
	}
	return stored, nil
}

// GetParameter returns a single parameter, optionally decrypting secure values.
func (s *ParamStore) GetParameter(ctx context.Context, accountID, projectName, paramName string, decrypt bool) (*database.Parameter, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}

	p, err := scanParameter(s.DB.QueryRow(ctx, `
		SELECT `+paramCols+`
		FROM parameters
		WHERE project_id = $1 AND name = $2 AND status = 'active'`,
		proj.ID, strings.TrimPrefix(paramName, "/")))
	if err != nil {
		return nil, err
	}

	if p.Encrypted {
		if !decrypt {
			p.Value = ""
		} else if s.Cipher != nil {
			plain, derr := s.Cipher.Decrypt(p.Value)
			if derr != nil {
				return nil, fmt.Errorf("decrypt value: %w", derr)
			}
			p.Value = plain
		}
	}
	return p, nil
}

// ListParameters returns all parameters in a project, optionally filtered by
// a name prefix. Secure values are never returned.
func (s *ParamStore) ListParameters(ctx context.Context, accountID, projectName, prefix string) ([]database.Parameter, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}

	query := `SELECT ` + paramCols + ` FROM parameters
	          WHERE project_id = $1 AND status = 'active'`
	args := []interface{}{proj.ID}
	if prefix != "" {
		query += ` AND name LIKE $2`
		args = append(args, strings.TrimPrefix(prefix, "/")+"%")
	}
	query += ` ORDER BY name`

	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Parameter
	for rows.Next() {
		p, err := scanParameter(rows)
		if err != nil {
			return nil, err
		}
		if p.Encrypted {
			p.Value = ""
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// DeleteParameter removes a parameter (and its full version history).
func (s *ParamStore) DeleteParameter(ctx context.Context, accountID, projectName, paramName string) error {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(ctx,
		`DELETE FROM parameters WHERE project_id = $1 AND name = $2`, proj.ID, strings.TrimPrefix(paramName, "/"))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = s.DB.Exec(ctx,
		`UPDATE projects SET parameter_count = (SELECT COUNT(*) FROM parameters WHERE project_id = $1) WHERE id = $1`,
		proj.ID)
	return err
}

// GetParameterHistory returns the immutable version history of a parameter.
func (s *ParamStore) GetParameterHistory(ctx context.Context, accountID, projectName, paramName string) ([]database.Parameter, error) {
	proj, err := s.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}

	rows, err := s.DB.Query(ctx, `
		SELECT v.parameter_id AS id, p.project_id, p.name, v.value, v.data_type,
		       COALESCE(v.description,''), v.is_encrypted, p.tier, v.version,
		       COALESCE(v.key_id,''), p.created_at, v.created_at, COALESCE(v.last_modified_by,''),
		       p.status, v.tags
		FROM parameter_versions v
		JOIN parameters p ON p.id = v.parameter_id
		WHERE p.project_id = $1 AND p.name = $2
		ORDER BY v.version`, proj.ID, strings.TrimPrefix(paramName, "/"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Parameter
	for rows.Next() {
		p, err := scanParameter(rows)
		if err != nil {
			return nil, err
		}
		if p.Encrypted {
			p.Value = ""
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// Audit records an operation in the audit trail.
func (s *ParamStore) Audit(ctx context.Context, accountID, projectID, paramName, operation, status string) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO audit_logs (account_id, project_id, parameter_name, operation, status)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5)`,
		accountID, projectID, paramName, operation, status)
	return err
}

// rowScanner abstracts *sql.Row and *sql.Rows for shared scanning.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanParameter scans a parameter row using the canonical paramCols projection.
func scanParameter(s rowScanner) (*database.Parameter, error) {
	var p database.Parameter
	var tags []byte
	err := s.Scan(
		&p.ID, &p.ProjectID, &p.Name, &p.Value, &p.Type, &p.Description,
		&p.Encrypted, &p.Tier, &p.Version, &p.KeyID, &p.CreatedAt, &p.UpdatedAt,
		&p.LastModifiedBy, &p.Status, &tags)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(tags) > 0 {
		_ = json.Unmarshal(tags, &p.Tags)
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
