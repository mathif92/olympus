package pkg

import (
	"context"
	"database/sql"
	"errors"
)

// Instance states managed by the provisioner and persisted in Postgres.
const (
	StatePending    = "pending"
	StateRunning    = "running"
	StateStopped    = "stopped"
	StateTerminated = "terminated"
)

// ErrNotFound is returned when a requested project or resource does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when an operation cannot transition from the current state.
var ErrConflict = errors.New("state conflict")

// IsNotFound reports whether an error is ErrNotFound (incl. wrapped sql.ErrNoRows).
func IsNotFound(err error) bool {
	return err == ErrNotFound || err == sql.ErrNoRows
}

// InstanceSpec describes everything needed to launch an instance.
type InstanceSpec struct {
	ID            string
	ProjectID     string
	Name          string
	Type          string
	ImageID       string
	KeyPairName   string
	SecurityGroup []string
	VolumeSizeGB  int
	LaunchedBy    string
}

// Provisioner is the pluggable "data plane" behind the control plane. A mock
// implementation ships with Hephaestus; a real hypervisor backend (e.g. QEMU,
// Firecracker, or a cloud API) can implement the same interface.
type Provisioner interface {
	// Launch boots a new instance and returns its backend reference.
	Launch(ctx context.Context, spec InstanceSpec) (providerRef, privateIP string, err error)
	// Start powers on a previously stopped instance.
	Start(ctx context.Context, providerRef string) error
	// Stop powers off an instance.
	Stop(ctx context.Context, providerRef string) error
	// Terminate tears the instance down for good.
	Terminate(ctx context.Context, providerRef string) error
	// Healthy reports whether the data plane is reachable.
	Healthy(ctx context.Context) error
}
