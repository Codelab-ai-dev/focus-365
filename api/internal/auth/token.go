package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	accessTTL  = 15 * time.Minute
	refreshTTL = 7 * 24 * time.Hour
)

type TokenManager struct {
	secret []byte
}

func NewTokenManager(secret string) *TokenManager {
	return &TokenManager{secret: []byte(secret)}
}

func (tm *TokenManager) issue(userID uuid.UUID, ttl time.Duration, kind string) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"typ": kind,
		"exp": time.Now().Add(ttl).Unix(),
		"iat": time.Now().Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(tm.secret)
}

func (tm *TokenManager) IssueAccess(id uuid.UUID) (string, error) {
	return tm.issue(id, accessTTL, "access")
}

func (tm *TokenManager) IssueRefresh(id uuid.UUID) (string, error) {
	return tm.issue(id, refreshTTL, "refresh")
}

func (tm *TokenManager) parse(tokenStr, kind string) (uuid.UUID, error) {
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("método de firma inesperado")
		}
		return tm.secret, nil
	})
	if err != nil || !tok.Valid {
		return uuid.Nil, errors.New("token inválido")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok || claims["typ"] != kind {
		return uuid.Nil, errors.New("tipo de token inválido")
	}
	sub, _ := claims["sub"].(string)
	return uuid.Parse(sub)
}

func (tm *TokenManager) ParseAccess(tokenStr string) (uuid.UUID, error) {
	return tm.parse(tokenStr, "access")
}

func (tm *TokenManager) ParseRefresh(tokenStr string) (uuid.UUID, error) {
	return tm.parse(tokenStr, "refresh")
}

func AccessTTL() time.Duration  { return accessTTL }
func RefreshTTL() time.Duration { return refreshTTL }
