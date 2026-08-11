package pkg

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/mathif92/olympus/prometheus/pkg/database"
)

// MockExecutor is an in-process Executor for development and tests: it records
// invocations and echoes the event back so callers can exercise the full
// control-plane pipeline (upload -> invoke -> audit) without a Docker daemon.
type MockExecutor struct {
	mu          sync.Mutex
	invocations []*InvocationResult
}

// NewMockExecutor creates a MockExecutor.
func NewMockExecutor() *MockExecutor {
	return &MockExecutor{}
}

// Invoke echoes the event back as the handler result.
func (m *MockExecutor) Invoke(_ context.Context, _ database.Function, _ database.FunctionVersion, event json.RawMessage) (*InvocationResult, error) {
	res := &InvocationResult{
		Status:     StatusSuccess,
		Response:   string(event),
		DurationMS: 1,
	}
	m.mu.Lock()
	m.invocations = append(m.invocations, res)
	m.mu.Unlock()
	return res, nil
}

// Healthy reports the mock backend is always reachable.
func (m *MockExecutor) Healthy(context.Context) error { return nil }

// Invocations returns a copy of the recorded results (used by tests).
func (m *MockExecutor) Invocations() []*InvocationResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*InvocationResult, len(m.invocations))
	copy(out, m.invocations)
	return out
}
