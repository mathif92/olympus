package pkg

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// DockerProvisioner provisions real PostgreSQL containers (via a Docker daemon,
// using testcontainers) for every managed Clio instance. It hands back the
// container's mapped endpoint as the instance endpoint and takes real logical
// backups (pg_dump) when a snapshot is requested, so the data plane is real.
type DockerProvisioner struct {
	mu            sync.Mutex
	containers    map[string]*postgres.PostgresContainer
	users         map[string]string
	passwords     map[string]string
	dumps         map[string][]byte
	postgresImage string
}

// NewDockerProvisioner builds a provisioner that launches real engine containers.
// postgresImage selects the engine image (default postgres:16-alpine).
func NewDockerProvisioner(postgresImage string) *DockerProvisioner {
	if postgresImage == "" {
		postgresImage = "postgres:16-alpine"
	}
	return &DockerProvisioner{
		containers:    make(map[string]*postgres.PostgresContainer),
		users:         make(map[string]string),
		passwords:     make(map[string]string),
		dumps:         make(map[string][]byte),
		postgresImage: postgresImage,
	}
}

// CreateInstance starts a real PostgreSQL container for the instance.
func (d *DockerProvisioner) CreateInstance(ctx context.Context, spec InstanceSpec) (*InstanceCreds, error) {
	if spec.MasterUsername == "" {
		return nil, fmt.Errorf("master username is required")
	}
	if spec.MasterPassword == "" {
		return nil, fmt.Errorf("master password is required")
	}

	pg, err := postgres.Run(ctx, d.postgresImage,
		postgres.WithDatabase("clio"),
		postgres.WithUsername(spec.MasterUsername),
		postgres.WithPassword(spec.MasterPassword),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}

	host, err := pg.Host(ctx)
	if err != nil {
		_ = pg.Terminate(ctx)
		return nil, fmt.Errorf("instance host: %w", err)
	}
	port, err := pg.MappedPort(ctx, "5432/tcp")
	if err != nil {
		_ = pg.Terminate(ctx)
		return nil, fmt.Errorf("instance port: %w", err)
	}

	ref := "inst-" + sanitizeDocker(spec.Name)
	d.mu.Lock()
	d.containers[ref] = pg
	d.users[ref] = spec.MasterUsername
	d.passwords[ref] = spec.MasterPassword
	d.mu.Unlock()

	return &InstanceCreds{
		ProviderRef:    ref,
		Endpoint:       fmt.Sprintf("%s:%s", host, port.Port()),
		MasterUsername: spec.MasterUsername,
		MasterPassword: spec.MasterPassword,
	}, nil
}

// DeleteInstance terminates a managed instance container for good.
func (d *DockerProvisioner) DeleteInstance(ctx context.Context, providerRef string) error {
	d.mu.Lock()
	pg, ok := d.containers[providerRef]
	if ok {
		delete(d.containers, providerRef)
		delete(d.users, providerRef)
		delete(d.passwords, providerRef)
	}
	d.mu.Unlock()
	if !ok {
		return nil
	}
	return pg.Terminate(ctx)
}

// StartInstance resumes a stopped instance container.
func (d *DockerProvisioner) StartInstance(ctx context.Context, providerRef string) error {
	d.mu.Lock()
	pg, ok := d.containers[providerRef]
	d.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown instance %q", providerRef)
	}
	return pg.Start(ctx)
}

// StopInstance pauses a running instance container.
func (d *DockerProvisioner) StopInstance(ctx context.Context, providerRef string) error {
	d.mu.Lock()
	pg, ok := d.containers[providerRef]
	d.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown instance %q", providerRef)
	}
	return pg.Stop(ctx, nil)
}

// CreateSnapshot takes a real logical backup (pg_dump) of the instance database.
func (d *DockerProvisioner) CreateSnapshot(ctx context.Context, spec SnapshotSpec) (*ProvisionedSnapshot, error) {
	d.mu.Lock()
	pg, ok := d.containers[spec.InstanceRef]
	user := d.users[spec.InstanceRef]
	password := d.passwords[spec.InstanceRef]
	d.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown instance %q for snapshot", spec.InstanceRef)
	}

	// Stream a real pg_dump of the whole "clio" database from inside the
	// container, connecting as the superuser we provisioned.
	_, reader, err := pg.Exec(ctx, []string{"pg_dump", "-U", user, "-Fc", "-d", "clio", "--no-owner"},
		tcexec.WithEnv([]string{"PGPASSWORD=" + password}))
	if err != nil {
		return nil, fmt.Errorf("pg_dump: %w", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read pg_dump output: %w", err)
	}
	sizeGB := len(data) / (1 << 30)
	if sizeGB == 0 {
		sizeGB = 1
	}

	ref := "snap-" + sanitizeDocker(spec.Name)
	d.mu.Lock()
	d.dumps[ref] = data
	d.mu.Unlock()
	return &ProvisionedSnapshot{ProviderRef: ref, SizeGB: sizeGB}, nil
}

// DeleteSnapshot removes a stored logical backup.
func (d *DockerProvisioner) DeleteSnapshot(_ context.Context, providerRef string) error {
	d.mu.Lock()
	delete(d.dumps, providerRef)
	d.mu.Unlock()
	return nil
}

// Healthy verifies a Docker daemon is reachable.
func (d *DockerProvisioner) Healthy(ctx context.Context) error {
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return fmt.Errorf("docker unavailable: %w", err)
	}
	defer func() { _ = provider.Close() }()
	return provider.Health(ctx)
}

// sanitizeDocker returns a DNS-safe name for container/provider refs.
func sanitizeDocker(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
