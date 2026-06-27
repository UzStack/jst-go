package auth

import (
	"context"

	"github.com/UzStack/jst-go/internal/modules/user"
	"github.com/UzStack/jst-go/internal/shared/httpx"
	"github.com/google/uuid"
)

// Usecase orchestrates registration, login, token refresh, and logout by
// composing the user module's usecase, the token issuer, and a refresh-token
// store (for revocation/rotation).
type Usecase struct {
	users  user.Usecase
	tokens *TokenIssuer
	store  RefreshStore
}

func NewUsecase(users user.Usecase, tokens *TokenIssuer, store RefreshStore) *Usecase {
	return &Usecase{users: users, tokens: tokens, store: store}
}

func (u *Usecase) Register(ctx context.Context, in RegisterRequest) (TokenResponse, error) {
	created, err := u.users.Register(ctx, user.RegisterInput{
		Email:    in.Email,
		Name:     in.Name,
		Password: in.Password,
	})
	if err != nil {
		return TokenResponse{}, err
	}
	return u.issueTokens(ctx, created.ID, created.Role)
}

func (u *Usecase) Login(ctx context.Context, in LoginRequest) (TokenResponse, error) {
	found, err := u.users.Authenticate(ctx, in.Email, in.Password)
	if err != nil {
		return TokenResponse{}, err
	}
	return u.issueTokens(ctx, found.ID, found.Role)
}

// Refresh validates a refresh token, rotates it (revoking the presented one),
// and issues a fresh pair. A revoked/expired/unknown token is rejected.
func (u *Usecase) Refresh(ctx context.Context, in RefreshRequest) (TokenResponse, error) {
	uidStr, jti, err := u.tokens.VerifyRefreshToken(in.RefreshToken)
	if err != nil {
		return TokenResponse{}, httpx.Unauthorized("auth.invalid_refresh", "invalid refresh token")
	}
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		return TokenResponse{}, httpx.Unauthorized("auth.invalid_refresh", "invalid refresh token")
	}

	active, err := u.store.IsActive(ctx, jti)
	if err != nil {
		return TokenResponse{}, httpx.Internal(err)
	}
	if !active {
		return TokenResponse{}, httpx.Unauthorized("auth.invalid_refresh", "refresh token revoked or expired")
	}

	usr, err := u.users.GetByID(ctx, uid)
	if err != nil {
		return TokenResponse{}, err
	}

	// Rotation: invalidate the old token so it cannot be replayed.
	if err := u.store.Revoke(ctx, jti); err != nil {
		return TokenResponse{}, httpx.Internal(err)
	}
	return u.issueTokens(ctx, usr.ID, usr.Role)
}

// Logout revokes the presented refresh token. An invalid token is treated as
// success (idempotent) so logout never leaks token validity.
func (u *Usecase) Logout(ctx context.Context, in RefreshRequest) error {
	_, jti, err := u.tokens.VerifyRefreshToken(in.RefreshToken)
	if err != nil {
		return nil
	}
	if err := u.store.Revoke(ctx, jti); err != nil {
		return httpx.Internal(err)
	}
	return nil
}

func (u *Usecase) issueTokens(ctx context.Context, userID uuid.UUID, role string) (TokenResponse, error) {
	access, accessExp, err := u.tokens.NewAccessToken(userID, role)
	if err != nil {
		return TokenResponse{}, httpx.Internal(err)
	}
	refresh, jti, refreshExp, err := u.tokens.NewRefreshToken(userID)
	if err != nil {
		return TokenResponse{}, httpx.Internal(err)
	}
	if err := u.store.Save(ctx, jti, userID, refreshExp); err != nil {
		return TokenResponse{}, httpx.Internal(err)
	}
	return TokenResponse{
		AccessToken:      access,
		RefreshToken:     refresh,
		AccessExpiresAt:  accessExp,
		RefreshExpiresAt: refreshExp,
		TokenType:        "Bearer",
	}, nil
}
