package pkg

import (
	"context"
	"database/sql"
	"errors"
)

// Cluster and snapshot states managed by the provisioner and persisted in Postgres.
const (
	StatePending  = "pending"
	StateCreating = "creating"
	StateActive   = "active"
	StateDeleting = "deleting"
	StateDeleted  = "deleted"
)

// ErrNotFound is returned when a requested project or resource does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when an operation cannot transition from the current state.
var ErrConflict = errors.New("state conflict")

// IsNotFound reports whether an error is ErrNotFound (incl. wrapped sql.ErrNoRows).
func IsNotFound(err error) bool {
	return err == ErrNotFound || err == sql.ErrNoRows
}

// ClusterSpec describes everything needed to provision a cache cluster.
type ClusterSpec struct {
	ID            string
	ProjectID     string
	Name          string
	Engine        string
	EngineVersion string
	NodeType      string
	NumNodes      int
}

// ProvisionedCluster is the result of provisioning a cache cluster: where to
// reach it and the provider-side reference.
type ProvisionedCluster struct {
	ProviderRef string
	Endpoint    string
}

// SnapshotSpec describes a point-in-time copy of a managed cluster.
type SnapshotSpec struct {
	ClusterRef string
	Size       int
	Name       string
}

// ProvisionedSnapshot is the provider-side reference for a snapshot.
type ProvisionedSnapshot struct {
	ProviderRef string
	SizeMB      int
}

// Provisioner is the pluggable "data plane" behind the Mneme control plane.
// A mock implementation ships with Mneme; a real backend (e.g. Redis containers,
// ElastiCache, or a home-grown cache fleet) can implement the same interface.
type Provisioner interface {
	// CreateCluster provisions an in-memory cache engine and returns where to
	// reach it.
	CreateCluster(ctx context.Context, spec ClusterSpec) (*ProvisionedCluster, error)
	// DeleteCluster tears a cache cluster down for good.
	DeleteCluster(ctx context.Context, providerRef string) error
	// CreateSnapshot produces a point-in-time copy of a managed cluster.
	CreateSnapshot(ctx context.Context, spec SnapshotSpec) (*ProvisionedSnapshot, error)
	// DeleteSnapshot removes a stored snapshot.
	DeleteSnapshot(ctx context.Context, providerRef string) error
	// Healthy reports whether the data plane is reachable.
	Healthy(ctx context.Context) error
}
