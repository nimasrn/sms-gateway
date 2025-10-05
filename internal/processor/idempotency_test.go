package processor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nimasrn/message-gateway/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
)

// Mock Redis adapter for testing
type mockRedisAdapter struct {
	data map[string][]byte
	ttls map[string]time.Time
}

func newMockRedisAdapter() *mockRedisAdapter {
	return &mockRedisAdapter{
		data: make(map[string][]byte),
		ttls: make(map[string]time.Time),
	}
}

func (m *mockRedisAdapter) SetNX(key string, value []byte, ttl time.Duration) (bool, error) {
	if _, exists := m.data[key]; exists {
		return false, nil
	}
	m.data[key] = value
	if ttl > 0 {
		m.ttls[key] = time.Now().Add(ttl)
	}
	return true, nil
}

func (m *mockRedisAdapter) Set(key string, value []byte, ttl time.Duration) error {
	m.data[key] = value
	if ttl > 0 {
		m.ttls[key] = time.Now().Add(ttl)
	}
	return nil
}

func (m *mockRedisAdapter) Get(key string) ([]byte, error) {
	if ttl, ok := m.ttls[key]; ok && time.Now().After(ttl) {
		delete(m.data, key)
		delete(m.ttls, key)
		return nil, redis.NilError
	}
	if value, ok := m.data[key]; ok {
		return value, nil
	}
	return nil, redis.NilError
}

func (m *mockRedisAdapter) Del(key string) error {
	delete(m.data, key)
	delete(m.ttls, key)
	return nil
}

func (m *mockRedisAdapter) Exist(key string) (int64, error) {
	if ttl, ok := m.ttls[key]; ok && time.Now().After(ttl) {
		delete(m.data, key)
		delete(m.ttls, key)
		return 0, nil
	}
	if _, ok := m.data[key]; ok {
		return 1, nil
	}
	return 0, nil
}

// Stub implementations for unused methods
func (m *mockRedisAdapter) SMembers(key string) ([]string, error)         { return nil, nil }
func (m *mockRedisAdapter) SAdd(key string, value ...interface{}) error   { return nil }
func (m *mockRedisAdapter) HGet(key string, field string) ([]byte, error) { return nil, nil }
func (m *mockRedisAdapter) HGetAll(key string) (map[string]string, error) { return nil, nil }
func (m *mockRedisAdapter) HScan(key string, cursor uint64, match string, count int64) ([]string, uint64, error) {
	return nil, 0, nil
}
func (m *mockRedisAdapter) SScan(key string, cursor uint64, match string, count int64) ([]string, uint64, error) {
	return nil, 0, nil
}
func (m *mockRedisAdapter) HGetMultiple(keys ...string) (map[string]map[string]string, error) {
	return nil, nil
}
func (m *mockRedisAdapter) HSetIfNotExists(key string, field string, value interface{}) error {
	return nil
}
func (m *mockRedisAdapter) HSet(key string, field string, value interface{}) error { return nil }
func (m *mockRedisAdapter) HIncrement(key string, field string, value int64) error { return nil }
func (m *mockRedisAdapter) HIncrementBatch(coreName, keySuffix string, fieldAndValues map[string]int64, ttl time.Duration) error {
	return nil
}
func (m *mockRedisAdapter) TxPipelined(fn func(goredis.Pipeliner) error) ([]goredis.Cmder, error) {
	return nil, nil
}
func (m *mockRedisAdapter) Client() goredis.UniversalClient { return nil }
func (m *mockRedisAdapter) XAdd(key string, values map[string]interface{}) (string, error) {
	return "", nil
}
func (m *mockRedisAdapter) XAddWithID(key string, id string, values map[string]interface{}) (string, error) {
	return "", nil
}
func (m *mockRedisAdapter) XRead(key string, id string, count int64) ([]redis.StreamMessage, error) {
	return nil, nil
}
func (m *mockRedisAdapter) XReadGroup(group, consumer, key, id string, count int64) ([]redis.StreamMessage, error) {
	return nil, nil
}
func (m *mockRedisAdapter) XAck(key, group string, ids ...string) error           { return nil }
func (m *mockRedisAdapter) XGroupCreate(key, group, start string) error           { return nil }
func (m *mockRedisAdapter) XGroupCreateMkStream(key, group, start string) error   { return nil }
func (m *mockRedisAdapter) XLen(key string) (int64, error)                        { return 0, nil }
func (m *mockRedisAdapter) XDel(key string, ids ...string) error                  { return nil }
func (m *mockRedisAdapter) XTrim(key string, maxLen int64) error                  { return nil }
func (m *mockRedisAdapter) XTrimApprox(key string, maxLen int64) error            { return nil }
func (m *mockRedisAdapter) XPending(key, group string) (*goredis.XPending, error) { return nil, nil }
func (m *mockRedisAdapter) XPendingExt(key, group string, start, end string, count int64) ([]goredis.XPendingExt, error) {
	return nil, nil
}
func (m *mockRedisAdapter) XClaim(key, group, consumer string, minIdle time.Duration, ids ...string) ([]redis.StreamMessage, error) {
	return nil, nil
}

func TestIdempotencyService_AcquireProcessingLock_FirstAttempt(t *testing.T) {
	mockRedis := newMockRedisAdapter()
	config := DefaultIdempotencyConfig()
	service := NewIdempotencyService(mockRedis, config)

	ctx := context.Background()
	messageID := "test-message-1"

	// First attempt should succeed
	procCtx, err := service.AcquireProcessingLock(ctx, messageID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if procCtx == nil {
		t.Fatal("Expected processing context, got nil")
	}

	if procCtx.MessageID != messageID {
		t.Errorf("Expected message ID %s, got %s", messageID, procCtx.MessageID)
	}

	if procCtx.RetryCount != 0 {
		t.Errorf("Expected retry count 0, got %d", procCtx.RetryCount)
	}

	if procCtx.IsRetry {
		t.Error("Expected IsRetry to be false")
	}

	if !procCtx.lockAcquired {
		t.Error("Expected lock to be acquired")
	}
}

func TestIdempotencyService_AcquireProcessingLock_Concurrent(t *testing.T) {
	mockRedis := newMockRedisAdapter()
	config := DefaultIdempotencyConfig()
	service := NewIdempotencyService(mockRedis, config)

	ctx := context.Background()
	messageID := "test-message-2"

	// First consumer acquires lock
	procCtx1, err := service.AcquireProcessingLock(ctx, messageID)
	if err != nil {
		t.Fatalf("First lock acquisition failed: %v", err)
	}

	// Second consumer tries to acquire same lock
	procCtx2, err := service.AcquireProcessingLock(ctx, messageID)
	if err != ErrLockAcquireFailed {
		t.Errorf("Expected ErrLockAcquireFailed, got: %v", err)
	}

	if procCtx2 != nil {
		t.Error("Expected nil context for second consumer")
	}

	// First consumer still has lock
	if !procCtx1.lockAcquired {
		t.Error("First consumer should still have lock")
	}
}

func TestIdempotencyService_MarkSuccess(t *testing.T) {
	mockRedis := newMockRedisAdapter()
	config := DefaultIdempotencyConfig()
	service := NewIdempotencyService(mockRedis, config)

	ctx := context.Background()
	messageID := "test-message-3"

	// Acquire lock
	procCtx, err := service.AcquireProcessingLock(ctx, messageID)
	if err != nil {
		t.Fatalf("Lock acquisition failed: %v", err)
	}

	// Mark as success
	err = service.MarkSuccess(ctx, procCtx)
	if err != nil {
		t.Fatalf("MarkSuccess failed: %v", err)
	}

	// Verify processed marker exists
	processed, err := service.IsProcessed(ctx, messageID)
	if err != nil {
		t.Fatalf("IsProcessed check failed: %v", err)
	}

	if !processed {
		t.Error("Message should be marked as processed")
	}

	// Try to acquire lock again - should fail with ErrAlreadyProcessed
	procCtx2, err := service.AcquireProcessingLock(ctx, messageID)
	if err != ErrAlreadyProcessed {
		t.Errorf("Expected ErrAlreadyProcessed, got: %v", err)
	}

	if procCtx2 != nil {
		t.Error("Expected nil context for already processed message")
	}
}

func TestIdempotencyService_MarkFailure_WithRetry(t *testing.T) {
	mockRedis := newMockRedisAdapter()
	config := DefaultIdempotencyConfig()
	config.MaxRetries = 3
	service := NewIdempotencyService(mockRedis, config)

	ctx := context.Background()
	messageID := "test-message-4"

	// First attempt
	procCtx1, err := service.AcquireProcessingLock(ctx, messageID)
	if err != nil {
		t.Fatalf("First lock acquisition failed: %v", err)
	}

	if procCtx1.RetryCount != 0 {
		t.Errorf("Expected retry count 0, got %d", procCtx1.RetryCount)
	}

	// Mark as failed
	err = service.MarkFailure(ctx, procCtx1, nil)
	if err != nil {
		t.Fatalf("MarkFailure failed: %v", err)
	}

	// Second attempt (retry)
	procCtx2, err := service.AcquireProcessingLock(ctx, messageID)
	if err != nil {
		t.Fatalf("Second lock acquisition failed: %v", err)
	}

	if procCtx2.RetryCount != 1 {
		t.Errorf("Expected retry count 1, got %d", procCtx2.RetryCount)
	}

	if !procCtx2.IsRetry {
		t.Error("Expected IsRetry to be true")
	}
}

func TestIdempotencyService_MaxRetriesExceeded(t *testing.T) {
	mockRedis := newMockRedisAdapter()
	config := DefaultIdempotencyConfig()
	config.MaxRetries = 2
	service := NewIdempotencyService(mockRedis, config)

	ctx := context.Background()
	messageID := "test-message-5"

	// Simulate reaching max retries
	for i := 0; i < config.MaxRetries; i++ {
		procCtx, err := service.AcquireProcessingLock(ctx, messageID)
		if err != nil {
			t.Fatalf("Lock acquisition %d failed: %v", i, err)
		}
		err = service.MarkFailure(ctx, procCtx, nil)
		if err != nil {
			t.Fatalf("MarkFailure %d failed: %v", i, err)
		}
	}

	// Next attempt should fail with ErrMaxRetriesExceeded
	procCtx, err := service.AcquireProcessingLock(ctx, messageID)
	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("Expected ErrMaxRetriesExceeded, got: %v", err)
	}

	if procCtx != nil {
		t.Error("Expected nil context after max retries")
	}
}

func TestIdempotencyService_ReleaseLock(t *testing.T) {
	mockRedis := newMockRedisAdapter()
	config := DefaultIdempotencyConfig()
	service := NewIdempotencyService(mockRedis, config)

	ctx := context.Background()
	messageID := "test-message-6"

	// Acquire lock
	procCtx, err := service.AcquireProcessingLock(ctx, messageID)
	if err != nil {
		t.Fatalf("Lock acquisition failed: %v", err)
	}

	// Release lock
	err = service.ReleaseLock(ctx, procCtx)
	if err != nil {
		t.Fatalf("ReleaseLock failed: %v", err)
	}

	if procCtx.lockAcquired {
		t.Error("Lock should be marked as released")
	}

	// Should be able to acquire lock again
	procCtx2, err := service.AcquireProcessingLock(ctx, messageID)
	if err != nil {
		t.Fatalf("Second lock acquisition failed: %v", err)
	}

	if procCtx2 == nil {
		t.Fatal("Expected processing context, got nil")
	}
}

func TestIdempotencyService_GetRetryCount(t *testing.T) {
	mockRedis := newMockRedisAdapter()
	config := DefaultIdempotencyConfig()
	service := NewIdempotencyService(mockRedis, config)

	ctx := context.Background()
	messageID := "test-message-7"

	// Initially should be 0
	count, err := service.GetRetryCount(ctx, messageID)
	if err != nil {
		t.Fatalf("GetRetryCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected retry count 0, got %d", count)
	}

	// After failure, should increment
	procCtx, _ := service.AcquireProcessingLock(ctx, messageID)
	service.MarkFailure(ctx, procCtx, nil)

	count, err = service.GetRetryCount(ctx, messageID)
	if err != nil {
		t.Fatalf("GetRetryCount failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected retry count 1, got %d", count)
	}
}
