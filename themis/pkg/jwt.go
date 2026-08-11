package pkg

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Token claims minted by Themis for an authenticated principal.
type TokenClaims struct {
	Issuer        string    `json:"iss"`
	Subject       string    `json:"sub"`
	PrincipalType string    `json:"principal_type"`
	AccountID     string    `json:"account"`
	ProjectID     string    `json:"project"`
	IssuedAt      time.Time `json:"iat"`
	ExpiresAt     time.Time `json:"exp"`
}

// TokenTTL is how long minted tokens remain valid.
const TokenTTL = 1 * time.Hour

// JWT implements minimal HS256-signed tokens with only stdlib dependencies.
type JWT struct {
	secret []byte
	now    func() time.Time
}

// NewJWT creates a signer/verifier bound to the given HMAC secret.
func NewJWT(secret string) *JWT {
	return &JWT{secret: []byte(secret), now: time.Now}
}

// NewRandomSecret returns a cryptographically random string suitable for a
// one-shot signing secret.
func NewRandomSecret(n int) string {
	return randomString(secretAlphabet, n)
}

// Sign issues a signed JWT for the given claims. The token embeds an expiry.
func (j *JWT) Sign(c TokenClaims) (string, error) {
	now := j.now().UTC()
	if c.IssuedAt.IsZero() {
		c.IssuedAt = now
	}
	if c.ExpiresAt.IsZero() {
		c.ExpiresAt = now.Add(TokenTTL)
	}
	header := `{"alg":"HS256","typ":"JWT"}`
	headerB64 := base64.RawURLEncoding.EncodeToString([]byte(header))
	claimsJSON, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerB64 + "." + claimsB64
	mac := hmac.New(sha256.New, j.secret)
	_, _ = mac.Write([]byte(signingInput))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sigB64, nil
}

// Verify checks the signature and expiry, returning the decoded claims.
func (j *JWT) Verify(token string) (*TokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed token")
	}
	// Constant-time signature check.
	mac := hmac.New(sha256.New, j.secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	expect := mac.Sum(nil)
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, expect) {
		return nil, fmt.Errorf("invalid signature")
	}
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}
	var c TokenClaims
	if err := json.Unmarshal(claimsJSON, &c); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}
	if !c.ExpiresAt.After(j.now().UTC()) {
		return nil, fmt.Errorf("token expired")
	}
	return &c, nil
}
