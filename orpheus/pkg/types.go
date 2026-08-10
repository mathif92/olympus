package pkg

import (
	"context"
	"database/sql"
	"errors"
)

// Cluster and node-group states managed by the provisioner and persisted in Postgres.
const (
	StatePending  = "pending"
	StateCreating = "creating"
	StateActive   = "active"
	StateUpdating = "updating"
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

// ClusterSpec describes everything needed to provision a control plane.
type ClusterSpec struct {
	ID                string
	ProjectID         string
	Name              string
	KubernetesVersion string
	Region            string
}

// NodeGroupSpec describes everything needed to attach a worker node group.
type NodeGroupSpec struct {
	ClusterRef  string
	Name        string
	NodeSize    string
	MinSize     int
	DesiredSize int
	MaxSize     int
}

// ProvisionedCluster is the result of provisioning a control plane: where to
// reach it and the material a client needs to talk to it.
type ProvisionedCluster struct {
	ProviderRef string
	Endpoint    string
	CAData      string
}

// ProvisionedNodeGroup is the provider-side reference for a node group.
type ProvisionedNodeGroup struct {
	ProviderRef string
}

// Provisioner is the pluggable "data plane" behind the Orpheus control plane.
// A mock implementation ships with Orpheus; a real backend (e.g. a Kubernetes
// API, EKS, or a home-grown control plane) can implement the same interface.
type Provisioner interface {
	// CreateCluster provisions a Kubernetes control plane and returns its
	// endpoint plus the API-server CA material for building a kubeconfig.
	CreateCluster(ctx context.Context, spec ClusterSpec) (*ProvisionedCluster, error)
	// DeleteCluster tears a control plane down for good.
	DeleteCluster(ctx context.Context, providerRef string) error
	// CreateNodeGroup attaches a worker node group to a control plane.
	CreateNodeGroup(ctx context.Context, spec NodeGroupSpec) (*ProvisionedNodeGroup, error)
	// ScaleNodeGroup sets the desired size of a node group.
	ScaleNodeGroup(ctx context.Context, spec NodeGroupSpec) error
	// DeleteNodeGroup removes a worker node group.
	DeleteNodeGroup(ctx context.Context, spec NodeGroupSpec) error
	// Healthy reports whether the data plane is reachable.
	Healthy(ctx context.Context) error
}
