// Package pkg contains the Orpheus business-logic layer: multi-tenant
// managed-Kubernetes control plane with pluggable provisioning, cluster
// lifecycle, node groups and audit trails.
package pkg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/mathif92/olympus/orpheus/pkg/database"
)

// Audit operations recorded in the audit_logs table.
const (
	OpCreate     = "create"
	OpList       = "list"
	OpDelete     = "delete"
	OpScale      = "scale"
	OpKubeconfig = "kubeconfig"
)

// Orpheus is the managed-Kubernetes control-plane orchestrator.
type Orpheus struct {
	DB          *database.Client
	Provisioner Provisioner
}

// NewOrpheus wires the control plane to Postgres and a provisioner.
func NewOrpheus(db *database.Client, provisioner Provisioner) *Orpheus {
	return &Orpheus{DB: db, Provisioner: provisioner}
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// EnsureAccount upserts the tenant so requests referencing it always resolve.
func (o *Orpheus) EnsureAccount(ctx context.Context, a database.Account) error {
	_, err := o.DB.Exec(ctx, `
		INSERT INTO accounts (id, display_name, email, plan, cluster_limit)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET display_name = EXCLUDED.display_name`,
		a.ID, a.DisplayName, a.Email, a.Plan, a.ClusterLimit)
	return err
}

// CreateProject provisions a project namespace inside the account.
func (o *Orpheus) CreateProject(ctx context.Context, accountID string, p database.Project) error {
	if p.Name == "" {
		return errors.New("project name is required")
	}
	p.ID = newID()
	_, err := o.DB.Exec(ctx, `
		INSERT INTO projects (id, account_id, name, description)
		VALUES ($1, $2, $3, $4)`,
		p.ID, accountID, p.Name, p.Description)
	return err
}

// ListProjects returns all projects for an account.
func (o *Orpheus) ListProjects(ctx context.Context, accountID string) ([]database.Project, error) {
	rows, err := o.DB.Query(ctx, `
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

func (o *Orpheus) resolveProject(ctx context.Context, accountID, projectName string) (*database.Project, error) {
	var p database.Project
	err := o.DB.QueryRow(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), cluster_count, created_at, status
		FROM projects WHERE account_id = $1 AND name = $2 AND status = 'active'`,
		accountID, projectName).Scan(
		&p.ID, &p.AccountID, &p.Name, &p.Description, &p.ClusterCount, &p.CreatedAt, &p.Status)
	if IsNotFound(err) {
		return nil, ErrNotFound
	}
	return &p, err
}

// ListKubernetesVersions returns the catalog of supported control-plane versions.
func (o *Orpheus) ListKubernetesVersions(ctx context.Context) ([]database.KubernetesVersion, error) {
	rows, err := o.DB.Query(ctx, `
		SELECT version, channel, status
		FROM kubernetes_versions WHERE status = 'active' ORDER BY version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.KubernetesVersion
	for rows.Next() {
		var v database.KubernetesVersion
		if err := rows.Scan(&v.Version, &v.Channel, &v.Status); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListNodeSizes returns the catalog of available worker node sizes.
func (o *Orpheus) ListNodeSizes(ctx context.Context) ([]database.NodeSize, error) {
	rows, err := o.DB.Query(ctx, `
		SELECT name, vcpus, memory_gb, price_per_hour_cents, status
		FROM node_sizes WHERE status = 'active' ORDER BY memory_gb`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.NodeSize
	for rows.Next() {
		var t database.NodeSize
		if err := rows.Scan(&t.Name, &t.VCPUs, &t.MemoryGB, &t.PricePerHourCents, &t.Status); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateCluster provisions a Kubernetes control plane via the pluggable
// provisioner and records it in Postgres.
func (o *Orpheus) CreateCluster(ctx context.Context, accountID, projectName string, spec ClusterSpec) (*database.Cluster, error) {
	proj, err := o.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	if spec.Name == "" || spec.KubernetesVersion == "" {
		return nil, errors.New("cluster name and kubernetes_version are required")
	}

	var one int
	err = o.DB.QueryRow(ctx,
		`SELECT 1 FROM kubernetes_versions WHERE version = $1 AND status = 'active'`,
		spec.KubernetesVersion).Scan(&one)
	if IsNotFound(err) {
		return nil, fmt.Errorf("unsupported kubernetes version %q", spec.KubernetesVersion)
	}
	if err != nil {
		return nil, err
	}

	spec.ID = newID()
	spec.ProjectID = proj.ID
	if spec.Region == "" {
		spec.Region = "eu-west-1"
	}

	cluster := &database.Cluster{
		ID:                spec.ID,
		ProjectID:         proj.ID,
		Name:              spec.Name,
		KubernetesVersion: spec.KubernetesVersion,
		Region:            spec.Region,
		State:             StateCreating,
		Status:            "active",
	}
	if _, err := o.DB.Exec(ctx, `
		INSERT INTO clusters (id, project_id, name, kubernetes_version, region, state, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		cluster.ID, proj.ID, spec.Name, spec.KubernetesVersion, spec.Region, StateCreating, "active"); err != nil {
		return nil, err
	}

	// Delegate the actual control-plane provisioning to the pluggable backend.
	provisioned, err := o.Provisioner.CreateCluster(ctx, spec)
	if err != nil {
		_, _ = o.DB.Exec(ctx,
			`UPDATE clusters SET state = 'failed', updated_at = CURRENT_TIMESTAMP WHERE id = $1`, cluster.ID)
		return nil, fmt.Errorf("provisioner create cluster failed: %w", err)
	}

	kubeconfig, err := renderKubeconfig(spec.Name, provisioned.Endpoint, provisioned.CAData)
	if err != nil {
		kubeconfig = ""
	}

	_, err = o.DB.Exec(ctx, `
		UPDATE clusters
		SET state = $2, endpoint = $3, ca_cert = $4, kubeconfig = $5, provider_ref = $6, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
		cluster.ID, StateActive, provisioned.Endpoint, provisioned.CAData, kubeconfig, provisioned.ProviderRef)
	if err != nil {
		return nil, err
	}
	o.refreshCounts(ctx, proj.ID)

	full, err := scanCluster(o.DB.QueryRow(ctx, `
		SELECT id, project_id, name, kubernetes_version, region, state,
		       COALESCE(endpoint,''), COALESCE(ca_cert,''), COALESCE(kubeconfig,''),
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM clusters WHERE id = $1`, cluster.ID))
	if err != nil {
		return nil, err
	}
	return full, nil
}

// GetCluster returns a single cluster within a project.
func (o *Orpheus) GetCluster(ctx context.Context, accountID, projectName, name string) (*database.Cluster, error) {
	proj, err := o.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	return scanCluster(o.DB.QueryRow(ctx, `
		SELECT id, project_id, name, kubernetes_version, region, state,
		       COALESCE(endpoint,''), COALESCE(ca_cert,''), COALESCE(kubeconfig,''),
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM clusters WHERE project_id = $1 AND name = $2`,
		proj.ID, name))
}

// ListClusters returns all clusters in a project.
func (o *Orpheus) ListClusters(ctx context.Context, accountID, projectName string) ([]database.Cluster, error) {
	proj, err := o.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := o.DB.Query(ctx, `
		SELECT id, project_id, name, kubernetes_version, region, state,
		       COALESCE(endpoint,''), COALESCE(ca_cert,''), COALESCE(kubeconfig,''),
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM clusters WHERE project_id = $1 ORDER BY created_at`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Cluster
	for rows.Next() {
		c, err := scanCluster(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// DeleteCluster tears a cluster down for good.
func (o *Orpheus) DeleteCluster(ctx context.Context, accountID, projectName, name string) error {
	proj, err := o.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	cluster, err := scanCluster(o.DB.QueryRow(ctx, `
		SELECT id, project_id, name, kubernetes_version, region, state,
		       COALESCE(endpoint,''), COALESCE(ca_cert,''), COALESCE(kubeconfig,''),
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM clusters WHERE project_id = $1 AND name = $2`,
		proj.ID, name))
	if err != nil {
		return err
	}
	if cluster.State == StateDeleted {
		return ErrConflict
	}

	if err := o.Provisioner.DeleteCluster(ctx, cluster.ProviderRef); err != nil {
		return err
	}
	_, err = o.DB.Exec(ctx, `
		UPDATE clusters SET state = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		cluster.ID, StateDeleted)
	if err != nil {
		return err
	}
	o.refreshCounts(ctx, proj.ID)
	return nil
}

// ClusterKubeconfig returns the rendered kubeconfig for a cluster.
func (o *Orpheus) ClusterKubeconfig(ctx context.Context, accountID, projectName, name string) (string, error) {
	cluster, err := o.GetCluster(ctx, accountID, projectName, name)
	if err != nil {
		return "", err
	}
	return cluster.Kubeconfig, nil
}

// CreateNodeGroup attaches a worker node group to a cluster.
func (o *Orpheus) CreateNodeGroup(ctx context.Context, accountID, projectName, clusterName string, spec NodeGroupSpec) (*database.NodeGroup, error) {
	cluster, err := o.GetCluster(ctx, accountID, projectName, clusterName)
	if err != nil {
		return nil, err
	}
	if cluster.State != StateActive {
		return nil, fmt.Errorf("cluster %q is not active (state: %s)", clusterName, cluster.State)
	}
	if spec.Name == "" || spec.NodeSize == "" {
		return nil, errors.New("node group name and node_size are required")
	}
	if spec.DesiredSize <= 0 {
		spec.DesiredSize = 1
	}
	if spec.MinSize <= 0 {
		spec.MinSize = 1
	}
	if spec.MaxSize < spec.DesiredSize {
		spec.MaxSize = spec.DesiredSize
	}

	spec.ClusterRef = cluster.ProviderRef

	ng := &database.NodeGroup{
		ID:          newID(),
		ClusterID:   cluster.ID,
		Name:        spec.Name,
		NodeSize:    spec.NodeSize,
		MinSize:     spec.MinSize,
		DesiredSize: spec.DesiredSize,
		MaxSize:     spec.MaxSize,
		State:       StateCreating,
		Status:      "active",
	}
	if _, err := o.DB.Exec(ctx, `
		INSERT INTO node_groups (id, cluster_id, name, node_size, min_size, desired_size, max_size, state, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		ng.ID, cluster.ID, spec.Name, spec.NodeSize, spec.MinSize, spec.DesiredSize, spec.MaxSize, StateCreating, "active"); err != nil {
		return nil, err
	}

	provisioned, err := o.Provisioner.CreateNodeGroup(ctx, spec)
	if err != nil {
		_, _ = o.DB.Exec(ctx,
			`UPDATE node_groups SET state = 'failed', updated_at = CURRENT_TIMESTAMP WHERE id = $1`, ng.ID)
		return nil, fmt.Errorf("provisioner create node group failed: %w", err)
	}

	_, err = o.DB.Exec(ctx, `
		UPDATE node_groups SET state = $2, provider_ref = $3, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		ng.ID, StateActive, provisioned.ProviderRef)
	if err != nil {
		return nil, err
	}
	return o.GetNodeGroup(ctx, accountID, projectName, clusterName, spec.Name)
}

// GetNodeGroup returns a single node group within a cluster.
func (o *Orpheus) GetNodeGroup(ctx context.Context, accountID, projectName, clusterName, name string) (*database.NodeGroup, error) {
	cluster, err := o.GetCluster(ctx, accountID, projectName, clusterName)
	if err != nil {
		return nil, err
	}
	return scanNodeGroup(o.DB.QueryRow(ctx, `
		SELECT id, cluster_id, name, node_size, min_size, desired_size, max_size, state,
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM node_groups WHERE cluster_id = $1 AND name = $2`,
		cluster.ID, name))
}

// ListNodeGroups returns all node groups attached to a cluster.
func (o *Orpheus) ListNodeGroups(ctx context.Context, accountID, projectName, clusterName string) ([]database.NodeGroup, error) {
	cluster, err := o.GetCluster(ctx, accountID, projectName, clusterName)
	if err != nil {
		return nil, err
	}
	rows, err := o.DB.Query(ctx, `
		SELECT id, cluster_id, name, node_size, min_size, desired_size, max_size, state,
		       COALESCE(provider_ref,''), created_at, updated_at, status
		FROM node_groups WHERE cluster_id = $1 ORDER BY created_at`, cluster.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.NodeGroup
	for rows.Next() {
		ng, err := scanNodeGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ng)
	}
	return out, rows.Err()
}

// ScaleNodeGroup resizes a node group, validating against its min/max bounds.
func (o *Orpheus) ScaleNodeGroup(ctx context.Context, accountID, projectName, clusterName, name string, desired int) (*database.NodeGroup, error) {
	ng, err := o.GetNodeGroup(ctx, accountID, projectName, clusterName, name)
	if err != nil {
		return nil, err
	}
	if ng.State != StateActive {
		return nil, fmt.Errorf("node group %q is not active (state: %s)", name, ng.State)
	}
	if desired < ng.MinSize {
		return nil, fmt.Errorf("desired %d below min %d", desired, ng.MinSize)
	}
	if desired > ng.MaxSize {
		return nil, fmt.Errorf("desired %d above max %d", desired, ng.MaxSize)
	}

	spec := NodeGroupSpec{
		ClusterRef:  ng.ProviderRef,
		Name:        ng.Name,
		NodeSize:    ng.NodeSize,
		MinSize:     ng.MinSize,
		DesiredSize: desired,
		MaxSize:     ng.MaxSize,
	}
	if err := o.Provisioner.ScaleNodeGroup(ctx, spec); err != nil {
		return nil, err
	}
	_, err = o.DB.Exec(ctx, `
		UPDATE node_groups SET desired_size = $2, state = $3, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		ng.ID, desired, StateActive)
	if err != nil {
		return nil, err
	}
	return o.GetNodeGroup(ctx, accountID, projectName, clusterName, name)
}

// DeleteNodeGroup removes a node group from its cluster.
func (o *Orpheus) DeleteNodeGroup(ctx context.Context, accountID, projectName, clusterName, name string) error {
	ng, err := o.GetNodeGroup(ctx, accountID, projectName, clusterName, name)
	if err != nil {
		return err
	}
	if ng.State == StateDeleted {
		return ErrConflict
	}
	if err := o.Provisioner.DeleteNodeGroup(ctx, NodeGroupSpec{ClusterRef: ng.ProviderRef}); err != nil {
		return err
	}
	_, err = o.DB.Exec(ctx, `
		UPDATE node_groups SET state = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		ng.ID, StateDeleted)
	return err
}

// Audit records an operation in the audit trail.
func (o *Orpheus) Audit(ctx context.Context, accountID, projectID, entity, operation, status string) error {
	_, err := o.DB.Exec(ctx, `
		INSERT INTO audit_logs (account_id, project_id, entity, operation, status)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5)`,
		accountID, projectID, entity, operation, status)
	return err
}

func (o *Orpheus) refreshCounts(ctx context.Context, projectID string) {
	_, _ = o.DB.Exec(ctx,
		`UPDATE projects SET cluster_count = (SELECT COUNT(*) FROM clusters WHERE project_id = $1 AND state <> 'deleted') WHERE id = $1`,
		projectID)
}

// rowScanner abstracts *sql.Row and *sql.Rows for shared scanning.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanCluster(s rowScanner) (*database.Cluster, error) {
	var c database.Cluster
	err := s.Scan(
		&c.ID, &c.ProjectID, &c.Name, &c.KubernetesVersion, &c.Region, &c.State,
		&c.Endpoint, &c.CAData, &c.Kubeconfig, &c.ProviderRef,
		&c.CreatedAt, &c.UpdatedAt, &c.Status)
	if IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func scanNodeGroup(s rowScanner) (*database.NodeGroup, error) {
	var ng database.NodeGroup
	err := s.Scan(
		&ng.ID, &ng.ClusterID, &ng.Name, &ng.NodeSize,
		&ng.MinSize, &ng.DesiredSize, &ng.MaxSize, &ng.State,
		&ng.ProviderRef, &ng.CreatedAt, &ng.UpdatedAt, &ng.Status)
	if IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ng, nil
}
