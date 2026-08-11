package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to Themis to make policy decisions. It is used by service
// middleware to check whether a principal may perform an action on a resource.
type Client struct {
	themisURL string
	verifier  *Verifier
	http      *http.Client
}

// NewClient builds a client bound to the Themis service and the shared JWT
// secret. ThemisURL should be the base URL (e.g. http://localhost:8091).
func NewClient(themisURL, jwtSecret string) *Client {
	return &Client{
		themisURL: strings.TrimRight(themisURL, "/"),
		verifier:  NewVerifier(jwtSecret),
		http:      &http.Client{Timeout: 5 * time.Second},
	}
}

// Verify validates a Themis token and returns its claims.
func (c *Client) Verify(token string) (*Claims, error) {
	return c.verifier.Verify(token)
}

// Authorize asks Themis whether the principal may perform action on resource
// within account/project. project may be a project name or the project ID that
// appears in the token claims. It returns an error if Themis cannot be reached
// or rejects the evaluation (fail closed).
func (c *Client) Authorize(ctx context.Context, accountID, project, principalType, principalName, action, resource string) (bool, error) {
	body, err := json.Marshal(map[string]string{
		"project":        project,
		"principal_type": principalType,
		"principal_name": principalName,
		"action":         action,
		"resource":       resource,
	})
	if err != nil {
		return false, err
	}
	u, err := url.JoinPath(c.themisURL, "authorize")
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if accountID != "" {
		req.Header.Set("X-Account-Id", accountID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("themis authorize unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return false, fmt.Errorf("themis authorize failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var decision struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decision); err != nil {
		return false, fmt.Errorf("themis authorize invalid response: %w", err)
	}
	return decision.Allowed, nil
}

// ErrNoToken is returned by Middleware when a request carries no bearer token.
var ErrNoToken = errors.New("missing bearer token")

// Mapper derives the IAM action and resource for an incoming request. Services
// implement it against their own routes, e.g. action "iris:POST" on resource
// "/queues".
type Mapper func(r *http.Request) (action, resource string)

// ServiceMapper returns a Mapper that derives action "<service>:<METHOD>" (e.g.
// "iris:POST") and resource "<request path>" (e.g. "/queues") for each request.
// Policy documents can then use AWS-style wildcards like "iris:*" on "/queues/*".
func ServiceMapper(service string) Mapper {
	return func(r *http.Request) (string, string) {
		return service + ":" + r.Method, r.URL.Path
	}
}
