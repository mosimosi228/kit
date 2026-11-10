package iam

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultHTTPTimeout = 10 * time.Second
	defaultJWKSTTL     = 5 * time.Minute
)

// Config configures the IAM service client.
type Config struct {
	BaseURL      string        // IAM base URL (e.g. "http://127.0.0.1:18080")
	Base         string        // Alias for BaseURL (matches IAM documentation snippets)
	ProjectID    string        // Project UUID (required)
	OrgID        string        // Default Organization UUID (used in Register / Login)
	RoleID       string        // Default Role UUID (used in Register)
	ClientID     string        // Service client_id
	ClientSecret string        // Service client_secret
	HTTPClient   *http.Client  // Custom HTTP client; defaults to 10s timeout
	Timeout      time.Duration // HTTP timeout if HTTPClient is nil (default 10s)
	JWKSTTL      time.Duration // In-memory JWKS cache TTL (default 5m)
}

// Client is a reusable client for the IAM API and local RS256 JWT verification.
// Both iam.New(Config{...}) and direct struct initialization &iam.Client{Base: ...} are supported.
type Client struct {
	Base         string
	ProjectID    string
	OrgID        string
	RoleID       string
	ClientID     string
	ClientSecret string
	HTTPClient   *http.Client
	JWKSTTL      time.Duration

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	keysExpAt time.Time
}

// New validates configuration and returns a new IAM Client.
func New(cfg Config) (*Client, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = strings.TrimSpace(cfg.Base)
	}
	base = strings.TrimRight(base, "/")
	if base == "" {
		return nil, fmt.Errorf("%w: base url is required", ErrNotConfigured)
	}
	projectID := strings.TrimSpace(cfg.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("%w: project_id is required", ErrNotConfigured)
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = defaultHTTPTimeout
		}
		httpClient = &http.Client{Timeout: timeout}
	}

	jwksTTL := cfg.JWKSTTL
	if jwksTTL <= 0 {
		jwksTTL = defaultJWKSTTL
	}

	return &Client{
		Base:         base,
		ProjectID:    projectID,
		OrgID:        strings.TrimSpace(cfg.OrgID),
		RoleID:       strings.TrimSpace(cfg.RoleID),
		ClientID:     strings.TrimSpace(cfg.ClientID),
		ClientSecret: strings.TrimSpace(cfg.ClientSecret),
		HTTPClient:   httpClient,
		JWKSTTL:      jwksTTL,
		keys:         make(map[string]*rsa.PublicKey),
	}, nil
}

func (c *Client) baseURL() (string, error) {
	if c == nil {
		return "", ErrNotConfigured
	}
	b := strings.TrimRight(strings.TrimSpace(c.Base), "/")
	if b == "" {
		return "", fmt.Errorf("%w: base url is required", ErrNotConfigured)
	}
	return b, nil
}

func (c *Client) httpClient() *http.Client {
	if c != nil && c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Client) jwksTTL() time.Duration {
	if c != nil && c.JWKSTTL > 0 {
		return c.JWKSTTL
	}
	return defaultJWKSTTL
}

func (c *Client) post(ctx context.Context, path string, body any, withClientHeaders bool, out any) error {
	base, err := c.baseURL()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if withClientHeaders {
		c.setClientHeaders(req)
	}
	return c.do(req, out)
}

func (c *Client) get(ctx context.Context, path string, query url.Values, withClientHeaders bool, out any) error {
	base, err := c.baseURL()
	if err != nil {
		return err
	}
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if withClientHeaders {
		c.setClientHeaders(req)
	}
	return c.do(req, out)
}

func (c *Client) setClientHeaders(req *http.Request) {
	if c == nil {
		return
	}
	if c.ClientID != "" {
		req.Header.Set("X-Client-ID", c.ClientID)
	}
	if c.ClientSecret != "" {
		req.Header.Set("X-Client-Secret", c.ClientSecret)
	}
	if c.ProjectID != "" {
		req.Header.Set("X-Project-ID", c.ProjectID)
	}
}

func (c *Client) do(req *http.Request, out any) error {
	res, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		apiErr := &Error{StatusCode: res.StatusCode}
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, apiErr)
		}
		if apiErr.Status == "" {
			apiErr.Status = http.StatusText(res.StatusCode)
		}
		return apiErr
	}

	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

func applyPagination(q url.Values, page, limit int) {
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
}
