package redis

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Options configures a go-redis client.
type Options struct {
	Host     string
	Port     int // default 6379
	Password string
	Database int
	PoolSize int // default 10 * NumCPU
}

// Client wraps go-redis. The project holds the singleton, not this package.
type Client struct {
	*goredis.Client
}

// Open connects and pings.
func Open(ctx context.Context, opt Options) (*Client, error) {
	if opt.Host == "" {
		return nil, fmt.Errorf("redis: host is required")
	}
	port := opt.Port
	if port <= 0 {
		port = 6379
	}
	pool := opt.PoolSize
	if pool <= 0 {
		pool = 10 * runtime.NumCPU()
	}

	rdb := goredis.NewClient(&goredis.Options{
		Addr:         fmt.Sprintf("%s:%d", opt.Host, port),
		Password:     opt.Password,
		DB:           opt.Database,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     pool,
		MinIdleConns: 10,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis: ping: %w", err)
	}

	slog.Info("redis connected", slog.String("addr", fmt.Sprintf("%s:%d", opt.Host, port)))
	return &Client{Client: rdb}, nil
}

// Close shuts down the client.
func (c *Client) Close() error {
	if c == nil || c.Client == nil {
		return nil
	}
	err := c.Client.Close()
	slog.Info("redis client closed")
	return err
}
