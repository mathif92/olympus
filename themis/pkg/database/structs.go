package database

import "time"

// Principal types that policies can be attached to.
const (
	PrincipalUser  = "user"
	PrincipalGroup = "group"
	PrincipalRole  = "role"
)

// Account represents a tenant in the identity store.
type Account struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	Plan        string    `json:"plan"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"`
}

// Project represents a named namespace of IAM resources within an account.
type Project struct {
	ID            string    `json:"id"`
	AccountID     string    `json:"account_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	ResourceCount int       `json:"resource_count"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"`
}

// User is an IAM principal that can hold access keys.
type User struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Path        string            `json:"path"`
	Tags        map[string]string `json:"tags"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Status      string            `json:"status"`
}

// Group is a collection of users; policies attached to a group apply to all
// members.
type Group struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Tags        map[string]string `json:"tags"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Status      string            `json:"status"`
}

// Role is a principal that holds policies and can be assumed via a token.
type Role struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Tags        map[string]string `json:"tags"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Status      string            `json:"status"`
}

// Policy is a named, attachable IAM policy document.
type Policy struct {
	ID          string         `json:"id"`
	ProjectID   string         `json:"project_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Document    map[string]any `json:"document"`
	Version     int            `json:"version"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	Status      string         `json:"status"`
}

// GroupMembership is a single user-in-group row.
type GroupMembership struct {
	GroupID   string    `json:"group_id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// PolicyAttachment is a policy attached to a principal.
type PolicyAttachment struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	PrincipalType string    `json:"principal_type"`
	PrincipalID   string    `json:"principal_id"`
	PrincipalName string    `json:"principal_name,omitempty"`
	PolicyID      string    `json:"policy_id"`
	PolicyName    string    `json:"policy_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// AccessKey is an AWS-style credential pair. The secret is never stored; only
// SecretHash is persisted. Secret is populated transiently at creation.
type AccessKey struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	UserID     string    `json:"user_id"`
	UserName   string    `json:"user_name,omitempty"`
	SecretHash string    `json:"-"`
	Secret     string    `json:"secret_access_key,omitempty"`
	Status     string    `json:"status"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
