package database

import "time"

// Parameter types supported by Paramdora.
const (
	TypeString       = "string"
	TypeStringList   = "string_list"
	TypeSecureString = "secure_string"
)

// Account represents a tenant in the parameter store.
type Account struct {
	ID             string    `json:"id"`
	DisplayName    string    `json:"display_name"`
	Email          string    `json:"email"`
	Plan           string    `json:"plan"`
	ParameterLimit int       `json:"parameter_limit"`
	UsedParameters int       `json:"used_parameters"`
	CreatedAt      time.Time `json:"created_at"`
	Status         string    `json:"status"`
}

// Project represents a named namespace of parameters within an account.
type Project struct {
	ID             string    `json:"id"`
	AccountID      string    `json:"account_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	ParameterCount int       `json:"parameter_count"`
	CreatedAt      time.Time `json:"created_at"`
	Status         string    `json:"status"`
}

// Parameter represents a key/value entry with versioning.
type Parameter struct {
	ID             string            `json:"id"`
	ProjectID      string            `json:"project_id"`
	Name           string            `json:"name"`
	Value          string            `json:"value"`
	Type           string            `json:"data_type"`
	Description    string            `json:"description"`
	Encrypted      bool              `json:"is_encrypted"`
	Tier           string            `json:"tier"`
	Version        int               `json:"version"`
	KeyID          string            `json:"key_id"`
	Tags           map[string]string `json:"tags"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	LastModifiedBy string            `json:"last_modified_by"`
	Status         string            `json:"status"`
}
