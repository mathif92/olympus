package pkg

import (
	"context"
	"encoding/json"

	"github.com/mathif92/olympus/prometheus/pkg/database"
)

// Invocation statuses.
const (
	StatusSuccess = "success"
	StatusError   = "error"
	StatusTimeout = "timeout"
	StatusOOM     = "oom"
)

// InvocationResult is what an executor produces after running a handler.
type InvocationResult struct {
	Status     string
	Response   string
	Error      string
	ExitCode   int
	DurationMS int64
}

// Executor runs function code. Implementations may be in-process mocks (dev /
// tests) or Docker-backed executors that build per-runtime images on the fly
// and run the handler in a constrained container.
type Executor interface {
	// Invoke ensures the version's image is built (if the executor builds)
	// and runs the handler with the given event, enforcing the function's
	// timeout and memory limits. A returned error means the invocation could
	// not be executed at all (build failure, backend down); a handler that ran
	// and failed still returns a result with Status=error.
	Invoke(ctx context.Context, fn database.Function, v database.FunctionVersion, event json.RawMessage) (*InvocationResult, error)
	// Healthy reports whether the execution backend is reachable.
	Healthy(ctx context.Context) error
}
