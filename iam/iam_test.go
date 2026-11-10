package iam

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func generateRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func signTestToken(t *testing.T, key *rsa.PrivateKey, kid, projectID, userID, orgID, role string, exp time.Time) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    TokenIssuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{projectID},
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ID:        "jti-1",
		},
		OrgID: orgID,
		Roles: []string{role},
		SID:   "sid-1",
	})
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func jwkFromPublic(kid string, pub *rsa.PublicKey) JWK {
	return JWK{
		Kty: "RSA",
		Use: "sig",
		Alg: AlgRS256,
		Kid: kid,
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

func TestRegisterLoginRefreshLogoutAndErrors(t *testing.T) {
	const (
		projID = "11111111-1111-4111-8111-111111111111"
		orgID  = "22222222-2222-4222-8222-222222222222"
		roleID = "33333333-3333-4333-8333-333333333333"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)

		if body["client_id"] != "billing-api" || body["client_secret"] != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(Error{
				Status:  "Unauthorized!",
				Code:    CodeInvalidClient,
				Message: "invalid client",
			})
			return
		}

		switch r.URL.Path {
		case "/api/v1/register":
			if body["email"] == "taken@example.com" {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(Error{
					Status:  "Conflict",
					Code:    CodeConflict,
					Message: "already exists",
				})
				return
			}
			if body["project_id"] != projID || body["org_id"] != orgID || body["role_id"] != roleID {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(Error{
					Status:  "Bad Request",
					Code:    CodeInvalidReference,
					Message: "related record not found",
				})
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(Tokens{
				AccessToken:  "acc-reg",
				RefreshToken: "ref-reg",
				TokenType:    "Bearer",
				ExpiresIn:    900,
			})
		case "/api/v1/login":
			if body["password"] != "correct" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(Error{
					Status:  "Unauthorized!",
					Code:    CodeInvalidGrant,
					Message: "invalid credentials",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(Tokens{
				AccessToken:  "acc-login",
				RefreshToken: "ref-login",
				TokenType:    "Bearer",
				ExpiresIn:    900,
			})
		case "/api/v1/token/refresh":
			if body["refresh_token"] != "ref-login" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(Error{
					Status:  "Unauthorized!",
					Code:    CodeInvalidGrant,
					Message: "invalid refresh",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(Tokens{
				AccessToken:  "acc-next",
				RefreshToken: "ref-next",
				TokenType:    "Bearer",
				ExpiresIn:    900,
			})
		case "/api/v1/logout":
			_ = json.NewEncoder(w).Encode(map[string]bool{"revoked": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, err := New(Config{
		BaseURL:      srv.URL,
		ProjectID:    projID,
		OrgID:        orgID,
		RoleID:       roleID,
		ClientID:     "billing-api",
		ClientSecret: "s3cret",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	tok, err := c.Register(ctx, "user@example.com", "correct")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if tok.AccessToken != "acc-reg" || tok.RefreshToken != "ref-reg" {
		t.Fatalf("unexpected tokens: %+v", tok)
	}

	if _, err := c.Register(ctx, "taken@example.com", "correct"); !errors.Is(err, ErrConflict) || !IsCode(err, CodeConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

	tok, err = c.Login(ctx, "user@example.com", "correct")
	if err != nil || tok.AccessToken != "acc-login" {
		t.Fatalf("Login: tok=%+v err=%v", tok, err)
	}

	if _, err := c.Login(ctx, "user@example.com", "wrong"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant, got %v", err)
	}

	tok, err = c.Refresh(ctx, "ref-login")
	if err != nil || tok.AccessToken != "acc-next" {
		t.Fatalf("Refresh: tok=%+v err=%v", tok, err)
	}

	if err := c.Logout(ctx, "ref-next"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
}

func TestJWKSVerifyCacheAndRotation(t *testing.T) {
	const (
		projID = "11111111-1111-4111-8111-111111111111"
		userID = "44444444-4444-4444-8444-444444444444"
		orgID  = "22222222-2222-4222-8222-222222222222"
	)

	key1 := generateRSAKey(t)
	key2 := generateRSAKey(t)

	var jwksCalls atomic.Int32
	var rotate atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwksCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		keys := []JWK{jwkFromPublic("kid-1", &key1.PublicKey)}
		if rotate.Load() {
			keys = append(keys, jwkFromPublic("kid-2", &key2.PublicKey))
		}
		_ = json.NewEncoder(w).Encode(JWKS{Keys: keys})
	}))
	defer srv.Close()

	c := &Client{
		Base:      srv.URL,
		ProjectID: projID,
	}

	ctx := context.Background()
	tok1 := signTestToken(t, key1, "kid-1", projID, userID, orgID, "admin", time.Now().Add(15*time.Minute))

	claimsMap, err := c.Verify(ctx, tok1)
	if err != nil {
		t.Fatalf("Verify tok1: %v", err)
	}
	gotUser, gotOrg, gotRole := Caller(claimsMap)
	if gotUser != userID || gotOrg != orgID || gotRole != "admin" {
		t.Fatalf("Caller = (%q, %q, %q)", gotUser, gotOrg, gotRole)
	}

	// Second verification with same kid should hit in-memory JWKS cache
	typedClaims, err := c.VerifyClaims(ctx, tok1)
	if err != nil {
		t.Fatalf("VerifyClaims tok1: %v", err)
	}
	if typedClaims.UserID() != userID || typedClaims.Role() != "admin" || typedClaims.SID != "sid-1" {
		t.Fatalf("unexpected Claims: %+v", typedClaims)
	}
	if got := jwksCalls.Load(); got != 1 {
		t.Fatalf("expected 1 JWKS call due to cache, got %d", got)
	}

	// Rotate signing key to kid-2; client should auto-refresh JWKS on unknown kid
	rotate.Store(true)
	tok2 := signTestToken(t, key2, "kid-2", projID, userID, orgID, "editor", time.Now().Add(15*time.Minute))
	typed2, err := c.VerifyClaims(ctx, tok2)
	if err != nil {
		t.Fatalf("VerifyClaims tok2 after rotation: %v", err)
	}
	if typed2.Role() != "editor" {
		t.Fatalf("expected role editor, got %q", typed2.Role())
	}
	if got := jwksCalls.Load(); got != 2 {
		t.Fatalf("expected 2 JWKS calls after rotation, got %d", got)
	}

	// Token for another project (wrong aud) must fail
	foreignTok := signTestToken(t, key2, "kid-2", "99999999-9999-4999-8999-999999999999", userID, orgID, "admin", time.Now().Add(15*time.Minute))
	if _, err := c.Verify(ctx, foreignTok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for wrong audience, got %v", err)
	}

	// Expired token must return ErrTokenExpired
	expiredTok := signTestToken(t, key2, "kid-2", projID, userID, orgID, "admin", time.Now().Add(-time.Minute))
	if _, err := c.Verify(ctx, expiredTok); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestPermissionsUsersTimelineAndMiddleware(t *testing.T) {
	const (
		projID = "11111111-1111-4111-8111-111111111111"
		userID = "44444444-4444-4444-8444-444444444444"
		orgID  = "22222222-2222-4222-8222-222222222222"
	)

	key := generateRSAKey(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/jwks" {
			_ = json.NewEncoder(w).Encode(JWKS{Keys: []JWK{jwkFromPublic("kid-1", &key.PublicKey)}})
			return
		}

		if r.Header.Get("X-Client-ID") != "svc" || r.Header.Get("X-Client-Secret") != "sec" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(Error{Code: CodeInvalidClient, Message: "invalid client"})
			return
		}

		switch r.URL.Path {
		case "/api/v1/role-permissions":
			roleName := r.URL.Query().Get("role_name")
			if roleName == "admin" {
				_ = json.NewEncoder(w).Encode(Page[RolePermission]{
					Lists: []RolePermission{
						{RoleID: "r1", Permission: "orders.read"},
						{RoleID: "r1", Permission: "orders.write"},
					},
					Page:  1,
					Limit: 500,
					Total: 2,
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(Error{Code: CodeNotFound, Message: "not found"})
		case "/api/v1/users":
			_ = json.NewEncoder(w).Encode(Page[User]{
				Lists: []User{
					{
						ID:       userID,
						Username: "alice",
						Email:    "alice@example.com",
						Status:   "active",
						Memberships: []Membership{
							{ID: "m1", OrgID: orgID, OrgName: "Core", RoleID: "r1", RoleName: "admin", Status: "active"},
						},
					},
				},
				Page:  1,
				Limit: 100,
				Total: 1,
			})
		case "/api/v1/users/" + userID:
			_ = json.NewEncoder(w).Encode(User{
				ID:       userID,
				Username: "alice",
				Email:    "alice@example.com",
				Status:   "active",
			})
		case "/api/v1/timeline":
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(TimelineEntry{
					ID:        "tl-1",
					ProjectID: projID,
					Event:     "order.created",
					Message:   "Created order #42",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(Page[TimelineEntry]{
				Lists: []TimelineEntry{{ID: "tl-1", ProjectID: projID, Event: "order.created"}},
				Page:  1,
				Limit: 100,
				Total: 1,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, err := New(Config{
		BaseURL:      srv.URL,
		ProjectID:    projID,
		ClientID:     "svc",
		ClientSecret: "sec",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	ok, err := c.Allows(ctx, "admin", "orders.write")
	if err != nil || !ok {
		t.Fatalf("Allows(admin, orders.write) = (%v, %v)", ok, err)
	}
	ok, err = c.Allows(ctx, "admin", "users.delete")
	if err != nil || ok {
		t.Fatalf("Allows(admin, users.delete) = (%v, %v)", ok, err)
	}
	ok, err = c.Allows(ctx, "unknown", "orders.read")
	if err != nil || ok {
		t.Fatalf("Allows(unknown) = (%v, %v)", ok, err)
	}

	usersPage, err := c.ListUsers(ctx, ListUsersFilter{Search: "alice", IDs: []string{userID}})
	if err != nil || len(usersPage.Lists) != 1 || usersPage.Lists[0].Username != "alice" {
		t.Fatalf("ListUsers: page=%+v err=%v", usersPage, err)
	}

	u, err := c.GetUser(ctx, userID)
	if err != nil || u.Email != "alice@example.com" {
		t.Fatalf("GetUser: u=%+v err=%v", u, err)
	}

	if err := c.RecordTimeline(ctx, userID, "order.created", "Created order #42", map[string]any{"order_id": 42}); err != nil {
		t.Fatalf("RecordTimeline: %v", err)
	}
	tlPage, err := c.ListTimeline(ctx, ListTimelineFilter{UserID: userID})
	if err != nil || len(tlPage.Lists) != 1 {
		t.Fatalf("ListTimeline: page=%+v err=%v", tlPage, err)
	}

	// Test Middleware + RequirePermission
	tok := signTestToken(t, key, "kid-1", projID, userID, orgID, "admin", time.Now().Add(15*time.Minute))
	protected := c.Middleware(c.RequirePermission("orders.write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cl, ok := ClaimsFromContext(r.Context())
		if !ok || cl.UserID() != userID {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("middleware allowed request status = %d, want 200", rec.Code)
	}

	deniedHandler := c.Middleware(c.RequirePermission("orders.delete")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	rec2 := httptest.NewRecorder()
	deniedHandler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("middleware denied request status = %d, want 403", rec2.Code)
	}
}
