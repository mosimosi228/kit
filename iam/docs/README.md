# iam

Reusable Go client for the IAM service (`/srv/www/iam`): atomic user registration, sign-in, refresh token rotation, logout, local RS256 JWT verification via JWKS (with automatic key rotation reload), single-request role permission checks, project user catalog, timeline events, and HTTP middleware.

Import: `github.com/mosimosi228/kit/iam`

## Configuration

```go
client, err := iam.New(iam.Config{
    BaseURL:      "http://127.0.0.1:18080",
    ProjectID:    os.Getenv("IAM_PROJECT_ID"),
    OrgID:        os.Getenv("IAM_ORG_ID"),
    RoleID:       os.Getenv("IAM_ROLE_ID"),
    ClientID:     os.Getenv("IAM_CLIENT_ID"),
    ClientSecret: os.Getenv("IAM_CLIENT_SECRET"),
})
```

Direct struct initialization matching the IAM guide snippets is also supported:

```go
client := &iam.Client{
    Base:         "http://127.0.0.1:18080",
    ProjectID:    projectID,
    OrgID:        orgID,
    RoleID:       roleID,
    ClientID:     clientID,
    ClientSecret: clientSecret,
}
```

Optional process-wide singleton helpers: `iam.CreateClient(cfg)`, `iam.SetClient(c)`, `iam.GetClient()`.

## Registration, Sign-In, Refresh & Logout

```go
// Atomic registration (POST /api/v1/register)
tokens, err := client.Register(ctx, email, password)

// Registration with optional migration UUID, username, or custom org/role
tokens, err = client.RegisterWith(ctx, iam.RegisterRequest{
    ID:       legacyUUID, // optional
    Username: "alice",    // optional
    Email:    email,
    Password: password,
})

// Sign-in (POST /api/v1/login)
tokens, err = client.Login(ctx, email, password)

// Single-use refresh rotation (POST /api/v1/token/refresh)
tokens, err = client.Refresh(ctx, tokens.RefreshToken)

// Revoke session (POST /api/v1/logout)
err = client.Logout(ctx, tokens.RefreshToken)
```

## JWT Verification (RS256 + JWKS)

`PublicKey`, `Verify`, and `VerifyClaims` validate `alg == "RS256"`, `iss == "iam"`, and `aud == ProjectID` using an in-memory JWKS cache (`JWKSTTL`, default 5m). When an incoming token carries a `kid` not yet in the cache (e.g. after key rotation in the IAM console), the client automatically re-fetches `GET /api/v1/jwks?project_id=...`.

```go
// Typed claims
claims, err := client.VerifyClaims(ctx, accessToken)
userID := claims.UserID()
orgID  := claims.OrgID
role   := claims.Role()

// Or jwt.MapClaims + Caller helper (as in IAM guide)
rawClaims, err := client.Verify(ctx, accessToken)
userID, orgID, role := iam.Caller(rawClaims)
```

## Role Permissions (`X-Client-ID` / `X-Client-Secret`)

```go
// Single-request check via GET /api/v1/role-permissions?project_id=...&role_name=...
allowed, err := client.Allows(ctx, role, "orders.write")

// All permissions of a role
perms, err := client.RolePermissions(ctx, role)
```

## Project Users & Timeline (`X-Client-ID` / `X-Client-Secret`)

```go
// Get single project user with memberships
user, err := client.GetUser(ctx, userID)

// List project users with filters
page, err := client.ListUsers(ctx, iam.ListUsersFilter{
    Search: "alice",
    Status: "active",
    OrgID:  orgID,
    IDs:    []string{userID},
    Page:   1,
    Limit:  50,
})

// Record user timeline event (POST /api/v1/timeline)
err = client.RecordTimeline(ctx, userID, "order.created", "Created order #42", map[string]any{
    "order_id": 42,
})

// List timeline events (GET /api/v1/timeline)
events, err := client.ListTimeline(ctx, iam.ListTimelineFilter{UserID: userID})
```

## HTTP Middleware

```go
r.Group(func(r chi.Router) {
    r.Use(client.Middleware)
    r.With(client.RequirePermission("orders.write")).Post("/orders", createOrder)
    r.With(client.RequireRole("admin")).Delete("/orders/{id}", deleteOrder)
})

func createOrder(w http.ResponseWriter, r *http.Request) {
    claims, _ := iam.ClaimsFromContext(r.Context())
    userID := claims.UserID()
    // ...
}
```

## Error Handling

All API errors unwrap to sentinel errors (`errors.Is`) and can be inspected via `*iam.Error` or `iam.IsCode(err, code)`:

| Code | Sentinel Error | HTTP |
|------|----------------|------|
| `invalid_client` | `iam.ErrInvalidClient` | 401 |
| `invalid_grant` | `iam.ErrInvalidGrant` | 401 |
| `no_signing_key` | `iam.ErrNoSigningKey` | 400 |
| `validation` | `iam.ErrValidation` | 400 |
| `invalid_reference` | `iam.ErrInvalidReference` | 400 |
| `conflict` | `iam.ErrConflict` | 409 |
| `in_use` | `iam.ErrInUse` | 409 |
| `not_found` | `iam.ErrNotFound` | 404 |
| `internal` | `iam.ErrInternal` | 500 |
