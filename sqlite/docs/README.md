# sqlite

SQLite connection, migrations, and global `*sql.DB`.

Import: `github.com/mosimosi228/kit/sqlite`

## Purpose

Open the DB once at service startup, run golang-migrate from an embed FS, and expose the connection pool.

## API

| Function | Description |
|----------|-------------|
| `New(ctx, Option)` | create directory, run migrations, `Ping`, store in `sqlite.DB` |
| `Close()` | close the connection |
| `DefaultDBPath` | `./tmp/public.db` when Path is empty |

```go
// Files must be at the FS root (e.g. package with //go:embed *.sql).
err := sqlite.New(ctx, sqlite.Option{
    Path:         "runtime/app.db",
    MigrationsFS: migrations.FS,
})
defer sqlite.Close()

rows, err := sqlite.DB.QueryContext(ctx, "SELECT 1")
```

## Behavior

- Driver: `mattn/go-sqlite3`.
- PRAGMA: `foreign_keys=1`, `busy_timeout=5000`, `journal_mode=WAL`.
- Pool: `MaxOpenConns(1)`, `MaxIdleConns(1)` — typical for SQLite.
- Migrations: files from `Option.MigrationsFS` (`*.up.sql` / `*.down.sql` in the FS root).
- If `MigrationsFS` is nil or has no migration files — migrations are skipped with a log message.

## Option

```go
type Option struct {
    Path         string // path to the DB file
    MigrationsFS fs.FS  // optional embed FS with migration SQL files
}
```

## Dependencies

- `github.com/mattn/go-sqlite3`
- `github.com/golang-migrate/migrate/v4`
