package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"demo_echo/internal/cache"
	"demo_echo/internal/dto"
	"demo_echo/internal/middleware"
)

func xCache(rec *httptest.ResponseRecorder) string {
	return rec.Header().Get(middleware.HeaderXCache)
}

func expectCache(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := xCache(rec); got != want {
		t.Fatalf("X-Cache = %q, want %q", got, want)
	}
}

func TestTagsCache(t *testing.T) {
	app := newTestApp(t)
	token, _ := app.register(t, "ha")
	app.createTag(t, token, "go")

	first := app.do(t, http.MethodGet, "/api/tags", nil, "")
	expectCache(t, first, "MISS")
	second := app.do(t, http.MethodGet, "/api/tags", nil, "")
	expectCache(t, second, "HIT")
	if first.Body.String() != second.Body.String() {
		t.Errorf("cached body differs:\n%s\n%s", first.Body.String(), second.Body.String())
	}

	// Ghi dữ liệu thành công -> invalidate -> lần đọc sau phải thấy dữ liệu mới.
	app.createTag(t, token, "echo")
	rec := app.do(t, http.MethodGet, "/api/tags", nil, "")
	expectCache(t, rec, "MISS")
	if tags := decode[[]dto.TagResponse](t, rec, http.StatusOK).Data; len(tags) != 2 {
		t.Errorf("expected new tag after invalidation, got %+v", tags)
	}

	// Ghi thất bại (thiếu token) không được invalidate.
	expectCache(t, app.do(t, http.MethodGet, "/api/tags", nil, ""), "HIT")
	decode[any](t, app.do(t, http.MethodPost, "/api/tags", `{"name":"x"}`, ""), http.StatusUnauthorized)
	decode[any](t, app.do(t, http.MethodPost, "/api/tags", `{"name":"go"}`, token), http.StatusConflict)
	expectCache(t, app.do(t, http.MethodGet, "/api/tags", nil, ""), "HIT")
}

func TestArticlesCache(t *testing.T) {
	app := newTestApp(t)
	ownerToken, owner := app.register(t, "owner")
	otherToken, _ := app.register(t, "other")
	app.makeAdmin(t, owner.ID) // đổi tên tag cần admin
	tag := app.createTag(t, ownerToken, "go")
	app.createArticle(t, ownerToken, "First", tag.ID)

	t.Run("same query is cached, query order does not matter", func(t *testing.T) {
		expectCache(t, app.do(t, http.MethodGet, "/api/articles?limit=5&tag=go", nil, ""), "MISS")
		expectCache(t, app.do(t, http.MethodGet, "/api/articles?tag=go&limit=5", nil, ""), "HIT")
		expectCache(t, app.do(t, http.MethodGet, "/api/articles?tag=go&limit=6", nil, ""), "MISS")
	})

	warm := func(t *testing.T) {
		t.Helper()
		app.do(t, http.MethodGet, "/api/articles", nil, "")
		expectCache(t, app.do(t, http.MethodGet, "/api/articles", nil, ""), "HIT")
	}

	t.Run("creating an article invalidates", func(t *testing.T) {
		warm(t)
		app.createArticle(t, ownerToken, "Second")
		rec := app.do(t, http.MethodGet, "/api/articles", nil, "")
		expectCache(t, rec, "MISS")
		if page := decode[dto.ArticleListResponse](t, rec, http.StatusOK).Data; page.ArticlesCount != 2 {
			t.Errorf("articles_count = %d, want 2", page.ArticlesCount)
		}
	})

	t.Run("forbidden update does not invalidate", func(t *testing.T) {
		warm(t)
		decode[any](t, app.do(t, http.MethodPut, "/api/articles/first", `{"body":"x"}`, otherToken), http.StatusForbidden)
		expectCache(t, app.do(t, http.MethodGet, "/api/articles", nil, ""), "HIT")
	})

	t.Run("renaming a tag invalidates articles (tags are embedded)", func(t *testing.T) {
		warm(t)
		decode[any](t, app.do(t, http.MethodPut, "/api/tags/"+itoa(tag.ID), `{"name":"golang"}`, ownerToken), http.StatusOK)
		rec := app.do(t, http.MethodGet, "/api/articles?tag=golang", nil, "")
		expectCache(t, rec, "MISS")
		if page := decode[dto.ArticleListResponse](t, rec, http.StatusOK).Data; page.ArticlesCount != 1 {
			t.Errorf("expected article under renamed tag, got %d", page.ArticlesCount)
		}
	})

	t.Run("updating author profile invalidates articles (author is embedded)", func(t *testing.T) {
		warm(t)
		writerToken, me := app.register(t, "writer")
		app.createArticle(t, writerToken, "By writer")
		warm(t)
		decode[any](t, app.do(t, http.MethodPut, "/api/users/"+itoa(me.ID), `{"bio":"new bio"}`, writerToken), http.StatusOK)
		rec := app.do(t, http.MethodGet, "/api/articles?author=writer", nil, "")
		expectCache(t, rec, "MISS")
		if page := decode[dto.ArticleListResponse](t, rec, http.StatusOK).Data; page.Articles[0].Author.Bio != "new bio" {
			t.Errorf("stale author in cached list: %+v", page.Articles[0].Author)
		}
	})

	t.Run("feed is per-user and never cached", func(t *testing.T) {
		rec := app.do(t, http.MethodGet, "/api/articles/feed", nil, otherToken)
		if rec.Code != http.StatusOK || xCache(rec) != "" {
			t.Errorf("status=%d X-Cache=%q, want 200 and no cache header", rec.Code, xCache(rec))
		}
	})

	t.Run("error responses are not cached", func(t *testing.T) {
		expectError(t, app.do(t, http.MethodGet, "/api/articles?limit=abc", nil, ""), http.StatusBadRequest, "")
		expectError(t, app.do(t, http.MethodGet, "/api/articles?limit=abc", nil, ""), http.StatusBadRequest, "")
	})
}

// brokenCache mô phỏng Redis bị down.
type brokenCache struct{}

var errCacheDown = errors.New("redis: connection refused")

func (brokenCache) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, errCacheDown
}
func (brokenCache) Set(context.Context, string, []byte, time.Duration) error { return errCacheDown }
func (brokenCache) Incr(context.Context, string) (int64, error)              { return 0, errCacheDown }

func TestCacheFailureDoesNotBreakAPI(t *testing.T) {
	app := newTestApp(t, withCache(brokenCache{}))
	token, _ := app.register(t, "ha")
	app.createTag(t, token, "go") // invalidate lỗi nhưng request vẫn 201

	rec := app.do(t, http.MethodGet, "/api/tags", nil, "")
	expectCache(t, rec, "BYPASS")
	if tags := decode[[]dto.TagResponse](t, rec, http.StatusOK).Data; len(tags) != 1 {
		t.Errorf("tags = %+v", tags)
	}
}

func TestNoopCacheWhenRedisDisabled(t *testing.T) {
	app := newTestApp(t, withCache(cache.Noop{}))
	for i := 0; i < 2; i++ {
		expectCache(t, app.do(t, http.MethodGet, "/api/tags", nil, ""), "MISS")
	}
}
