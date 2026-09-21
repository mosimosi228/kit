# blob

S3-compatible object store (MinIO, R2, S3). No process-wide global.

Import: `github.com/mosimosi228/kit/blob`

## API

| Function | Description |
|----------|-------------|
| `Open(ctx, Options)` | connect + bucket exists |
| `(*Client).Put` | upload object |
| `(*Client).Delete` | remove object |
| `(*Client).PublicURL` | `PublicBase/key` or endpoint/bucket/key |
| `(*Client).Close` | drop client |

```go
c, err := blob.Open(ctx, blob.Options{
    Endpoint:   cfg.Endpoint,
    Bucket:     cfg.Bucket,
    AccessKey:  cfg.AccessKey,
    SecretKey:  cfg.SecretKey,
    PublicBase: cfg.PublicBase,
    Secure:     true,
})
defer c.Close()

err = c.Put(ctx, "p/proj/a.mp4", r, size, "video/mp4")
url := c.PublicURL("p/proj/a.mp4")
```

Scheme on `Endpoint` wins (`http://` → no TLS). Bare host defaults to TLS except loopback.

Live test: set `KIT_BLOB_ENDPOINT` and `KIT_BLOB_BUCKET`.
