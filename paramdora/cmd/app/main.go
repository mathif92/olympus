package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mathif92/olympus/authz"
	"github.com/mathif92/olympus/paramdora/internal/handler"
	"github.com/mathif92/olympus/paramdora/pkg"
	"github.com/mathif92/olympus/paramdora/pkg/database"
)

func main() {
	var (
		addr = flag.String("addr", getenv("PARAMDORA_ADDR", ":8083"), "listen address")
	)
	flag.Parse()

	dbCfg := database.Config{
		PostgresURL: getenv("POSTGRES_DSN",
			"host=localhost port=15433 user=olympus password=olympus_secret dbname=olympus_parameters sslmode=disable"),
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

	// Build the value-encryption cipher. When no master key is configured a
	// random one-shot key is used (secure values are unreadable after restart).
	masterKey := getenv("PARAMDORA_MASTER_KEY", getenv("KEY_ENC_MASTER", ""))
	cipher, err := pkg.NewCipher(masterKey)
	if err != nil {
		log.Fatalf("Cipher initialization failed: %v", err)
	}
	if masterKey == "" {
		log.Printf("⚠️  no PARAMDORA_MASTER_KEY set — using a random one-shot encryption key")
	}

	store := pkg.NewParamStore(dbClient, cipher)
	ph := handler.NewParamdoraHandler(store)

	// Every control-plane request is authorized against Themis: the bearer JWT
	// is verified locally, then the action/resource is checked with Themis
	// /authorize (fail closed - no token, deny, or Themis outage all reject).
	authzClient := authz.NewClient(getenv("THEMIS_URL", "http://localhost:8091"), getenv("THEMIS_JWT_SECRET", ""))
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler(dbClient))
	mux.Handle("/", authzClient.Middleware(authz.ServiceMapper("paramdora"))(ph.Router()))

	log.Printf("🪷 Paramdora running on %s (db: %s)...", *addr, dbCfg.PostgresURL)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("Could not start server: %v", err)
	}
}

// healthHandler reports connectivity to the configured dependencies.
func healthHandler(dbClient *database.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := dbClient.Ping(r.Context()); err != nil {
			http.Error(w, "PostgreSQL unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"status":"healthy","postgres":"ok"}`))
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
