package iam

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

// Allows checks whether roleName in the project has the given permission string
// by querying GET /api/v1/role-permissions?project_id=...&role_name=... in a single request.
func (c *Client) Allows(ctx context.Context, roleName, permission string) (bool, error) {
	if c == nil {
		return false, ErrNotConfigured
	}
	roleName = strings.TrimSpace(roleName)
	permission = strings.TrimSpace(permission)
	if roleName == "" || permission == "" {
		return false, nil
	}
	page, err := c.ListRolePermissions(ctx, ListRolePermissionsFilter{
		RoleName: roleName,
		Limit:    500,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	for _, row := range page.Lists {
		if row.Permission == permission {
			return true, nil
		}
	}
	return false, nil
}

// RolePermissions returns all permission strings assigned to roleName in the project.
func (c *Client) RolePermissions(ctx context.Context, roleName string) ([]string, error) {
	page, err := c.ListRolePermissions(ctx, ListRolePermissionsFilter{
		RoleName: roleName,
		Limit:    500,
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(page.Lists))
	for _, row := range page.Lists {
		out = append(out, row.Permission)
	}
	return out, nil
}

// ListRolePermissions fetches a page of permissions for a role (by RoleID or RoleName)
// via GET /api/v1/role-permissions with X-Client-ID and X-Client-Secret headers.
func (c *Client) ListRolePermissions(ctx context.Context, filter ListRolePermissionsFilter) (Page[RolePermission], error) {
	if c == nil {
		return Page[RolePermission]{}, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("project_id", c.ProjectID)
	if id := strings.TrimSpace(filter.RoleID); id != "" {
		q.Set("role_id", id)
	}
	if name := strings.TrimSpace(filter.RoleName); name != "" {
		q.Set("role_name", name)
	}
	applyPagination(q, filter.Page, filter.Limit)

	var page Page[RolePermission]
	err := c.get(ctx, "/api/v1/role-permissions", q, true, &page)
	return page, err
}

// GetRolePermission fetches a single role permission via GET /api/v1/role-permissions/{role_id}.
func (c *Client) GetRolePermission(ctx context.Context, roleID, permission string) (RolePermission, error) {
	if c == nil {
		return RolePermission{}, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("project_id", c.ProjectID)
	q.Set("permission", strings.TrimSpace(permission))

	var rp RolePermission
	err := c.get(ctx, "/api/v1/role-permissions/"+url.PathEscape(strings.TrimSpace(roleID)), q, true, &rp)
	return rp, err
}

// ListRoles fetches a page of project roles via GET /api/v1/roles.
func (c *Client) ListRoles(ctx context.Context, page, limit int) (Page[Role], error) {
	if c == nil {
		return Page[Role]{}, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("project_id", c.ProjectID)
	applyPagination(q, page, limit)

	var out Page[Role]
	err := c.get(ctx, "/api/v1/roles", q, true, &out)
	return out, err
}

// GetRole fetches a single project role by ID via GET /api/v1/roles/{id}.
func (c *Client) GetRole(ctx context.Context, roleID string) (Role, error) {
	if c == nil {
		return Role{}, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("project_id", c.ProjectID)

	var role Role
	err := c.get(ctx, "/api/v1/roles/"+url.PathEscape(strings.TrimSpace(roleID)), q, true, &role)
	return role, err
}
