package database

import "time"

// Account represents a tenant in the managed-Kubernetes service.
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

// Project represents a named namespace of Kubernetes resources within an account.
type Project struct {
	ID           string    `json:"id"`
	AccountID    string    `json:"account_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	ClusterCount int       `json:"cluster_count"`
	CreatedAt    time.Time `json:"created_at"`
	Status       string    `json:"status"`
}

// KubernetesVersion describes a control-plane version Orpheus can manage.
type KubernetesVersion struct {
	Version string `json:"version"`
	Channel string `json:"channel"`
	Status  string `json:"status"`
}

// NodeSize describes a catalog size for worker nodes.
type NodeSize struct {
	Name              string `json:"name"`
	VCPUs             int    `json:"vcpus"`
	MemoryGB          int    `json:"memory_gb"`
	PricePerHourCents int64  `json:"price_per_hour_cents"`
	Status            string `json:"status"`
}

// Cluster represents a managed Kubernetes control plane.
type Cluster struct {
	ID                string    `json:"id"`
	ProjectID         string    `json:"project_id"`
	Name              string    `json:"name"`
	KubernetesVersion string    `json:"kubernetes_version"`
	Region            string    `json:"region"`
	State             string    `json:"state"`
	Endpoint          string    `json:"endpoint,omitempty"`
	CAData            string    `json:"-"`
	Kubeconfig        string    `json:"kubeconfig,omitempty"`
	ProviderRef       string    `json:"provider_ref,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Status            string    `json:"status"`
}

// NodeGroup represents a group of worker nodes attached to a cluster.
type NodeGroup struct {
	ID          string    `json:"id"`
	ClusterID   string    `json:"cluster_id"`
	Name        string    `json:"name"`
	NodeSize    string    `json:"node_size"`
	MinSize     int       `json:"min_size"`
	DesiredSize int       `json:"desired_size"`
	MaxSize     int       `json:"max_size"`
	State       string    `json:"state"`
	ProviderRef string    `json:"provider_ref,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Status      string    `json:"status"`
}
