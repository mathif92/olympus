package pkg

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
)

// MockProvisioner is an in-process Provisioner that simulates a managed
// relational data plane. It tracks instance and snapshot state per provider
// reference and emits synthetic endpoints, so the control plane can be
// exercised end-to-end without any real database engine.
type MockProvisioner struct {
	mu        sync.Mutex
	endpoints map[string]string
}

// NewMockProvisioner creates an empty mock provisioner.
func NewMockProvisioner() *MockProvisioner {
	return &MockProvisioner{endpoints: make(map[string]string)}
}

// CreateInstance simulates provisioning a database engine.
func (m *MockProvisioner) CreateInstance(_ context.Context, spec InstanceSpec) (*InstanceCreds, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref := "inst-" + spec.Name
	endpoint := fmt.Sprintf("%d.%d.%d.%d:5432", rand.Intn(127)+10, rand.Intn(255), rand.Intn(255), rand.Intn(255))
	m.endpoints[ref] = endpoint
	return &InstanceCreds{
		ProviderRef:    ref,
		Endpoint:       endpoint,
		MasterUsername: spec.MasterUsername,
		MasterPassword: spec.MasterPassword,
	}, nil
}

// DeleteInstance simulates tearing a database instance down.
func (m *MockProvisioner) DeleteInstance(_ context.Context, providerRef string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.endpoints, providerRef)
	return nil
}

// StartInstance simulates resuming a stopped instance.
func (m *MockProvisioner) StartInstance(_ context.Context, _ string) error {
	return nil
}

// StopInstance simulates pausing a running instance.
func (m *MockProvisioner) StopInstance(_ context.Context, _ string) error {
	return nil
}

// CreateSnapshot simulates a point-in-time backup.
func (m *MockProvisioner) CreateSnapshot(_ context.Context, spec SnapshotSpec) (*ProvisionedSnapshot, error) {
	size := spec.Size
	if size == 0 {
		size = 20
	}
	return &ProvisionedSnapshot{ProviderRef: "snap-" + spec.Name, SizeGB: size}, nil
}

// DeleteSnapshot simulates removing a backup.
func (m *MockProvisioner) DeleteSnapshot(_ context.Context, _ string) error {
	return nil
}

// Healthy always reports the mock is reachable.
func (m *MockProvisioner) Healthy(_ context.Context) error {
	return nil
}
