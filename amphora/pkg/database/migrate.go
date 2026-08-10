package database

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

// Migrate applies all pending goose migrations from dir in order.
func Migrate(db *sql.DB, dir string) error {
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	return goose.Up(db, dir)
}

// Rollback rolls back the most recent goose migration from dir.
func Rollback(db *sql.DB, dir string) error {
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	return goose.Down(db, dir)
}
