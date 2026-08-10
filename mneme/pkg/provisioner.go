package pkg

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
)

// MockProvisioner is an in-process Provisioner that simulates a managed
// in-memory cache data plane. It tracks cluster and snapshot state per
// provider reference and emits synthetic endpoints, so the control plane can
// be exercised end-to-end without any real cache engine.
type MockProvisioner struct {
	mu        sync.Mutex
	endpoints map[string]string
}

// NewMockProvisioner creates an empty mock provisioner.
func NewMockProvisioner() *MockProvisioner {
	return &MockProvisioner{endpoints: make(map[string]string)}
}

// CreateCluster simulates provisioning a cache cluster.
func (m *MockProvisioner) CreateCluster(_ context.Context, spec ClusterSpec) (*ProvisionedCluster, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref := "cache-" + spec.Name
	endpoint := fmt.Sprintf("%d.%d.%d.%d:6379", rand.Intn(127)+10, rand.Intn(255), rand.Intn(255), rand.Intn(255))
	m.endpoints[ref] = endpoint
	return &ProvisionedCluster{ProviderRef: ref, Endpoint: endpoint}, nil
}

// DeleteCluster simulates tearing a cache cluster down.
func (m *MockProvisioner) DeleteCluster(_ context.Context, providerRef string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.endpoints, providerRef)
	return nil
}

// CreateSnapshot simulates a point-in-time backup.
func (m *MockProvisioner) CreateSnapshot(_ context.Context, spec SnapshotSpec) (*ProvisionedSnapshot, error) {
	size := spec.Size
	if size == 0 {
		size = 20
	}
	return &ProvisionedSnapshot{ProviderRef: "snap-" + spec.Name, SizeMB: size}, nil
}

// DeleteSnapshot simulates removing a backup.
func (m *MockProvisioner) DeleteSnapshot(_ context.Context, _ string) error {
	return nil
}

// Healthy always reports the mock is reachable.
func (m *MockProvisioner) Healthy(_ context.Context) error {
	return nil
}
