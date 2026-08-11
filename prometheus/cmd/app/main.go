package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mathif92/olympus/authz"
	"github.com/mathif92/olympus/prometheus/internal/handler"
	"github.com/mathif92/olympus/prometheus/pkg"
	"github.com/mathif92/olympus/prometheus/pkg/database"
)

func main() {
	var (
		addr = flag.String("addr", getenv("PROMETHEUS_ADDR", ":8092"), "listen address")
	)
	flag.Parse()

	dbCfg := database.Config{
		PostgresURL: getenv("POSTGRES_DSN",
			"host=localhost port=15440 user=olympus password=olympus_secret dbname=olympus_functions sslmode=disable"),
		PoolMax:     getEnvInt("POOL_MAX", 20),
		PoolMin:     getEnvInt("POOL_MIN", 5),
		PoolTimeout: time.Duration(getEnvInt("POOL_TIMEOUT_MS", 30000)) * time.Millisecond,
	}

	dbClient, err := database.NewClient(dbCfg)
	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	defer dbClient.Close()

	migrationsDir := getenv("MIGRATIONS_DIR", "migrations")
	if err := database.Migrate(dbClient.DB, migrationsDir); err != nil {
		log.Fatalf("Database migration failed: %v", err)
	}

	// Select the pluggable execution backend. The real backend builds a
	// per-runtime Docker image from the uploaded code and runs the handler in
	// a constrained container; the mock is an in-process echo used for dev and
	// tests.
	var executor pkg.Executor
	switch getenv("PROVISIONER", "mock") {
	case "mock":
		executor = pkg.NewMockExecutor()
	case "docker":
		executor = pkg.NewDockerExecutor(pkg.DockerExecutorConfig{
			ImagePrefix:  getenv("PROMETHEUS_IMAGE_PREFIX", "olympus/prometheus-fn"),
			BuildTimeout: time.Duration(getEnvInt("PROMETHEUS_BUILD_TIMEOUT_MS", 120000)) * time.Millisecond,
			DockerBinary: getenv("DOCKER_BIN", "docker"),
		})
	default:
		log.Fatalf("unknown PROVISIONER %q (want mock|docker)", getenv("PROVISIONER", "mock"))
	}

	prom := pkg.NewPrometheus(dbClient, executor)
	ch := handler.NewPrometheusHandler(prom)

	// Every control-plane request is authorized against Themis: the bearer JWT
	// is verified locally, then the action/resource is checked with Themis
	// /authorize (fail closed - no token, deny, or Themis outage all reject).
	authzClient := authz.NewClient(getenv("THEMIS_URL", "http://localhost:8091"), getenv("THEMIS_JWT_SECRET", ""))
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler(dbClient, executor))
	mux.Handle("/", authzClient.Middleware(authz.ServiceMapper("prometheus"))(ch.Router()))

	log.Printf("🔥 Prometheus running on %s (executor: %T)...", *addr, executor)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("Could not start server: %v", err)
	}
}

// healthHandler reports connectivity to the configured dependencies.
func healthHandler(dbClient *database.Client, executor pkg.Executor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := dbClient.Ping(r.Context()); err != nil {
			http.Error(w, "PostgreSQL unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := executor.Healthy(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"status":"healthy","postgres":"ok","executor":"ok"}`))
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
