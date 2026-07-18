# utils

Small helpers without domain logic.

Import: `github.com/mosimosi228/kit/utils`

## JSON

| Function | Description |
|----------|-------------|
| `PrettyJSON(v)` | indent JSON |
| `MinifyJSON(s)` | compact JSON (or original string) |
| `FixBrokenJSON(s)` | heuristic fix for quotes/escapes |

## Strings / slices

| Function | Description |
|----------|-------------|
| `Map(s, f)` | apply `f` to each element |
| `Deref(p)` | `*p` or zero |
| `TrimAndJoinCompact(s, sep)` | trim + drop empty parts |
| `ReturnValOrNil` | return value or nil |

## URL / DSN

| Function | Description |
|----------|-------------|
| `IsValidURL(s)` | http/https (including `//host/path`) |
| `MaskDSN(dsn)` | redact password in URL |

## Secrets / UUID

| Function | Description |
|----------|-------------|
| `GenerateJWTSecret()` | 32 random bytes → hex (for HS256) |
| `ParsePgUUIDParam(key, r)` | query → `pgtype.UUID` |
| `ParsePgStringParam(key, r)` | query → `pgtype.Text` |
| `ParsePgBoolParam(key, r)` | query → `pgtype.Bool` |

## Example

```go
secret, err := utils.GenerateJWTSecret()
if err != nil {
    return err
}

if !utils.IsValidURL(raw) {
    return errors.New("bad url")
}
slog.Info("db", "dsn", utils.MaskDSN(dsn))
```
