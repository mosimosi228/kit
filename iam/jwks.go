package iam

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWKS fetches the current public key set for the project from GET /api/v1/jwks.
func (c *Client) JWKS(ctx context.Context) (JWKS, error) {
	if c == nil {
		return JWKS{}, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("project_id", c.ProjectID)
	var set JWKS
	if err := c.get(ctx, "/api/v1/jwks", q, false, &set); err != nil {
		return JWKS{}, err
	}
	return set, nil
}

// PublicKey returns the RSA public key for kid.
// It checks the in-memory JWKS cache first and automatically refreshes from GET /api/v1/jwks
// when the cache is expired or kid is unknown (e.g. after key rotation in the IAM console).
func (c *Client) PublicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if c == nil {
		return nil, ErrNotConfigured
	}
	if kid == "" {
		return nil, fmt.Errorf("%w: empty kid", ErrKeyNotFound)
	}

	now := time.Now()
	c.mu.RLock()
	if now.Before(c.keysExpAt) && c.keys != nil {
		if key, ok := c.keys[kid]; ok {
			c.mu.RUnlock()
			return key, nil
		}
	}
	c.mu.RUnlock()

	set, err := c.JWKS(ctx)
	if err != nil {
		return nil, err
	}

	parsed := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		pub, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			return nil, err
		}
		parsed[k.Kid] = pub
	}

	c.mu.Lock()
	c.keys = parsed
	c.keysExpAt = time.Now().Add(c.jwksTTL())
	c.mu.Unlock()

	key, ok := parsed[kid]
	if !ok {
		return nil, fmt.Errorf("%w: kid %s is not in jwks", ErrKeyNotFound, kid)
	}
	return key, nil
}

// InvalidateJWKS clears the cached JWKS keys so the next verification reloads from IAM.
func (c *Client) InvalidateJWKS() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.keys = nil
	c.keysExpAt = time.Time{}
	c.mu.Unlock()
}

// Verify validates an RS256 access token against the project's JWKS, issuer ("iam"),
// and audience (ProjectID), returning the raw jwt.MapClaims as shown in the IAM documentation.
func (c *Client) Verify(ctx context.Context, access string) (jwt.MapClaims, error) {
	if c == nil {
		return nil, ErrNotConfigured
	}
	if access == "" {
		return nil, ErrInvalidToken
	}
	token, err := jwt.Parse(access, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodRS256 {
			return nil, fmt.Errorf("%w: %v", ErrUnexpectedMethod, token.Header["alg"])
		}
		kid, _ := token.Header["kid"].(string)
		return c.PublicKey(ctx, kid)
	}, jwt.WithIssuer(TokenIssuer), jwt.WithAudience(c.ProjectID), jwt.WithValidMethods([]string{AlgRS256}))
	if err != nil {
		return nil, mapJWTError(err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// VerifyClaims validates an RS256 access token and returns typed *Claims.
func (c *Client) VerifyClaims(ctx context.Context, access string) (*Claims, error) {
	if c == nil {
		return nil, ErrNotConfigured
	}
	if access == "" {
		return nil, ErrInvalidToken
	}
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(access, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodRS256 {
			return nil, fmt.Errorf("%w: %v", ErrUnexpectedMethod, token.Header["alg"])
		}
		kid, _ := token.Header["kid"].(string)
		return c.PublicKey(ctx, kid)
	}, jwt.WithIssuer(TokenIssuer), jwt.WithAudience(c.ProjectID), jwt.WithValidMethods([]string{AlgRS256}))
	if err != nil {
		return nil, mapJWTError(err)
	}
	if !token.Valid || claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// Caller extracts userID (sub), orgID (org_id), and the primary role name (roles[0])
// from jwt.MapClaims returned by Verify.
func Caller(claims jwt.MapClaims) (userID, orgID, role string) {
	if claims == nil {
		return "", "", ""
	}
	userID, _ = claims["sub"].(string)
	orgID, _ = claims["org_id"].(string)
	switch roles := claims["roles"].(type) {
	case []any:
		if len(roles) > 0 {
			role, _ = roles[0].(string)
		}
	case []string:
		if len(roles) > 0 {
			role = roles[0]
		}
	}
	return userID, orgID, role
}

func parseRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nRaw, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("iam jwks: decode modulus: %w", err)
	}
	eRaw, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("iam jwks: decode exponent: %w", err)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nRaw),
		E: int(new(big.Int).SetBytes(eRaw).Int64()),
	}, nil
}

func mapJWTError(err error) error {
	if errors.Is(err, jwt.ErrTokenExpired) {
		return fmt.Errorf("%w: %v", ErrTokenExpired, err)
	}
	if errors.Is(err, ErrUnexpectedMethod) || errors.Is(err, ErrKeyNotFound) {
		return err
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrInvalidToken, err)
}
