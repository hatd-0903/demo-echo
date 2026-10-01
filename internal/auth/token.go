package auth

import (
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims là payload của JWT. Subject chứa user ID.
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func (c *Claims) UserID() (uint, error) {
	id, err := strconv.ParseUint(c.Subject, 10, 64)
	return uint(id), err
}

type TokenManager struct {
	secret []byte
	expiry time.Duration
}

func NewTokenManager(secret string, expiry time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), expiry: expiry}
}

func (m *TokenManager) Secret() []byte {
	return m.secret
}

// Generate ký JWT (HS256) cho user.
func (m *TokenManager) Generate(userID uint, username string) (string, error) {
	now := time.Now()
	claims := Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(userID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.expiry)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}
