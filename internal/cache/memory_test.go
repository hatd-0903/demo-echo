package cache

import (
	"context"
	"testing"
	"time"
)

func TestMemoryGetSetExpire(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }

	if _, ok, _ := m.Get(ctx, "k"); ok {
		t.Fatal("empty cache should miss")
	}
	_ = m.Set(ctx, "k", []byte("v"), time.Minute)
	if v, ok, _ := m.Get(ctx, "k"); !ok || string(v) != "v" {
		t.Fatalf("Get = %q, %v", v, ok)
	}

	now = now.Add(2 * time.Minute)
	if _, ok, _ := m.Get(ctx, "k"); ok {
		t.Error("entry should expire after TTL")
	}
}

func TestMemoryIncrBehavesLikeRedis(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	for want := int64(1); want <= 3; want++ {
		if got, _ := m.Incr(ctx, "counter"); got != want {
			t.Fatalf("Incr = %d, want %d", got, want)
		}
	}
	// Giống Redis: counter đọc lại bằng Get dưới dạng chuỗi số.
	if v, ok, _ := m.Get(ctx, "counter"); !ok || string(v) != "3" {
		t.Errorf("Get(counter) = %q, %v", v, ok)
	}
}
