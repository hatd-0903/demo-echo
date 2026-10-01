package handlers_test

import (
	"net/http"
	"testing"

	"demo_echo/internal/dto"
)

func TestCreateArticle(t *testing.T) {
	app := newTestApp(t)
	token, user := app.register(t, "ha")
	tag := app.createTag(t, token, "golang")

	t.Run("requires login", func(t *testing.T) {
		rec := app.do(t, http.MethodPost, "/api/articles", `{"title":"x"}`, "")
		expectError(t, rec, http.StatusUnauthorized, "missing or invalid token")
	})

	t.Run("author comes from the token, not the body", func(t *testing.T) {
		rec := app.do(t, http.MethodPost, "/api/articles",
			map[string]any{"title": "Học Go cơ bản", "tag_ids": []uint{tag.ID}, "author_id": 999}, token)
		article := decode[dto.ArticleResponse](t, rec, http.StatusCreated).Data

		if article.Slug != "hoc-go-co-ban" {
			t.Errorf("slug = %q", article.Slug)
		}
		if article.Author == nil || article.Author.Username != user.Username {
			t.Errorf("author = %+v, want %s", article.Author, user.Username)
		}
		if len(article.Tags) != 1 || article.Tags[0].Name != "golang" {
			t.Errorf("tags = %+v", article.Tags)
		}
	})

	t.Run("duplicate titles get unique slugs", func(t *testing.T) {
		second := app.createArticle(t, token, "Học Go cơ bản")
		if second.Slug != "hoc-go-co-ban-2" {
			t.Errorf("slug = %q, want hoc-go-co-ban-2", second.Slug)
		}
	})

	t.Run("reserved slug feed is never used", func(t *testing.T) {
		if a := app.createArticle(t, token, "Feed"); a.Slug != "feed-2" {
			t.Errorf("slug = %q, want feed-2", a.Slug)
		}
	})

	t.Run("title is required", func(t *testing.T) {
		expectError(t, app.do(t, http.MethodPost, "/api/articles", `{"title":"  "}`, token),
			http.StatusUnprocessableEntity, "title is required")
	})
}

func TestGetArticleBySlug(t *testing.T) {
	app := newTestApp(t)
	token, _ := app.register(t, "ha")
	created := app.createArticle(t, token, "Hello Echo")

	got := decode[dto.ArticleResponse](t, app.do(t, http.MethodGet, "/api/articles/hello-echo", nil, ""), http.StatusOK).Data
	if got.ID != created.ID || got.Title != "Hello Echo" {
		t.Errorf("got %+v", got)
	}

	expectError(t, app.do(t, http.MethodGet, "/api/articles/does-not-exist", nil, ""), http.StatusNotFound, "resource not found")
}

// seedArticles: lan viết 3 bài (2 bài tag "go"), minh viết 2 bài (1 bài tag "go").
// Thứ tự tạo = thứ tự created_at tăng dần.
func seedArticles(t *testing.T, app *testApp) (lanToken, minhToken, haToken string) {
	t.Helper()
	lanToken, _ = app.register(t, "lan")
	minhToken, _ = app.register(t, "minh")
	haToken, _ = app.register(t, "ha")
	goTag := app.createTag(t, lanToken, "go")
	echoTag := app.createTag(t, lanToken, "echo")

	app.createArticle(t, lanToken, "A lan go", goTag.ID)
	app.createArticle(t, lanToken, "B lan go echo", goTag.ID, echoTag.ID)
	app.createArticle(t, lanToken, "C lan notes")
	app.createArticle(t, minhToken, "D minh go", goTag.ID)
	app.createArticle(t, minhToken, "E minh echo", echoTag.ID)
	return
}

func TestListArticlesFilterSortPaginate(t *testing.T) {
	app := newTestApp(t)
	seedArticles(t, app)

	tests := []struct {
		name      string
		query     string
		wantSlugs []string
		wantCount int64
	}{
		{"default: newest first", "", []string{"e-minh-echo", "d-minh-go", "c-lan-notes", "b-lan-go-echo", "a-lan-go"}, 5},
		{"filter by tag", "?tag=go", []string{"d-minh-go", "b-lan-go-echo", "a-lan-go"}, 3},
		{"filter by author", "?author=lan", []string{"c-lan-notes", "b-lan-go-echo", "a-lan-go"}, 3},
		{"tag and author combined", "?tag=go&author=minh", []string{"d-minh-go"}, 1},
		{"unknown tag", "?tag=rust", []string{}, 0},
		{"first page", "?limit=2&offset=0", []string{"e-minh-echo", "d-minh-go"}, 5},
		{"second page", "?limit=2&offset=2", []string{"c-lan-notes", "b-lan-go-echo"}, 5},
		{"last partial page", "?limit=2&offset=4", []string{"a-lan-go"}, 5},
		{"offset past the end", "?limit=2&offset=10", []string{}, 5},
		{"sort by title asc", "?sort=title&limit=3", []string{"a-lan-go", "b-lan-go-echo", "c-lan-notes"}, 5},
		{"sort by title desc", "?sort=-title&limit=2", []string{"e-minh-echo", "d-minh-go"}, 5},
		{"oldest first", "?sort=created_at&limit=1", []string{"a-lan-go"}, 5},
		{"filter + paginate", "?tag=go&limit=1&offset=1", []string{"b-lan-go-echo"}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := decode[dto.ArticleListResponse](t, app.do(t, http.MethodGet, "/api/articles"+tt.query, nil, ""), http.StatusOK).Data
			if got := slugsOf(page.Articles); !equalStrings(got, tt.wantSlugs) {
				t.Errorf("slugs = %v, want %v", got, tt.wantSlugs)
			}
			if page.ArticlesCount != tt.wantCount {
				t.Errorf("articles_count = %d, want %d", page.ArticlesCount, tt.wantCount)
			}
		})
	}
}

func TestListArticlesQueryValidation(t *testing.T) {
	app := newTestApp(t)

	t.Run("defaults and limit clamp", func(t *testing.T) {
		page := decode[dto.ArticleListResponse](t, app.do(t, http.MethodGet, "/api/articles", nil, ""), http.StatusOK).Data
		if page.Limit != 20 || page.Offset != 0 {
			t.Errorf("defaults: limit=%d offset=%d", page.Limit, page.Offset)
		}
		page = decode[dto.ArticleListResponse](t, app.do(t, http.MethodGet, "/api/articles?limit=1000", nil, ""), http.StatusOK).Data
		if page.Limit != 100 {
			t.Errorf("limit should be clamped to 100, got %d", page.Limit)
		}
	})

	for _, query := range []string{
		"?limit=abc",
		"?limit=0",
		"?limit=-5",
		"?offset=-1",
		"?offset=x",
		"?sort=password",
		"?sort=-id",
	} {
		t.Run(query, func(t *testing.T) {
			decode[any](t, app.do(t, http.MethodGet, "/api/articles"+query, nil, ""), http.StatusBadRequest)
		})
	}
}

func TestUpdateAndDeleteArticleOwnership(t *testing.T) {
	app := newTestApp(t)
	ownerToken, _ := app.register(t, "owner")
	otherToken, _ := app.register(t, "other")
	app.createArticle(t, ownerToken, "Original title")

	t.Run("no token", func(t *testing.T) {
		expectError(t, app.do(t, http.MethodPut, "/api/articles/original-title", `{"body":"x"}`, ""),
			http.StatusUnauthorized, "")
	})

	t.Run("non-owner cannot update", func(t *testing.T) {
		expectError(t, app.do(t, http.MethodPut, "/api/articles/original-title", `{"body":"hacked"}`, otherToken),
			http.StatusForbidden, "you are not the author of this article")
	})

	t.Run("non-owner cannot delete", func(t *testing.T) {
		expectError(t, app.do(t, http.MethodDelete, "/api/articles/original-title", nil, otherToken),
			http.StatusForbidden, "you are not the author of this article")
	})

	t.Run("missing article is 404 before ownership check", func(t *testing.T) {
		expectError(t, app.do(t, http.MethodPut, "/api/articles/nope", `{"body":"x"}`, otherToken),
			http.StatusNotFound, "resource not found")
	})

	t.Run("owner updates title -> new slug", func(t *testing.T) {
		rec := app.do(t, http.MethodPut, "/api/articles/original-title", `{"title":"New title","body":"updated"}`, ownerToken)
		article := decode[dto.ArticleResponse](t, rec, http.StatusOK).Data
		if article.Slug != "new-title" || article.Body != "updated" {
			t.Errorf("got slug=%q body=%q", article.Slug, article.Body)
		}
	})

	t.Run("owner deletes", func(t *testing.T) {
		if rec := app.do(t, http.MethodDelete, "/api/articles/new-title", nil, ownerToken); rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		expectError(t, app.do(t, http.MethodGet, "/api/articles/new-title", nil, ""), http.StatusNotFound, "")
	})
}

func TestFeed(t *testing.T) {
	app := newTestApp(t)
	_, _, haToken := seedArticles(t, app)

	feed := func(query string) dto.ArticleListResponse {
		t.Helper()
		return decode[dto.ArticleListResponse](t, app.do(t, http.MethodGet, "/api/articles/feed"+query, nil, haToken), http.StatusOK).Data
	}

	expectError(t, app.do(t, http.MethodGet, "/api/articles/feed", nil, ""), http.StatusUnauthorized, "")

	if page := feed(""); page.ArticlesCount != 0 || len(page.Articles) != 0 {
		t.Fatalf("feed should be empty before following anyone, got %v", slugsOf(page.Articles))
	}

	decode[any](t, app.do(t, http.MethodPost, "/api/profiles/minh/follow", nil, haToken), http.StatusOK)
	if got := slugsOf(feed("").Articles); !equalStrings(got, []string{"e-minh-echo", "d-minh-go"}) {
		t.Errorf("feed after following minh = %v", got)
	}

	decode[any](t, app.do(t, http.MethodPost, "/api/profiles/lan/follow", nil, haToken), http.StatusOK)
	page := feed("?limit=2&offset=2")
	if page.ArticlesCount != 5 || !equalStrings(slugsOf(page.Articles), []string{"c-lan-notes", "b-lan-go-echo"}) {
		t.Errorf("paginated feed = %v (count %d)", slugsOf(page.Articles), page.ArticlesCount)
	}

	decode[any](t, app.do(t, http.MethodDelete, "/api/profiles/minh/follow", nil, haToken), http.StatusOK)
	if page := feed(""); page.ArticlesCount != 3 {
		t.Errorf("feed after unfollowing minh should only have lan's 3 articles, got %v", slugsOf(page.Articles))
	}
}
