package integration

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/mathif92/olympus/mneme/pkg"
)

// TestDockerProvisionerRealRedis exercises the real provisioner against a live
// Docker daemon: each managed cluster is an actual Redis container, and
// snapshots are real RDB dumps (SAVE) taken from inside that container.
func TestDockerProvisionerRealRedis(t *testing.T) {
	if os.Getenv("RUN_DOCKER_TESTS") == "" {
		t.Skip("set RUN_DOCKER_TESTS=1 to exercise the real docker provisioner (needs a Docker daemon)")
	}

	ctx := context.Background()
	provisioner := pkg.NewDockerProvisioner("redis:7-alpine")

	provisioned, err := provisioner.CreateCluster(ctx, pkg.ClusterSpec{
		Name:   "session-cache",
		Engine: "redis",
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	t.Cleanup(func() { _ = provisioner.DeleteCluster(ctx, provisioned.ProviderRef) })

	if provisioned.Endpoint == "" {
		t.Fatalf("expected full provisioned cluster: %+v", provisioned)
	}

	// The cluster is a real, reachable Redis — write and read through it.
	client := redis.NewClient(&redis.Options{Addr: provisioned.Endpoint})
	defer client.Close()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping real redis (%s): %v", provisioned.Endpoint, err)
	}
	if err := client.Set(ctx, "session:user:42", "olympus", 0).Err(); err != nil {
		t.Fatalf("set key on real redis: %v", err)
	}
	got, err := client.Get(ctx, "session:user:42").Result()
	if err != nil {
		t.Fatalf("get key from real redis: %v", err)
	}
	if got != "olympus" {
		t.Fatalf("expected 'olympus' from real redis, got %q", got)
	}

	// Take a real RDB snapshot with the data in it.
	snap, err := provisioner.CreateSnapshot(ctx, pkg.SnapshotSpec{
		ClusterRef: provisioned.ProviderRef,
		Name:       "pre-release",
	})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if snap.ProviderRef == "" || snap.SizeMB <= 0 {
		t.Fatalf("expected non-empty real snapshot: %+v", snap)
	}

	if err := provisioner.DeleteSnapshot(ctx, snap.ProviderRef); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}
}
