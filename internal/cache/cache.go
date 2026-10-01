// Package cache cung cấp key-value cache với 3 implementation:
// Redis (production), Memory (test) và Noop (khi không cấu hình REDIS_URL).
package cache

import (
	"context"
	"time"
)

type Cache interface {
	// Get trả về (value, true, nil) khi hit, (nil, false, nil) khi miss.
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// Incr tăng counter (tạo mới = 1 nếu chưa có). Dùng làm version để invalidate cả namespace.
	Incr(ctx context.Context, key string) (int64, error)
}

// Noop luôn miss; dùng khi chạy không có Redis để app vẫn hoạt động bình thường.
type Noop struct{}

func (Noop) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (Noop) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (Noop) Incr(context.Context, string) (int64, error)              { return 0, nil }
