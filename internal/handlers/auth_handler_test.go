package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"demo_echo/internal/auth"
	"demo_echo/internal/dto"
)

func TestRegister(t *testing.T) {
	app := newTestApp(t)

	rec := app.do(t, http.MethodPost, "/api/users",
		`{"username":"ha","email":"  HA@Example.com ","password":"secret123"}`, "")
	user := decode[dto.AuthUserResponse](t, rec, http.StatusCreated).Data

	if user.Username != "ha" || user.Email != "ha@example.com" {
		t.Errorf("unexpected user: %+v (email should be trimmed + lowercased)", user.UserResponse)
	}
	if user.Token == "" {
		t.Fatal("expected a token")
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Errorf("response must not contain password: %s", rec.Body.String())
	}

	stored := app.db.users[user.ID]
	if stored.Password == "secret123" || !auth.CheckPassword(stored.Password, "secret123") {
		t.Errorf("password must be stored as a bcrypt hash, got %q", stored.Password)
	}
}

func TestRegisterValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		status  int
		message string
	}{
		{"missing username", `{"email":"a@b.com","password":"secret123"}`, http.StatusUnprocessableEntity, "username is required"},
		{"invalid email", `{"username":"a","email":"nope","password":"secret123"}`, http.StatusUnprocessableEntity, "email is invalid"},
		{"short password", `{"username":"a","email":"a@b.com","password":"123"}`, http.StatusUnprocessableEntity, "password must be at least 6 characters"},
		{"malformed JSON", `{"username":`, http.StatusBadRequest, "invalid request body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApp(t)
			expectError(t, app.do(t, http.MethodPost, "/api/users", tt.body, ""), tt.status, tt.message)
		})
	}
}

func TestRegisterDuplicate(t *testing.T) {
	app := newTestApp(t)
	app.register(t, "ha")

	rec := app.do(t, http.MethodPost, "/api/users",
		`{"username":"ha","email":"other@example.com","password":"secret123"}`, "")
	expectError(t, rec, http.StatusConflict, "username or email already taken")
}

func TestLogin(t *testing.T) {
	app := newTestApp(t)
	app.register(t, "ha")

	tests := []struct {
		name   string
		body   string
		status int
	}{
		{"valid credentials", `{"email":"ha@example.com","password":"secret123"}`, http.StatusOK},
		{"email is case-insensitive", `{"email":"HA@example.com","password":"secret123"}`, http.StatusOK},
		{"wrong password", `{"email":"ha@example.com","password":"wrong-pass"}`, http.StatusUnauthorized},
		{"unknown email", `{"email":"ghost@example.com","password":"secret123"}`, http.StatusUnauthorized},
		{"missing fields", `{}`, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := app.do(t, http.MethodPost, "/api/users/login", tt.body, "")
			if tt.status != http.StatusOK {
				env := decode[any](t, rec, tt.status)
				// Sai email và sai mật khẩu phải cùng thông báo để không lộ email nào tồn tại.
				if tt.status == http.StatusUnauthorized && env.Message != "invalid email or password" {
					t.Errorf("message = %q", env.Message)
				}
				return
			}
			if user := decode[dto.AuthUserResponse](t, rec, http.StatusOK).Data; user.Token == "" {
				t.Error("expected a token")
			}
		})
	}
}

func TestCurrentUser(t *testing.T) {
	app := newTestApp(t)
	token, registered := app.register(t, "ha")

	t.Run("with Token scheme", func(t *testing.T) {
		user := decode[dto.AuthUserResponse](t, app.do(t, http.MethodGet, "/api/user", nil, token), http.StatusOK).Data
		if user.ID != registered.ID || user.Username != "ha" {
			t.Errorf("got user %+v", user.UserResponse)
		}
	})

	t.Run("with Bearer scheme", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		decode[dto.AuthUserResponse](t, app.serve(req), http.StatusOK)
	})

	expired, err := auth.NewTokenManager(testSecret, -time.Minute).Generate(registered.ID, "ha")
	if err != nil {
		t.Fatal(err)
	}
	otherSecret, _ := auth.NewTokenManager("another-secret-0123456789abcdef", time.Hour).Generate(registered.ID, "ha")

	for name, tok := range map[string]string{
		"no token":                   "",
		"garbage token":              "not-a-jwt",
		"expired token":              expired,
		"signed with another secret": otherSecret,
	} {
		t.Run(name, func(t *testing.T) {
			expectError(t, app.do(t, http.MethodGet, "/api/user", nil, tok), http.StatusUnauthorized, "missing or invalid token")
		})
	}

	t.Run("user deleted after token was issued", func(t *testing.T) {
		delete(app.db.users, registered.ID)
		expectError(t, app.do(t, http.MethodGet, "/api/user", nil, token), http.StatusUnauthorized, "user no longer exists")
	})
}
