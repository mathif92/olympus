package database

import "time"

// Account represents a tenant in the managed-relational-database service.
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

// Project represents a named namespace of database resources within an account.
type Project struct {
	ID            string    `json:"id"`
	AccountID     string    `json:"account_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	InstanceCount int       `json:"instance_count"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"`
}

// DatabaseEngine describes a managed relational database engine/version pair.
type DatabaseEngine struct {
	Engine  string `json:"engine"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

// InstanceSize describes a catalog size for managed database instances.
type InstanceSize struct {
	Name              string `json:"name"`
	VCPUs             int    `json:"vcpus"`
	MemoryGB          int    `json:"memory_gb"`
	StorageGB         int    `json:"storage_gb"`
	PricePerHourCents int64  `json:"price_per_hour_cents"`
	Status            string `json:"status"`
}

// DBInstance represents a managed relational database instance.
type DBInstance struct {
	ID                 string    `json:"id"`
	ProjectID          string    `json:"project_id"`
	Name               string    `json:"name"`
	Engine             string    `json:"engine"`
	EngineVersion      string    `json:"engine_version"`
	Size               string    `json:"size"`
	AllocatedStorageGB int       `json:"allocated_storage_gb"`
	State              string    `json:"state"`
	Endpoint           string    `json:"endpoint,omitempty"`
	MasterUsername     string    `json:"master_username,omitempty"`
	MasterPassword     string    `json:"master_password,omitempty"`
	ProviderRef        string    `json:"provider_ref,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Status             string    `json:"status"`
}

// DBSnapshot represents a point-in-time backup of a database instance.
type DBSnapshot struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	InstanceID  string    `json:"instance_id"`
	Instance    string    `json:"instance,omitempty"`
	Name        string    `json:"name"`
	SizeGB      int       `json:"size_gb"`
	State       string    `json:"state"`
	ProviderRef string    `json:"provider_ref,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Status      string    `json:"status"`
}
