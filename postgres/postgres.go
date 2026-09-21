package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultMaxConns        int32 = 50
	defaultMinConns        int32 = 1
	defaultMaxConnLifetime       = 30 * time.Minute
	defaultMaxConnIdleTime       = 10 * time.Minute
	defaultHealthCheck           = time.Minute
	defaultConnectTimeout        = 5 * time.Second
)

// Options configures a pgx pool. Open never runs migrations.
type Options struct {
	DSN             string
	Schema          string // optional search_path
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	AfterConnect    func(context.Context, *pgx.Conn) error
}

// Pool is a pgx pool handle. The project holds the singleton, not this package.
type Pool struct {
	*pgxpool.Pool
}

// Open connects and pings. It does not apply migrations.
func Open(ctx context.Context, opt Options) (*Pool, error) {
	if opt.DSN == "" {
		return nil, fmt.Errorf("postgres: dsn is required")
	}

	dsn := opt.DSN
	if opt.Schema != "" {
		dsn = withQuery(dsn, "search_path", opt.Schema)
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}

	if opt.MaxConns > 0 {
		cfg.MaxConns = opt.MaxConns
	} else {
		cfg.MaxConns = defaultMaxConns
	}
	if opt.MinConns > 0 {
		cfg.MinConns = opt.MinConns
	} else {
		cfg.MinConns = defaultMinConns
	}
	if opt.MaxConnLifetime > 0 {
		cfg.MaxConnLifetime = opt.MaxConnLifetime
	} else {
		cfg.MaxConnLifetime = defaultMaxConnLifetime
	}
	if opt.MaxConnIdleTime > 0 {
		cfg.MaxConnIdleTime = opt.MaxConnIdleTime
	} else {
		cfg.MaxConnIdleTime = defaultMaxConnIdleTime
	}
	cfg.HealthCheckPeriod = defaultHealthCheck
	if cfg.ConnConfig.ConnectTimeout == 0 {
		cfg.ConnConfig.ConnectTimeout = defaultConnectTimeout
	}
	if opt.AfterConnect != nil {
		cfg.AfterConnect = opt.AfterConnect
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	slog.Info("postgres connected",
		slog.Int("max_conns", int(cfg.MaxConns)),
		slog.String("schema", opt.Schema),
	)
	return &Pool{Pool: pool}, nil
}

// Close shuts down the pool.
func (p *Pool) Close() {
	if p == nil || p.Pool == nil {
		return
	}
	p.Pool.Close()
	slog.Info("postgres pool closed")
}
