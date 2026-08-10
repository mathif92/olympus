// Package pkg contains the Hephaestus business-logic layer: multi-tenant
// compute control-plane, pluggable provisioning, volumes, snapshots, key
// pairs and security groups.
package pkg

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/mathif92/olympus/hephaestus/pkg/database"
)

// Audit operations recorded in the audit_logs table.
const (
	OpLaunch    = "launch"
	OpStart     = "start"
	OpStop      = "stop"
	OpTerminate = "terminate"
	OpCreate    = "create"
	OpList      = "list"
	OpDelete    = "delete"
)

// Hephaestus is the compute control-plane orchestrator.
type Hephaestus struct {
	DB          *database.Client
	Provisioner Provisioner
}

// NewHephaestus wires the control plane to Postgres and a provisioner.
func NewHephaestus(db *database.Client, provisioner Provisioner) *Hephaestus {
	return &Hephaestus{DB: db, Provisioner: provisioner}
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// EnsureAccount upserts the tenant so requests referencing it always resolve.
func (h *Hephaestus) EnsureAccount(ctx context.Context, a database.Account) error {
	_, err := h.DB.Exec(ctx, `
		INSERT INTO accounts (id, display_name, email, plan, instance_limit)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET display_name = EXCLUDED.display_name`,
		a.ID, a.DisplayName, a.Email, a.Plan, a.InstanceLimit)
	return err
}

// CreateProject provisions a project namespace inside the account.
func (h *Hephaestus) CreateProject(ctx context.Context, accountID string, p database.Project) error {
	if p.Name == "" {
		return errors.New("project name is required")
	}
	p.ID = newID()
	_, err := h.DB.Exec(ctx, `
		INSERT INTO projects (id, account_id, name, description)
		VALUES ($1, $2, $3, $4)`,
		p.ID, accountID, p.Name, p.Description)
	return err
}

// ListProjects returns all projects for an account.
func (h *Hephaestus) ListProjects(ctx context.Context, accountID string) ([]database.Project, error) {
	rows, err := h.DB.Query(ctx, `
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

func (h *Hephaestus) resolveProject(ctx context.Context, accountID, projectName string) (*database.Project, error) {
	var p database.Project
	err := h.DB.QueryRow(ctx, `
		SELECT id, account_id, name, COALESCE(description,''), instance_count, created_at, status
		FROM projects WHERE account_id = $1 AND name = $2 AND status = 'active'`,
		accountID, projectName).Scan(
		&p.ID, &p.AccountID, &p.Name, &p.Description, &p.InstanceCount, &p.CreatedAt, &p.Status)
	if IsNotFound(err) {
		return nil, ErrNotFound
	}
	return &p, err
}

// ListInstanceTypes returns the catalog of available instance sizes.
func (h *Hephaestus) ListInstanceTypes(ctx context.Context) ([]database.InstanceType, error) {
	rows, err := h.DB.Query(ctx, `
		SELECT name, vcpus, memory_gb, storage_gb, price_per_hour_cents, status
		FROM instance_types WHERE status = 'active' ORDER BY memory_gb`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.InstanceType
	for rows.Next() {
		var t database.InstanceType
		if err := rows.Scan(&t.Name, &t.VCPUs, &t.MemoryGB, &t.StorageGB, &t.PricePerHourCents, &t.Status); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// LaunchInstance boots a new instance and its boot volume via the provisioner.
func (h *Hephaestus) LaunchInstance(ctx context.Context, accountID, projectName string, spec InstanceSpec) (*database.Instance, error) {
	proj, err := h.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	if spec.Name == "" || spec.Type == "" {
		return nil, errors.New("instance name and type are required")
	}

	// Validate the requested type exists.
	var typeStorage int
	err = h.DB.QueryRow(ctx,
		`SELECT COALESCE(storage_gb,0) FROM instance_types WHERE name = $1`, spec.Type).Scan(&typeStorage)
	if IsNotFound(err) {
		return nil, fmt.Errorf("unknown instance type %q", spec.Type)
	}
	if err != nil {
		return nil, err
	}

	// Validate the key pair, when referenced, belongs to this project.
	if spec.KeyPairName != "" {
		var one int
		err = h.DB.QueryRow(ctx,
			`SELECT 1 FROM key_pairs WHERE project_id = $1 AND name = $2`,
			proj.ID, spec.KeyPairName).Scan(&one)
		if IsNotFound(err) {
			return nil, fmt.Errorf("unknown key pair %q in project", spec.KeyPairName)
		}
		if err != nil {
			return nil, err
		}
	}

	spec.ID = newID()
	spec.ProjectID = proj.ID
	if spec.ImageID == "" {
		spec.ImageID = "olympus-ami-linux-2"
	}
	if spec.VolumeSizeGB <= 0 {
		spec.VolumeSizeGB = typeStorage
	}

	inst := &database.Instance{
		ID:          spec.ID,
		ProjectID:   proj.ID,
		Name:        spec.Name,
		Type:        spec.Type,
		ImageID:     spec.ImageID,
		State:       StatePending,
		KeyPairName: spec.KeyPairName,
		LaunchedBy:  spec.LaunchedBy,
	}
	if _, err := h.DB.Exec(ctx, `
		INSERT INTO instances (id, project_id, name, instance_type, image_id, state, key_pair_name, launched_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		inst.ID, proj.ID, spec.Name, spec.Type, spec.ImageID, StatePending, spec.KeyPairName, spec.LaunchedBy); err != nil {
		return nil, err
	}

	// Boot volume, sized from the instance type.
	volID := newID()
	if _, err := h.DB.Exec(ctx, `
		INSERT INTO volumes (id, project_id, name, instance_id, size_gb, volume_type, state)
		VALUES ($1, $2, $3, $4, $5, 'gp2', 'in-use')`,
		volID, proj.ID, spec.Name+"-root", inst.ID, spec.VolumeSizeGB); err != nil {
		return nil, err
	}

	// Delegate the actual boot to the pluggable data plane.
	ref, privateIP, err := h.Provisioner.Launch(ctx, spec)
	if err != nil {
		_, _ = h.DB.Exec(ctx,
			`UPDATE instances SET state = 'pending', updated_at = CURRENT_TIMESTAMP WHERE id = $1`, inst.ID)
		return nil, fmt.Errorf("provisioner launch failed: %w", err)
	}

	now := time.Now()
	_, err = h.DB.Exec(ctx, `
		UPDATE instances
		SET state = $2, provider_ref = $3, private_ip = $4, launched_at = $5, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
		inst.ID, StateRunning, ref, privateIP, now)
	if err != nil {
		return nil, err
	}
	h.refreshCounts(ctx, proj.ID)

	full, err := scanInstance(h.DB.QueryRow(ctx, `
		SELECT id, project_id, name, instance_type, image_id, state,
		       COALESCE(private_ip,''), COALESCE(public_ip,''), COALESCE(key_pair_name,''),
		       COALESCE(provider_ref,''), COALESCE(launched_by,''), launched_at, terminated_at,
		       created_at, updated_at
		FROM instances WHERE id = $1`, inst.ID))
	if err != nil {
		return nil, err
	}
	return full, nil
}

// GetInstance returns a single instance within a project.
func (h *Hephaestus) GetInstance(ctx context.Context, accountID, projectName, name string) (*database.Instance, error) {
	proj, err := h.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	inst, err := scanInstance(h.DB.QueryRow(ctx, `
		SELECT id, project_id, name, instance_type, image_id, state,
		       COALESCE(private_ip,''), COALESCE(public_ip,''), COALESCE(key_pair_name,''),
		       COALESCE(provider_ref,''), COALESCE(launched_by,''), launched_at, terminated_at,
		       created_at, updated_at
		FROM instances WHERE project_id = $1 AND name = $2`,
		proj.ID, name))
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// ListInstances returns all instances in a project.
func (h *Hephaestus) ListInstances(ctx context.Context, accountID, projectName string) ([]database.Instance, error) {
	proj, err := h.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := h.DB.Query(ctx, `
		SELECT id, project_id, name, instance_type, image_id, state,
		       COALESCE(private_ip,''), COALESCE(public_ip,''), COALESCE(key_pair_name,''),
		       COALESCE(provider_ref,''), COALESCE(launched_by,''), launched_at, terminated_at,
		       created_at, updated_at
		FROM instances WHERE project_id = $1 ORDER BY created_at`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Instance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inst)
	}
	return out, rows.Err()
}

// StartInstance powers on an instance.
func (h *Hephaestus) StartInstance(ctx context.Context, accountID, projectName, name string) (*database.Instance, error) {
	inst, err := h.instanceFor(ctx, accountID, projectName, name)
	if err != nil {
		return nil, err
	}
	if inst.State == StateTerminated {
		return nil, ErrConflict
	}
	if err := h.Provisioner.Start(ctx, inst.ProviderRef); err != nil {
		return nil, err
	}
	_, err = h.DB.Exec(ctx,
		`UPDATE instances SET state = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		inst.ID, StateRunning)
	if err != nil {
		return nil, err
	}
	inst.State = StateRunning
	return inst, nil
}

// StopInstance powers off an instance.
func (h *Hephaestus) StopInstance(ctx context.Context, accountID, projectName, name string) (*database.Instance, error) {
	inst, err := h.instanceFor(ctx, accountID, projectName, name)
	if err != nil {
		return nil, err
	}
	if inst.State == StateTerminated {
		return nil, ErrConflict
	}
	if err := h.Provisioner.Stop(ctx, inst.ProviderRef); err != nil {
		return nil, err
	}
	_, err = h.DB.Exec(ctx,
		`UPDATE instances SET state = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		inst.ID, StateStopped)
	if err != nil {
		return nil, err
	}
	inst.State = StateStopped
	return inst, nil
}

// TerminateInstance destroys an instance for good.
func (h *Hephaestus) TerminateInstance(ctx context.Context, accountID, projectName, name string) error {
	inst, err := h.instanceFor(ctx, accountID, projectName, name)
	if err != nil {
		return err
	}
	if err := h.Provisioner.Terminate(ctx, inst.ProviderRef); err != nil {
		return err
	}
	now := time.Now()
	_, err = h.DB.Exec(ctx, `
		UPDATE instances SET state = $2, terminated_at = $3, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		inst.ID, StateTerminated, now)
	if err != nil {
		return err
	}
	h.refreshCounts(ctx, inst.ProjectID)
	return nil
}

func (h *Hephaestus) instanceFor(ctx context.Context, accountID, projectName, name string) (*database.Instance, error) {
	return h.GetInstance(ctx, accountID, projectName, name)
}

func (h *Hephaestus) refreshCounts(ctx context.Context, projectID string) {
	_, _ = h.DB.Exec(ctx,
		`UPDATE projects SET instance_count = (SELECT COUNT(*) FROM instances WHERE project_id = $1) WHERE id = $1`,
		projectID)
}

// CreateVolume provisions an unattached block volume.
func (h *Hephaestus) CreateVolume(ctx context.Context, accountID, projectName string, v database.Volume) (*database.Volume, error) {
	proj, err := h.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	if v.Name == "" || v.SizeGB <= 0 {
		return nil, errors.New("volume name and size_gb are required")
	}
	if v.Type == "" {
		v.Type = "gp2"
	}
	v.ID = newID()
	v.ProjectID = proj.ID
	v.State = "available"
	if _, err := h.DB.Exec(ctx, `
		INSERT INTO volumes (id, project_id, name, size_gb, volume_type, state)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		v.ID, proj.ID, v.Name, v.SizeGB, v.Type, v.State); err != nil {
		return nil, err
	}
	return &v, nil
}

// ListVolumes returns all volumes in a project.
func (h *Hephaestus) ListVolumes(ctx context.Context, accountID, projectName string) ([]database.Volume, error) {
	proj, err := h.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := h.DB.Query(ctx, `
		SELECT id, project_id, name, COALESCE(instance_id,''), size_gb, volume_type, state, created_at, updated_at
		FROM volumes WHERE project_id = $1 ORDER BY created_at`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Volume
	for rows.Next() {
		var v database.Volume
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Name, &v.InstanceID, &v.SizeGB, &v.Type, &v.State, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// DeleteVolume removes a volume (must not be in-use).
func (h *Hephaestus) DeleteVolume(ctx context.Context, accountID, projectName, name string) error {
	proj, err := h.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return err
	}
	var instanceID string
	err = h.DB.QueryRow(ctx,
		`SELECT COALESCE(instance_id,'') FROM volumes WHERE project_id = $1 AND name = $2`,
		proj.ID, name).Scan(&instanceID)
	if IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if instanceID != "" {
		return errors.New("cannot delete a volume that is in-use")
	}
	_, err = h.DB.Exec(ctx, `DELETE FROM volumes WHERE project_id = $1 AND name = $2`, proj.ID, name)
	return err
}

// CreateSnapshot captures a volume's backup.
func (h *Hephaestus) CreateSnapshot(ctx context.Context, accountID, projectName string, s database.Snapshot) (*database.Snapshot, error) {
	proj, err := h.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	if s.Name == "" || s.VolumeID == "" {
		return nil, errors.New("snapshot name and volume are required")
	}
	var size int
	err = h.DB.QueryRow(ctx,
		`SELECT size_gb FROM volumes WHERE project_id = $1 AND id = $2`, proj.ID, s.VolumeID).Scan(&size)
	if IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.ID = newID()
	s.ProjectID = proj.ID
	s.SizeGB = size
	s.State = "completed"
	s.ProviderRef = "snap-" + s.ID
	if _, err := h.DB.Exec(ctx, `
		INSERT INTO snapshots (id, project_id, name, volume_id, size_gb, state, provider_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		s.ID, proj.ID, s.Name, s.VolumeID, s.SizeGB, s.State, s.ProviderRef); err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSnapshots returns all snapshots in a project.
func (h *Hephaestus) ListSnapshots(ctx context.Context, accountID, projectName string) ([]database.Snapshot, error) {
	proj, err := h.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := h.DB.Query(ctx, `
		SELECT id, project_id, name, COALESCE(volume_id,''), size_gb, state, COALESCE(provider_ref,''), created_at, updated_at
		FROM snapshots WHERE project_id = $1 ORDER BY created_at`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.Snapshot
	for rows.Next() {
		var s database.Snapshot
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Name, &s.VolumeID, &s.SizeGB, &s.State, &s.ProviderRef, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CreateKeyPair generates an RSA key pair; only the public key is persisted.
// The generated private key is returned to the caller exactly once.
func (h *Hephaestus) CreateKeyPair(ctx context.Context, accountID, projectName, name string) (*database.KeyPair, string, error) {
	proj, err := h.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, "", err
	}
	if name == "" {
		return nil, "", errors.New("key pair name is required")
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", err
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	pubDer, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, "", err
	}
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDer}))

	sum := sha256.Sum256(pubDer)
	fingerprint := "SHA256:" + base64.StdEncoding.EncodeToString(sum[:])

	kp := &database.KeyPair{
		ID:          newID(),
		ProjectID:   proj.ID,
		Name:        name,
		Fingerprint: fingerprint,
		PublicKey:   pubPEM,
	}
	if _, err := h.DB.Exec(ctx, `
		INSERT INTO key_pairs (id, project_id, name, fingerprint, public_key)
		VALUES ($1, $2, $3, $4, $5)`,
		kp.ID, proj.ID, name, fingerprint, pubPEM); err != nil {
		return nil, "", err
	}
	return kp, string(privPEM), nil
}

// ListKeyPairs returns the stored key pairs for a project.
func (h *Hephaestus) ListKeyPairs(ctx context.Context, accountID, projectName string) ([]database.KeyPair, error) {
	proj, err := h.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := h.DB.Query(ctx, `
		SELECT id, project_id, name, fingerprint, public_key, created_at
		FROM key_pairs WHERE project_id = $1 ORDER BY name`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.KeyPair
	for rows.Next() {
		var k database.KeyPair
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.Fingerprint, &k.PublicKey, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// CreateSecurityGroup stores a named ruleset for a project.
func (h *Hephaestus) CreateSecurityGroup(ctx context.Context, accountID, projectName string, sg database.SecurityGroup) (*database.SecurityGroup, error) {
	proj, err := h.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	if sg.Name == "" {
		return nil, errors.New("security group name is required")
	}
	sg.ID = newID()
	sg.ProjectID = proj.ID
	if len(sg.Rules) == 0 {
		sg.Rules = []database.SecurityRule{{Port: 22, CIDR: "0.0.0.0/0"}}
	}
	rules, err := json.Marshal(sg.Rules)
	if err != nil {
		return nil, err
	}
	if _, err := h.DB.Exec(ctx, `
		INSERT INTO security_groups (id, project_id, name, description, rules)
		VALUES ($1, $2, $3, $4, $5)`,
		sg.ID, proj.ID, sg.Name, sg.Description, string(rules)); err != nil {
		return nil, err
	}
	return &sg, nil
}

// ListSecurityGroups returns the rulesets for a project.
func (h *Hephaestus) ListSecurityGroups(ctx context.Context, accountID, projectName string) ([]database.SecurityGroup, error) {
	proj, err := h.resolveProject(ctx, accountID, projectName)
	if err != nil {
		return nil, err
	}
	rows, err := h.DB.Query(ctx, `
		SELECT id, project_id, name, COALESCE(description,''), rules, created_at
		FROM security_groups WHERE project_id = $1 ORDER BY name`, proj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []database.SecurityGroup
	for rows.Next() {
		var sg database.SecurityGroup
		var rules []byte
		if err := rows.Scan(&sg.ID, &sg.ProjectID, &sg.Name, &sg.Description, &rules, &sg.CreatedAt); err != nil {
			return nil, err
		}
		if len(rules) > 0 {
			_ = json.Unmarshal(rules, &sg.Rules)
		}
		out = append(out, sg)
	}
	return out, rows.Err()
}

// Audit records an operation in the audit trail.
func (h *Hephaestus) Audit(ctx context.Context, accountID, projectID, entity, operation, status string) error {
	_, err := h.DB.Exec(ctx, `
		INSERT INTO audit_logs (account_id, project_id, entity, operation, status)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5)`,
		accountID, projectID, entity, operation, status)
	return err
}

// rowScanner abstracts *sql.Row and *sql.Rows for shared scanning.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanInstance(s rowScanner) (*database.Instance, error) {
	var inst database.Instance
	err := s.Scan(
		&inst.ID, &inst.ProjectID, &inst.Name, &inst.Type, &inst.ImageID, &inst.State,
		&inst.PrivateIP, &inst.PublicIP, &inst.KeyPairName, &inst.ProviderRef, &inst.LaunchedBy,
		&inst.LaunchedAt, &inst.TerminatedAt, &inst.CreatedAt, &inst.UpdatedAt)
	if IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if inst.LaunchedAt != nil && inst.LaunchedAt.IsZero() {
		inst.LaunchedAt = nil
	}
	if inst.TerminatedAt != nil && inst.TerminatedAt.IsZero() {
		inst.TerminatedAt = nil
	}
	return &inst, nil
}
