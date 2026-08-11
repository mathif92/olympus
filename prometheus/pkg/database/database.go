// Package database provides the PostgreSQL access layer for Prometheus.
// It handles connection pooling and multi-tenant serverless function data.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// Config holds database connection parameters.
type Config struct {
	PostgresURL string
	PoolMax     int
	PoolMin     int
	PoolTimeout time.Duration
}

// Client wraps the PostgreSQL connection for multi-tenant access.
type Client struct {
	DB *sql.DB
}

// NewClient creates a new database client with connection pooling.
func NewClient(cfg Config) (*Client, error) {
	db, err := sql.Open("postgres", cfg.PostgresURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

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

	if err := pingWithRetry(db); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return &Client{DB: db}, nil
}

// Close closes the database connection.
func (c *Client) Close() error {
	if c.DB != nil {
		return c.DB.Close()
	}
	return nil
}

// Ping verifies database connectivity.
func (c *Client) Ping(ctx context.Context) error {
	return c.DB.PingContext(ctx)
}

// QueryRow executes a query that is expected to return at most one row.
func (c *Client) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return c.DB.QueryRowContext(ctx, query, args...)
}

// Query executes a query that returns rows.
func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return c.DB.QueryContext(ctx, query, args...)
}

// Exec executes a query without returning rows.
func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return c.DB.ExecContext(ctx, query, args...)
}

// Begin starts a new transaction.
func (c *Client) Begin(ctx context.Context) (*sql.Tx, error) {
	return c.DB.BeginTx(ctx, nil)
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
