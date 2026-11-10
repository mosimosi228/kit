package iam

import (
	"encoding/json"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	TokenIssuer = "iam"
	AlgRS256    = "RS256"
)

// Tokens holds the access and refresh token pair issued on register, login, or refresh.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// Page is the standard paginated list envelope returned by IAM list endpoints.
type Page[T any] struct {
	Lists []T   `json:"lists"`
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
	Total int64 `json:"total"`
}

// RegisterRequest configures an atomic user registration call (POST /api/v1/register).
// OrgID and RoleID default to Client.OrgID and Client.RoleID when omitted.
type RegisterRequest struct {
	ID       string `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgID    string `json:"org_id,omitempty"`
	RoleID   string `json:"role_id,omitempty"`
}

// Claims represents the verified RS256 JWT payload issued by IAM.
type Claims struct {
	jwt.RegisteredClaims
	OrgID string   `json:"org_id"`
	Roles []string `json:"roles"`
	SID   string   `json:"sid"`
}

// UserID returns the authenticated user's UUID from the sub claim.
func (c *Claims) UserID() string {
	if c == nil {
		return ""
	}
	return c.Subject
}

// Role returns the primary role name in the organization from the roles claim.
func (c *Claims) Role() string {
	if c == nil || len(c.Roles) == 0 {
		return ""
	}
	return c.Roles[0]
}

// JWK is a single JSON Web Key from GET /api/v1/jwks.
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS is the JSON Web Key Set returned by GET /api/v1/jwks.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// Role represents a project role in IAM.
type Role struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	IsSystem  bool   `json:"is_system"`
}

// RolePermission represents a permission string assigned to a role.
type RolePermission struct {
	RoleID     string `json:"role_id"`
	Permission string `json:"permission"`
}

// ListRolePermissionsFilter filters GET /api/v1/role-permissions.
type ListRolePermissionsFilter struct {
	RoleID   string
	RoleName string
	Page     int
	Limit    int
}

// Membership describes a user's organization and role membership inside a project.
type Membership struct {
	ID       string `json:"id"`
	OrgID    string `json:"org_id"`
	OrgName  string `json:"org_name"`
	RoleID   string `json:"role_id"`
	RoleName string `json:"role_name"`
	Status   string `json:"status"`
}

// User represents a project-scoped user returned by GET /api/v1/users and GET /api/v1/users/{id}.
type User struct {
	ID            string       `json:"id"`
	Username      string       `json:"username"`
	Email         string       `json:"email"`
	EmailVerified bool         `json:"email_verified"`
	Status        string       `json:"status"`
	CreatedAt     time.Time    `json:"created_at"`
	LastLoginAt   *time.Time   `json:"last_login_at"`
	Memberships   []Membership `json:"memberships,omitempty"`
}

// ListUsersFilter filters GET /api/v1/users for the project.
type ListUsersFilter struct {
	OrgID  string
	Status string
	Search string
	IDs    []string
	Page   int
	Limit  int
}

// TimelineEntry represents a user activity event in the project audit log.
type TimelineEntry struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	ProjectName string          `json:"project_name,omitempty"`
	UserID      *string         `json:"user_id"`
	UserEmail   string          `json:"user_email,omitempty"`
	Event       string          `json:"event"`
	Message     string          `json:"message"`
	Meta        json.RawMessage `json:"meta"`
	CreatedAt   time.Time       `json:"created_at"`
}

// CreateTimelineRequest is the payload for POST /api/v1/timeline.
type CreateTimelineRequest struct {
	UserID  string         `json:"user_id"`
	Event   string         `json:"event"`
	Message string         `json:"message,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

// ListTimelineFilter filters GET /api/v1/timeline.
type ListTimelineFilter struct {
	UserID string
	Page   int
	Limit  int
}
