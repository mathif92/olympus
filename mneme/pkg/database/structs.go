package database

import "time"

// Account represents a tenant in the managed in-memory caching service.
type Account struct {
	ID           string    `json:"id"`
	DisplayName  string    `json:"display_name"`
	Email        string    `json:"email"`
	Plan         string    `json:"plan"`
	ClusterLimit int       `json:"cluster_limit"`
	UsedClusters int       `json:"used_clusters"`
	CreatedAt    time.Time `json:"created_at"`
	Status       string    `json:"status"`
}

// Project represents a named namespace of cache resources within an account.
type Project struct {
	ID           string    `json:"id"`
	AccountID    string    `json:"account_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	ClusterCount int       `json:"cluster_count"`
	CreatedAt    time.Time `json:"created_at"`
	Status       string    `json:"status"`
}

// CacheEngine describes a managed in-memory cache engine/version pair.
type CacheEngine struct {
	Engine  string `json:"engine"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

// NodeType describes a catalog size for cache nodes.
type NodeType struct {
	Name              string `json:"name"`
	VCPUs             int    `json:"vcpus"`
	MemoryGB          int    `json:"memory_gb"`
	PricePerHourCents int64  `json:"price_per_hour_cents"`
	Status            string `json:"status"`
}

// CacheCluster represents a managed in-memory cache cluster.
type CacheCluster struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	Name          string    `json:"name"`
	Engine        string    `json:"engine"`
	EngineVersion string    `json:"engine_version"`
	NodeType      string    `json:"node_type"`
	NumNodes      int       `json:"num_nodes"`
	State         string    `json:"state"`
	Endpoint      string    `json:"endpoint,omitempty"`
	ProviderRef   string    `json:"provider_ref,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Status        string    `json:"status"`
}

// CacheSnapshot represents a point-in-time backup of a cache cluster.
type CacheSnapshot struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	ClusterID   string    `json:"cluster_id"`
	Name        string    `json:"name"`
	SizeMB      int       `json:"size_mb"`
	State       string    `json:"state"`
	ProviderRef string    `json:"provider_ref,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Status      string    `json:"status"`
}
