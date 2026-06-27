package auth_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/UzStack/jst-go/internal/modules/auth"
	"github.com/UzStack/jst-go/internal/modules/user"
	"github.com/UzStack/jst-go/internal/shared/config"
	"github.com/UzStack/jst-go/internal/shared/database"
	"github.com/UzStack/jst-go/internal/shared/httpx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// fakeUserRepo is a minimal in-memory user.Repository — handler tests do not
// hit a database. Integration tests that exercise the real Postgres path live
// alongside the repository (see README -> testcontainers section).
type fakeUserRepo struct {
	byID    map[uuid.UUID]*user.User
	byEmail map[string]*user.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		byID:    map[uuid.UUID]*user.User{},
		byEmail: map[string]*user.User{},
	}
}

func (r *fakeUserRepo) Create(_ context.Context, u *user.User) error {
	if _, ok := r.byEmail[u.Email]; ok {
		return user.ErrEmailTaken
	}
	r.byID[u.ID] = u
	r.byEmail[u.Email] = u
	return nil
}
func (r *fakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*user.User, error) {
	if u, ok := r.byID[id]; ok {
		return u, nil
	}
	return nil, database.ErrNotFound
}
func (r *fakeUserRepo) GetByEmail(_ context.Context, email string) (*user.User, error) {
	if u, ok := r.byEmail[email]; ok {
		return u, nil
	}
	return nil, database.ErrNotFound
}
func (r *fakeUserRepo) UpdateName(_ context.Context, id uuid.UUID, name string) (*user.User, error) {
	u, ok := r.byID[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	u.Name = name
	return u, nil
}
func (r *fakeUserRepo) UpdateRole(_ context.Context, id uuid.UUID, role string) (*user.User, error) {
	u, ok := r.byID[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	u.Role = role
	return u, nil
}
func (r *fakeUserRepo) Delete(_ context.Context, id uuid.UUID) error {
	u, ok := r.byID[id]
	if !ok {
		return database.ErrNotFound
	}
	delete(r.byID, id)
	delete(r.byEmail, u.Email)
	return nil
}
func (r *fakeUserRepo) List(_ context.Context, _ user.ListFilter) ([]user.User, error) {
	out := make([]user.User, 0, len(r.byID))
	for _, u := range r.byID {
		out = append(out, *u)
	}
	return out, nil
}
func (r *fakeUserRepo) Count(_ context.Context, _ string) (int64, error) {
	return int64(len(r.byID)), nil
}

// fakeStore is an in-memory auth.RefreshStore for handler tests.
type fakeStore struct {
	revoked map[uuid.UUID]bool
	known   map[uuid.UUID]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{revoked: map[uuid.UUID]bool{}, known: map[uuid.UUID]bool{}}
}
func (s *fakeStore) Save(_ context.Context, jti, _ uuid.UUID, _ time.Time) error {
	s.known[jti] = true
	return nil
}
func (s *fakeStore) IsActive(_ context.Context, jti uuid.UUID) (bool, error) {
	return s.known[jti] && !s.revoked[jti], nil
}
func (s *fakeStore) Revoke(_ context.Context, jti uuid.UUID) error {
	s.revoked[jti] = true
	return nil
}
func (s *fakeStore) RevokeAllForUser(_ context.Context, _ uuid.UUID) error { return nil }

func newTestServer(t *testing.T) (*gin.Engine, *auth.Handler) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := newFakeUserRepo()
	uc := user.NewUsecase(repo)
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tokens := auth.NewTokenIssuer(priv, &priv.PublicKey, config.JWTConfig{
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
		Issuer:     "goapp-test",
	})
	authUC := auth.NewUsecase(uc, tokens, newFakeStore())
	h := auth.NewHandler(authUC)

	r := gin.New()
	auth.RegisterRoutes(r.Group("/api/v1"), h)
	return r, h
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRegister_Success(t *testing.T) {
	r, _ := newTestServer(t)

	w := doJSON(t, r, http.MethodPost, "/api/v1/auth/register", auth.RegisterRequest{
		Email:    "alice@example.com",
		Name:     "Alice",
		Password: "supersecret",
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var resp auth.TokenResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("tokens empty: %+v", resp)
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want Bearer", resp.TokenType)
	}
}

func TestRegister_ValidationError(t *testing.T) {
	r, _ := newTestServer(t)

	w := doJSON(t, r, http.MethodPost, "/api/v1/auth/register", auth.RegisterRequest{
		Email:    "not-an-email",
		Name:     "",
		Password: "short",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	var resp httpx.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Code != "request.invalid" {
		t.Errorf("error.code = %q, want request.invalid", resp.Error.Code)
	}
	if len(resp.Error.Details) == 0 {
		t.Errorf("expected validation details, got none")
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	r, _ := newTestServer(t)

	// register first
	_ = doJSON(t, r, http.MethodPost, "/api/v1/auth/register", auth.RegisterRequest{
		Email:    "bob@example.com",
		Name:     "Bob",
		Password: "correctpassword",
	})

	w := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", auth.LoginRequest{
		Email:    "bob@example.com",
		Password: "wrongpassword",
	})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
	var resp httpx.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Code != "auth.invalid_credentials" {
		t.Errorf("error.code = %q, want auth.invalid_credentials", resp.Error.Code)
	}
}

func TestLogin_Success(t *testing.T) {
	r, _ := newTestServer(t)

	_ = doJSON(t, r, http.MethodPost, "/api/v1/auth/register", auth.RegisterRequest{
		Email:    "carol@example.com",
		Name:     "Carol",
		Password: "correctpassword",
	})

	w := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", auth.LoginRequest{
		Email:    "carol@example.com",
		Password: "correctpassword",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestRefresh_Success(t *testing.T) {
	r, _ := newTestServer(t)

	wReg := doJSON(t, r, http.MethodPost, "/api/v1/auth/register", auth.RegisterRequest{
		Email:    "dave@example.com",
		Name:     "Dave",
		Password: "correctpassword",
	})
	var tokens auth.TokenResponse
	_ = json.Unmarshal(wReg.Body.Bytes(), &tokens)

	w := doJSON(t, r, http.MethodPost, "/api/v1/auth/refresh", auth.RefreshRequest{
		RefreshToken: tokens.RefreshToken,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp auth.TokenResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AccessToken == "" {
		t.Errorf("expected new access token")
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	r, _ := newTestServer(t)

	w := doJSON(t, r, http.MethodPost, "/api/v1/auth/refresh", auth.RefreshRequest{
		RefreshToken: "garbage.token.value",
	})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
}

// TestRefresh_RotationRevokesOld verifies a refresh token cannot be replayed
// after it has been used once (rotation).
func TestRefresh_RotationRevokesOld(t *testing.T) {
	r, _ := newTestServer(t)

	wReg := doJSON(t, r, http.MethodPost, "/api/v1/auth/register", auth.RegisterRequest{
		Email: "eve@example.com", Name: "Eve", Password: "correctpassword",
	})
	var tokens auth.TokenResponse
	_ = json.Unmarshal(wReg.Body.Bytes(), &tokens)

	// first use succeeds
	w1 := doJSON(t, r, http.MethodPost, "/api/v1/auth/refresh", auth.RefreshRequest{RefreshToken: tokens.RefreshToken})
	if w1.Code != http.StatusOK {
		t.Fatalf("first refresh = %d, want 200", w1.Code)
	}
	// replaying the same (now revoked) token fails
	w2 := doJSON(t, r, http.MethodPost, "/api/v1/auth/refresh", auth.RefreshRequest{RefreshToken: tokens.RefreshToken})
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("replayed refresh = %d, want 401; body=%s", w2.Code, w2.Body.String())
	}
}

func TestLogout_RevokesToken(t *testing.T) {
	r, _ := newTestServer(t)

	wReg := doJSON(t, r, http.MethodPost, "/api/v1/auth/register", auth.RegisterRequest{
		Email: "frank@example.com", Name: "Frank", Password: "correctpassword",
	})
	var tokens auth.TokenResponse
	_ = json.Unmarshal(wReg.Body.Bytes(), &tokens)

	wOut := doJSON(t, r, http.MethodPost, "/api/v1/auth/logout", auth.RefreshRequest{RefreshToken: tokens.RefreshToken})
	if wOut.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204; body=%s", wOut.Code, wOut.Body.String())
	}
	// token is now unusable
	w := doJSON(t, r, http.MethodPost, "/api/v1/auth/refresh", auth.RefreshRequest{RefreshToken: tokens.RefreshToken})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout = %d, want 401", w.Code)
	}
}
