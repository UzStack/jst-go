package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/UzStack/jst-go/internal/modules/user"
	"github.com/UzStack/jst-go/internal/shared/database"
	"github.com/UzStack/jst-go/internal/shared/httpx"
	"github.com/google/uuid"
)

// fakeRepo is a minimal in-memory Repository for usecase tests. Real
// integration tests should hit a real postgres (see README for testcontainers
// pattern).
type fakeRepo struct {
	byID    map[uuid.UUID]*user.User
	byEmail map[string]*user.User
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		byID:    map[uuid.UUID]*user.User{},
		byEmail: map[string]*user.User{},
	}
}

func (r *fakeRepo) Create(_ context.Context, u *user.User) error {
	if _, exists := r.byEmail[u.Email]; exists {
		return user.ErrEmailTaken
	}
	r.byID[u.ID] = u
	r.byEmail[u.Email] = u
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*user.User, error) {
	u, ok := r.byID[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	return u, nil
}

func (r *fakeRepo) GetByEmail(_ context.Context, email string) (*user.User, error) {
	u, ok := r.byEmail[email]
	if !ok {
		return nil, database.ErrNotFound
	}
	return u, nil
}

func (r *fakeRepo) UpdateName(_ context.Context, id uuid.UUID, name string) (*user.User, error) {
	u, ok := r.byID[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	u.Name = name
	return u, nil
}

func (r *fakeRepo) UpdateRole(_ context.Context, id uuid.UUID, role string) (*user.User, error) {
	u, ok := r.byID[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	u.Role = role
	return u, nil
}

func (r *fakeRepo) Delete(_ context.Context, id uuid.UUID) error {
	u, ok := r.byID[id]
	if !ok {
		return database.ErrNotFound
	}
	delete(r.byID, id)
	delete(r.byEmail, u.Email)
	return nil
}

func (r *fakeRepo) List(_ context.Context, f user.ListFilter) ([]user.User, error) {
	out := make([]user.User, 0, len(r.byID))
	for _, u := range r.byID {
		out = append(out, *u)
	}
	return out, nil
}

func (r *fakeRepo) Count(_ context.Context, _ string) (int64, error) {
	return int64(len(r.byID)), nil
}

func TestRegister_AndAuthenticate(t *testing.T) {
	uc := user.NewUsecase(newFakeRepo())

	created, err := uc.Register(context.Background(), user.RegisterInput{
		Email:    "alice@example.com",
		Name:     "Alice",
		Password: "supersecret",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if created.Email != "alice@example.com" {
		t.Errorf("unexpected email: %s", created.Email)
	}

	got, err := uc.Authenticate(context.Background(), "alice@example.com", "supersecret")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("got different user id")
	}

	if _, err := uc.Authenticate(context.Background(), "alice@example.com", "wrong"); err == nil {
		t.Fatal("expected error on wrong password")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	uc := user.NewUsecase(newFakeRepo())
	in := user.RegisterInput{Email: "dup@example.com", Name: "Dup", Password: "longenough"}
	if _, err := uc.Register(context.Background(), in); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err := uc.Register(context.Background(), in)
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	var ae *httpx.AppError
	if !errors.As(err, &ae) || ae.Code != "user.email_taken" {
		t.Errorf("expected email_taken AppError, got %v", err)
	}
}
