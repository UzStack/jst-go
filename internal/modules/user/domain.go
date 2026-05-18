package user

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// User is the core entity. It deliberately does not carry persistence or
// transport concerns — those live in repository.go / dto.go respectively.
type User struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Repository is the persistence port. The usecase depends only on this
// interface, which keeps storage details swappable (postgres, in-memory tests).
type Repository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	UpdateName(ctx context.Context, id uuid.UUID, name string) (*User, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
