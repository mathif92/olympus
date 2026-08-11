// Package pkg contains the Prometheus business-logic layer: serverless
// functions (AWS Lambda equivalent), immutable code versions, invocations and
// audit trails. Code execution is delegated to an Executor (mock for dev, or a
// Docker-backed executor that builds per-runtime images and runs them with
// enforced limits).
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

	"github.com/mathif92/olympus/prometheus/pkg/database"
)

// Audit operations recorded in the audit_logs table.
const (
	OpCreate = "create"
	OpList   = "list"
	OpDelete = "delete"
	OpDeploy = "deploy"
	OpInvoke = "invoke"
)

// ErrNotFound is returned when a requested project or resource does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when an operation cannot complete from the current state.
var ErrConflict = errors.New("state conflict")

// Prometheus is the serverless control-plane orchestrator.
type Prometheus struct {
	DB       *database.Client
	Executor Executor
}

// NewPrometheus wires the serverless control plane to Postgres and an executor.
func NewPrometheus(db *database.Client, executor Executor) *Prometheus {
	return &Prometheus{DB: db, Executor: executor}
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// EnsureAccount upserts the tenant so requests referencing it always resolve.
func (p *Prometheus) EnsureAccount(ctx context.Context, a database.Account) error {
	_, err := p.DB.Exec(ctx, `
		INSERT INTO accounts (id, display_name, email, plan, function_limit)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET display_name = EXCLUDED.display_name`,
		a.ID, a.DisplayName, a.Email, a.Plan, a.FunctionLimit)
	return err
}

// CreateProject provisions a project namespace inside the account.
func (p *Prometheus) CreateProject(ctx context.Context, accountID string, pr database.Project) error {
	if pr.Name == "" {
		return errors.New("project name is required")
	}
	pr.ID = newID()
	_, err := p.DB.Exec(ctx, `
		INSERT INTO projects (id, account_id, name, description)
		VALUES ($1, $2, $3, $4)`,
		pr.ID, accountID, pr.Name, pr.Description)
	return err
}

// ListProjects returns all projects for an account.
func (p *Prometheus) ListProjects(ctx context.Context, accountID string) ([]database.Project, error) {
	rows, err := p.DB.Query(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), function_count, created_at, status
		FROM projects WHERE account_id = $1 ORDER BY name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Project
	for rows.Next() {
		var pr database.Project
		if err := rows.Scan(&pr.ID, &pr.AccountID, &pr.Name, &pr.Description, &pr.FunctionCount, &pr.CreatedAt, &pr.Status); err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

func (p *Prometheus) resolveProject(ctx context.Context, accountID, projectName string) (*database.Project, error) {
	var pr database.Project
	err := p.DB.QueryRow(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), function_count, created_at, status
		FROM projects WHERE account_id = $1 AND name = $2 AND status = 'active'`,
		accountID, projectName).Scan(
		&pr.ID, &pr.AccountID, &pr.Name, &pr.Description, &pr.FunctionCount, &pr.CreatedAt, &pr.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &pr, nil
}

// IsNotFound reports whether an error wraps sql.ErrNoRows / ErrNotFound.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err == ErrNotFound || errors.Is(err, sql.ErrNoRows)
}

// --- Functions ---

// CreateFunction creates a serverless function configuration in a project.
func (p *Prometheus) CreateFunction(ctx context.Context, accountID, projectName string, f database.Function) (*database.Function, error) {
	proj, err := p.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	if f.Name == "" {
		return nil, errors.New("function name is required")
	}
	rt, ok := GetRuntime(f.Runtime)
	if !ok {
		return nil, fmt.Errorf("unknown runtime %q (see /runtimes)", f.Runtime)
	}
	if f.TimeoutMS <= 0 {
		f.TimeoutMS = 30000
	}
	if f.MemoryMB <= 0 {
		f.MemoryMB = 128
	}
	if f.CPUs <= 0 {
		f.CPUs = 0.5
	}
	if f.Handler == "" {
		f.Handler = rt.Handler
	}
	f.ID = newID()
	f.AccountID = accountID
	f.ProjectID = proj.ID
	f.Status = "active"
	f.CurrentVersion = 0
	if _, err := p.DB.Exec(ctx, `
		INSERT INTO functions (id, account_id, project_id, name, description, runtime, handler,
		                       timeout_ms, memory_mb, cpus, env_vars, status)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, $10, $11, $12)`,
		f.ID, accountID, proj.ID, f.Name, f.Description, f.Runtime, f.Handler,
		f.TimeoutMS, f.MemoryMB, f.CPUs, f.EnvVarsParam(), f.Status); err != nil {
		return nil, err
	}
	p.refreshCounts(ctx, proj.ID)
	return p.GetFunction(ctx, accountID, projectName, f.Name)
}

// GetFunction returns a single function in a project.
func (p *Prometheus) GetFunction(ctx context.Context, accountID, projectName, name string) (*database.Function, error) {
	proj, err := p.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	f := &database.Function{}
	var env []byte
	err = p.DB.QueryRow(ctx, `
		SELECT id, account_id, project_id, name, COALESCE(description,''), runtime, handler,
		       timeout_ms, memory_mb, cpus, COALESCE(env_vars, '{}'::jsonb), current_version,
		       created_at, updated_at, status
		FROM functions WHERE project_id = $1 AND name = $2`,
		proj.ID, name).Scan(
		&f.ID, &f.AccountID, &f.ProjectID, &f.Name, &f.Description, &f.Runtime, &f.Handler,
		&f.TimeoutMS, &f.MemoryMB, &f.CPUs, &env, &f.CurrentVersion,
		&f.CreatedAt, &f.UpdatedAt, &f.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(env, &f.EnvVars)
	return f, nil
}

// ListFunctions returns all functions in a project.
func (p *Prometheus) ListFunctions(ctx context.Context, accountID, projectName string) ([]database.Function, error) {
	proj, err := p.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := p.DB.Query(ctx, `
		SELECT id, account_id, project_id, name, COALESCE(description,''), runtime, handler,
		       timeout_ms, memory_mb, cpus, COALESCE(env_vars, '{}'::jsonb), current_version,
		       created_at, updated_at, status
		FROM functions WHERE project_id = $1 ORDER BY name`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Function
	for rows.Next() {
		var f database.Function
		var env []byte
		if err := rows.Scan(&f.ID, &f.AccountID, &f.ProjectID, &f.Name, &f.Description, &f.Runtime, &f.Handler,
			&f.TimeoutMS, &f.MemoryMB, &f.CPUs, &env, &f.CurrentVersion,
			&f.CreatedAt, &f.UpdatedAt, &f.Status); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(env, &f.EnvVars)
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteFunction removes a function and cascades its versions and invocations.
func (p *Prometheus) DeleteFunction(ctx context.Context, accountID, projectName, name string) error {
	proj, err := p.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	_, err = p.DB.Exec(ctx, `
		DELETE FROM functions WHERE project_id = $1 AND name = $2`,
		proj.ID, name)
	if err != nil {
		return err
	}
	p.refreshCounts(ctx, proj.ID)
	return nil
}

// --- Versions ---

// UploadVersion snapshots the given code archive as the function's new active
// version. It validates the archive against the runtime's required entrypoint.
func (p *Prometheus) UploadVersion(ctx context.Context, accountID, projectName, name string, code []byte) (*database.FunctionVersion, error) {
	fn, err := p.GetFunction(ctx, accountID, projectName, name)
	if err != nil {
		return nil, err
	}
	rt, ok := GetRuntime(fn.Runtime)
	if !ok {
		return nil, fmt.Errorf("unknown runtime %q", fn.Runtime)
	}
	if err := ValidateFunctionCode(code, rt); err != nil {
		return nil, err
	}

	sum := sha256.Sum256(code)
	hash := hex.EncodeToString(sum[:])

	var next int
	if err := p.DB.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1 FROM function_versions WHERE function_id = $1`,
		fn.ID).Scan(&next); err != nil {
		return nil, err
	}

	ver := &database.FunctionVersion{
		ID:         newID(),
		FunctionID: fn.ID,
		Version:    next,
		Code:       code,
		CodeSHA256: hash,
		CodeSize:   len(code),
		IsActive:   true,
	}

	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		UPDATE function_versions SET is_active = false WHERE function_id = $1`, fn.ID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO function_versions (id, function_id, version, code, code_sha256, code_size, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, true)`,
		ver.ID, fn.ID, next, code, hash, len(code)); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE functions SET current_version = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		fn.ID, next); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ver, nil
}

// ListVersions returns all versions of a function, newest first.
func (p *Prometheus) ListVersions(ctx context.Context, accountID, projectName, name string) ([]database.FunctionVersion, error) {
	fn, err := p.GetFunction(ctx, accountID, projectName, name)
	if err != nil {
		return nil, err
	}
	rows, err := p.DB.Query(ctx, `
		SELECT id, function_id, version, code_sha256, code_size, is_active, created_at
		FROM function_versions WHERE function_id = $1 ORDER BY version DESC`, fn.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.FunctionVersion
	for rows.Next() {
		var v database.FunctionVersion
		if err := rows.Scan(&v.ID, &v.FunctionID, &v.Version, &v.CodeSHA256, &v.CodeSize, &v.IsActive, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (p *Prometheus) getActiveVersion(ctx context.Context, functionID string) (*database.FunctionVersion, error) {
	v := &database.FunctionVersion{}
	err := p.DB.QueryRow(ctx, `
		SELECT id, function_id, version, code, code_sha256, code_size, is_active, created_at
		FROM function_versions WHERE function_id = $1 AND is_active = true`,
		functionID).Scan(
		&v.ID, &v.FunctionID, &v.Version, &v.Code, &v.CodeSHA256, &v.CodeSize, &v.IsActive, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

// --- Invocations ---

// InvokeFunction runs the function's active version with the given event and
// records the invocation. A returned non-nil error means execution could not
// happen at all (e.g. image build failure, backend down); the invocation is
// still recorded with status=error.
func (p *Prometheus) InvokeFunction(ctx context.Context, accountID, projectName, name string, event json.RawMessage) (*database.Invocation, error) {
	fn, err := p.GetFunction(ctx, accountID, projectName, name)
	if err != nil {
		return nil, err
	}
	ver, err := p.getActiveVersion(ctx, fn.ID)
	if err != nil {
		return nil, fmt.Errorf("function %s has no deployed code: upload a zip first", name)
	}

	rec := &database.Invocation{
		ID:         newID(),
		FunctionID: fn.ID,
		Version:    ver.Version,
		Request:    event,
	}

	res, execErr := p.Executor.Invoke(ctx, *fn, *ver, event)
	if execErr != nil {
		rec.Status = StatusError
		rec.Error = execErr.Error()
		_ = p.insertInvocation(ctx, rec)
		return rec, execErr
	}
	rec.Status = res.Status
	rec.Response = res.Response
	rec.Error = res.Error
	rec.ExitCode = res.ExitCode
	rec.DurationMS = res.DurationMS
	if err := p.insertInvocation(ctx, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

func (p *Prometheus) insertInvocation(ctx context.Context, rec *database.Invocation) error {
	_, err := p.DB.Exec(ctx, `
		INSERT INTO invocations (id, function_id, version, status, request, response, error, exit_code, duration_ms)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, 0), $9)`,
		rec.ID, rec.FunctionID, rec.Version, rec.Status, rec.Request, rec.Response, rec.Error, rec.ExitCode, rec.DurationMS)
	return err
}

// ListInvocations returns the most recent invocations of a function.
func (p *Prometheus) ListInvocations(ctx context.Context, accountID, projectName, name string, limit int) ([]database.Invocation, error) {
	fn, err := p.GetFunction(ctx, accountID, projectName, name)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.DB.Query(ctx, `
		SELECT id, function_id, version, status, COALESCE(request, '{}'::jsonb),
		       COALESCE(response, ''), COALESCE(error, ''), COALESCE(exit_code, 0), duration_ms, invoked_at
		FROM invocations WHERE function_id = $1 ORDER BY invoked_at DESC LIMIT $2`,
		fn.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Invocation
	for rows.Next() {
		var inv database.Invocation
		var req []byte
		if err := rows.Scan(&inv.ID, &inv.FunctionID, &inv.Version, &inv.Status, &req,
			&inv.Response, &inv.Error, &inv.ExitCode, &inv.DurationMS, &inv.InvokedAt); err != nil {
			return nil, err
		}
		inv.Request = json.RawMessage(req)
		out = append(out, inv)
	}
	return out, rows.Err()
}

// Audit records an operation in the audit trail.
func (p *Prometheus) Audit(ctx context.Context, accountID, projectID, entity, operation, status string) error {
	_, err := p.DB.Exec(ctx, `
		INSERT INTO audit_logs (account_id, project_id, entity, operation, status)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5)`,
		accountID, projectID, entity, operation, status)
	return err
}

func (p *Prometheus) refreshCounts(ctx context.Context, projectID string) {
	_, _ = p.DB.Exec(ctx, `
		UPDATE projects SET
			function_count = (SELECT COUNT(*) FROM functions WHERE project_id = $1)
		WHERE id = $1`, projectID)
}
