package user

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Roles recognised by the RBAC middleware.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// User is the core entity. It deliberately does not carry persistence or
// transport concerns — those live in repository.go / dto.go respectively.
type User struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ListFilter narrows and paginates a user listing. Search matches email or
// name (case-insensitive); empty means no filter.
type ListFilter struct {
	Search string
	Limit  int32
	Offset int32
}

// Repository is the persistence port. The usecase depends only on this
// interface, which keeps storage details swappable (postgres, in-memory tests).
type Repository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	UpdateName(ctx context.Context, id uuid.UUID, name string) (*User, error)
	UpdateRole(ctx context.Context, id uuid.UUID, role string) (*User, error)
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, f ListFilter) ([]User, error)
	Count(ctx context.Context, search string) (int64, error)
}
