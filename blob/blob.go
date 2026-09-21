package blob

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Options configures an S3-compatible client.
type Options struct {
	Endpoint   string
	Bucket     string
	AccessKey  string
	SecretKey  string
	PublicBase string
	Secure     bool // used when Endpoint has no scheme; default TLS for non-loopback
}

// Client is an S3-compatible object store. The project holds the singleton.
type Client struct {
	mc         *minio.Client
	bucket     string
	publicBase string
}

// Open connects and checks that the bucket exists.
func Open(ctx context.Context, opt Options) (*Client, error) {
	if strings.TrimSpace(opt.Endpoint) == "" {
		return nil, fmt.Errorf("blob: endpoint is required")
	}
	if strings.TrimSpace(opt.Bucket) == "" {
		return nil, fmt.Errorf("blob: bucket is required")
	}

	endpoint, secure := normalizeEndpoint(opt.Endpoint, opt.Secure)
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(opt.AccessKey, opt.SecretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("blob: client: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	ok, err := mc.BucketExists(pingCtx, opt.Bucket)
	if err != nil {
		return nil, fmt.Errorf("blob: bucket: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("blob: bucket %q does not exist", opt.Bucket)
	}

	slog.Info("blob connected", slog.String("endpoint", endpoint), slog.String("bucket", opt.Bucket))
	return &Client{mc: mc, bucket: opt.Bucket, publicBase: strings.TrimRight(opt.PublicBase, "/")}, nil
}

func (c *Client) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if c == nil || c.mc == nil {
		return fmt.Errorf("blob: client is not open")
	}
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		return fmt.Errorf("blob: key is required")
	}
	opts := minio.PutObjectOptions{}
	if contentType != "" {
		opts.ContentType = contentType
	}
	if _, err := c.mc.PutObject(ctx, c.bucket, key, r, size, opts); err != nil {
		return fmt.Errorf("blob: put %s: %w", key, err)
	}
	return nil
}

func (c *Client) Delete(ctx context.Context, key string) error {
	if c == nil || c.mc == nil {
		return fmt.Errorf("blob: client is not open")
	}
	key = strings.TrimPrefix(key, "/")
	if err := c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("blob: delete %s: %w", key, err)
	}
	return nil
}

func (c *Client) PublicURL(key string) string {
	key = strings.TrimPrefix(key, "/")
	if c == nil {
		return key
	}
	if c.publicBase != "" {
		return strings.TrimRight(c.publicBase, "/") + "/" + key
	}
	if c.mc == nil {
		return key
	}
	return strings.TrimRight(c.mc.EndpointURL().String(), "/") + "/" + c.bucket + "/" + key
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mc = nil
	slog.Info("blob client closed")
	return nil
}

func normalizeEndpoint(raw string, secure bool) (string, bool) {
	raw = strings.TrimSpace(raw)
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Host, u.Scheme != "http"
	}
	host := raw
	useTLS := secure
	if !strings.Contains(raw, "://") && !secure {
		h := host
		if i := strings.Index(h, ":"); i >= 0 {
			h = h[:i]
		}
		if h != "localhost" && h != "127.0.0.1" && h != "::1" && h != "0.0.0.0" {
			useTLS = true
		}
	}
	return host, useTLS
}
