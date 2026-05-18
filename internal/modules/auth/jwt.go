package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/example/goapp/internal/shared/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenIssuer creates and verifies access tokens. It implements
// middleware.TokenVerifier.
type TokenIssuer struct {
	cfg config.JWTConfig
}

func NewTokenIssuer(cfg config.JWTConfig) *TokenIssuer {
	return &TokenIssuer{cfg: cfg}
}

type Claims struct {
	jwt.RegisteredClaims
	TokenType string `json:"typ"`
}

const (
	typeAccess  = "access"
	typeRefresh = "refresh"
)

func (t *TokenIssuer) NewAccessToken(userID uuid.UUID) (string, time.Time, error) {
	return t.sign(userID, typeAccess, t.cfg.AccessTTL)
}

func (t *TokenIssuer) NewRefreshToken(userID uuid.UUID) (string, time.Time, error) {
	return t.sign(userID, typeRefresh, t.cfg.RefreshTTL)
}

func (t *TokenIssuer) sign(userID uuid.UUID, kind string, ttl time.Duration) (string, time.Time, error) {
	expiresAt := time.Now().Add(ttl)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    t.cfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		TokenType: kind,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(t.cfg.Secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, expiresAt, nil
}

// VerifyAccessToken implements middleware.TokenVerifier.
func (t *TokenIssuer) VerifyAccessToken(token string) (string, error) {
	return t.verify(token, typeAccess)
}

func (t *TokenIssuer) VerifyRefreshToken(token string) (string, error) {
	return t.verify(token, typeRefresh)
}

func (t *TokenIssuer) verify(token, expectedType string) (string, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(tk *jwt.Token) (any, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", tk.Header["alg"])
		}
		return []byte(t.cfg.Secret), nil
	}, jwt.WithIssuer(t.cfg.Issuer), jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return "", err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return "", errors.New("invalid token")
	}
	if claims.TokenType != expectedType {
		return "", errors.New("wrong token type")
	}
	if claims.Subject == "" {
		return "", errors.New("missing subject")
	}
	return claims.Subject, nil
}
