//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"demo_echo/internal/cache"
	"demo_echo/internal/dto"
)

// Review #3 trên MySQL thật: xoá user là DELETE thật, FK ON DELETE CASCADE dọn sạch
// article (kể cả bảng nối tag), comment, follow; email/username đăng ký lại được.
func TestDeleteUserCascadesInMySQL(t *testing.T) {
	e := setup(t, options{cache: cache.Noop{}})

	var leaver, stayer dto.AuthUserResponse
	e.call(t, "POST", "/api/users", "", map[string]string{"username": "leaver", "email": "leaver@example.com", "password": "secret123"}).
		expect(t, http.StatusCreated, &leaver)
	e.call(t, "POST", "/api/users", "", map[string]string{"username": "stayer", "email": "stayer@example.com", "password": "secret123"}).
		expect(t, http.StatusCreated, &stayer)

	var tag dto.TagResponse
	e.call(t, "POST", "/api/tags", leaver.Token, map[string]string{"name": "go"}).expect(t, http.StatusCreated, &tag)
	e.call(t, "POST", "/api/articles", leaver.Token, map[string]any{"title": "Leaver post", "tag_ids": []uint{tag.ID}}).expect(t, http.StatusCreated, nil)
	e.call(t, "POST", "/api/articles", stayer.Token, map[string]any{"title": "Stayer post"}).expect(t, http.StatusCreated, nil)
	e.call(t, "POST", "/api/articles/leaver-post/comments", stayer.Token, map[string]string{"body": "on leaver post"}).expect(t, http.StatusCreated, nil)
	e.call(t, "POST", "/api/articles/stayer-post/comments", leaver.Token, map[string]string{"body": "by leaver"}).expect(t, http.StatusCreated, nil)
	e.call(t, "POST", "/api/profiles/stayer/follow", leaver.Token, nil).expect(t, http.StatusOK, nil)
	e.call(t, "POST", "/api/profiles/leaver/follow", stayer.Token, nil).expect(t, http.StatusOK, nil)
	// Bài đã soft-delete của leaver cũng phải bị dọn khi xoá user.
	e.call(t, "POST", "/api/articles", leaver.Token, map[string]any{"title": "Old post"}).expect(t, http.StatusCreated, nil)
	e.call(t, "DELETE", "/api/articles/old-post", leaver.Token, nil).expect(t, http.StatusNoContent, nil)

	e.call(t, "DELETE", fmt.Sprintf("/api/users/%d", leaver.ID), leaver.Token, nil).expect(t, http.StatusNoContent, nil)

	count := func(query string, args ...any) int64 {
		var n int64
		if err := e.db.Raw(query, args...).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	checks := map[string]int64{
		"users row":                count("SELECT COUNT(*) FROM users WHERE id = ?", leaver.ID),
		"articles (incl. deleted)": count("SELECT COUNT(*) FROM articles WHERE author_id = ?", leaver.ID),
		"article_tags":             count("SELECT COUNT(*) FROM article_tags WHERE tag_id = ?", tag.ID),
		"comments by user":         count("SELECT COUNT(*) FROM comments WHERE user_id = ?", leaver.ID),
		"comments on user's posts": count("SELECT COUNT(*) FROM comments WHERE body = 'on leaver post'"),
		"follows":                  count("SELECT COUNT(*) FROM user_follows WHERE follower_id = ? OR following_id = ?", leaver.ID, leaver.ID),
	}
	for what, n := range checks {
		if n != 0 {
			t.Errorf("%s: %d rows left after deleting user", what, n)
		}
	}
	if n := count("SELECT COUNT(*) FROM articles WHERE author_id = ?", stayer.ID); n != 1 {
		t.Errorf("other user's article must survive, got %d", n)
	}

	e.call(t, "GET", "/api/user", leaver.Token, nil).expect(t, http.StatusUnauthorized, nil)
	e.call(t, "POST", "/api/users", "", map[string]string{"username": "leaver", "email": "leaver@example.com", "password": "secret123"}).
		expect(t, http.StatusCreated, nil)
}

// Review #4 với request đồng thời thật: N request cùng title cùng lúc đều phải 201
// với slug khác nhau (trước khi sửa: một số request bị 409).
func TestConcurrentSameTitle(t *testing.T) {
	e := setup(t, options{cache: cache.Noop{}})
	var u dto.AuthUserResponse
	e.call(t, "POST", "/api/users", "", map[string]string{"username": "racer", "email": "racer@example.com", "password": "secret123"}).
		expect(t, http.StatusCreated, &u)

	const n = 3 // = maxSlugAttempts: mỗi request thua race tối đa n-1 lần
	for round := 0; round < 5; round++ {
		title := fmt.Sprintf("Race round %d", round)
		statuses := make([]int, n)
		slugs := make([]string, n)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				res := e.call(t, "POST", "/api/articles", u.Token, map[string]any{"title": title})
				statuses[i] = res.status
				if res.status == http.StatusCreated {
					var a dto.ArticleResponse
					res.expect(t, http.StatusCreated, &a)
					slugs[i] = a.Slug
				}
			}(i)
		}
		close(start)
		wg.Wait()

		seen := map[string]bool{}
		for i := range statuses {
			if statuses[i] != http.StatusCreated {
				t.Errorf("round %d: request %d got %d", round, i, statuses[i])
			}
			if seen[slugs[i]] {
				t.Errorf("round %d: duplicate slug %q", round, slugs[i])
			}
			seen[slugs[i]] = true
		}
	}
}

// Migration: mọi FK phải là ON DELETE CASCADE và users không còn cột deleted_at.
func TestSchemaConstraints(t *testing.T) {
	e := setup(t, options{cache: cache.Noop{}})
	var rows []struct{ TableName, ConstraintName, DeleteRule string }
	e.db.Raw(`SELECT table_name AS table_name, constraint_name AS constraint_name, delete_rule AS delete_rule
		FROM information_schema.referential_constraints WHERE constraint_schema = DATABASE()`).Scan(&rows)
	if len(rows) != 7 {
		t.Errorf("expected 7 foreign keys, got %d: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.DeleteRule != "CASCADE" {
			t.Errorf("%s.%s is ON DELETE %s, want CASCADE", r.TableName, r.ConstraintName, r.DeleteRule)
		}
	}
	if e.db.Migrator().HasColumn("users", "deleted_at") {
		t.Error("users.deleted_at must not exist (hard delete)")
	}
}
