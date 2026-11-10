package iam

import (
	"context"
	"net/url"
	"strings"
)

// GetUser fetches a project-scoped user (including their project memberships)
// via GET /api/v1/users/{id}?project_id=... with X-Client-ID and X-Client-Secret headers.
func (c *Client) GetUser(ctx context.Context, userID string) (User, error) {
	if c == nil {
		return User{}, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("project_id", c.ProjectID)

	var user User
	err := c.get(ctx, "/api/v1/users/"+url.PathEscape(strings.TrimSpace(userID)), q, true, &user)
	return user, err
}

// ListUsers fetches a paginated list of users attached to the project
// via GET /api/v1/users?project_id=... with X-Client-ID and X-Client-Secret headers.
// Supports filtering by Search (email/username), Status, OrgID, and IDs.
func (c *Client) ListUsers(ctx context.Context, filter ListUsersFilter) (Page[User], error) {
	if c == nil {
		return Page[User]{}, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("project_id", c.ProjectID)
	if orgID := strings.TrimSpace(filter.OrgID); orgID != "" {
		q.Set("org_id", orgID)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		q.Set("status", status)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		q.Set("search", search)
	}
	if len(filter.IDs) > 0 {
		clean := make([]string, 0, len(filter.IDs))
		for _, id := range filter.IDs {
			if s := strings.TrimSpace(id); s != "" {
				clean = append(clean, s)
			}
		}
		if len(clean) > 0 {
			q.Set("ids", strings.Join(clean, ","))
		}
	}
	applyPagination(q, filter.Page, filter.Limit)

	var page Page[User]
	err := c.get(ctx, "/api/v1/users", q, true, &page)
	return page, err
}
