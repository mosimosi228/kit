package postgres

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

const defaultMigrationsTable = "schema_migrations"

// MigrateOptions applies golang-migrate from an embed FS.
type MigrateOptions struct {
	DSN           string
	Schema        string
	Table         string // default schema_migrations
	MigrationsFS  fs.FS
	MigrationsDir string // default "."
}

// Migrate applies pending up migrations. No-op if the FS has no files.
func Migrate(opt MigrateOptions) error {
	if err := opt.validate(); err != nil {
		return err
	}
	dir := opt.dir()
	if !hasMigrationFS(opt.MigrationsFS, dir) {
		slog.Info("postgres migrations skipped: no files in embed FS")
		return nil
	}

	m, err := newMigrate(opt)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("postgres: migrate: %w", err)
	}

	version, dirty, _ := m.Version()
	if dirty {
		slog.Warn("postgres migrations dirty", slog.Uint64("version", uint64(version)))
	} else {
		slog.Info("postgres migrations applied", slog.String("dir", dir), slog.Uint64("version", uint64(version)))
	}
	return nil
}

// Version returns the current migration version.
func Version(opt MigrateOptions) (version uint, dirty bool, err error) {
	if err := opt.validate(); err != nil {
		return 0, false, err
	}
	if !hasMigrationFS(opt.MigrationsFS, opt.dir()) {
		return 0, false, fmt.Errorf("postgres: no migration files")
	}
	m, err := newMigrate(opt)
	if err != nil {
		return 0, false, err
	}
	defer m.Close()
	return m.Version()
}

// RequireMigrated fails if migrations are missing or dirty.
// Use from serve after Open.
func RequireMigrated(opt MigrateOptions) error {
	version, dirty, err := Version(opt)
	if dirty {
		return fmt.Errorf("postgres: migrations dirty at version %d", version)
	}
	if errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("postgres: migrations not applied")
	}
	return err
}

func (opt MigrateOptions) validate() error {
	if opt.DSN == "" {
		return fmt.Errorf("postgres: dsn is required")
	}
	if opt.MigrationsFS == nil {
		return fmt.Errorf("postgres: migrations fs is required")
	}
	return nil
}

func (opt MigrateOptions) dir() string {
	if opt.MigrationsDir == "" {
		return "."
	}
	return opt.MigrationsDir
}

func (opt MigrateOptions) table() string {
	if opt.Table == "" {
		return defaultMigrationsTable
	}
	return opt.Table
}

func newMigrate(opt MigrateOptions) (*migrate.Migrate, error) {
	source, err := iofs.New(opt.MigrationsFS, opt.dir())
	if err != nil {
		return nil, fmt.Errorf("postgres: migrate source: %w", err)
	}
	dsn := withQuery(opt.DSN, "x-migrations-table", opt.table())
	if opt.Schema != "" {
		dsn = withQuery(dsn, "search_path", opt.Schema)
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: migrate open: %w", err)
	}
	return m, nil
}

func withQuery(dsn, key, val string) string {
	if key == "" {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + url.QueryEscape(key) + "=" + url.QueryEscape(val)
}

func hasMigrationFS(files fs.FS, dir string) bool {
	if files == nil {
		return false
	}
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".up.sql") || strings.HasSuffix(name, ".down.sql") {
			return true
		}
	}
	return false
}
