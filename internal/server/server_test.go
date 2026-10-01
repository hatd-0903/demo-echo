package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"demo_echo/docs"
	"demo_echo/internal/auth"
	"demo_echo/internal/routes"
)

func newTestServer(swagger bool) http.Handler {
	// Repository nil là đủ: test này chỉ kiểm tra bảng route, không gọi handler nghiệp vụ.
	deps := routes.Deps{Tokens: auth.NewTokenManager("test-secret", time.Hour)}
	return New(deps, Options{Swagger: swagger})
}

var echoParam = regexp.MustCompile(`:(\w+)`)

// TestSwaggerDocsMatchRoutes: mọi route /api/* phải có trong swagger.json và ngược lại.
// Fail nghĩa là thêm/xoá route mà quên viết annotation hoặc quên chạy `make swagger`.
func TestSwaggerDocsMatchRoutes(t *testing.T) {
	e := New(routes.Deps{Tokens: auth.NewTokenManager("test-secret", time.Hour)}, Options{})

	registered := map[string]bool{}
	for _, r := range e.Routes() {
		if !strings.HasPrefix(r.Path, "/api/") || r.Method == "echo_route_any" {
			continue
		}
		path := echoParam.ReplaceAllString(strings.TrimPrefix(r.Path, "/api"), "{$1}")
		registered[r.Method+" "+path] = true
	}

	var spec struct {
		BasePath string                                `json:"basePath"`
		Paths    map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal([]byte(docs.SwaggerInfo.ReadDoc()), &spec); err != nil {
		t.Fatalf("invalid swagger doc: %v", err)
	}
	if spec.BasePath != "/api" {
		t.Errorf("basePath = %q, want /api", spec.BasePath)
	}
	documented := map[string]bool{}
	for path, ops := range spec.Paths {
		for method := range ops {
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}

	var missing, stale []string
	for r := range registered {
		if !documented[r] {
			missing = append(missing, r)
		}
	}
	for d := range documented {
		if !registered[d] {
			stale = append(stale, d)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("routes without swagger docs (add annotations, run `make swagger`):\n  %s", strings.Join(missing, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("swagger documents routes that do not exist:\n  %s", strings.Join(stale, "\n  "))
	}
	if len(registered) == 0 {
		t.Fatal("no /api routes found")
	}
}

func TestSwaggerUI(t *testing.T) {
	get := func(h http.Handler, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	enabled := newTestServer(true)
	if rec := get(enabled, "/swagger/index.html"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "swagger-ui") {
		t.Errorf("index.html: status %d", rec.Code)
	}
	if rec := get(enabled, "/swagger/doc.json"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"TokenAuth"`) {
		t.Errorf("doc.json: status %d", rec.Code)
	}
	if rec := get(enabled, "/swagger"); rec.Code != http.StatusMovedPermanently {
		t.Errorf("/swagger should redirect, got %d", rec.Code)
	}

	if rec := get(newTestServer(false), "/swagger/index.html"); rec.Code != http.StatusNotFound {
		t.Errorf("swagger must be off when disabled, got %d", rec.Code)
	}
}
