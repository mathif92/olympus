// Package database provides the database access layer for OlympusStore.
// It handles PostgreSQL connections, caching with Redis, and multi-tenant data access.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/redis/go-redis/v9"
)

// Config holds database connection parameters.
type Config struct {
	PostgresURL string
	RedisURL    string
	PoolMax     int
	PoolMin     int
	PoolTimeout time.Duration
}

// Client wraps database connections for multi-tenant access.
type Client struct {
	DB    *sql.DB
	Cache *redis.Client
}

// NewClient creates a new database client with connection pooling.
func NewClient(cfg Config) (*Client, error) {
	// Connect to PostgreSQL
	db, err := sql.Open("postgres", cfg.PostgresURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	// Set connection pool parameters
	if cfg.PoolMax > 0 {
		db.SetMaxOpenConns(cfg.PoolMax)
	}
	if cfg.PoolMin > 0 {
		db.SetMaxIdleConns(cfg.PoolMin)
	}
	if cfg.PoolTimeout > 0 {
		db.SetConnMaxLifetime(cfg.PoolTimeout)
		db.SetConnMaxIdleTime(cfg.PoolTimeout / 2)
	}

	// Test connection, retrying briefly to tolerate databases that are still
	// warming up (e.g. freshly started containers or just-restarted servers).
	if err := pingWithRetry(db); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisURL,
		PoolSize: 10,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return &Client{
		DB:    db,
		Cache: rdb,
	}, nil
}

// Close closes all database connections.
func (c *Client) Close() error {
	if c.DB != nil {
		c.DB.Close()
	}
	if c.Cache != nil {
		c.Cache.Close()
	}
	return nil
}

// pingWithRetry attempts to ping the database several times with a short
// backoff to tolerate services still warming up.
func pingWithRetry(db *sql.DB) error {
	const attempts = 10
	backoff := 300 * time.Millisecond
	var lastErr error
	for i := 0; i < attempts; i++ {
		if lastErr = db.Ping(); lastErr == nil {
			return nil
		}
		time.Sleep(backoff)
		backoff *= 2
		if backoff > 3*time.Second {
			backoff = 3 * time.Second
		}
	}
	return lastErr
}

// Account represents an account/tenant in the storage system.
type Account struct {
	ID           string    `json:"id"`
	DisplayName  string    `json:"display_name"`
	Email        string    `json:"email"`
	Plan         string    `json:"plan"`
	StorageLimit int64     `json:"storage_limit_bytes"`
	UsedStorage  int64     `json:"used_storage_bytes"`
	CreatedAt    time.Time `json:"created_at"`
	Status       string    `json:"status"`
}

// Space represents a bucket/space for an account.
type Space struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"account_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	MaxSize     int64     `json:"max_size_bytes"`
	CurrentSize int64     `json:"current_size_bytes"`
	ObjectCount int64     `json:"object_count"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"`
}

// Object represents a stored object with metadata.
type Object struct {
	ID            string    `json:"id"`
	SpaceID       string    `json:"space_id"`
	KeyPath       string    `json:"key_path"`
	OriginalName  string    `json:"original_filename"`
	ContentType   string    `json:"content_type"`
	ContentLength int64     `json:"content_length"`
	ETag          string    `json:"etag"`
	VersionID     string    `json:"version_id"`
	Checksum      string    `json:"checksum_value"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"`
}

// Ping verifies database (PostgreSQL) connectivity.
func (c *Client) Ping(ctx context.Context) error {
	return c.DB.PingContext(ctx)
}

// QueryRow executes a query that is expected to return at most one row.
func (c *Client) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return c.DB.QueryRowContext(ctx, query, args...)
}

// Exec executes a query without returning rows.
func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return c.DB.ExecContext(ctx, query, args...)
}
