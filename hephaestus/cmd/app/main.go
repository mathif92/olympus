package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mathif92/olympus/authz"
	"github.com/mathif92/olympus/hephaestus/internal/handler"
	"github.com/mathif92/olympus/hephaestus/pkg"
	"github.com/mathif92/olympus/hephaestus/pkg/database"
)

func main() {
	var (
		addr = flag.String("addr", getenv("HEPHAESTUS_ADDR", ":8084"), "listen address")
	)
	flag.Parse()

	dbCfg := database.Config{
		PostgresURL: getenv("POSTGRES_DSN",
			"host=localhost port=15434 user=olympus password=olympus_secret dbname=olympus_compute sslmode=disable"),
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

	// Select the pluggable data-plane provisioner (mock by default).
	var provisioner pkg.Provisioner
	switch getenv("PROVISIONER", "mock") {
	case "mock":
		provisioner = pkg.NewMockProvisioner()
	default:
		log.Fatalf("unknown PROVISIONER %q (only 'mock' ships today)", getenv("PROVISIONER", "mock"))
	}

	compute := pkg.NewHephaestus(dbClient, provisioner)
	ch := handler.NewComputeHandler(compute)

	// Every control-plane request is authorized against Themis: the bearer JWT
	// is verified locally, then the action/resource is checked with Themis
	// /authorize (fail closed - no token, deny, or Themis outage all reject).
	authzClient := authz.NewClient(getenv("THEMIS_URL", "http://localhost:8091"), getenv("THEMIS_JWT_SECRET", ""))
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler(dbClient, provisioner))
	mux.Handle("/", authzClient.Middleware(authz.ServiceMapper("hephaestus"))(ch.Router()))

	log.Printf("🏛️  Hephaestus running on %s (provisioner: %T)...", *addr, provisioner)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("Could not start server: %v", err)
	}
}

// healthHandler reports connectivity to the configured dependencies.
func healthHandler(dbClient *database.Client, provisioner pkg.Provisioner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := dbClient.Ping(r.Context()); err != nil {
			http.Error(w, "PostgreSQL unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := provisioner.Healthy(r.Context()); err != nil {
			http.Error(w, "provisioner unavailable: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"status":"healthy","postgres":"ok","provisioner":"ok"}`))
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
