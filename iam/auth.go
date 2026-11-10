package iam

import (
	"context"
	"strings"
)

// Register atomically registers or links a user in the client's default organization and role
// via POST /api/v1/register and returns an access_token and refresh_token pair.
func (c *Client) Register(ctx context.Context, email, password string) (Tokens, error) {
	return c.RegisterWith(ctx, RegisterRequest{
		Email:    email,
		Password: password,
	})
}

// RegisterWith atomically registers or links a user via POST /api/v1/register,
// supporting optional migration UUID (ID), custom Username, and per-call OrgID / RoleID overrides.
func (c *Client) RegisterWith(ctx context.Context, in RegisterRequest) (Tokens, error) {
	if c == nil {
		return Tokens{}, ErrNotConfigured
	}
	orgID := strings.TrimSpace(in.OrgID)
	if orgID == "" {
		orgID = c.OrgID
	}
	roleID := strings.TrimSpace(in.RoleID)
	if roleID == "" {
		roleID = c.RoleID
	}
	payload := map[string]string{
		"project_id":    c.ProjectID,
		"client_id":     c.ClientID,
		"client_secret": c.ClientSecret,
		"org_id":        orgID,
		"role_id":       roleID,
		"email":         in.Email,
		"password":      in.Password,
	}
	if id := strings.TrimSpace(in.ID); id != "" {
		payload["id"] = id
	}
	if username := strings.TrimSpace(in.Username); username != "" {
		payload["username"] = username
	}
	var tokens Tokens
	err := c.post(ctx, "/api/v1/register", payload, false, &tokens)
	return tokens, err
}

// Login authenticates a user in the client's default organization via POST /api/v1/login
// and returns an access_token (15m) and refresh_token (30d).
func (c *Client) Login(ctx context.Context, email, password string) (Tokens, error) {
	if c == nil {
		return Tokens{}, ErrNotConfigured
	}
	return c.LoginInOrg(ctx, c.OrgID, email, password)
}

// LoginInOrg authenticates a user in a specific organization (falling back to Client.OrgID if orgID is empty).
func (c *Client) LoginInOrg(ctx context.Context, orgID, email, password string) (Tokens, error) {
	if c == nil {
		return Tokens{}, ErrNotConfigured
	}
	org := strings.TrimSpace(orgID)
	if org == "" {
		org = c.OrgID
	}
	var tokens Tokens
	err := c.post(ctx, "/api/v1/login", map[string]string{
		"project_id":    c.ProjectID,
		"client_id":     c.ClientID,
		"client_secret": c.ClientSecret,
		"org_id":        org,
		"email":         email,
		"password":      password,
	}, false, &tokens)
	return tokens, err
}

// Refresh rotates a single-use refresh token via POST /api/v1/token/refresh and returns a new token pair.
func (c *Client) Refresh(ctx context.Context, refresh string) (Tokens, error) {
	if c == nil {
		return Tokens{}, ErrNotConfigured
	}
	var tokens Tokens
	err := c.post(ctx, "/api/v1/token/refresh", map[string]string{
		"project_id":    c.ProjectID,
		"client_id":     c.ClientID,
		"client_secret": c.ClientSecret,
		"refresh_token": refresh,
	}, false, &tokens)
	return tokens, err
}

// Logout revokes the session associated with the refresh token via POST /api/v1/logout.
func (c *Client) Logout(ctx context.Context, refresh string) error {
	if c == nil {
		return ErrNotConfigured
	}
	return c.post(ctx, "/api/v1/logout", map[string]string{
		"project_id":    c.ProjectID,
		"client_id":     c.ClientID,
		"client_secret": c.ClientSecret,
		"refresh_token": refresh,
	}, false, nil)
}
