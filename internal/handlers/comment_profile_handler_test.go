package handlers_test

import (
	"net/http"
	"strings"
	"testing"

	"demo_echo/internal/dto"
)

func TestComments(t *testing.T) {
	app := newTestApp(t)
	authorToken, _ := app.register(t, "author")
	readerToken, _ := app.register(t, "reader")
	app.createArticle(t, authorToken, "Post")
	path := "/api/articles/post/comments"

	t.Run("posting requires login", func(t *testing.T) {
		expectError(t, app.do(t, http.MethodPost, path, `{"body":"hi"}`, ""), http.StatusUnauthorized, "")
	})

	t.Run("empty body rejected", func(t *testing.T) {
		expectError(t, app.do(t, http.MethodPost, path, `{"body":"   "}`, readerToken),
			http.StatusUnprocessableEntity, "body is required")
	})

	t.Run("unknown article", func(t *testing.T) {
		expectError(t, app.do(t, http.MethodPost, "/api/articles/nope/comments", `{"body":"hi"}`, readerToken),
			http.StatusNotFound, "")
		expectError(t, app.do(t, http.MethodGet, "/api/articles/nope/comments", nil, ""), http.StatusNotFound, "")
	})

	t.Run("post and list, newest first, author hides email", func(t *testing.T) {
		first := decode[dto.CommentResponse](t, app.do(t, http.MethodPost, path, `{"body":"first"}`, readerToken), http.StatusCreated).Data
		if first.Author == nil || first.Author.Username != "reader" {
			t.Errorf("author = %+v", first.Author)
		}
		decode[dto.CommentResponse](t, app.do(t, http.MethodPost, path, `{"body":"second"}`, authorToken), http.StatusCreated)

		rec := app.do(t, http.MethodGet, path, nil, "")
		page := decode[dto.CommentListResponse](t, rec, http.StatusOK).Data
		comments := page.Comments
		if len(comments) != 2 || comments[0].Body != "second" || comments[1].Body != "first" || page.CommentsCount != 2 {
			t.Errorf("comments = %+v", page)
		}
		if strings.Contains(rec.Body.String(), "@example.com") {
			t.Errorf("comment authors must not expose email: %s", rec.Body.String())
		}
	})

	t.Run("article without comments returns empty array", func(t *testing.T) {
		app.createArticle(t, authorToken, "Quiet")
		rec := app.do(t, http.MethodGet, "/api/articles/quiet/comments", nil, "")
		if !strings.Contains(rec.Body.String(), `"comments":[]`) {
			t.Errorf("want comments:[], got %s", rec.Body.String())
		}
	})

	t.Run("paginated", func(t *testing.T) {
		app.createArticle(t, authorToken, "Busy")
		for i := 1; i <= 5; i++ {
			decode[any](t, app.do(t, http.MethodPost, "/api/articles/busy/comments",
				map[string]string{"body": "c" + itoa(uint(i))}, readerToken), http.StatusCreated)
		}
		page := decode[dto.CommentListResponse](t, app.do(t, http.MethodGet, "/api/articles/busy/comments?limit=2&offset=2", nil, ""), http.StatusOK).Data
		if page.CommentsCount != 5 || len(page.Comments) != 2 || page.Comments[0].Body != "c3" || page.Comments[1].Body != "c2" {
			t.Errorf("page = %+v", page)
		}
		expectError(t, app.do(t, http.MethodGet, "/api/articles/busy/comments?limit=0", nil, ""), http.StatusBadRequest, "")
		page = decode[dto.CommentListResponse](t, app.do(t, http.MethodGet, "/api/articles/busy/comments?limit=500", nil, ""), http.StatusOK).Data
		if page.Limit != 100 {
			t.Errorf("limit should clamp to 100, got %d", page.Limit)
		}
	})
}

func TestProfilesAndFollow(t *testing.T) {
	app := newTestApp(t)
	haToken, _ := app.register(t, "ha")
	app.register(t, "lan")

	following := func(token string) bool {
		t.Helper()
		p := decode[dto.ProfileResponse](t, app.do(t, http.MethodGet, "/api/profiles/lan", nil, token), http.StatusOK).Data
		if p.Following == nil {
			t.Fatal("following field missing")
		}
		return *p.Following
	}

	if following("") {
		t.Error("guest should see following=false")
	}
	if following(haToken) {
		t.Error("ha is not following lan yet")
	}

	for i := 0; i < 2; i++ { // follow 2 lần: idempotent
		p := decode[dto.ProfileResponse](t, app.do(t, http.MethodPost, "/api/profiles/lan/follow", nil, haToken), http.StatusOK).Data
		if !*p.Following {
			t.Error("follow response should have following=true")
		}
	}
	if !following(haToken) {
		t.Error("ha should be following lan")
	}

	decode[any](t, app.do(t, http.MethodDelete, "/api/profiles/lan/follow", nil, haToken), http.StatusOK)
	if following(haToken) {
		t.Error("ha should no longer follow lan")
	}

	expectError(t, app.do(t, http.MethodPost, "/api/profiles/ha/follow", nil, haToken), http.StatusUnprocessableEntity, "you cannot follow yourself")
	expectError(t, app.do(t, http.MethodPost, "/api/profiles/ghost/follow", nil, haToken), http.StatusNotFound, "")
	expectError(t, app.do(t, http.MethodPost, "/api/profiles/lan/follow", nil, ""), http.StatusUnauthorized, "")
	expectError(t, app.do(t, http.MethodGet, "/api/profiles/lan", nil, "bad-token"), http.StatusUnauthorized, "")

	rec := app.do(t, http.MethodGet, "/api/profiles/lan", nil, "")
	if strings.Contains(rec.Body.String(), "email") {
		t.Errorf("profile must not expose email: %s", rec.Body.String())
	}
}

func TestUserAccountOwnership(t *testing.T) {
	app := newTestApp(t)
	haToken, ha := app.register(t, "ha")
	lanToken, _ := app.register(t, "lan")

	path := "/api/users/" + itoa(ha.ID)
	expectError(t, app.do(t, http.MethodPut, path, `{"bio":"x"}`, lanToken), http.StatusForbidden, "you can only modify your own account")
	expectError(t, app.do(t, http.MethodDelete, path, nil, lanToken), http.StatusForbidden, "")
	expectError(t, app.do(t, http.MethodPut, "/api/users/abc", `{}`, haToken), http.StatusBadRequest, "invalid id")

	updated := decode[dto.UserResponse](t, app.do(t, http.MethodPut, path, `{"bio":"Gopher"}`, haToken), http.StatusOK).Data
	if updated.Bio != "Gopher" {
		t.Errorf("bio = %q", updated.Bio)
	}

	// Đổi mật khẩu rồi đăng nhập bằng mật khẩu mới.
	decode[any](t, app.do(t, http.MethodPut, path, `{"password":"brand-new-pass"}`, haToken), http.StatusOK)
	decode[any](t, app.do(t, http.MethodPost, "/api/users/login", `{"email":"ha@example.com","password":"brand-new-pass"}`, ""), http.StatusOK)
	decode[any](t, app.do(t, http.MethodPost, "/api/users/login", `{"email":"ha@example.com","password":"secret123"}`, ""), http.StatusUnauthorized)
}

func TestUnknownRouteUsesJSONErrorFormat(t *testing.T) {
	app := newTestApp(t)
	expectError(t, app.do(t, http.MethodGet, "/api/does-not-exist", nil, ""), http.StatusNotFound, "Not Found")
}
