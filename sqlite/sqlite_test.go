package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestHasMigrationFS(t *testing.T) {
	if hasMigrationFS(nil) {
		t.Fatal("expected nil FS to have no migrations")
	}

	if hasMigrationFS(fstest.MapFS{}) {
		t.Fatal("expected empty FS to have no migrations")
	}

	fsys := fstest.MapFS{
		"001_init.up.sql": &fstest.MapFile{Data: []byte("CREATE TABLE t (id INTEGER);")},
	}
	if !hasMigrationFS(fsys) {
		t.Fatal("expected migration file to be detected")
	}
}

func TestEnsureDirCreatesDatabaseFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "kit.db")

	if err := ensureDir(path); err != nil {
		t.Fatalf("ensureDir() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.IsDir() {
		t.Fatal("expected database path to be a file")
	}
}

func TestDSNHelpers(t *testing.T) {
	path := "/tmp/kit.db"

	if got := toMigrateDSN(path); got != "sqlite3:///tmp/kit.db" {
		t.Fatalf("toMigrateDSN() = %q", got)
	}

	conn := toConnDSN(path)
	if conn != "file:/tmp/kit.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)" {
		t.Fatalf("toConnDSN() = %q", conn)
	}
}

func TestNewAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kit.db")

	if err := New(context.Background(), Option{Path: path}); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if DB == nil {
		t.Fatal("DB is nil after New()")
	}

	var n int
	if err := DB.QueryRowContext(context.Background(), "SELECT 1").Scan(&n); err != nil {
		t.Fatalf("QueryRow() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", n)
	}

	Close()
}

func TestNewAppliesMigrationsFromFS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kit.db")
	fsys := fstest.MapFS{
		"001_init.up.sql": &fstest.MapFile{Data: []byte("CREATE TABLE items (id INTEGER PRIMARY KEY);")},
	}

	if err := New(context.Background(), Option{Path: path, MigrationsFS: fsys}); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer Close()

	var name string
	if err := DB.QueryRowContext(context.Background(),
		"SELECT name FROM sqlite_master WHERE type='table' AND name='items'",
	).Scan(&name); err != nil {
		t.Fatalf("QueryRow() error = %v", err)
	}
	if name != "items" {
		t.Fatalf("table name = %q, want items", name)
	}
}

func TestNewUsesDefaultPathWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := New(context.Background(), Option{}); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer Close()

	if _, err := os.Stat(DefaultDBPath); err != nil {
		t.Fatalf("Stat(%q) error = %v", DefaultDBPath, err)
	}
}
