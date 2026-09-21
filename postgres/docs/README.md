# postgres

pgx pool and golang-migrate. No process-wide global.

Import: `github.com/mosimosi228/kit/postgres`

## Purpose

Open Postgres once at service startup. Apply migrations from a separate call (`migrate` command), never from `Open`.

## API

| Function | Description |
|----------|-------------|
| `Open(ctx, Options)` | parse DSN, pool, `Ping` — **no migrate** |
| `(*Pool).Close()` | close the pool |
| `Migrate(MigrateOptions)` | `Up` from embed FS |
| `Version(MigrateOptions)` | current version + dirty |
| `RequireMigrated(MigrateOptions)` | error if nil version or dirty |

```go
pool, err := postgres.Open(ctx, postgres.Options{
    DSN: cfg.Postgres.DSN,
})
defer pool.Close()

mig := postgres.MigrateOptions{
    DSN:          cfg.Postgres.DSN,
    MigrationsFS: migrations.FS,
    MigrationsDir: "migrations",
}

// cobra migrate:
_ = postgres.Migrate(mig)

// cobra serve:
if err := postgres.RequireMigrated(mig); err != nil {
    return err
}
q := mapping.New(pool)
```

## Options

```go
type Options struct {
    DSN             string
    Schema          string // optional search_path
    MaxConns        int32  // default 50
    MinConns        int32  // default 1
    MaxConnLifetime time.Duration
    MaxConnIdleTime time.Duration
    AfterConnect    func(context.Context, *pgx.Conn) error
}
```

## Behavior

- Driver (pool): `jackc/pgx/v5`.
- Driver (migrate): `golang-migrate` + `database/postgres`.
- Empty / missing migration FS → `Migrate` skips; `RequireMigrated` errors.
- The project owns the singleton (`var Pool`, `db.Q`), not this package.

## Dependencies

- `github.com/jackc/pgx/v5`
- `github.com/golang-migrate/migrate/v4`
