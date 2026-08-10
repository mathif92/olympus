package database

import "time"

// Account represents a tenant in the compute service.
type Account struct {
	ID            string    `json:"id"`
	DisplayName   string    `json:"display_name"`
	Email         string    `json:"email"`
	Plan          string    `json:"plan"`
	InstanceLimit int       `json:"instance_limit"`
	UsedInstances int       `json:"used_instances"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"`
}

// Project represents a named namespace of compute resources within an account.
type Project struct {
	ID            string    `json:"id"`
	AccountID     string    `json:"account_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	InstanceCount int       `json:"instance_count"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"`
}

// InstanceType describes a catalog size for launching instances.
type InstanceType struct {
	Name              string `json:"name"`
	VCPUs             int    `json:"vcpus"`
	MemoryGB          int    `json:"memory_gb"`
	StorageGB         int    `json:"storage_gb"`
	PricePerHourCents int64  `json:"price_per_hour_cents"`
	Status            string `json:"status"`
}

// Instance represents a virtual compute instance.
type Instance struct {
	ID           string            `json:"id"`
	ProjectID    string            `json:"project_id"`
	Name         string            `json:"name"`
	Type         string            `json:"instance_type"`
	ImageID      string            `json:"image_id"`
	State        string            `json:"state"`
	PrivateIP    string            `json:"private_ip,omitempty"`
	PublicIP     string            `json:"public_ip,omitempty"`
	KeyPairName  string            `json:"key_pair_name,omitempty"`
	ProviderRef  string            `json:"provider_ref,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	LaunchedBy   string            `json:"launched_by"`
	LaunchedAt   *time.Time        `json:"launched_at,omitempty"`
	TerminatedAt *time.Time        `json:"terminated_at,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// Volume represents an attachable block-storage volume.
type Volume struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Name       string    `json:"name"`
	InstanceID string    `json:"instance_id,omitempty"`
	SizeGB     int       `json:"size_gb"`
	Type       string    `json:"volume_type"`
	State      string    `json:"state"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Snapshot represents a point-in-time backup of a volume.
type Snapshot struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	VolumeID    string    `json:"volume_id,omitempty"`
	SizeGB      int       `json:"size_gb"`
	State       string    `json:"state"`
	ProviderRef string    `json:"provider_ref,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// KeyPair holds the stored SSH public key material (private key never persisted).
type KeyPair struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Fingerprint string    `json:"fingerprint"`
	PublicKey   string    `json:"public_key"`
	CreatedAt   time.Time `json:"created_at"`
}

// SecurityGroup defines a firewall ruleset for instances.
type SecurityGroup struct {
	ID          string         `json:"id"`
	ProjectID   string         `json:"project_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Rules       []SecurityRule `json:"rules"`
	CreatedAt   time.Time      `json:"created_at"`
}

// SecurityRule is a single ingress rule.
type SecurityRule struct {
	Port int    `json:"port"`
	CIDR string `json:"cidr"`
}
