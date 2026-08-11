package database

import (
	"encoding/json"
	"time"
)

// Account represents a tenant in the serverless functions service.
type Account struct {
	ID            string    `json:"id"`
	DisplayName   string    `json:"display_name"`
	Email         string    `json:"email"`
	Plan          string    `json:"plan"`
	FunctionLimit int       `json:"function_limit"`
	UsedFunctions int       `json:"used_functions"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"`
}

// Project represents a named namespace of functions within an account.
type Project struct {
	ID            string    `json:"id"`
	AccountID     string    `json:"account_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	FunctionCount int       `json:"function_count"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"`
}

// Function represents a serverless function (AWS Lambda equivalent).
type Function struct {
	ID             string            `json:"id"`
	AccountID      string            `json:"account_id"`
	ProjectID      string            `json:"project_id"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Runtime        string            `json:"runtime"`
	Handler        string            `json:"handler"`
	TimeoutMS      int               `json:"timeout_ms"`
	MemoryMB       int               `json:"memory_mb"`
	CPUs           float64           `json:"cpus"`
	EnvVars        map[string]string `json:"env_vars,omitempty"`
	CurrentVersion int               `json:"current_version"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Status         string            `json:"status"`
}

// EnvVarsParam returns a value for the jsonb column: SQL NULL when empty,
// otherwise the JSON document bytes.
func (f Function) EnvVarsParam() any {
	if len(f.EnvVars) == 0 {
		return nil
	}
	b, _ := json.Marshal(f.EnvVars)
	return b
}

// FunctionVersion is an immutable snapshot of deployed function code.
type FunctionVersion struct {
	ID         string    `json:"id"`
	FunctionID string    `json:"function_id"`
	Version    int       `json:"version"`
	Code       []byte    `json:"-"`
	CodeSHA256 string    `json:"code_sha256"`
	CodeSize   int       `json:"code_size"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
}

// Invocation records one handler execution.
type Invocation struct {
	ID         string          `json:"id"`
	FunctionID string          `json:"function_id"`
	Version    int             `json:"version"`
	Status     string          `json:"status"`
	Request    json.RawMessage `json:"request"`
	Response   string          `json:"response,omitempty"`
	Error      string          `json:"error,omitempty"`
	ExitCode   int             `json:"exit_code,omitempty"`
	DurationMS int64           `json:"duration_ms"`
	InvokedAt  time.Time       `json:"invoked_at"`
}
