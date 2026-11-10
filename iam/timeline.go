package iam

import (
	"context"
	"net/url"
	"strings"
)

// RecordTimeline records a user activity event in the project timeline
// via POST /api/v1/timeline with X-Client-ID and X-Client-Secret headers.
func (c *Client) RecordTimeline(ctx context.Context, userID, event, message string, meta map[string]any) error {
	_, err := c.CreateTimeline(ctx, CreateTimelineRequest{
		UserID:  userID,
		Event:   event,
		Message: message,
		Meta:    meta,
	})
	return err
}

// CreateTimeline records a user activity event in the project timeline and returns the created entry.
func (c *Client) CreateTimeline(ctx context.Context, in CreateTimelineRequest) (TimelineEntry, error) {
	if c == nil {
		return TimelineEntry{}, ErrNotConfigured
	}
	payload := map[string]any{
		"project_id": c.ProjectID,
		"user_id":    strings.TrimSpace(in.UserID),
		"event":      strings.TrimSpace(in.Event),
	}
	if msg := strings.TrimSpace(in.Message); msg != "" {
		payload["message"] = msg
	}
	if in.Meta != nil {
		payload["meta"] = in.Meta
	}
	var entry TimelineEntry
	err := c.post(ctx, "/api/v1/timeline", payload, true, &entry)
	return entry, err
}

// ListTimeline fetches a paginated list of timeline events for the project (optionally filtered by UserID)
// via GET /api/v1/timeline with X-Client-ID and X-Client-Secret headers.
func (c *Client) ListTimeline(ctx context.Context, filter ListTimelineFilter) (Page[TimelineEntry], error) {
	if c == nil {
		return Page[TimelineEntry]{}, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("project_id", c.ProjectID)
	if uid := strings.TrimSpace(filter.UserID); uid != "" {
		q.Set("user_id", uid)
	}
	applyPagination(q, filter.Page, filter.Limit)

	var page Page[TimelineEntry]
	err := c.get(ctx, "/api/v1/timeline", q, true, &page)
	return page, err
}
