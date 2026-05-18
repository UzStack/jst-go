package user

import (
	"context"
	"errors"

	"github.com/example/goapp/internal/shared/database"
	"github.com/example/goapp/internal/shared/httpx"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// ErrEmailTaken is returned by the repository when a unique-violation occurs
// on the email column.
var ErrEmailTaken = errors.New("email already in use")

// Usecase exposes business operations for the user module. Other modules
// (e.g. auth) consume it via this interface, not the concrete type.
type Usecase interface {
	Register(ctx context.Context, input RegisterInput) (*User, error)
	Authenticate(ctx context.Context, email, password string) (*User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	UpdateName(ctx context.Context, id uuid.UUID, name string) (*User, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type RegisterInput struct {
	Email    string
	Name     string
	Password string
}

type usecase struct {
	repo Repository
}

func NewUsecase(repo Repository) Usecase {
	return &usecase{repo: repo}
}

func (u *usecase) Register(ctx context.Context, input RegisterInput) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, httpx.Internal(err)
	}

	user := &User{
		ID:           uuid.New(),
		Email:        input.Email,
		Name:         input.Name,
		PasswordHash: string(hash),
	}

	if err := u.repo.Create(ctx, user); err != nil {
		if errors.Is(err, ErrEmailTaken) {
			return nil, httpx.Conflict("user.email_taken", "email already in use")
		}
		return nil, httpx.Internal(err)
	}
	return user, nil
}

func (u *usecase) Authenticate(ctx context.Context, email, password string) (*User, error) {
	user, err := u.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, httpx.Unauthorized("auth.invalid_credentials", "invalid email or password")
		}
		return nil, httpx.Internal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, httpx.Unauthorized("auth.invalid_credentials", "invalid email or password")
	}
	return user, nil
}

func (u *usecase) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	user, err := u.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, httpx.NotFound("user.not_found", "user not found")
		}
		return nil, httpx.Internal(err)
	}
	return user, nil
}

func (u *usecase) UpdateName(ctx context.Context, id uuid.UUID, name string) (*User, error) {
	user, err := u.repo.UpdateName(ctx, id, name)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, httpx.NotFound("user.not_found", "user not found")
		}
		return nil, httpx.Internal(err)
	}
	return user, nil
}

func (u *usecase) Delete(ctx context.Context, id uuid.UUID) error {
	if err := u.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return httpx.NotFound("user.not_found", "user not found")
		}
		return httpx.Internal(err)
	}
	return nil
}
