//go:build e2e

package e2e

import (
	"net/http"
	"strings"
	"testing"

	"demo_echo/internal/dto"
)

// TestUserJourney: Đăng ký -> Đăng nhập -> Đăng bài -> Tương tác (comment, follow, feed)
// -> Sửa/Xoá, trên MySQL + Redis + HTTP thật. Các bước phụ thuộc nhau nên chạy tuần tự.
func TestUserJourney(t *testing.T) {
	e := setup(t, options{cache: redisOrSkip(t)})

	var aliceToken, bobToken string
	var alice dto.AuthUserResponse
	var article dto.ArticleResponse
	var tagGo, tagEcho dto.TagResponse

	step := func(name string, fn func(t *testing.T)) {
		if !t.Run(name, fn) {
			t.FailNow() // bước sau dựa vào bước trước
		}
	}

	step("1. register alice and bob", func(t *testing.T) {
		e.call(t, "POST", "/api/users", "", map[string]string{
			"username": "alice", "email": "alice@example.com", "password": "alice-pass",
		}).expect(t, http.StatusCreated, &alice)
		var bob dto.AuthUserResponse
		e.call(t, "POST", "/api/users", "", map[string]string{
			"username": "bob", "email": "bob@example.com", "password": "bob-pass",
		}).expect(t, http.StatusCreated, &bob)
		bobToken = bob.Token

		e.call(t, "POST", "/api/users", "", map[string]string{
			"username": "alice", "email": "other@example.com", "password": "whatever",
		}).expect(t, http.StatusConflict, nil)
	})

	step("2. login and fetch current user", func(t *testing.T) {
		e.call(t, "POST", "/api/users/login", "", map[string]string{
			"email": "alice@example.com", "password": "wrong",
		}).expect(t, http.StatusUnauthorized, nil)

		var logged dto.AuthUserResponse
		e.call(t, "POST", "/api/users/login", "", map[string]string{
			"email": "ALICE@example.com", "password": "alice-pass",
		}).expect(t, http.StatusOK, &logged)
		aliceToken = logged.Token

		var me dto.AuthUserResponse
		e.call(t, "GET", "/api/user", aliceToken, nil).expect(t, http.StatusOK, &me)
		if me.ID != alice.ID || me.Email != "alice@example.com" {
			t.Errorf("current user = %+v", me.UserResponse)
		}

		var stored string
		e.db.Raw("SELECT password FROM users WHERE id = ?", alice.ID).Scan(&stored)
		if !strings.HasPrefix(stored, "$2a$") {
			t.Errorf("password must be bcrypt-hashed in DB, got %q", stored)
		}
	})

	step("3. alice publishes an article with tags", func(t *testing.T) {
		e.call(t, "POST", "/api/tags", aliceToken, map[string]string{"name": "golang"}).expect(t, http.StatusCreated, &tagGo)
		e.call(t, "POST", "/api/tags", aliceToken, map[string]string{"name": "echo"}).expect(t, http.StatusCreated, &tagEcho)

		e.call(t, "POST", "/api/articles", "", map[string]any{"title": "x"}).expect(t, http.StatusUnauthorized, nil)
		e.call(t, "POST", "/api/articles", aliceToken, map[string]any{
			"title": "Xây dựng API với Echo", "body": "Nội dung...", "tag_ids": []uint{tagGo.ID, tagEcho.ID},
		}).expect(t, http.StatusCreated, &article)

		if article.Slug != "xay-dung-api-voi-echo" || article.Author.Username != "alice" || len(article.Tags) != 2 {
			t.Errorf("article = %+v", article)
		}
	})

	step("4. article is discoverable (filter + cache)", func(t *testing.T) {
		for _, q := range []string{"", "?tag=golang", "?author=alice", "?tag=echo&author=alice&limit=1"} {
			res := e.call(t, "GET", "/api/articles"+q, "", nil)
			var page dto.ArticleListResponse
			res.expect(t, http.StatusOK, &page)
			if page.ArticlesCount != 1 || page.Articles[0].Slug != article.Slug {
				t.Errorf("GET /api/articles%s = %+v", q, page)
			}
		}
		e.call(t, "GET", "/api/articles?author=bob", "", nil)
		res := e.call(t, "GET", "/api/articles?tag=golang", "", nil)
		if res.header.Get("X-Cache") != "HIT" {
			t.Errorf("second identical request should hit Redis, X-Cache = %q", res.header.Get("X-Cache"))
		}

		var got dto.ArticleResponse
		e.call(t, "GET", "/api/articles/"+article.Slug, "", nil).expect(t, http.StatusOK, &got)
		if got.ID != article.ID {
			t.Errorf("by slug = %+v", got)
		}
	})

	step("5. bob and alice comment", func(t *testing.T) {
		path := "/api/articles/" + article.Slug + "/comments"
		e.call(t, "POST", path, "", map[string]string{"body": "anon"}).expect(t, http.StatusUnauthorized, nil)
		e.call(t, "POST", path, bobToken, map[string]string{"body": "Bài viết hay quá!"}).expect(t, http.StatusCreated, nil)
		e.call(t, "POST", path, aliceToken, map[string]string{"body": "Cảm ơn bạn"}).expect(t, http.StatusCreated, nil)

		var comments []dto.CommentResponse
		res := e.call(t, "GET", path, "", nil)
		res.expect(t, http.StatusOK, &comments)
		if len(comments) != 2 || comments[0].Author.Username != "alice" || comments[1].Author.Username != "bob" {
			t.Errorf("comments (newest first) = %+v", comments)
		}
		if strings.Contains(string(res.body), "@example.com") {
			t.Error("comment authors must not expose emails")
		}
	})

	step("6. bob follows alice and sees her article in his feed", func(t *testing.T) {
		var feed dto.ArticleListResponse
		e.call(t, "GET", "/api/articles/feed", bobToken, nil).expect(t, http.StatusOK, &feed)
		if feed.ArticlesCount != 0 {
			t.Fatalf("feed before follow = %+v", feed)
		}

		var profile dto.ProfileResponse
		e.call(t, "POST", "/api/profiles/alice/follow", bobToken, nil).expect(t, http.StatusOK, &profile)
		e.call(t, "GET", "/api/profiles/alice", bobToken, nil).expect(t, http.StatusOK, &profile)
		if !*profile.Following {
			t.Error("bob should be following alice")
		}
		e.call(t, "GET", "/api/profiles/alice", "", nil).expect(t, http.StatusOK, &profile)
		if *profile.Following {
			t.Error("guest view must show following=false")
		}

		e.call(t, "GET", "/api/articles/feed", bobToken, nil).expect(t, http.StatusOK, &feed)
		if feed.ArticlesCount != 1 || feed.Articles[0].Slug != article.Slug {
			t.Errorf("feed after follow = %+v", feed)
		}
	})

	step("7. only the author can edit; edits invalidate the cache", func(t *testing.T) {
		e.call(t, "PUT", "/api/articles/"+article.Slug, bobToken, map[string]string{"title": "hacked"}).
			expect(t, http.StatusForbidden, nil)

		var updated dto.ArticleResponse
		e.call(t, "PUT", "/api/articles/"+article.Slug, aliceToken, map[string]any{
			"title": "Echo nâng cao", "tag_ids": []uint{tagGo.ID},
		}).expect(t, http.StatusOK, &updated)
		if updated.Slug != "echo-nang-cao" || len(updated.Tags) != 1 {
			t.Errorf("updated = %+v", updated)
		}

		res := e.call(t, "GET", "/api/articles?tag=golang", "", nil)
		var page dto.ArticleListResponse
		res.expect(t, http.StatusOK, &page)
		if res.header.Get("X-Cache") != "MISS" || page.Articles[0].Title != "Echo nâng cao" {
			t.Errorf("stale cache after update: X-Cache=%q title=%q", res.header.Get("X-Cache"), page.Articles[0].Title)
		}
		e.call(t, "GET", "/api/articles?tag=echo", "", nil).expect(t, http.StatusOK, &page)
		if page.ArticlesCount != 0 {
			t.Errorf("tag echo was removed from the article, got %d", page.ArticlesCount)
		}
		article = updated
	})

	step("8. unfollow empties the feed", func(t *testing.T) {
		e.call(t, "DELETE", "/api/profiles/alice/follow", bobToken, nil).expect(t, http.StatusOK, nil)
		var feed dto.ArticleListResponse
		e.call(t, "GET", "/api/articles/feed", bobToken, nil).expect(t, http.StatusOK, &feed)
		if feed.ArticlesCount != 0 {
			t.Errorf("feed after unfollow = %+v", feed)
		}
	})

	step("9. author deletes the article", func(t *testing.T) {
		e.call(t, "DELETE", "/api/articles/"+article.Slug, bobToken, nil).expect(t, http.StatusForbidden, nil)
		e.call(t, "DELETE", "/api/articles/"+article.Slug, aliceToken, nil).expect(t, http.StatusNoContent, nil)
		e.call(t, "GET", "/api/articles/"+article.Slug, "", nil).expect(t, http.StatusNotFound, nil)
		e.call(t, "GET", "/api/articles/"+article.Slug+"/comments", "", nil).expect(t, http.StatusNotFound, nil)

		var page dto.ArticleListResponse
		e.call(t, "GET", "/api/articles", "", nil).expect(t, http.StatusOK, &page)
		if page.ArticlesCount != 0 {
			t.Errorf("deleted article still listed: %+v", page)
		}
	})
}
