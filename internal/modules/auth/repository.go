package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/UzStack/jst-go/internal/shared/database"
	sqlcdb "github.com/UzStack/jst-go/internal/shared/database/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RefreshStore persists issued refresh tokens by their jti so they can be
// rotated and revoked (stateless JWTs alone cannot be invalidated early).
type RefreshStore interface {
	Save(ctx context.Context, jti, userID uuid.UUID, expiresAt time.Time) error
	IsActive(ctx context.Context, jti uuid.UUID) (bool, error)
	Revoke(ctx context.Context, jti uuid.UUID) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
}

// Queries are built per call from Store.DB(ctx) rather than bound once, so these
// writes join whatever transaction the caller's context carries.
type pgRefreshStore struct {
	store *database.Store
}

func NewRefreshStore(store *database.Store) RefreshStore {
	return &pgRefreshStore{store: store}
}

func (s *pgRefreshStore) q(ctx context.Context) *sqlcdb.Queries {
	return sqlcdb.New(s.store.DB(ctx))
}

func (s *pgRefreshStore) Save(ctx context.Context, jti, userID uuid.UUID, expiresAt time.Time) error {
	err := s.q(ctx).CreateRefreshToken(ctx, sqlcdb.CreateRefreshTokenParams{
		ID:        jti,
		UserID:    userID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

// IsActive reports whether the jti exists, is not revoked, and is not expired.
func (s *pgRefreshStore) IsActive(ctx context.Context, jti uuid.UUID) (bool, error) {
	row, err := s.q(ctx).GetRefreshToken(ctx, jti)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get refresh token: %w", err)
	}
	if row.RevokedAt.Valid || time.Now().After(row.ExpiresAt) {
		return false, nil
	}
	return true, nil
}

func (s *pgRefreshStore) Revoke(ctx context.Context, jti uuid.UUID) error {
	if _, err := s.q(ctx).RevokeRefreshToken(ctx, jti); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

func (s *pgRefreshStore) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	if err := s.q(ctx).RevokeAllUserRefreshTokens(ctx, userID); err != nil {
		return fmt.Errorf("revoke user refresh tokens: %w", err)
	}
	return nil
}
