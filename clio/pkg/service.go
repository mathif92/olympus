// Package pkg contains the Clio business-logic layer: multi-tenant managed
// relational database control plane with pluggable provisioning, instance
// lifecycle, point-in-time snapshots and audit trails.
package pkg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/mathif92/olympus/clio/pkg/database"
)

// Audit operations recorded in the audit_logs table.
const (
	OpCreate   = "create"
	OpList     = "list"
	OpDelete   = "delete"
	OpSnapshot = "snapshot"
)

// Clio is the managed-relational-database control-plane orchestrator.
type Clio struct {
	DB          *database.Client
	Provisioner Provisioner
}

// NewClio wires the control plane to Postgres and a provisioner.
func NewClio(db *database.Client, provisioner Provisioner) *Clio {
	return &Clio{DB: db, Provisioner: provisioner}
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

const masterPasswordCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// generateMasterPassword produces a strong random master password for instances.
func generateMasterPassword() (string, error) {
	b := make([]byte, 20)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(masterPasswordCharset))))
		if err != nil {
			return "", err
		}
		b[i] = masterPasswordCharset[n.Int64()]
	}
	return string(b), nil
}

// EnsureAccount upserts the tenant so requests referencing it always resolve.
func (c *Clio) EnsureAccount(ctx context.Context, a database.Account) error {
	_, err := c.DB.Exec(ctx, `
		INSERT INTO accounts (id, display_name, email, plan, instance_limit)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET display_name = EXCLUDED.display_name`,
		a.ID, a.DisplayName, a.Email, a.Plan, a.InstanceLimit)
	return err
}

// CreateProject provisions a project namespace inside the account.
func (c *Clio) CreateProject(ctx context.Context, accountID string, p database.Project) error {
	if p.Name == "" {
		return errors.New("project name is required")
	}
	p.ID = newID()
	_, err := c.DB.Exec(ctx, `
		INSERT INTO projects (id, account_id, name, description)
		VALUES ($1, $2, $3, $4)`,
		p.ID, accountID, p.Name, p.Description)
	return err
}

// ListProjects returns all projects for an account.
func (c *Clio) ListProjects(ctx context.Context, accountID string) ([]database.Project, error) {
	rows, err := c.DB.Query(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), instance_count, created_at, status
		FROM projects WHERE account_id = $1 ORDER BY name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Project
	for rows.Next() {
		var p database.Project
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &p.Description, &p.InstanceCount, &p.CreatedAt, &p.Status); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (c *Clio) resolveProject(ctx context.Context, accountID, projectName string) (*database.Project, error) {
	var p database.Project
	err := c.DB.QueryRow(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), instance_count, created_at, status
		FROM projects WHERE account_id = $1 AND name = $2 AND status = 'active'`,
		accountID, projectName).Scan(
		&p.ID, &p.AccountID, &p.Name, &p.Description, &p.InstanceCount, &p.CreatedAt, &p.Status)
	if IsNotFound(err) {
		return nil, ErrNotFound
	}
	return &p, err
}

// ListEngines returns the catalog of supported database engines.
func (c *Clio) ListEngines(ctx context.Context) ([]database.DatabaseEngine, error) {
	rows, err := c.DB.Query(ctx, `
		SELECT engine, version, status
		FROM database_engines WHERE status = 'active' ORDER BY engine, version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.DatabaseEngine
	for rows.Next() {
		var e database.DatabaseEngine
		if err := rows.Scan(&e.Engine, &e.Version, &e.Status); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListInstanceSizes returns the catalog of available database sizes.
func (c *Clio) ListInstanceSizes(ctx context.Context) ([]database.InstanceSize, error) {
	rows, err := c.DB.Query(ctx, `
		SELECT name, vcpus, memory_gb, storage_gb, price_per_hour_cents, status
		FROM instance_sizes WHERE status = 'active' ORDER BY memory_gb`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.InstanceSize
	for rows.Next() {
		var t database.InstanceSize
		if err := rows.Scan(&t.Name, &t.VCPUs, &t.MemoryGB, &t.StorageGB, &t.PricePerHourCents, &t.Status); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateInstance provisions a managed database engine via the pluggable
// provisioner and records it in Postgres.
func (c *Clio) CreateInstance(ctx context.Context, accountID, projectName string, spec InstanceSpec) (*database.DBInstance, error) {
	proj, err := c.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	if spec.Name == "" || spec.Engine == "" || spec.EngineVersion == "" {
		return nil, errors.New("instance name, engine and engine_version are required")
	}

	var one int
	err = c.DB.QueryRow(ctx,
		`SELECT 1 FROM database_engines WHERE engine = $1 AND version = $2 AND status = 'active'`,
		spec.Engine, spec.EngineVersion).Scan(&one)
	if IsNotFound(err) {
		return nil, fmt.Errorf("unsupported engine %q %q", spec.Engine, spec.EngineVersion)
	}
	if err != nil {
		return nil, err
	}

	var sizeGB int
	err = c.DB.QueryRow(ctx,
		`SELECT storage_gb FROM instance_sizes WHERE name = $1 AND status = 'active'`,
		spec.Size).Scan(&sizeGB)
	if IsNotFound(err) {
		return nil, fmt.Errorf("unsupported instance size %q", spec.Size)
	}
	if err != nil {
		return nil, err
	}

	masterPassword, err := generateMasterPassword()
	if err != nil {
		return nil, err
	}
	spec.ID = newID()
	spec.ProjectID = proj.ID
	spec.AllocatedStorageGB = sizeGB
	spec.MasterUsername = "master_" + sanitize(spec.Name) + "_admin"
	spec.MasterPassword = masterPassword

	instance := &database.DBInstance{
		ID:                 spec.ID,
		ProjectID:          proj.ID,
		Name:               spec.Name,
		Engine:             spec.Engine,
		EngineVersion:      spec.EngineVersion,
		Size:               spec.Size,
		AllocatedStorageGB: sizeGB,
		State:              StateCreating,
		Status:             "active",
	}
	if _, err := c.DB.Exec(ctx, `
		INSERT INTO db_instances (id, project_id, name, engine, engine_version, size, allocated_storage_gb, state, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		instance.ID, proj.ID, spec.Name, spec.Engine, spec.EngineVersion, spec.Size, sizeGB, StateCreating, "active"); err != nil {
		return nil, err
	}

	// Delegate the actual engine provisioning to the pluggable backend.
	provisioned, err := c.Provisioner.CreateInstance(ctx, spec)
	if err != nil {
		_, _ = c.DB.Exec(ctx,
			`UPDATE db_instances SET state = 'failed', updated_at = CURRENT_TIMESTAMP WHERE id = $1`, instance.ID)
		return nil, fmt.Errorf("provisioner create instance failed: %w", err)
	}

	_, err = c.DB.Exec(ctx, `
		UPDATE db_instances
		SET state = $2, endpoint = $3, master_username = $4, provider_ref = $5, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
		instance.ID, StateActive, provisioned.Endpoint, provisioned.MasterUsername, provisioned.ProviderRef)
	if err != nil {
		return nil, err
	}
	c.refreshCounts(ctx, proj.ID)

	full, err := scanInstance(c.DB.QueryRow(ctx, `
		SELECT id, project_id, name, engine, engine_version, size, allocated_storage_gb, state,
		       COALESCE(endpoint,''), COALESCE(master_username,''),
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM db_instances WHERE id = $1`, instance.ID))
	if err != nil {
		return nil, err
	}
	// The master password is returned exactly once at creation; it is not
	// persisted by the control plane.
	full.MasterPassword = masterPassword
	return full, nil
}

// GetInstance returns a single managed instance within a project.
func (c *Clio) GetInstance(ctx context.Context, accountID, projectName, name string) (*database.DBInstance, error) {
	proj, err := c.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	return scanInstance(c.DB.QueryRow(ctx, `
		SELECT id, project_id, name, engine, engine_version, size, allocated_storage_gb, state,
		       COALESCE(endpoint,''), COALESCE(master_username,''),
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM db_instances WHERE project_id = $1 AND name = $2`,
		proj.ID, name))
}

// ListInstances returns all managed instances in a project.
func (c *Clio) ListInstances(ctx context.Context, accountID, projectName string) ([]database.DBInstance, error) {
	proj, err := c.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := c.DB.Query(ctx, `
		SELECT id, project_id, name, engine, engine_version, size, allocated_storage_gb, state,
		       COALESCE(endpoint,''), COALESCE(master_username,''),
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM db_instances WHERE project_id = $1 ORDER BY created_at`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.DBInstance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inst)
	}
	return out, rows.Err()
}

// DeleteInstance tears a managed instance down for good.
func (c *Clio) DeleteInstance(ctx context.Context, accountID, projectName, name string) error {
	proj, err := c.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	inst, err := scanInstance(c.DB.QueryRow(ctx, `
		SELECT id, project_id, name, engine, engine_version, size, allocated_storage_gb, state,
		       COALESCE(endpoint,''), COALESCE(master_username,''),
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM db_instances WHERE project_id = $1 AND name = $2`,
		proj.ID, name))
	if err != nil {
		return err
	}
	if inst.State == StateDeleted {
		return ErrConflict
	}

	if err := c.Provisioner.DeleteInstance(ctx, inst.ProviderRef); err != nil {
		return err
	}
	_, err = c.DB.Exec(ctx, `
		UPDATE db_instances SET state = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		inst.ID, StateDeleted)
	if err != nil {
		return err
	}
	c.refreshCounts(ctx, proj.ID)
	return nil
}

// StartInstance resumes a stopped managed instance.
func (c *Clio) StartInstance(ctx context.Context, accountID, projectName, name string) (*database.DBInstance, error) {
	inst, err := c.GetInstance(ctx, accountID, projectName, name)
	if err != nil {
		return nil, err
	}
	if inst.State != StateStopped {
		return nil, fmt.Errorf("instance %q is not stopped (state: %s)", name, inst.State)
	}
	if err := c.Provisioner.StartInstance(ctx, inst.ProviderRef); err != nil {
		return nil, err
	}
	_, err = c.DB.Exec(ctx, `
		UPDATE db_instances SET state = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		inst.ID, StateActive)
	if err != nil {
		return nil, err
	}
	return c.GetInstance(ctx, accountID, projectName, name)
}

// StopInstance pauses a running managed instance.
func (c *Clio) StopInstance(ctx context.Context, accountID, projectName, name string) (*database.DBInstance, error) {
	inst, err := c.GetInstance(ctx, accountID, projectName, name)
	if err != nil {
		return nil, err
	}
	if inst.State != StateActive {
		return nil, fmt.Errorf("instance %q is not active (state: %s)", name, inst.State)
	}
	if err := c.Provisioner.StopInstance(ctx, inst.ProviderRef); err != nil {
		return nil, err
	}
	_, err = c.DB.Exec(ctx, `
		UPDATE db_instances SET state = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		inst.ID, StateStopped)
	if err != nil {
		return nil, err
	}
	return c.GetInstance(ctx, accountID, projectName, name)
}

// CreateSnapshot takes a point-in-time copy of a managed instance.
func (c *Clio) CreateSnapshot(ctx context.Context, accountID, projectName, instanceName, snapshotName string) (*database.DBSnapshot, error) {
	inst, err := c.GetInstance(ctx, accountID, projectName, instanceName)
	if err != nil {
		return nil, err
	}
	if inst.State != StateActive && inst.State != StateStopped {
		return nil, fmt.Errorf("instance %q is not snapshot-able (state: %s)", instanceName, inst.State)
	}
	if snapshotName == "" {
		return nil, errors.New("snapshot name is required")
	}

	provisioned, err := c.Provisioner.CreateSnapshot(ctx, SnapshotSpec{
		InstanceRef: inst.ProviderRef,
		ProjectID:   inst.ProjectID,
		Name:        snapshotName,
		Engine:      inst.Engine,
		Size:        inst.AllocatedStorageGB,
	})
	if err != nil {
		return nil, fmt.Errorf("provisioner create snapshot failed: %w", err)
	}

	snap := &database.DBSnapshot{
		ID:         newID(),
		ProjectID:  inst.ProjectID,
		InstanceID: inst.ID,
		Name:       snapshotName,
		SizeGB:     provisioned.SizeGB,
		State:      StateActive,
		Status:     "active",
	}
	if _, err := c.DB.Exec(ctx, `
		INSERT INTO db_snapshots (id, project_id, instance_id, name, size_gb, state, provider_ref, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		snap.ID, inst.ProjectID, inst.ID, snapshotName, provisioned.SizeGB, StateActive, provisioned.ProviderRef, "active"); err != nil {
		return nil, err
	}
	return scanSnapshot(c.DB.QueryRow(ctx, `
		SELECT id, project_id, instance_id, name, size_gb, state,
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM db_snapshots WHERE id = $1`, snap.ID))
}

// ListSnapshots returns all snapshots for a managed instance.
func (c *Clio) ListSnapshots(ctx context.Context, accountID, projectName, instanceName string) ([]database.DBSnapshot, error) {
	inst, err := c.GetInstance(ctx, accountID, projectName, instanceName)
	if err != nil {
		return nil, err
	}
	rows, err := c.DB.Query(ctx, `
		SELECT s.id, s.project_id, s.instance_id, s.name, s.size_gb, s.state,
		       COALESCE(s.provider_ref,''), s.created_at, s.updated_at, s.status
		FROM db_snapshots s WHERE s.instance_id = $1 ORDER BY s.created_at`, inst.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.DBSnapshot
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *snapshot)
	}
	return out, rows.Err()
}

// DeleteSnapshot removes a stored instance snapshot.
func (c *Clio) DeleteSnapshot(ctx context.Context, accountID, projectName, instanceName, snapshotName string) error {
	inst, err := c.GetInstance(ctx, accountID, projectName, instanceName)
	if err != nil {
		return err
	}
	snap, err := scanSnapshot(c.DB.QueryRow(ctx, `
		SELECT id, project_id, instance_id, name, size_gb, state,
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM db_snapshots WHERE instance_id = $1 AND name = $2`,
		inst.ID, snapshotName))
	if err != nil {
		return err
	}
	if snap.State == StateDeleted {
		return ErrConflict
	}
	if err := c.Provisioner.DeleteSnapshot(ctx, snap.ProviderRef); err != nil {
		return err
	}
	_, err = c.DB.Exec(ctx, `
		UPDATE db_snapshots SET state = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		snap.ID, StateDeleted)
	return err
}

// Audit records an operation in the audit trail.
func (c *Clio) Audit(ctx context.Context, accountID, projectID, entity, operation, status string) error {
	_, err := c.DB.Exec(ctx, `
		INSERT INTO audit_logs (account_id, project_id, entity, operation, status)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5)`,
		accountID, projectID, entity, operation, status)
	return err
}

func (c *Clio) refreshCounts(ctx context.Context, projectID string) {
	_, _ = c.DB.Exec(ctx,
		`UPDATE projects SET instance_count = (SELECT COUNT(*) FROM db_instances WHERE project_id = $1 AND state <> 'deleted') WHERE id = $1`,
		projectID)
}

// rowScanner abstracts *sql.Row and *sql.Rows for shared scanning.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanInstance(s rowScanner) (*database.DBInstance, error) {
	var i database.DBInstance
	err := s.Scan(
		&i.ID, &i.ProjectID, &i.Name, &i.Engine, &i.EngineVersion, &i.Size, &i.AllocatedStorageGB, &i.State,
		&i.Endpoint, &i.MasterUsername, &i.ProviderRef,
		&i.CreatedAt, &i.UpdatedAt, &i.Status)
	if IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func scanSnapshot(s rowScanner) (*database.DBSnapshot, error) {
	var snap database.DBSnapshot
	err := s.Scan(
		&snap.ID, &snap.ProjectID, &snap.InstanceID, &snap.Name, &snap.SizeGB, &snap.State,
		&snap.ProviderRef, &snap.CreatedAt, &snap.UpdatedAt, &snap.Status)
	if IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

// sanitize returns a DNS-safe lowercase identifier derived from s.
func sanitize(s string) string {
	var b []byte
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b = append(b, byte(r))
		case r >= '0' && r <= '9':
			b = append(b, byte(r))
		case r == '-':
			b = append(b, '-')
		default:
			if len(b) > 0 {
				b = append(b, '-')
			}
		}
	}
	return string(b)
}
