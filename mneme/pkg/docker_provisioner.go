package pkg

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

// DockerProvisioner provisions real Redis containers (via a Docker daemon,
// using testcontainers) for every managed Mneme cluster. It hands back the
// container's mapped endpoint as the cluster endpoint and takes true backups
// (RDB dumps via SAVE) when a snapshot is requested, so the data plane is real.
type DockerProvisioner struct {
	mu         sync.Mutex
	containers map[string]*redis.RedisContainer
	dumps      map[string][]byte
	redisImage string
}

// NewDockerProvisioner builds a provisioner that launches real engine containers.
// redisImage selects the engine image (default redis:7-alpine).
func NewDockerProvisioner(redisImage string) *DockerProvisioner {
	if redisImage == "" {
		redisImage = "redis:7-alpine"
	}
	return &DockerProvisioner{
		containers: make(map[string]*redis.RedisContainer),
		dumps:      make(map[string][]byte),
		redisImage: redisImage,
	}
}

// CreateCluster starts a real Redis container for the cluster.
func (d *DockerProvisioner) CreateCluster(ctx context.Context, spec ClusterSpec) (*ProvisionedCluster, error) {
	if spec.Engine != "redis" {
		return nil, fmt.Errorf("docker provisioner only supports engine 'redis', got %q", spec.Engine)
	}

	rc, err := redis.Run(ctx, d.redisImage)
	if err != nil {
		return nil, fmt.Errorf("start redis container: %w", err)
	}

	host, err := rc.Host(ctx)
	if err != nil {
		_ = rc.Terminate(ctx)
		return nil, fmt.Errorf("cluster host: %w", err)
	}
	port, err := rc.MappedPort(ctx, "6379/tcp")
	if err != nil {
		_ = rc.Terminate(ctx)
		return nil, fmt.Errorf("cluster port: %w", err)
	}

	ref := "cache-" + sanitizeDocker(spec.Name)
	d.mu.Lock()
	d.containers[ref] = rc
	d.mu.Unlock()

	return &ProvisionedCluster{
		ProviderRef: ref,
		Endpoint:    fmt.Sprintf("%s:%s", host, port.Port()),
	}, nil
}

// DeleteCluster terminates a managed cluster container for good.
func (d *DockerProvisioner) DeleteCluster(ctx context.Context, providerRef string) error {
	d.mu.Lock()
	rc, ok := d.containers[providerRef]
	if ok {
		delete(d.containers, providerRef)
	}
	d.mu.Unlock()
	if !ok {
		return nil
	}
	return rc.Terminate(ctx)
}

// CreateSnapshot forces an RDB dump (SAVE) inside the real container and
// streams out the dump bytes as the snapshot payload.
func (d *DockerProvisioner) CreateSnapshot(ctx context.Context, spec SnapshotSpec) (*ProvisionedSnapshot, error) {
	d.mu.Lock()
	rc, ok := d.containers[spec.ClusterRef]
	d.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown cluster %q for snapshot", spec.ClusterRef)
	}

	// Force a synchronous RDB save, then read the produced dump.rdb.
	if _, _, err := rc.Exec(ctx, []string{"redis-cli", "SAVE"}); err != nil {
		return nil, fmt.Errorf("trigger SAVE: %w", err)
	}
	reader, err := rc.CopyFileFromContainer(ctx, "/data/dump.rdb")
	if err != nil {
		return nil, fmt.Errorf("copy dump.rdb: %w", err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read dump.rdb: %w", err)
	}
	sizeMB := len(data) / (1 << 20)
	if sizeMB == 0 {
		sizeMB = 1
	}

	ref := "snap-" + sanitizeDocker(spec.Name)
	d.mu.Lock()
	d.dumps[ref] = data
	d.mu.Unlock()
	return &ProvisionedSnapshot{ProviderRef: ref, SizeMB: sizeMB}, nil
}

// DeleteSnapshot removes a stored RDB dump.
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
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			b = append(b, c)
		case c >= 'A' && c <= 'Z':
			b = append(b, c+32)
		default:
			b = append(b, '-')
		}
	}
	return string(b)
}
