//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"

	"demo_echo/internal/cache"
	"demo_echo/internal/dto"
)

type world struct {
	e           *env
	readerToken string
	tagIDs      []uint
	firstSlug   string
	authors     int
}

// grow thêm tác giả cho tới khi đủ n: mỗi tác giả viết 1 bài (2 tag), bình luận vào
// bài đầu tiên, và được reader follow.
func (w *world) grow(t *testing.T, n int) {
	t.Helper()
	for ; w.authors < n; w.authors++ {
		name := fmt.Sprintf("author%02d", w.authors)
		var u dto.AuthUserResponse
		w.e.call(t, "POST", "/api/users", "", map[string]string{
			"username": name, "email": name + "@example.com", "password": "secret123",
		}).expect(t, http.StatusCreated, &u)

		var a dto.ArticleResponse
		w.e.call(t, "POST", "/api/articles", u.Token, map[string]any{
			"title": "Post by " + name, "tag_ids": w.tagIDs,
		}).expect(t, http.StatusCreated, &a)
		if w.firstSlug == "" {
			w.firstSlug = a.Slug
		}
		w.e.call(t, "POST", "/api/articles/"+w.firstSlug+"/comments", u.Token,
			map[string]string{"body": "hi from " + name}).expect(t, http.StatusCreated, nil)
		w.e.call(t, "POST", "/api/profiles/"+name+"/follow", w.readerToken, nil).expect(t, http.StatusOK, nil)
	}
}

func newWorld(t *testing.T) *world {
	e := setup(t, options{cache: cache.Noop{}}) // tắt cache để đếm query thật
	var reader dto.AuthUserResponse
	e.call(t, "POST", "/api/users", "", map[string]string{
		"username": "reader", "email": "reader@example.com", "password": "secret123",
	}).expect(t, http.StatusCreated, &reader)

	w := &world{e: e, readerToken: reader.Token}
	for _, name := range []string{"go", "orm"} {
		var tag dto.TagResponse
		e.call(t, "POST", "/api/tags", reader.Token, map[string]string{"name": name}).expect(t, http.StatusCreated, &tag)
		w.tagIDs = append(w.tagIDs, tag.ID)
	}
	return w
}

// TestNoNPlusOne: số câu SQL của các endpoint danh sách không được tăng theo số bản ghi.
func TestNoNPlusOne(t *testing.T) {
	w := newWorld(t)

	// budget: articles = trang + users + article_tags + tags (COUNT bỏ qua khi trang chưa đầy).
	// Route cần đăng nhập (feed) +1 query: middleware JWT nạp user để chặn token của user đã xoá.
	endpoints := []struct {
		name, path string
		token      bool
		budget     int64
	}{
		{"list articles", "/api/articles?limit=100", false, 4},
		{"list articles by tag", "/api/articles?tag=go&limit=100", false, 4},
		{"feed", "/api/articles/feed?limit=100", true, 5},
		{"list comments", "", false, 3}, // path điền sau khi có bài đầu tiên
		{"list tags", "/api/tags", false, 1},
	}

	measure := func() map[string]int64 {
		out := map[string]int64{}
		for _, ep := range endpoints {
			path := ep.path
			if path == "" {
				path = "/api/articles/" + w.firstSlug + "/comments"
			}
			token := ""
			if ep.token {
				token = w.readerToken
			}
			out[ep.name] = w.e.countQueries(func() {
				w.e.call(t, "GET", path, token, nil).expect(t, http.StatusOK, nil)
			})
		}
		return out
	}

	w.grow(t, 2)
	small := measure()
	w.grow(t, 12)
	large := measure()

	t.Logf("%-22s %8s %9s", "endpoint", "2 rows", "12 rows")
	for _, ep := range endpoints {
		t.Logf("%-22s %8d %9d", ep.name, small[ep.name], large[ep.name])
		if small[ep.name] != large[ep.name] {
			t.Errorf("%s: %d queries with 2 rows but %d with 12 rows -> N+1", ep.name, small[ep.name], large[ep.name])
		}
		if large[ep.name] > ep.budget {
			t.Errorf("%s: %d queries, budget is %d", ep.name, large[ep.name], ep.budget)
		}
	}
}

// TestQueryBudget ghi lại số câu SQL của từng thao tác, phát hiện khi có ai đó
// vô tình thêm query thừa (budget = số query hiện tại sau khi tối ưu).
func TestQueryBudget(t *testing.T) {
	w := newWorld(t)
	w.grow(t, 1)
	e := w.e

	var author dto.AuthUserResponse
	e.call(t, "POST", "/api/users/login", "", map[string]string{
		"email": "author00@example.com", "password": "secret123",
	}).expect(t, http.StatusOK, &author)
	tok := author.Token

	// 3 bài cùng title để đo chi phí tạo slug khi bị trùng.
	for i := 0; i < 3; i++ {
		e.call(t, "POST", "/api/articles", tok, map[string]any{"title": "Same title"}).expect(t, http.StatusCreated, nil)
	}

	// budget = số query hiện tại; (trước tối ưu) ghi lại để so sánh.
	// Route cần đăng nhập tốn 1 query nạp user trong middleware JWT (review #3); tạo bài /
	// comment không tăng vì handler dùng lại user đó thay vì tự query.
	steps := []struct {
		name         string
		method, path string
		token        string
		body         any
		status       int
		budget       int64
	}{
		{"GET article by slug", "GET", "/api/articles/" + w.firstSlug, "", nil, 200, 4},                                        // (4)
		{"GET profile (logged in)", "GET", "/api/profiles/author00", w.readerToken, nil, 200, 3},                               // (2)
		{"POST article (2 tags)", "POST", "/api/articles", tok, map[string]any{"title": "Fresh", "tag_ids": w.tagIDs}, 201, 5}, // (10)
		{"POST article (4th same title)", "POST", "/api/articles", tok, map[string]any{"title": "Same title"}, 201, 3},         // (9, tăng theo số bài trùng)
		{"PUT article body", "PUT", "/api/articles/fresh", tok, map[string]any{"body": "new"}, 200, 7},                         // (9)
		{"PUT article tags", "PUT", "/api/articles/fresh", tok, map[string]any{"tag_ids": w.tagIDs[:1]}, 200, 10},              // (14)
		{"PUT article forbidden", "PUT", "/api/articles/fresh", w.readerToken, map[string]any{"body": "x"}, 403, 2},            // (4)
		{"POST comment", "POST", "/api/articles/fresh/comments", w.readerToken, map[string]string{"body": "nice"}, 201, 3},     // (8)
		{"DELETE article", "DELETE", "/api/articles/fresh", tok, nil, 204, 3},                                                  // (5)
	}

	t.Logf("%-32s %7s %7s", "operation", "queries", "budget")
	for _, s := range steps {
		n := e.countQueries(func() {
			e.call(t, s.method, s.path, s.token, s.body).expect(t, s.status, nil)
		})
		t.Logf("%-32s %7d %7d", s.name, n, s.budget)
		if n > s.budget {
			t.Errorf("%s: %d queries, budget is %d (run with E2E_SQL=1 to see them)", s.name, n, s.budget)
		}
	}
}
