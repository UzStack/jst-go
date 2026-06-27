package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/UzStack/jst-go/internal/shared/database"
	sqlcdb "github.com/UzStack/jst-go/internal/shared/database/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgRepo wraps sqlc-generated queries. sqlc handles the SQL <-> Go mapping;
// this adapter only does:
//   - domain <-> persistence model translation (sqlcdb.User -> user.User)
//   - error translation (pgx.ErrNoRows -> database.ErrNotFound, unique
//     violation -> ErrEmailTaken)
//
// Keeping the persistence model (sqlcdb.User) out of the domain means the
// usecase has no idea sqlc exists and stays trivially testable.
type pgRepo struct {
	queries *sqlcdb.Queries
}

func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &pgRepo{queries: sqlcdb.New(pool)}
}

const uniqueViolation = "23505"

func (r *pgRepo) Create(ctx context.Context, u *User) error {
	row, err := r.queries.CreateUser(ctx, sqlcdb.CreateUserParams{
		ID:           u.ID,
		Email:        u.Email,
		Name:         u.Name,
		PasswordHash: u.PasswordHash,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return ErrEmailTaken
		}
		return fmt.Errorf("create user: %w", err)
	}
	*u = fromSQLC(row)
	return nil
}

func (r *pgRepo) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	row, err := r.queries.GetUserByID(ctx, id)
	return mapOne(row, err)
}

func (r *pgRepo) GetByEmail(ctx context.Context, email string) (*User, error) {
	row, err := r.queries.GetUserByEmail(ctx, email)
	return mapOne(row, err)
}

func (r *pgRepo) UpdateName(ctx context.Context, id uuid.UUID, name string) (*User, error) {
	row, err := r.queries.UpdateUserName(ctx, sqlcdb.UpdateUserNameParams{
		ID:   id,
		Name: name,
	})
	return mapOne(row, err)
}

func (r *pgRepo) UpdateRole(ctx context.Context, id uuid.UUID, role string) (*User, error) {
	row, err := r.queries.UpdateUserRole(ctx, sqlcdb.UpdateUserRoleParams{
		ID:   id,
		Role: role,
	})
	return mapOne(row, err)
}

func (r *pgRepo) List(ctx context.Context, f ListFilter) ([]User, error) {
	var search *string
	if f.Search != "" {
		search = &f.Search
	}
	rows, err := r.queries.ListUsers(ctx, sqlcdb.ListUsersParams{
		Search: search,
		Limit:  f.Limit,
		Offset: f.Offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	out := make([]User, len(rows))
	for i, row := range rows {
		out[i] = fromSQLC(row)
	}
	return out, nil
}

func (r *pgRepo) Count(ctx context.Context, search string) (int64, error) {
	var s *string
	if search != "" {
		s = &search
	}
	n, err := r.queries.CountUsers(ctx, s)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

func (r *pgRepo) Delete(ctx context.Context, id uuid.UUID) error {
	rows, err := r.queries.DeleteUser(ctx, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
}

func mapOne(row sqlcdb.User, err error) (*User, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, database.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	u := fromSQLC(row)
	return &u, nil
}

func fromSQLC(r sqlcdb.User) User {
	return User{
		ID:           r.ID,
		Email:        r.Email,
		Name:         r.Name,
		PasswordHash: r.PasswordHash,
		Role:         r.Role,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}
