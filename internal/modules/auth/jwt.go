package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/UzStack/jst-go/internal/shared/config"
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
	Role      string `json:"role,omitempty"`
}

const (
	typeAccess  = "access"
	typeRefresh = "refresh"
)

// NewAccessToken issues a short-lived access token carrying the user's role.
func (t *TokenIssuer) NewAccessToken(userID uuid.UUID, role string) (string, time.Time, error) {
	signed, _, exp, err := t.sign(userID, typeAccess, role, t.cfg.AccessTTL)
	return signed, exp, err
}

// NewRefreshToken issues a refresh token and returns its jti so the caller can
// persist it for revocation/rotation.
func (t *TokenIssuer) NewRefreshToken(userID uuid.UUID) (token string, jti uuid.UUID, exp time.Time, err error) {
	return t.sign(userID, typeRefresh, "", t.cfg.RefreshTTL)
}

func (t *TokenIssuer) sign(userID uuid.UUID, kind, role string, ttl time.Duration) (string, uuid.UUID, time.Time, error) {
	expiresAt := time.Now().Add(ttl)
	jti := uuid.New()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti.String(),
			Subject:   userID.String(),
			Issuer:    t.cfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		TokenType: kind,
		Role:      role,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(t.cfg.Secret))
	if err != nil {
		return "", uuid.Nil, time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, jti, expiresAt, nil
}

// VerifyAccessToken implements middleware.TokenVerifier, returning the user id
// and role encoded in the token.
func (t *TokenIssuer) VerifyAccessToken(token string) (userID, role string, err error) {
	claims, err := t.verify(token, typeAccess)
	if err != nil {
		return "", "", err
	}
	return claims.Subject, claims.Role, nil
}

// VerifyRefreshToken returns the user id and jti of a valid refresh token.
func (t *TokenIssuer) VerifyRefreshToken(token string) (userID string, jti uuid.UUID, err error) {
	claims, err := t.verify(token, typeRefresh)
	if err != nil {
		return "", uuid.Nil, err
	}
	id, err := uuid.Parse(claims.ID)
	if err != nil {
		return "", uuid.Nil, errors.New("missing token id")
	}
	return claims.Subject, id, nil
}

func (t *TokenIssuer) verify(token, expectedType string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(tk *jwt.Token) (any, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", tk.Header["alg"])
		}
		return []byte(t.cfg.Secret), nil
	}, jwt.WithIssuer(t.cfg.Issuer), jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.TokenType != expectedType {
		return nil, errors.New("wrong token type")
	}
	if claims.Subject == "" {
		return nil, errors.New("missing subject")
	}
	return claims, nil
}
