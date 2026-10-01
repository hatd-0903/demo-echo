package middleware

import (
	"bytes"
	"net/http"
	"time"

	"demo_echo/internal/cache"

	"github.com/labstack/echo/v4"
)

const HeaderXCache = "X-Cache"

// Invalidate theo kiểu "version namespace": mỗi namespace có 1 counter
// (cache:<ns>:version), counter nằm trong key của mọi response đã cache.
// Tăng counter = mọi key cũ không bao giờ được đọc nữa (tự hết hạn theo TTL),
// không cần SCAN/DEL hàng loạt trên Redis.
func versionKey(ns string) string {
	return "cache:" + ns + ":version"
}

func currentVersion(c echo.Context, store cache.Cache, ns string) (string, error) {
	v, ok, err := store.Get(c.Request().Context(), versionKey(ns))
	if err != nil || !ok {
		return "0", err
	}
	return string(v), nil
}

// CacheResponse cache response 200 của các GET public (không phụ thuộc user đang đăng nhập).
// Key gồm namespace, version và URL đã chuẩn hoá query (?b=1&a=2 và ?a=2&b=1 dùng chung key).
// Lỗi cache không bao giờ làm hỏng request: cache down thì đi thẳng xuống handler.
func CacheResponse(store cache.Cache, ns string, ttl time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Method != http.MethodGet {
				return next(c)
			}
			ctx := c.Request().Context()

			version, err := currentVersion(c, store, ns)
			if err != nil {
				c.Logger().Warnf("cache unavailable, bypassing: %v", err)
				c.Response().Header().Set(HeaderXCache, "BYPASS")
				return next(c)
			}
			key := "cache:" + ns + ":v" + version + ":" + c.Request().URL.Path + "?" + c.QueryParams().Encode()

			if body, ok, err := store.Get(ctx, key); err == nil && ok {
				c.Response().Header().Set(HeaderXCache, "HIT")
				return c.JSONBlob(http.StatusOK, body)
			}

			// Ghi response vào buffer song song với client để lưu lại sau.
			rec := &bodyRecorder{ResponseWriter: c.Response().Writer}
			c.Response().Writer = rec
			c.Response().Header().Set(HeaderXCache, "MISS")

			if err := next(c); err != nil {
				return err
			}
			if c.Response().Status == http.StatusOK {
				if err := store.Set(ctx, key, rec.buf.Bytes(), ttl); err != nil {
					c.Logger().Warnf("cache set failed: %v", err)
				}
			}
			return nil
		}
	}
}

// InvalidateCache tăng version của các namespace sau khi request ghi dữ liệu thành công (2xx).
func InvalidateCache(store cache.Cache, namespaces ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if err := next(c); err != nil {
				return err
			}
			if status := c.Response().Status; status < 200 || status >= 300 {
				return nil
			}
			for _, ns := range namespaces {
				if _, err := store.Incr(c.Request().Context(), versionKey(ns)); err != nil {
					c.Logger().Warnf("cache invalidate %s failed: %v", ns, err)
				}
			}
			return nil
		}
	}
}

type bodyRecorder struct {
	http.ResponseWriter
	buf bytes.Buffer
}

func (r *bodyRecorder) Write(b []byte) (int, error) {
	r.buf.Write(b)
	return r.ResponseWriter.Write(b)
}

// CacheNamespaces dùng chung giữa routes và test.
const (
	CacheNSTags     = "tags"
	CacheNSArticles = "articles"
)
