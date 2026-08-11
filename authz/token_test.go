package authz

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

const testSecret = "test-secret-value"

func sign(t *testing.T, secret string, c Claims) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, _ := json.Marshal(c)
	hb := base64.RawURLEncoding.EncodeToString(header)
	pb := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(hb + "." + pb))
	return hb + "." + pb + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestVerifyValid(t *testing.T) {
	tok := sign(t, testSecret, Claims{
		Issuer:        "themis",
		Subject:       "alice",
		PrincipalType: "user",
		AccountID:     "default",
		ProjectID:     "1f204086478bc922ea2e3a93cb012464",
		ExpiresAt:     time.Now().Add(time.Hour),
	})
	c, err := NewVerifier(testSecret).Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if c.Subject != "alice" || c.ProjectID == "" {
		t.Fatalf("unexpected claims: %+v", c)
	}
}

func TestVerifyWrongSecret(t *testing.T) {
	tok := sign(t, testSecret, Claims{Subject: "alice", ExpiresAt: time.Now().Add(time.Hour)})
	if _, err := NewVerifier("other-secret").Verify(tok); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestVerifyExpired(t *testing.T) {
	tok := sign(t, testSecret, Claims{Subject: "alice", ExpiresAt: time.Now().Add(-time.Minute)})
	if _, err := NewVerifier(testSecret).Verify(tok); err == nil {
		t.Fatal("expected expiry error")
	}
}

func TestVerifyMalformed(t *testing.T) {
	if _, err := NewVerifier(testSecret).Verify("not-a-jwt"); err == nil {
		t.Fatal("expected malformed error")
	}
}
