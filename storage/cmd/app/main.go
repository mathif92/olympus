package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mathif92/olympus/storage/internal/handler"
	"github.com/mathif92/olympus/storage/pkg"
	"github.com/mathif92/olympus/storage/pkg/database"
)

const storageBaseDir = "storage/data"

func main() {
	var (
		reconcileOnce  = flag.Bool("reconcile", false, "run a metadata/content reconciliation pass against MinIO, then exit")
		reconcilePurge = flag.Bool("reconcile-purge", false, "delete orphaned MinIO objects during reconciliation")
		reconcileMark  = flag.Bool("reconcile-mark-missing", false, "mark Postgres rows whose content is missing in MinIO")
		reconcileDrift = flag.Bool("reconcile-drift", false, "repair Postgres ETags to match stored content in MinIO")
		backfillETag   = flag.Bool("backfill-etag", false, "stamp x-olympus-etag metadata on pre-existing MinIO objects, then exit")
	)
	flag.Parse()

	// Initialize database connection.
	dbCfg := database.Config{
		PostgresURL: getenv("POSTGRES_DSN", "host=localhost port=5432 user=olympus password=olympus_secret dbname=olympus_storage sslmode=disable"),
		RedisURL:    getenv("REDIS_URL", "localhost:6379"),
		PoolMax:     getEnvInt("POOL_MAX", 20),
		PoolMin:     getEnvInt("POOL_MIN", 5),
		PoolTimeout: time.Duration(getEnvInt("POOL_TIMEOUT_MS", 30000)) * time.Millisecond,
	}

	dbClient, err := database.NewClient(dbCfg)
	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	defer dbClient.Close()

	// Apply pending schema migrations on startup.
	migrationsDir := getenv("MIGRATIONS_DIR", "migrations")
	if err := database.Migrate(dbClient.DB, migrationsDir); err != nil {
		log.Fatalf("Database migration failed: %v", err)
	}

	// Initialize the storage backend (local | hybrid | minio).
	backend, err := buildBackend(getenv("STORAGE_BACKEND", "local"), dbClient)
	if err != nil {
		log.Fatalf("Storage backend initialization failed: %v", err)
	}

	// One-shot maintenance modes (each exits when done).
	if mb, ok := backend.(*pkg.MinioStorageBackend); ok {
		if *backfillETag {
			count, err := pkg.BackfillEtags(context.Background(), mb, dbClient)
			if err != nil {
				log.Fatalf("etag backfill failed: %v", err)
			}
			log.Printf("backfilled x-olympus-etag on %d pre-existing object(s)", count)
			return
		}

		if *reconcileOnce {
			result, err := pkg.Reconcile(context.Background(), mb, dbClient, pkg.ReconcileOptions{
				PurgeOrphans: *reconcilePurge,
				MarkMissing:  *reconcileMark,
				RepairDrift:  *reconcileDrift,
			})
			if err != nil {
				log.Fatalf("reconciliation failed: %v", err)
			}
			log.Printf("reconciled %d objects: %d orphans, %d missing, %d drift (removed=%d marked=%d repaired=%d)",
				result.Scanned, len(result.Orphans), len(result.Missing), len(result.Drift),
				result.Removed, result.Marked, result.Repaired)
			return
		}
	} else if *reconcileOnce || *backfillETag {
		log.Fatalf("-reconcile/-backfill-etag require STORAGE_BACKEND=minio (got %T)", backend)
	}

	service := pkg.NewStorageService(backend)

	// Create object handler and health handler for connectivity checks.
	objHandler := handler.NewObjectHandler(service)
	mux := http.NewServeMux()
	mux.HandleFunc("/object/", objHandler.HandleFunc)
	mux.HandleFunc("/health", healthHandler(dbClient, backend))

	log.Printf("🚀 OlympusStore running on :8080 (backend: %T)...", backend)
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Could not start server: %v", err)
	}
}

// buildBackend constructs the configured storage backend.
func buildBackend(kind string, dbClient *database.Client) (pkg.StorageBackend, error) {
	switch kind {
	case "minio":
		return pkg.NewMinioStorageBackend(pkg.MinioConfig{
			Endpoint:  getenv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKey: getenv("MINIO_ACCESS_KEY", "olympus_admin"),
			SecretKey: getenv("MINIO_SECRET_KEY", "olympus_secret_admin"),
			Bucket:    getenv("MINIO_BUCKET", "olympus"),
			Region:    getenv("MINIO_REGION", ""),
			UsingSSL:  getEnvInt("MINIO_USE_SSL", 0) == 1,
			DB:        dbClient,
		})
	case "hybrid":
		return pkg.NewHybridStorageBackend(storageBaseDir, dbClient, dbClient.Cache), nil
	case "local":
		return pkg.NewLocalFSBackend(storageBaseDir), nil
	default:
		return nil, fmt.Errorf("unknown STORAGE_BACKEND %q (want local|hybrid|minio)", kind)
	}
}

// healthHandler reports connectivity to the configured dependencies.
func healthHandler(dbClient *database.Client, backend pkg.StorageBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := dbClient.Ping(ctx); err != nil {
			http.Error(w, "PostgreSQL unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := dbClient.Cache.Ping(ctx).Err(); err != nil {
			http.Error(w, "Redis unavailable", http.StatusServiceUnavailable)
			return
		}
		if mb, ok := backend.(*pkg.MinioStorageBackend); ok {
			if err := mb.Healthy(ctx); err != nil {
				http.Error(w, "MinIO unavailable: "+err.Error(), http.StatusServiceUnavailable)
				return
			}
		}
		_, _ = w.Write([]byte(`{"status":"healthy","postgres":"ok","redis":"ok"}`))
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
