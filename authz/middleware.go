package authz

import (
	"context"
	"net/http"
	"strings"
)

type claimsKey struct{}

// ContextClaims returns the verified claims previously injected by the
// middleware, or nil when the request did not go through it.
func ContextClaims(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey{}).(*Claims)
	return c
}

// Middleware enforces Themis policy on every request it wraps. It is fail
// closed: a request with no/invalid bearer token gets 401, a request that
// Themis denies gets 403, and a request Themis cannot evaluate gets 503. On
// success the tenant header (X-Account-Id) is bound to the token's account and
// the claims are made available via ContextClaims.
func (c *Client) Middleware(mapper Mapper) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || len(auth) <= len("Bearer ") {
				http.Error(w, "authorization required: missing bearer token", http.StatusUnauthorized)
				return
			}
			claims, err := c.Verify(strings.TrimPrefix(auth, "Bearer "))
			if err != nil {
				http.Error(w, "authorization required: "+err.Error(), http.StatusUnauthorized)
				return
			}

			action, resource := mapper(r)
			if action == "" || resource == "" {
				http.Error(w, "forbidden: no authorization mapping for request", http.StatusForbidden)
				return
			}

			allowed, err := c.Authorize(r.Context(), claims.AccountID, claims.ProjectID, claims.PrincipalType, claims.Subject, action, resource)
			if err != nil {
				http.Error(w, "authorization unavailable: "+err.Error(), http.StatusServiceUnavailable)
				return
			}
			if !allowed {
				http.Error(w, "forbidden: "+claims.Subject+" may not "+action+" on "+resource, http.StatusForbidden)
				return
			}

			r.Header.Set("X-Account-Id", claims.AccountID)
			ctx := context.WithValue(r.Context(), claimsKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
