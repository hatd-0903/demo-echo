package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "secret123" {
		t.Fatal("hash must not equal the plain password")
	}
	if !CheckPassword(hash, "secret123") {
		t.Error("correct password should match")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("wrong password should not match")
	}

	other, _ := HashPassword("secret123")
	if other == hash {
		t.Error("bcrypt should salt: two hashes of the same password must differ")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	tm := NewTokenManager("test-secret", time.Hour)
	signed, err := tm.Generate(42, "ha")
	if err != nil {
		t.Fatal(err)
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(signed, claims, func(*jwt.Token) (any, error) { return tm.Secret(), nil })
	if err != nil || !token.Valid {
		t.Fatalf("token should be valid: %v", err)
	}
	if id, err := claims.UserID(); err != nil || id != 42 {
		t.Errorf("UserID() = %d, %v", id, err)
	}
	if claims.Username != "ha" {
		t.Errorf("username = %q", claims.Username)
	}
	if ttl := time.Until(claims.ExpiresAt.Time); ttl < 59*time.Minute || ttl > time.Hour {
		t.Errorf("unexpected expiry in %v", ttl)
	}
}

func TestTokenRejectsWrongSecretAndExpired(t *testing.T) {
	signed, _ := NewTokenManager("secret-a", time.Hour).Generate(1, "ha")
	if _, err := jwt.ParseWithClaims(signed, &Claims{}, func(*jwt.Token) (any, error) { return []byte("secret-b"), nil }); err == nil {
		t.Error("token signed with another secret must be rejected")
	}

	tm := NewTokenManager("secret-a", -time.Minute)
	expired, _ := tm.Generate(1, "ha")
	if _, err := jwt.ParseWithClaims(expired, &Claims{}, func(*jwt.Token) (any, error) { return tm.Secret(), nil }); err == nil {
		t.Error("expired token must be rejected")
	}
}
