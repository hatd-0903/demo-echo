package cache

import (
	"context"
	"strconv"
	"sync"
	"time"
)

// Memory là cache trong bộ nhớ, an toàn khi dùng đồng thời. Dùng cho test
// (không chia sẻ được giữa nhiều instance nên không dùng cho production).
type Memory struct {
	mu    sync.Mutex
	items map[string]memoryItem
	now   func() time.Time
}

type memoryItem struct {
	value     []byte
	expiresAt time.Time // zero = không hết hạn
}

func NewMemory() *Memory {
	return &Memory{items: map[string]memoryItem{}, now: time.Now}
}

func (m *Memory) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[key]
	if !ok {
		return nil, false, nil
	}
	if !item.expiresAt.IsZero() && m.now().After(item.expiresAt) {
		delete(m.items, key)
		return nil, false, nil
	}
	return item.value, true, nil
}

func (m *Memory) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := memoryItem{value: append([]byte(nil), value...)}
	if ttl > 0 {
		item.expiresAt = m.now().Add(ttl)
	}
	m.items[key] = item
	return nil
}

// Incr lưu số dưới dạng chuỗi giống Redis INCR, nên Get trên cùng key đọc được.
func (m *Memory) Incr(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, _ := strconv.ParseInt(string(m.items[key].value), 10, 64)
	n++
	m.items[key] = memoryItem{value: []byte(strconv.FormatInt(n, 10))}
	return n, nil
}
