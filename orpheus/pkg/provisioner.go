package pkg

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"sync"
)

// MockProvisioner is an in-process Provisioner that simulates a managed
// Kubernetes data plane. It tracks cluster and node-group state per provider
// reference and emits synthetic API endpoints, so the control plane can be
// exercised end-to-end without any real Kubernetes.
type MockProvisioner struct {
	mu        sync.Mutex
	endpoints map[string]string
}

// NewMockProvisioner creates an empty mock provisioner.
func NewMockProvisioner() *MockProvisioner {
	return &MockProvisioner{endpoints: make(map[string]string)}
}

// CreateCluster simulates provisioning a control plane.
func (m *MockProvisioner) CreateCluster(_ context.Context, spec ClusterSpec) (*ProvisionedCluster, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref := "cp-" + spec.Name
	endpoint := fmt.Sprintf("https://%d.%d.%d.1:6443", rand.Intn(127)+10, rand.Intn(255), rand.Intn(255))
	m.endpoints[ref] = endpoint
	// A mock CA cert — real provisioners inject the real API-server CA here.
	ca := base64.StdEncoding.EncodeToString([]byte("mock-ca-certificate-data"))
	return &ProvisionedCluster{ProviderRef: ref, Endpoint: endpoint, CAData: ca}, nil
}

// DeleteCluster simulates tearing a control plane down.
func (m *MockProvisioner) DeleteCluster(_ context.Context, providerRef string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.endpoints, providerRef)
	return nil
}

// CreateNodeGroup simulates attaching a worker node group.
func (m *MockProvisioner) CreateNodeGroup(_ context.Context, spec NodeGroupSpec) (*ProvisionedNodeGroup, error) {
	return &ProvisionedNodeGroup{ProviderRef: "ng-" + spec.Name}, nil
}

// ScaleNodeGroup simulates resizing a worker node group.
func (m *MockProvisioner) ScaleNodeGroup(_ context.Context, _ NodeGroupSpec) error {
	return nil
}

// DeleteNodeGroup simulates removing a worker node group.
func (m *MockProvisioner) DeleteNodeGroup(_ context.Context, _ NodeGroupSpec) error {
	return nil
}

// Healthy always reports the mock is reachable.
func (m *MockProvisioner) Healthy(_ context.Context) error {
	return nil
}
