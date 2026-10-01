package handlers_test

// Test hồi quy cho các điểm trong code review (commit b7f9f30). Mỗi test tái hiện
// đúng kịch bản được báo cáo.

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"demo_echo/internal/dto"
	"demo_echo/internal/models"
)

// #1: user thường không được sửa/xoá tag dùng chung; chỉ admin.
func TestReview1_OnlyAdminCanModifyTags(t *testing.T) {
	app := newTestApp(t)
	userToken, _ := app.register(t, "user")
	adminToken, admin := app.register(t, "admin")
	app.makeAdmin(t, admin.ID)

	tag := app.createTag(t, userToken, "go") // tạo tag: user thường vẫn được (để gắn vào bài)
	app.createArticle(t, userToken, "Tagged", tag.ID)
	path := "/api/tags/" + itoa(tag.ID)

	expectError(t, app.do(t, http.MethodDelete, path, nil, userToken), http.StatusForbidden, "admin role required")
	expectError(t, app.do(t, http.MethodPut, path, `{"name":"renamed"}`, userToken), http.StatusForbidden, "admin role required")
	got := decode[dto.ArticleResponse](t, app.do(t, http.MethodGet, "/api/articles/tagged", nil, ""), http.StatusOK).Data
	if len(got.Tags) != 1 || got.Tags[0].Name != "go" {
		t.Fatalf("tag must be untouched after forbidden requests, got %+v", got.Tags)
	}

	decode[dto.TagResponse](t, app.do(t, http.MethodPut, path, `{"name":"golang"}`, adminToken), http.StatusOK)

	// Role được đọc từ DB mỗi request: thu hồi quyền có hiệu lực ngay với token cũ.
	app.db.mu.Lock()
	u := app.db.users[admin.ID]
	u.Role = models.RoleUser
	app.db.users[admin.ID] = u
	app.db.mu.Unlock()
	expectError(t, app.do(t, http.MethodDelete, path, nil, adminToken), http.StatusForbidden, "admin role required")
}

// #2: GET /api/users và /api/users/:id không được lộ email.
func TestReview2_UserListDoesNotLeakEmails(t *testing.T) {
	app := newTestApp(t)
	token, me := app.register(t, "viewer")
	_, other := app.register(t, "victim")

	for _, path := range []string{"/api/users", "/api/users/" + itoa(other.ID), "/api/users/" + itoa(me.ID)} {
		rec := app.do(t, http.MethodGet, path, nil, token)
		decode[any](t, rec, http.StatusOK)
		if strings.Contains(rec.Body.String(), "@example.com") || strings.Contains(rec.Body.String(), `"email"`) {
			t.Errorf("GET %s leaks email: %s", path, rec.Body.String())
		}
	}
}

// #3: xoá tài khoản phải xoá thật: đăng ký lại được, nội dung bị dọn, token cũ vô hiệu.
func TestReview3_DeletedUserIsReallyGone(t *testing.T) {
	app := newTestApp(t)
	token, user := app.register(t, "leaver")
	otherToken, _ := app.register(t, "stayer")
	app.createArticle(t, token, "Leaver post")
	app.createArticle(t, otherToken, "Stayer post")
	decode[any](t, app.do(t, http.MethodPost, "/api/articles/stayer-post/comments", `{"body":"bye"}`, token), http.StatusCreated)
	decode[any](t, app.do(t, http.MethodPost, "/api/profiles/stayer/follow", nil, token), http.StatusOK)

	if rec := app.do(t, http.MethodDelete, "/api/users/"+itoa(user.ID), nil, token); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}

	t.Run("old token rejected on every protected route", func(t *testing.T) {
		for _, r := range []struct{ method, path string }{
			{"GET", "/api/user"},
			{"GET", "/api/users"},
			{"GET", "/api/articles/feed"},
			{"POST", "/api/profiles/stayer/follow"},
			{"DELETE", "/api/profiles/stayer/follow"},
			{"POST", "/api/tags"},
			{"GET", "/api/profiles/stayer"}, // OptionalJWT: có gửi token thì token phải còn hiệu lực
		} {
			expectError(t, app.do(t, r.method, r.path, `{"name":"x"}`, token), http.StatusUnauthorized, "user no longer exists")
		}
	})

	t.Run("content of deleted user is removed, not shown with author null", func(t *testing.T) {
		page := decode[dto.ArticleListResponse](t, app.do(t, http.MethodGet, "/api/articles", nil, ""), http.StatusOK).Data
		if got := slugsOf(page.Articles); !slices.Equal(got, []string{"stayer-post"}) {
			t.Errorf("articles = %v", got)
		}
		for _, a := range page.Articles {
			if a.Author == nil {
				t.Errorf("article %s has author null", a.Slug)
			}
		}
		comments := decode[dto.CommentListResponse](t, app.do(t, http.MethodGet, "/api/articles/stayer-post/comments", nil, ""), http.StatusOK).Data
		if comments.CommentsCount != 0 {
			t.Errorf("comments of deleted user still listed: %+v", comments)
		}
	})

	t.Run("same email and username can register again", func(t *testing.T) {
		app.register(t, "leaver")
	})
}

// #4: hai request cùng title đến gần như đồng thời không được làm request sau bị 409.
func TestReview4_SlugRaceIsRetried(t *testing.T) {
	app := newTestApp(t)
	token, user := app.register(t, "writer")

	// Mô phỏng: sau khi handler đã chọn slug "same-title" nhưng trước khi ghi,
	// một request khác ghi article với đúng slug đó.
	app.db.beforeArticleInsert = func(db *fakeDB, article *models.Article) {
		db.articles[999] = models.Article{ID: 999, Slug: article.Slug, Title: "Same title", AuthorID: user.ID}
	}
	got := app.createArticle(t, token, "Same title")
	if got.Slug != "same-title-2" {
		t.Errorf("slug = %q, want same-title-2 (retried with a fresh slug)", got.Slug)
	}
}

// #5: email không tồn tại phải tốn thời gian xấp xỉ như sai mật khẩu (đều chạy bcrypt).
func TestReview5_LoginTimingDoesNotRevealEmails(t *testing.T) {
	app := newTestApp(t)
	app.register(t, "known")
	app.do(t, http.MethodPost, "/api/users/login", `{"email":"warmup@example.com","password":"x"}`, "")

	median := func(body string) time.Duration {
		var ds []time.Duration
		for i := 0; i < 5; i++ {
			start := time.Now()
			expectError(t, app.do(t, http.MethodPost, "/api/users/login", body, ""), http.StatusUnauthorized, "invalid email or password")
			ds = append(ds, time.Since(start))
		}
		slices.Sort(ds)
		return ds[len(ds)/2]
	}
	unknown := median(`{"email":"nobody@example.com","password":"wrong-pass"}`)
	wrongPass := median(`{"email":"known@example.com","password":"wrong-pass"}`)
	t.Logf("unknown email: %v, wrong password: %v", unknown, wrongPass)

	// Trước khi sửa: ~1ms so với ~50ms. Ngưỡng rộng để không flaky trên máy chậm.
	if unknown < wrongPass/2 {
		t.Errorf("unknown email answered in %v vs %v for a wrong password: response time reveals which emails exist", unknown, wrongPass)
	}
}

// #6: tag_ids không tồn tại phải bị báo lỗi, không lặng lẽ bỏ qua.
func TestReview6_UnknownTagIDsRejected(t *testing.T) {
	app := newTestApp(t)
	token, _ := app.register(t, "writer")
	tag := app.createTag(t, token, "go")

	rec := app.do(t, http.MethodPost, "/api/articles", map[string]any{"title": "x", "tag_ids": []uint{tag.ID, 999, 1000}}, token)
	expectError(t, rec, http.StatusUnprocessableEntity, "unknown tag_ids: 999, 1000")

	// ID trùng lặp không bị coi là thiếu tag.
	a := decode[dto.ArticleResponse](t, app.do(t, http.MethodPost, "/api/articles",
		map[string]any{"title": "dup", "tag_ids": []uint{tag.ID, tag.ID}}, token), http.StatusCreated).Data
	if len(a.Tags) != 1 {
		t.Errorf("tags = %+v", a.Tags)
	}

	expectError(t, app.do(t, http.MethodPut, "/api/articles/dup", map[string]any{"tag_ids": []uint{999}}, token),
		http.StatusUnprocessableEntity, "unknown tag_ids: 999")
}

// #7: body quá lớn bị chặn trước khi đọc vào bộ nhớ.
func TestReview7_BodyLimit(t *testing.T) {
	app := newTestApp(t)
	token, _ := app.register(t, "writer")

	big := `{"title":"big","body":"` + strings.Repeat("a", 2<<20) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/articles", strings.NewReader(big))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+token)
	expectError(t, app.serve(req), http.StatusRequestEntityTooLarge, "")

	// Body bình thường vẫn qua.
	app.createArticle(t, token, "small")
}
