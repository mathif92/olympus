// Package pkg contains the Mneme business-logic layer: multi-tenant managed
// in-memory cache control plane with pluggable provisioning, cluster
// lifecycle, point-in-time snapshots and audit trails.
package pkg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/mathif92/olympus/mneme/pkg/database"
)

// Audit operations recorded in the audit_logs table.
const (
	OpCreate   = "create"
	OpList     = "list"
	OpDelete   = "delete"
	OpSnapshot = "snapshot"
)

// Mneme is the managed-cache control-plane orchestrator.
type Mneme struct {
	DB          *database.Client
	Provisioner Provisioner
}

// NewMneme wires the control plane to Postgres and a provisioner.
func NewMneme(db *database.Client, provisioner Provisioner) *Mneme {
	return &Mneme{DB: db, Provisioner: provisioner}
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// EnsureAccount upserts the tenant so requests referencing it always resolve.
func (m *Mneme) EnsureAccount(ctx context.Context, a database.Account) error {
	_, err := m.DB.Exec(ctx, `
		INSERT INTO accounts (id, display_name, email, plan, cluster_limit)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET display_name = EXCLUDED.display_name`,
		a.ID, a.DisplayName, a.Email, a.Plan, a.ClusterLimit)
	return err
}

// CreateProject provisions a project namespace inside the account.
func (m *Mneme) CreateProject(ctx context.Context, accountID string, p database.Project) error {
	if p.Name == "" {
		return errors.New("project name is required")
	}
	p.ID = newID()
	_, err := m.DB.Exec(ctx, `
		INSERT INTO projects (id, account_id, name, description)
		VALUES ($1, $2, $3, $4)`,
		p.ID, accountID, p.Name, p.Description)
	return err
}

// ListProjects returns all projects for an account.
func (m *Mneme) ListProjects(ctx context.Context, accountID string) ([]database.Project, error) {
	rows, err := m.DB.Query(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), cluster_count, created_at, status
		FROM projects WHERE account_id = $1 ORDER BY name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Project
	for rows.Next() {
		var p database.Project
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &p.Description, &p.ClusterCount, &p.CreatedAt, &p.Status); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (m *Mneme) resolveProject(ctx context.Context, accountID, projectName string) (*database.Project, error) {
	var p database.Project
	err := m.DB.QueryRow(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), cluster_count, created_at, status
		FROM projects WHERE account_id = $1 AND name = $2 AND status = 'active'`,
		accountID, projectName).Scan(
		&p.ID, &p.AccountID, &p.Name, &p.Description, &p.ClusterCount, &p.CreatedAt, &p.Status)
	if IsNotFound(err) {
		return nil, ErrNotFound
	}
	return &p, err
}

// ListEngines returns the catalog of supported cache engines.
func (m *Mneme) ListEngines(ctx context.Context) ([]database.CacheEngine, error) {
	rows, err := m.DB.Query(ctx, `
		SELECT engine, version, status
		FROM cache_engines WHERE status = 'active' ORDER BY engine, version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.CacheEngine
	for rows.Next() {
		var e database.CacheEngine
		if err := rows.Scan(&e.Engine, &e.Version, &e.Status); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListNodeTypes returns the catalog of available cache node types.
func (m *Mneme) ListNodeTypes(ctx context.Context) ([]database.NodeType, error) {
	rows, err := m.DB.Query(ctx, `
		SELECT name, vcpus, memory_gb, price_per_hour_cents, status
		FROM node_types WHERE status = 'active' ORDER BY memory_gb`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.NodeType
	for rows.Next() {
		var t database.NodeType
		if err := rows.Scan(&t.Name, &t.VCPUs, &t.MemoryGB, &t.PricePerHourCents, &t.Status); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateCluster provisions a managed cache cluster via the pluggable
// provisioner and records it in Postgres.
func (m *Mneme) CreateCluster(ctx context.Context, accountID, projectName string, spec ClusterSpec) (*database.CacheCluster, error) {
	proj, err := m.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	if spec.Name == "" || spec.Engine == "" || spec.EngineVersion == "" {
		return nil, errors.New("cluster name, engine and engine_version are required")
	}

	var one int
	err = m.DB.QueryRow(ctx,
		`SELECT 1 FROM cache_engines WHERE engine = $1 AND version = $2 AND status = 'active'`,
		spec.Engine, spec.EngineVersion).Scan(&one)
	if IsNotFound(err) {
		return nil, fmt.Errorf("unsupported engine %q %q", spec.Engine, spec.EngineVersion)
	}
	if err != nil {
		return nil, err
	}

	err = m.DB.QueryRow(ctx,
		`SELECT 1 FROM node_types WHERE name = $1 AND status = 'active'`, spec.NodeType).Scan(&one)
	if IsNotFound(err) {
		return nil, fmt.Errorf("unsupported node type %q", spec.NodeType)
	}
	if err != nil {
		return nil, err
	}

	spec.ID = newID()
	spec.ProjectID = proj.ID
	if spec.NumNodes <= 0 {
		spec.NumNodes = 1
	}

	cluster := &database.CacheCluster{
		ID:            spec.ID,
		ProjectID:     proj.ID,
		Name:          spec.Name,
		Engine:        spec.Engine,
		EngineVersion: spec.EngineVersion,
		NodeType:      spec.NodeType,
		NumNodes:      spec.NumNodes,
		State:         StateCreating,
		Status:        "active",
	}
	if _, err := m.DB.Exec(ctx, `
		INSERT INTO cache_clusters (id, project_id, name, engine, engine_version, node_type, num_nodes, state, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		cluster.ID, proj.ID, spec.Name, spec.Engine, spec.EngineVersion, spec.NodeType, spec.NumNodes, StateCreating, "active"); err != nil {
		return nil, err
	}

	// Delegate the actual cache provisioning to the pluggable backend.
	provisioned, err := m.Provisioner.CreateCluster(ctx, spec)
	if err != nil {
		_, _ = m.DB.Exec(ctx,
			`UPDATE cache_clusters SET state = 'failed', updated_at = CURRENT_TIMESTAMP WHERE id = $1`, cluster.ID)
		return nil, fmt.Errorf("provisioner create cluster failed: %w", err)
	}

	_, err = m.DB.Exec(ctx, `
		UPDATE cache_clusters
		SET state = $2, endpoint = $3, provider_ref = $4, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
		cluster.ID, StateActive, provisioned.Endpoint, provisioned.ProviderRef)
	if err != nil {
		return nil, err
	}
	m.refreshCounts(ctx, proj.ID)

	full, err := scanCluster(m.DB.QueryRow(ctx, `
		SELECT id, project_id, name, engine, engine_version, node_type, num_nodes, state,
		       COALESCE(endpoint,''), COALESCE(provider_ref,''), created_at, updated_at, status
		FROM cache_clusters WHERE id = $1`, cluster.ID))
	if err != nil {
		return nil, err
	}
	return full, nil
}

// GetCluster returns a single managed cluster within a project.
func (m *Mneme) GetCluster(ctx context.Context, accountID, projectName, name string) (*database.CacheCluster, error) {
	proj, err := m.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	return scanCluster(m.DB.QueryRow(ctx, `
		SELECT id, project_id, name, engine, engine_version, node_type, num_nodes, state,
		       COALESCE(endpoint,''), COALESCE(provider_ref,''), created_at, updated_at, status
		FROM cache_clusters WHERE project_id = $1 AND name = $2`,
		proj.ID, name))
}

// ListClusters returns all managed clusters in a project.
func (m *Mneme) ListClusters(ctx context.Context, accountID, projectName string) ([]database.CacheCluster, error) {
	proj, err := m.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := m.DB.Query(ctx, `
		SELECT id, project_id, name, engine, engine_version, node_type, num_nodes, state,
		       COALESCE(endpoint,''), COALESCE(provider_ref,''), created_at, updated_at, status
		FROM cache_clusters WHERE project_id = $1 ORDER BY created_at`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.CacheCluster
	for rows.Next() {
		cl, err := scanCluster(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *cl)
	}
	return out, rows.Err()
}

// DeleteCluster tears a managed cluster down for good.
func (m *Mneme) DeleteCluster(ctx context.Context, accountID, projectName, name string) error {
	proj, err := m.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	cl, err := scanCluster(m.DB.QueryRow(ctx, `
		SELECT id, project_id, name, engine, engine_version, node_type, num_nodes, state,
		       COALESCE(endpoint,''), COALESCE(provider_ref,''), created_at, updated_at, status
		FROM cache_clusters WHERE project_id = $1 AND name = $2`,
		proj.ID, name))
	if err != nil {
		return err
	}
	if cl.State == StateDeleted {
		return ErrConflict
	}

	if err := m.Provisioner.DeleteCluster(ctx, cl.ProviderRef); err != nil {
		return err
	}
	_, err = m.DB.Exec(ctx, `
		UPDATE cache_clusters SET state = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		cl.ID, StateDeleted)
	if err != nil {
		return err
	}
	m.refreshCounts(ctx, proj.ID)
	return nil
}

// CreateSnapshot takes a point-in-time copy of a managed cluster.
func (m *Mneme) CreateSnapshot(ctx context.Context, accountID, projectName, clusterName, snapshotName string) (*database.CacheSnapshot, error) {
	cl, err := m.GetCluster(ctx, accountID, projectName, clusterName)
	if err != nil {
		return nil, err
	}
	if cl.State != StateActive {
		return nil, fmt.Errorf("cluster %q is not snapshot-able (state: %s)", clusterName, cl.State)
	}
	if snapshotName == "" {
		return nil, errors.New("snapshot name is required")
	}

	provisioned, err := m.Provisioner.CreateSnapshot(ctx, SnapshotSpec{
		ClusterRef: cl.ProviderRef,
		Name:       snapshotName,
		Size:       0,
	})
	if err != nil {
		return nil, fmt.Errorf("provisioner create snapshot failed: %w", err)
	}

	snap := &database.CacheSnapshot{
		ID:        newID(),
		ProjectID: cl.ProjectID,
		ClusterID: cl.ID,
		Name:      snapshotName,
		SizeMB:    provisioned.SizeMB,
		State:     StateActive,
		Status:    "active",
	}
	if _, err := m.DB.Exec(ctx, `
		INSERT INTO cache_snapshots (id, project_id, cluster_id, name, size_mb, state, provider_ref, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		snap.ID, cl.ProjectID, cl.ID, snapshotName, provisioned.SizeMB, StateActive, provisioned.ProviderRef, "active"); err != nil {
		return nil, err
	}
	return scanSnapshot(m.DB.QueryRow(ctx, `
		SELECT id, project_id, cluster_id, name, size_mb, state,
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM cache_snapshots WHERE id = $1`, snap.ID))
}

// ListSnapshots returns all snapshots for a managed cluster.
func (m *Mneme) ListSnapshots(ctx context.Context, accountID, projectName, clusterName string) ([]database.CacheSnapshot, error) {
	cl, err := m.GetCluster(ctx, accountID, projectName, clusterName)
	if err != nil {
		return nil, err
	}
	rows, err := m.DB.Query(ctx, `
		SELECT s.id, s.project_id, s.cluster_id, s.name, s.size_mb, s.state,
		       COALESCE(s.provider_ref,''), s.created_at, s.updated_at, s.status
		FROM cache_snapshots s WHERE s.cluster_id = $1 ORDER BY s.created_at`, cl.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.CacheSnapshot
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *snapshot)
	}
	return out, rows.Err()
}

// DeleteSnapshot removes a stored cluster snapshot.
func (m *Mneme) DeleteSnapshot(ctx context.Context, accountID, projectName, clusterName, snapshotName string) error {
	cl, err := m.GetCluster(ctx, accountID, projectName, clusterName)
	if err != nil {
		return err
	}
	snap, err := scanSnapshot(m.DB.QueryRow(ctx, `
		SELECT id, project_id, cluster_id, name, size_mb, state,
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM cache_snapshots WHERE cluster_id = $1 AND name = $2`,
		cl.ID, snapshotName))
	if err != nil {
		return err
	}
	if snap.State == StateDeleted {
		return ErrConflict
	}
	if err := m.Provisioner.DeleteSnapshot(ctx, snap.ProviderRef); err != nil {
		return err
	}
	_, err = m.DB.Exec(ctx, `
		UPDATE cache_snapshots SET state = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		snap.ID, StateDeleted)
	return err
}

// Audit records an operation in the audit trail.
func (m *Mneme) Audit(ctx context.Context, accountID, projectID, entity, operation, status string) error {
	_, err := m.DB.Exec(ctx, `
		INSERT INTO audit_logs (account_id, project_id, entity, operation, status)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5)`,
		accountID, projectID, entity, operation, status)
	return err
}

func (m *Mneme) refreshCounts(ctx context.Context, projectID string) {
	_, _ = m.DB.Exec(ctx,
		`UPDATE projects SET cluster_count = (SELECT COUNT(*) FROM cache_clusters WHERE project_id = $1 AND state <> 'deleted') WHERE id = $1`,
		projectID)
}

// rowScanner abstracts *sql.Row and *sql.Rows for shared scanning.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanCluster(s rowScanner) (*database.CacheCluster, error) {
	var c database.CacheCluster
	err := s.Scan(
		&c.ID, &c.ProjectID, &c.Name, &c.Engine, &c.EngineVersion, &c.NodeType, &c.NumNodes, &c.State,
		&c.Endpoint, &c.ProviderRef, &c.CreatedAt, &c.UpdatedAt, &c.Status)
	if IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func scanSnapshot(s rowScanner) (*database.CacheSnapshot, error) {
	var snap database.CacheSnapshot
	err := s.Scan(
		&snap.ID, &snap.ProjectID, &snap.ClusterID, &snap.Name, &snap.SizeMB, &snap.State,
		&snap.ProviderRef, &snap.CreatedAt, &snap.UpdatedAt, &snap.Status)
	if IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &snap, nil
}
