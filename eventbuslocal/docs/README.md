# eventbuslocal

In-process worker-pool bus. No NATS and no package global.

Import: `github.com/mosimosi228/kit/eventbuslocal`

The envelope is `eventbus.Message`.

## Channels

**Work.** `Submit` runs the entity's write handler on one worker and waits for the result. A second call is a second job. One submit runs the handler once.

**Fan-out.** `Fanout` calls every local subscriber. It does not run the work handler. The work handler publishes fan-out itself after a successful write.

## API

| Function | Description |
|----------|-------------|
| `Open(Options)` | start the pool |
| `RegisterWork` | one write handler per entity |
| `RegisterFanout` | append a local subscriber |
| `Submit` | run work and wait |
| `Fanout` | notify subscribers |
| `Close` | wait for taken jobs, reject new ones |

```go
bus := eventbuslocal.Open(eventbuslocal.Options{})
defer bus.Close()

bus.RegisterWork("user", func(ctx context.Context, msg eventbus.Message) (any, error) {
    row, err := insert(ctx, msg)
    if err != nil {
        return nil, err
    }
    bus.Fanout(msg)
    return row, nil
})
bus.RegisterFanout("user", func(msg eventbus.Message) {
    cache.User().Del(fmt.Sprint(msg.EntityID))
})

row, err := bus.Submit(ctx, eventbus.Message{Entity: "user", EntityID: id})
```

`Options.Workers` defaults to `GOMAXPROCS`. A handler panic becomes an error from `Submit` and does not stop the pool.

`kit/eventbus` stays the NATS bus. Its handlers invalidate cache. They do not insert.
