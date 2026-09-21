package postgres

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestWithQuery(t *testing.T) {
	got := withQuery("postgres://u@h/db", "search_path", "app")
	if got != "postgres://u@h/db?search_path=app" {
		t.Fatalf("withQuery() = %q", got)
	}

	got = withQuery("postgres://u@h/db?sslmode=disable", "x-migrations-table", "schema_migrations")
	if !strings.Contains(got, "sslmode=disable") || !strings.Contains(got, "x-migrations-table=schema_migrations") {
		t.Fatalf("withQuery() = %q", got)
	}
}

func TestHasMigrationFS(t *testing.T) {
	if hasMigrationFS(nil, ".") {
		t.Fatal("nil FS")
	}
	if hasMigrationFS(fstest.MapFS{}, ".") {
		t.Fatal("empty FS")
	}
	fsys := fstest.MapFS{
		"001_init.up.sql": {Data: []byte("SELECT 1;")},
	}
	if !hasMigrationFS(fsys, ".") {
		t.Fatal("expected migration file")
	}
}

func TestMigrateOptionsDefaults(t *testing.T) {
	opt := MigrateOptions{}
	if opt.dir() != "." {
		t.Fatalf("dir = %q", opt.dir())
	}
	if opt.table() != defaultMigrationsTable {
		t.Fatalf("table = %q", opt.table())
	}
	if err := opt.validate(); err == nil {
		t.Fatal("expected validate error")
	}
}

func TestOpenRequiresDSN(t *testing.T) {
	if _, err := Open(t.Context(), Options{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRequireMigratedRejectsEmptyFS(t *testing.T) {
	err := RequireMigrated(MigrateOptions{
		DSN:          "postgres://u@h/db?sslmode=disable",
		MigrationsFS: fstest.MapFS{},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
