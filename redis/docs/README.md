# redis

go-redis client. No process-wide global.

Import: `github.com/mosimosi228/kit/redis`

## API

| Function | Description |
|----------|-------------|
| `Open(ctx, Options)` | connect + `Ping` |
| `(*Client).Close()` | close |

```go
rdb, err := redis.Open(ctx, redis.Options{
    Host:     cfg.Redis.Host,
    Port:     cfg.Redis.Port,
    Password: cfg.Redis.Password,
    Database: cfg.Redis.Database,
})
defer rdb.Close()
```

`Client` embeds `*redis.Client`.

## Options

```go
type Options struct {
    Host     string
    Port     int // default 6379
    Password string
    Database int
    PoolSize int // default 10 * NumCPU
}
```

## Dependencies

- `github.com/redis/go-redis/v9`
