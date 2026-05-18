package auth

import (
	"context"

	"github.com/example/goapp/internal/modules/user"
	"github.com/example/goapp/internal/shared/httpx"
	"github.com/google/uuid"
)

// Usecase orchestrates registration, login, and token refresh by composing
// the user module's usecase with the token issuer.
type Usecase struct {
	users  user.Usecase
	tokens *TokenIssuer
}

func NewUsecase(users user.Usecase, tokens *TokenIssuer) *Usecase {
	return &Usecase{users: users, tokens: tokens}
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
	return u.issueTokens(created.ID)
}

func (u *Usecase) Login(ctx context.Context, in LoginRequest) (TokenResponse, error) {
	found, err := u.users.Authenticate(ctx, in.Email, in.Password)
	if err != nil {
		return TokenResponse{}, err
	}
	return u.issueTokens(found.ID)
}

func (u *Usecase) Refresh(ctx context.Context, in RefreshRequest) (TokenResponse, error) {
	uidStr, err := u.tokens.VerifyRefreshToken(in.RefreshToken)
	if err != nil {
		return TokenResponse{}, httpx.Unauthorized("auth.invalid_refresh", "invalid refresh token")
	}
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		return TokenResponse{}, httpx.Unauthorized("auth.invalid_refresh", "invalid refresh token")
	}
	if _, err := u.users.GetByID(ctx, uid); err != nil {
		return TokenResponse{}, err
	}
	return u.issueTokens(uid)
}

func (u *Usecase) issueTokens(userID uuid.UUID) (TokenResponse, error) {
	access, accessExp, err := u.tokens.NewAccessToken(userID)
	if err != nil {
		return TokenResponse{}, httpx.Internal(err)
	}
	refresh, refreshExp, err := u.tokens.NewRefreshToken(userID)
	if err != nil {
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
