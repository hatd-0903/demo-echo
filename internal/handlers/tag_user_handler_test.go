package handlers_test

import (
	"net/http"
	"strings"
	"testing"

	"demo_echo/internal/dto"
)

func TestTagCRUD(t *testing.T) {
	app := newTestApp(t)
	token, admin := app.register(t, "ha")
	app.makeAdmin(t, admin.ID)
	tag := app.createTag(t, token, "go")
	article := app.createArticle(t, token, "Tagged", tag.ID)
	path := "/api/tags/" + itoa(tag.ID)

	if got := decode[dto.TagResponse](t, app.do(t, http.MethodGet, path, nil, ""), http.StatusOK).Data; got.Name != "go" {
		t.Errorf("tag = %+v", got)
	}
	expectError(t, app.do(t, http.MethodGet, "/api/tags/999", nil, ""), http.StatusNotFound, "")
	expectError(t, app.do(t, http.MethodPost, "/api/tags", `{"name":" "}`, token), http.StatusUnprocessableEntity, "name is required")
	expectError(t, app.do(t, http.MethodDelete, path, nil, ""), http.StatusUnauthorized, "")

	if rec := app.do(t, http.MethodDelete, path, nil, token); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	expectError(t, app.do(t, http.MethodGet, path, nil, ""), http.StatusNotFound, "")

	// Xoá tag thì article không còn gắn tag đó.
	got := decode[dto.ArticleResponse](t, app.do(t, http.MethodGet, "/api/articles/"+article.Slug, nil, ""), http.StatusOK).Data
	if len(got.Tags) != 0 {
		t.Errorf("article still has deleted tag: %+v", got.Tags)
	}
}

func TestUserEndpointsRequireLogin(t *testing.T) {
	app := newTestApp(t)
	haToken, ha := app.register(t, "ha")
	app.register(t, "lan")

	expectError(t, app.do(t, http.MethodGet, "/api/users", nil, ""), http.StatusUnauthorized, "")

	rec := app.do(t, http.MethodGet, "/api/users", nil, haToken)
	users := decode[[]dto.ProfileResponse](t, rec, http.StatusOK).Data
	if len(users) != 2 {
		t.Errorf("users = %+v", users)
	}
	for _, field := range []string{"password", "email", "@example.com", "role"} {
		if strings.Contains(rec.Body.String(), field) {
			t.Errorf("user list must not expose %q: %s", field, rec.Body.String())
		}
	}

	path := "/api/users/" + itoa(ha.ID)
	rec = app.do(t, http.MethodGet, path, nil, haToken)
	if got := decode[dto.ProfileResponse](t, rec, http.StatusOK).Data; got.Username != "ha" {
		t.Errorf("user = %+v", got)
	}
	if strings.Contains(rec.Body.String(), "@example.com") {
		t.Errorf("user detail must not expose email: %s", rec.Body.String())
	}

	// Email + role chỉ có ở thông tin của chính mình.
	me := decode[dto.AuthUserResponse](t, app.do(t, http.MethodGet, "/api/user", nil, haToken), http.StatusOK).Data
	if me.Email != "ha@example.com" || me.Role != "user" {
		t.Errorf("GET /api/user = %+v", me.UserResponse)
	}
	expectError(t, app.do(t, http.MethodGet, "/api/users/999", nil, haToken), http.StatusNotFound, "")

	if rec := app.do(t, http.MethodDelete, path, nil, haToken); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	// Token vẫn còn hạn nhưng tài khoản đã bị xoá.
	expectError(t, app.do(t, http.MethodGet, "/api/user", nil, haToken), http.StatusUnauthorized, "user no longer exists")
}
