package iam

import "sync"

var (
	defaultMu     sync.RWMutex
	defaultClient *Client
)

// CreateClient initializes and stores the process-wide default IAM client.
func CreateClient(cfg Config) (*Client, error) {
	c, err := New(cfg)
	if err != nil {
		return nil, err
	}
	defaultMu.Lock()
	defaultClient = c
	defaultMu.Unlock()
	return c, nil
}

// SetClient sets the process-wide default IAM client.
func SetClient(c *Client) {
	defaultMu.Lock()
	defaultClient = c
	defaultMu.Unlock()
}

// GetClient returns the process-wide default IAM client, or nil if not initialized.
func GetClient() *Client {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultClient
}
