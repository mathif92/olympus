// Package authz verifies Themis-issued JWTs and enforces policy decisions by
// calling the Themis /authorize endpoint. It is shared by every Olympus
// control-plane service so that access is checked uniformly, and it fails
// closed: requests without a valid token, or for which Themis cannot be
// reached, are rejected.
package authz

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Claims are the signed claims in a Themis-issued token. The JSON field names
// must match the claims minted by the Themis service (pkg/jwt.go).
type Claims struct {
	Issuer        string    `json:"iss"`
	Subject       string    `json:"sub"`
	PrincipalType string    `json:"principal_type"`
	AccountID     string    `json:"account"`
	ProjectID     string    `json:"project"`
	IssuedAt      time.Time `json:"iat"`
	ExpiresAt     time.Time `json:"exp"`
}

// Verifier validates Themis-issued HS256 tokens using the shared HMAC secret.
type Verifier struct {
	secret []byte
	now    func() time.Time
}

// NewVerifier creates a token verifier bound to the configured secret. This
// secret must be the same value Themis uses to sign tokens (THEMIS_JWT_SECRET).
func NewVerifier(secret string) *Verifier {
	return &Verifier{secret: []byte(secret), now: time.Now}
}

// Verify checks the token's signature and expiry, returning its claims. It
// rejects malformed tokens, bad signatures, and expired tokens.
func (v *Verifier) Verify(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	mac := hmac.New(sha256.New, v.secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, errors.New("invalid signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid claims encoding: %w", err)
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("invalid claims: %w", err)
	}
	if v.now().UTC().After(c.ExpiresAt) {
		return nil, errors.New("token expired")
	}
	return &c, nil
}
