package handlers_test

import (
	"net/http"
	"strings"
	"testing"

	"demo_echo/internal/dto"
)

func TestTagCRUD(t *testing.T) {
	app := newTestApp(t)
	token, _ := app.register(t, "ha")
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
	users := decode[[]dto.UserResponse](t, rec, http.StatusOK).Data
	if len(users) != 2 {
		t.Errorf("users = %+v", users)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Errorf("user list must not expose password: %s", rec.Body.String())
	}

	path := "/api/users/" + itoa(ha.ID)
	if got := decode[dto.UserResponse](t, app.do(t, http.MethodGet, path, nil, haToken), http.StatusOK).Data; got.Username != "ha" {
		t.Errorf("user = %+v", got)
	}
	expectError(t, app.do(t, http.MethodGet, "/api/users/999", nil, haToken), http.StatusNotFound, "")

	if rec := app.do(t, http.MethodDelete, path, nil, haToken); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	// Token vẫn còn hạn nhưng tài khoản đã bị xoá.
	expectError(t, app.do(t, http.MethodGet, "/api/user", nil, haToken), http.StatusUnauthorized, "user no longer exists")
}
