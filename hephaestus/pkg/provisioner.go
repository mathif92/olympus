package pkg

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
)

// MockProvisioner is an in-process Provisioner that simulates a data plane.
// It tracks instance state per provider reference and emits synthetic IPs,
// so the control plane can be exercised end-to-end without a hypervisor.
type MockProvisioner struct {
	mu     sync.Mutex
	states map[string]string
	ips    map[string]string
}

// NewMockProvisioner creates an empty mock provisioner.
func NewMockProvisioner() *MockProvisioner {
	return &MockProvisioner{
		states: make(map[string]string),
		ips:    make(map[string]string),
	}
}

// Launch simulates booting an instance by registering it in StateRunning.
func (m *MockProvisioner) Launch(_ context.Context, spec InstanceSpec) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref := fmt.Sprintf("mock-%s", spec.ID)
	m.states[ref] = StateRunning
	m.ips[ref] = fmt.Sprintf("10.0.%d.%d", rand.Intn(255), rand.Intn(254)+1)
	return ref, m.ips[ref], nil
}

// Start simulates powering an instance on.
func (m *MockProvisioner) Start(_ context.Context, providerRef string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[providerRef] = StateRunning
	return nil
}

// Stop simulates powering an instance off.
func (m *MockProvisioner) Stop(_ context.Context, providerRef string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.states[providerRef] == StateTerminated {
		return ErrConflict
	}
	m.states[providerRef] = StateStopped
	return nil
}

// Terminate simulates destroying an instance.
func (m *MockProvisioner) Terminate(_ context.Context, providerRef string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[providerRef] = StateTerminated
	return nil
}

// Healthy always reports the mock is reachable.
func (m *MockProvisioner) Healthy(_ context.Context) error {
	return nil
}
