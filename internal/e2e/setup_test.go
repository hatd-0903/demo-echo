//go:build e2e

// Package e2e chạy toàn bộ ứng dụng thật (MySQL + Redis + HTTP server) để kiểm tra
// các luồng nghiệp vụ end-to-end và số lượng câu SQL (phát hiện N+1).
//
//	make e2e   (cần: make up)
package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"demo_echo/internal/auth"
	"demo_echo/internal/cache"
	"demo_echo/internal/config"
	"demo_echo/internal/database"
	"demo_echo/internal/middleware"
	"demo_echo/internal/models"
	"demo_echo/internal/routes"
	"demo_echo/internal/server"

	_ "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testDBName = "demo_echo_test"

type env struct {
	srv     *httptest.Server
	db      *gorm.DB
	queries atomic.Int64 // số câu SQL đã chạy (đếm qua GORM callback)

	mu  sync.Mutex
	log []string // câu SQL đã chạy, bật bằng E2E_SQL=1 để debug
}

type options struct {
	cache cache.Cache
}

// setup tạo database demo_echo_test sạch (không đụng tới DB dev demo_echo),
// migrate, rồi chạy server thật trên một cổng ngẫu nhiên qua httptest.NewServer.
func setup(t *testing.T, opts options) *env {
	t.Helper()
	if os.Getenv("ENV_FILE") == "" {
		t.Setenv("ENV_FILE", "../../.env")
	}
	t.Setenv("DB_NAME", testDBName)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	admin, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%s)/", cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort))
	if err == nil {
		err = admin.Ping()
	}
	if err != nil {
		t.Skipf("MySQL not reachable (%v); run `make up` first", err)
	}
	if _, err := admin.Exec("CREATE DATABASE IF NOT EXISTS " + testDBName + " CHARACTER SET utf8mb4"); err != nil {
		t.Fatal(err)
	}
	admin.Close()

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db = db.Session(&gorm.Session{Logger: logger.Discard})
	if err := db.Migrator().DropTable("article_tags", &models.Follow{}, &models.Comment{}, &models.Article{}, &models.Tag{}, &models.User{}); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}

	e := &env{db: db}
	countQuery := func(tx *gorm.DB) {
		// GORM build subquery bằng một lần chạy DryRun (không gửi xuống DB): không đếm.
		if tx.DryRun || tx.Statement.DryRun {
			return
		}
		e.queries.Add(1)
		if os.Getenv("E2E_SQL") != "" {
			e.mu.Lock()
			e.log = append(e.log, tx.Dialector.Explain(tx.Statement.SQL.String(), tx.Statement.Vars...))
			e.mu.Unlock()
		}
	}
	_ = db.Callback().Query().After("gorm:query").Register("e2e:count_query", countQuery)
	_ = db.Callback().Row().After("gorm:row").Register("e2e:count_row", countQuery)
	_ = db.Callback().Raw().After("gorm:raw").Register("e2e:count_raw", countQuery)
	_ = db.Callback().Create().After("gorm:create").Register("e2e:count_create", countQuery)
	_ = db.Callback().Update().After("gorm:update").Register("e2e:count_update", countQuery)
	_ = db.Callback().Delete().After("gorm:delete").Register("e2e:count_delete", countQuery)

	deps := routes.GormRepositories(db)
	deps.Tokens = auth.NewTokenManager(cfg.JWTSecret, time.Hour)
	deps.Cache = opts.cache
	deps.CacheTTL = time.Minute

	e.srv = httptest.NewServer(server.New(deps, server.Options{}))
	t.Cleanup(func() {
		e.srv.Close()
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return e
}

// redisOrSkip dùng Redis thật ở DB 15 (tách khỏi DB 0 của app) nếu có.
func redisOrSkip(t *testing.T) cache.Cache {
	t.Helper()
	url := os.Getenv("E2E_REDIS_URL")
	if url == "" {
		url = "redis://127.0.0.1:6379/15"
	}
	r, err := cache.NewRedis(url)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := r.Ping(ctx); err != nil {
		t.Skipf("Redis not reachable (%v); run `make up` first", err)
	}
	// Bỏ cache của lần chạy trước (DB test vừa được tạo lại nên dữ liệu cũ đã sai).
	for _, ns := range []string{middleware.CacheNSTags, middleware.CacheNSArticles} {
		if _, err := r.Incr(ctx, "cache:"+ns+":version"); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// ---------- HTTP client ----------

type response struct {
	status int
	header http.Header
	body   []byte
}

func (e *env) call(t *testing.T, method, path, token string, body any) response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Token "+token)
	}
	res, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return response{status: res.StatusCode, header: res.Header, body: raw}
}

// expect kiểm tra status rồi decode field "data" vào out (nếu out != nil).
func (r response) expect(t *testing.T, status int, out any) {
	t.Helper()
	if r.status != status {
		t.Fatalf("status = %d, want %d; body = %s", r.status, status, r.body)
	}
	if out == nil {
		return
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(r.body, &env); err != nil {
		t.Fatalf("invalid JSON %s: %v", r.body, err)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		t.Fatalf("decode data %s: %v", env.Data, err)
	}
}

// countQueries trả về số câu SQL mà fn đã gây ra (in từng câu nếu E2E_SQL=1).
func (e *env) countQueries(fn func()) int64 {
	e.mu.Lock()
	e.log = nil
	e.mu.Unlock()
	before := e.queries.Load()
	fn()
	if os.Getenv("E2E_SQL") != "" {
		e.mu.Lock()
		for _, q := range e.log {
			fmt.Println("    SQL>", q)
		}
		e.mu.Unlock()
	}
	return e.queries.Load() - before
}
