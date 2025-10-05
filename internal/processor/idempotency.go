package processor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nimasrn/message-gateway/pkg/logger"
	"github.com/nimasrn/message-gateway/pkg/redis"
)

var (
	ErrAlreadyProcessed   = errors.New("message already processed")
	ErrLockAcquireFailed  = errors.New("failed to acquire processing lock")
	ErrMaxRetriesExceeded = errors.New("maximum retries exceeded")
)

type IdempotencyConfig struct {
	LockTTL time.Duration

	ProcessedTTL time.Duration

	MaxRetries int

	RetryKeyPrefix string

	LockKeyPrefix string

	ProcessedKeyPrefix string
}

func DefaultIdempotencyConfig() IdempotencyConfig {
	return IdempotencyConfig{
		LockTTL:            30 * time.Second,
		ProcessedTTL:       24 * time.Hour,
		MaxRetries:         3,
		RetryKeyPrefix:     "retry:",
		LockKeyPrefix:      "lock:",
		ProcessedKeyPrefix: "processed:",
	}
}

type IdempotencyService struct {
	redis  redis.RedisAdapter
	config IdempotencyConfig
}

func NewIdempotencyService(redisAdapter redis.RedisAdapter, config IdempotencyConfig) *IdempotencyService {
	return &IdempotencyService{
		redis:  redisAdapter,
		config: config,
	}
}

type ProcessingContext struct {
	MessageID    string
	RetryCount   int
	IsRetry      bool
	lockAcquired bool
	service      *IdempotencyService
}

func (s *IdempotencyService) AcquireProcessingLock(ctx context.Context, messageID string) (*ProcessingContext, error) {
	// Step 1: Check if already processed (long-term marker)
	processedKey := s.config.ProcessedKeyPrefix + messageID
	exists, err := s.redis.Exist(processedKey)
	if err != nil {
		logger.Warn("Failed to check processed status", "message_id", messageID, "error", err)
		// Continue even if check fails - better to risk duplicate than block processing
	} else if exists > 0 {
		logger.Info("Message already processed, skipping", "message_id", messageID)
		return nil, ErrAlreadyProcessed
	}

	// Step 2: Get current retry count
	retryKey := s.config.RetryKeyPrefix + messageID
	retryCountBytes, err := s.redis.Get(retryKey)
	retryCount := 0
	if err == nil && len(retryCountBytes) > 0 {
		fmt.Sscanf(string(retryCountBytes), "%d", &retryCount)
	}

	// Step 3: Check if max retries exceeded
	if retryCount >= s.config.MaxRetries {
		logger.Error("Max retries exceeded for message", "message_id", messageID, "retry_count", retryCount)
		return nil, fmt.Errorf("%w: message_id=%s, retries=%d", ErrMaxRetriesExceeded, messageID, retryCount)
	}

	// Step 4: Acquire short-term processing lock (prevents concurrent processing)
	lockKey := s.config.LockKeyPrefix + messageID
	lockValue := []byte(fmt.Sprintf("%d", time.Now().UnixNano()))

	acquired, err := s.redis.SetNX(lockKey, lockValue, s.config.LockTTL)
	if err != nil {
		logger.Error("Failed to acquire lock", "message_id", messageID, "error", err)
		return nil, fmt.Errorf("%w: %v", ErrLockAcquireFailed, err)
	}

	if !acquired {
		logger.Info("Lock already held by another consumer", "message_id", messageID)
		return nil, ErrLockAcquireFailed
	}

	logger.Info("Processing lock acquired",
		"message_id", messageID,
		"retry_count", retryCount,
		"lock_ttl", s.config.LockTTL)

	return &ProcessingContext{
		MessageID:    messageID,
		RetryCount:   retryCount,
		IsRetry:      retryCount > 0,
		lockAcquired: true,
		service:      s,
	}, nil
}

func (s *IdempotencyService) MarkSuccess(ctx context.Context, pc *ProcessingContext) error {
	messageID := pc.MessageID

	// Step 1: Set long-term processed marker (24 hours)
	processedKey := s.config.ProcessedKeyPrefix + messageID
	err := s.redis.Set(processedKey, []byte("1"), s.config.ProcessedTTL)
	if err != nil {
		logger.Error("Failed to mark message as processed", "message_id", messageID, "error", err)
		return fmt.Errorf("failed to mark as processed: %w", err)
	}

	// Step 2: Clean up lock and retry counter
	s.cleanup(ctx, pc)

	logger.Info("Message marked as successfully processed",
		"message_id", messageID,
		"retry_count", pc.RetryCount)

	return nil
}

func (s *IdempotencyService) MarkFailure(ctx context.Context, pc *ProcessingContext, reason error) error {
	messageID := pc.MessageID

	// Step 1: Increment retry counter
	retryKey := s.config.RetryKeyPrefix + messageID
	newRetryCount := pc.RetryCount + 1
	retryValue := []byte(fmt.Sprintf("%d", newRetryCount))

	// Keep retry counter for longer to track across retries
	err := s.redis.Set(retryKey, retryValue, s.config.ProcessedTTL)
	if err != nil {
		logger.Error("Failed to increment retry counter", "message_id", messageID, "error", err)
	}

	// Step 2: Remove lock to allow retry
	lockKey := s.config.LockKeyPrefix + messageID
	if err := s.redis.Del(lockKey); err != nil {
		logger.Warn("Failed to remove lock", "message_id", messageID, "error", err)
	}

	logger.Warn("Message processing failed, will retry",
		"message_id", messageID,
		"retry_count", newRetryCount,
		"max_retries", s.config.MaxRetries,
		"reason", reason)

	return nil
}

func (s *IdempotencyService) ReleaseLock(ctx context.Context, pc *ProcessingContext) error {
	if pc == nil || !pc.lockAcquired {
		return nil
	}

	lockKey := s.config.LockKeyPrefix + pc.MessageID
	if err := s.redis.Del(lockKey); err != nil {
		logger.Warn("Failed to release lock", "message_id", pc.MessageID, "error", err)
		return err
	}

	pc.lockAcquired = false
	logger.Debug("Processing lock released", "message_id", pc.MessageID)
	return nil
}

func (s *IdempotencyService) cleanup(ctx context.Context, pc *ProcessingContext) {
	messageID := pc.MessageID

	// Remove lock
	lockKey := s.config.LockKeyPrefix + messageID
	if err := s.redis.Del(lockKey); err != nil {
		logger.Warn("Failed to cleanup lock", "message_id", messageID, "error", err)
	}

	// Remove retry counter (no longer needed)
	retryKey := s.config.RetryKeyPrefix + messageID
	if err := s.redis.Del(retryKey); err != nil {
		logger.Warn("Failed to cleanup retry counter", "message_id", messageID, "error", err)
	}

	pc.lockAcquired = false
}

func (s *IdempotencyService) GetRetryCount(ctx context.Context, messageID string) (int, error) {
	retryKey := s.config.RetryKeyPrefix + messageID
	retryCountBytes, err := s.redis.Get(retryKey)
	if err != nil {
		if err == redis.NilError {
			return 0, nil
		}
		return 0, err
	}

	retryCount := 0
	fmt.Sscanf(string(retryCountBytes), "%d", &retryCount)
	return retryCount, nil
}

func (s *IdempotencyService) IsProcessed(ctx context.Context, messageID string) (bool, error) {
	processedKey := s.config.ProcessedKeyPrefix + messageID
	exists, err := s.redis.Exist(processedKey)
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}
