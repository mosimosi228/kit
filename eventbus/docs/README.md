# eventbus

NATS JetStream bus. No process-wide global.

Import: `github.com/mosimosi228/kit/eventbus`

## Purpose

Publish entity events and invalidate local state on every node. Same flow as adexchange: local dispatch, then JetStream.

## API

| Function | Description |
|----------|-------------|
| `Open(ctx, Options)` | connect, ensure stream, start durable pull consumer |
| `(*Bus).Publish` | local handler + JetStream |
| `(*Bus).RegisterHandler` | `entity` → callback |
| `(*Bus).Close` | stop worker, drain |

```go
bus, err := eventbus.Open(ctx, eventbus.Options{
    URL:      cfg.NATS.URL,
    Name:     cfg.AppName,
    Stream:   "EVENTBUS",
    Subject:  "eventbus.>",
    Consumer: "eventbus-worker-" + node,
})
defer bus.Close()

bus.RegisterHandler("user", func(msg eventbus.Message) {
    cache.User().Del(fmt.Sprint(msg.EntityID))
})

_ = bus.Publish(ctx, eventbus.Message{Entity: "user", EntityID: id})
```

`Consumer` is required and must be unique per process/node. A shared durable consumer load-balances messages so other nodes keep stale cache.

## Options

```go
type Options struct {
    URL      string
    Name     string
    Stream   string        // default EVENTBUS
    Subject  string        // default eventbus.>
    Consumer string        // required
    MaxAge   time.Duration // default 7d
}
```

Publish subject: `eventbus.>` + entity `user` → `eventbus.user`.

## Dependencies

- `github.com/nats-io/nats.go`
