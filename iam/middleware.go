package iam

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type claimsContextKey struct{}

// WithClaims stores verified IAM *Claims in ctx.
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsContextKey{}, claims)
}

// ClaimsFromContext retrieves verified IAM *Claims from ctx.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	if ctx == nil {
		return nil, false
	}
	claims, ok := ctx.Value(claimsContextKey{}).(*Claims)
	if !ok || claims == nil {
		return nil, false
	}
	return claims, true
}

// BearerToken extracts the Bearer token from the Authorization header.
func BearerToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// Middleware verifies the Authorization: Bearer <access_token> header using the project's JWKS
// and stores *Claims in the request context.
func (c *Client) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := BearerToken(r)
		if raw == "" {
			writeJSONError(w, http.StatusUnauthorized, CodeInvalidGrant, "access token is required")
			return
		}
		claims, err := c.VerifyClaims(r.Context(), raw)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, CodeInvalidGrant, "access token is invalid")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
	})
}

// RequireRole returns an HTTP middleware that ensures the authenticated user has one of the given role names.
func (c *Client) RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		if r := strings.TrimSpace(role); r != "" {
			allowed[r] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := c.ensureClaims(w, r)
			if !ok {
				return
			}
			if _, ok := allowed[claims.Role()]; !ok {
				writeJSONError(w, http.StatusForbidden, "forbidden", "insufficient role")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
		})
	}
}

// RequirePermission returns an HTTP middleware that checks whether the user's JWT role
// has the specified permission via Client.Allows.
func (c *Client) RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := c.ensureClaims(w, r)
			if !ok {
				return
			}
			okPerm, err := c.Allows(r.Context(), claims.Role(), permission)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, CodeInternal, "internal error")
				return
			}
			if !okPerm {
				writeJSONError(w, http.StatusForbidden, "forbidden", "permission denied")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
		})
	}
}

func (c *Client) ensureClaims(w http.ResponseWriter, r *http.Request) (*Claims, bool) {
	if claims, ok := ClaimsFromContext(r.Context()); ok {
		return claims, true
	}
	raw := BearerToken(r)
	if raw == "" {
		writeJSONError(w, http.StatusUnauthorized, CodeInvalidGrant, "access token is required")
		return nil, false
	}
	claims, err := c.VerifyClaims(r.Context(), raw)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, CodeInvalidGrant, "access token is invalid")
		return nil, false
	}
	return claims, true
}

func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	statusText := http.StatusText(status)
	switch status {
	case http.StatusUnauthorized:
		statusText = "Unauthorized!"
	case http.StatusForbidden:
		statusText = "Forbidden!"
	case http.StatusInternalServerError:
		statusText = "Internal server error!"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Error{
		StatusCode: status,
		Status:     statusText,
		Code:       code,
		Message:    msg,
	})
}
