package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mathif92/olympus/themis/internal/handler"
	"github.com/mathif92/olympus/themis/pkg"
	"github.com/mathif92/olympus/themis/pkg/database"
)

func main() {
	var (
		addr = flag.String("addr", getenv("THEMIS_ADDR", ":8091"), "listen address")
	)
	flag.Parse()

	dbCfg := database.Config{
		PostgresURL: getenv("POSTGRES_DSN",
			"host=localhost port=15439 user=olympus password=olympus_secret dbname=olympus_themis sslmode=disable"),
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

	// JWT signing secret for minted tokens. When unset a random one-shot
	// secret is used, invalidating tokens minted before a restart.
	jwtSecret := getenv("THEMIS_JWT_SECRET", "")
	if jwtSecret == "" {
		jwtSecret = pkg.NewRandomSecret(48)
		log.Printf("⚠️  no THEMIS_JWT_SECRET set — using a random one-shot signing secret")
	}
	jwt := pkg.NewJWT(jwtSecret)

	store := pkg.NewThemisStore(dbClient, jwt)
	th := handler.NewThemisHandler(store)

	mux := th.Router()
	mux.HandleFunc("/health", healthHandler(dbClient))

	log.Printf("⚖️  Themis running on %s (db: %s)...", *addr, dbCfg.PostgresURL)
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
