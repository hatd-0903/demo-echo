package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"demo_echo/internal/auth"
	"demo_echo/internal/cache"
	"demo_echo/internal/dto"
	"demo_echo/internal/routes"
	"demo_echo/internal/server"

	"github.com/labstack/echo/v4"
)

const testSecret = "test-secret-0123456789abcdef0123456789"

type testApp struct {
	e      *echo.Echo
	db     *fakeDB
	cache  cache.Cache
	tokens *auth.TokenManager
}

type appOption func(*routes.Deps)

func withCache(c cache.Cache) appOption {
	return func(d *routes.Deps) { d.Cache = c }
}

// newTestApp dựng đúng stack HTTP như production (server.New: error handler,
// middleware, routes) nhưng thay MySQL bằng fake repository trong bộ nhớ.
func newTestApp(t *testing.T, opts ...appOption) *testApp {
	t.Helper()
	db := newFakeDB()
	users, articles, tags, comments, follows := db.repos()
	deps := routes.Deps{
		Users: users, Articles: articles, Tags: tags, Comments: comments, Follows: follows,
		Tokens:   auth.NewTokenManager(testSecret, time.Hour),
		Cache:    cache.NewMemory(),
		CacheTTL: time.Minute,
	}
	for _, opt := range opts {
		opt(&deps)
	}
	return &testApp{
		e:      server.New(deps, server.Options{}),
		db:     db,
		cache:  deps.Cache,
		tokens: deps.Tokens,
	}
}

// do gửi request qua httptest.NewRecorder (không mở cổng mạng thật).
// body có thể là nil, string (JSON thô) hoặc struct/map (sẽ được marshal).
func (a *testApp) do(t *testing.T, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	switch b := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(b))
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if token != "" {
		req.Header.Set(echo.HeaderAuthorization, "Token "+token)
	}
	return a.serve(req)
}

// serve chạy một request tuỳ biến (header riêng...) qua toàn bộ stack.
func (a *testApp) serve(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	a.e.ServeHTTP(rec, req)
	return rec
}

type envelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// decode kiểm tra status code rồi parse body theo format chung {success, message, data}.
func decode[T any](t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) envelope[T] {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, wantStatus, rec.Body.String())
	}
	var env envelope[T]
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON body %q: %v", rec.Body.String(), err)
	}
	if env.Success != (wantStatus < 400) {
		t.Fatalf("success = %v for status %d; body = %s", env.Success, wantStatus, rec.Body.String())
	}
	return env
}

// expectError kiểm tra status và message lỗi.
func expectError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantMessage string) {
	t.Helper()
	env := decode[any](t, rec, wantStatus)
	if wantMessage != "" && env.Message != wantMessage {
		t.Fatalf("message = %q, want %q", env.Message, wantMessage)
	}
}

// register tạo user qua API và trả về token.
func (a *testApp) register(t *testing.T, username string) (string, dto.AuthUserResponse) {
	t.Helper()
	rec := a.do(t, http.MethodPost, "/api/users", map[string]string{
		"username": username,
		"email":    username + "@example.com",
		"password": "secret123",
	}, "")
	user := decode[dto.AuthUserResponse](t, rec, http.StatusCreated).Data
	return user.Token, user
}

func (a *testApp) createTag(t *testing.T, token, name string) dto.TagResponse {
	t.Helper()
	rec := a.do(t, http.MethodPost, "/api/tags", map[string]string{"name": name}, token)
	return decode[dto.TagResponse](t, rec, http.StatusCreated).Data
}

func (a *testApp) createArticle(t *testing.T, token, title string, tagIDs ...uint) dto.ArticleResponse {
	t.Helper()
	if tagIDs == nil {
		tagIDs = []uint{}
	}
	rec := a.do(t, http.MethodPost, "/api/articles", map[string]any{
		"title": title, "body": "body of " + title, "tag_ids": tagIDs,
	}, token)
	return decode[dto.ArticleResponse](t, rec, http.StatusCreated).Data
}

func slugsOf(articles []dto.ArticleResponse) []string {
	slugs := make([]string, len(articles))
	for i, a := range articles {
		slugs[i] = a.Slug
	}
	return slugs
}

func equalStrings(a, b []string) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func itoa(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
