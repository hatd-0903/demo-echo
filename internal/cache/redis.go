package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	client *redis.Client
}

// NewRedis tạo client từ URL dạng redis://[:password@]host:port/db.
func NewRedis(url string) (*Redis, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	// Cache chỉ là lớp tăng tốc: Redis chậm/treo thì bỏ qua nhanh thay vì kéo
	// mọi request chờ theo (mặc định go-redis: dial 5s, 5 lần dial lại, 3 lần retry lệnh).
	opt.DialTimeout = 500 * time.Millisecond
	opt.ReadTimeout = 300 * time.Millisecond
	opt.WriteTimeout = 300 * time.Millisecond
	opt.DialerRetries = 1
	opt.MaxRetries = -1 // -1 = không retry (0 nghĩa là dùng mặc định 3)
	return &Redis{client: redis.NewClient(opt)}, nil
}

func (r *Redis) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *Redis) Close() error {
	return r.client.Close()
}

func (r *Redis) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *Redis) Incr(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, key).Result()
}
