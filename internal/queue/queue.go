package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nimasrn/message-gateway/pkg/redis"
)

type Message struct {
	ID        string
	Data      []byte
	Metadata  map[string]string
	Timestamp time.Time
	Attempts  int
	acked     bool
	nacked    bool
	queue     *Queue
}

// Ack explicitly acknowledges the message (marks as successfully processed)
func (m *Message) Ack() error {
	if m.acked {
		return fmt.Errorf("message already acknowledged")
	}
	if m.nacked {
		return fmt.Errorf("message already rejected")
	}

	m.acked = true
	return m.queue.ackMessage(m.ID)
}

// Nack explicitly rejects the message (will be retried)
func (m *Message) Nack() error {
	if m.acked {
		return fmt.Errorf("message already acknowledged")
	}
	if m.nacked {
		return fmt.Errorf("message already rejected")
	}

	m.nacked = true
	// Don't ack - message stays pending and will be reclaimed
	return nil
}

// MessageHandler is a function that processes messages
// Return values:
//   - nil: Success - message will be auto-acked
//   - error: Failure - message will NOT be acked and will retry
type MessageHandler func(ctx context.Context, msg *Message) error

// MessageHandlerManual is for manual ACK control
type MessageHandlerManual func(ctx context.Context, msg *Message)

type AckMode int

const (
	// AckModeAuto - Automatically ack on success, nack on error (default)
	AckModeAuto AckMode = iota

	// AckModeManual - Handler must explicitly call msg.Ack() or msg.Nack()
	AckModeManual
)

type QueueConfig struct {
	Name              string
	ConsumerGroup     string
	ConsumerName      string
	MaxRetries        int
	VisibilityTimeout time.Duration
	PollInterval      time.Duration
	BatchSize         int64
	MaxLen            int64
	EnableDLQ         bool
}

type Queue struct {
	adapter       redis.RedisAdapter
	config        QueueConfig
	handler       MessageHandler
	handlerManual MessageHandlerManual
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.RWMutex
	processing    map[string]*Message
}

type QueueStats struct {
	TotalMessages   int64
	PendingMessages int64
	ProcessedCount  int64
	FailedCount     int64
	ConsumerCount   int64
}

// NewQueue creates a new queue instance
func NewQueue(adapter redis.RedisAdapter, config QueueConfig) (*Queue, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("queue name is required")
	}
	if config.ConsumerGroup == "" {
		config.ConsumerGroup = "default-group"
	}
	if config.ConsumerName == "" {
		config.ConsumerName = fmt.Sprintf("consumer-%d", time.Now().UnixNano())
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.VisibilityTimeout == 0 {
		config.VisibilityTimeout = 30 * time.Second
	}
	if config.PollInterval == 0 {
		config.PollInterval = 1 * time.Second
	}
	if config.BatchSize == 0 {
		config.BatchSize = 10
	}

	ctx, cancel := context.WithCancel(context.Background())

	q := &Queue{
		adapter:    adapter,
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
		processing: make(map[string]*Message),
	}

	if err := q.initConsumerGroup(); err != nil {
		// Group might already exist, which is fine
	}

	return q, nil
}

func (q *Queue) initConsumerGroup() error {
	return q.adapter.XGroupCreateMkStream(
		q.config.Name,
		q.config.ConsumerGroup,
		"0",
	)
}

// Publish adds a message to the queue
func (q *Queue) Publish(ctx context.Context, data []byte, metadata map[string]string) (string, error) {
	if metadata == nil {
		metadata = make(map[string]string)
	}

	values := map[string]interface{}{
		"data":      string(data),
		"timestamp": time.Now().Unix(),
		"attempts":  0,
	}

	for k, v := range metadata {
		values["meta_"+k] = v
	}

	id, err := q.adapter.XAdd(q.config.Name, values)
	if err != nil {
		return "", fmt.Errorf("failed to publish message: %w", err)
	}

	if q.config.MaxLen > 0 {
		_ = q.adapter.XTrimApprox(q.config.Name, q.config.MaxLen)
	}

	return id, nil
}

// PublishJSON publishes a JSON-encoded message
func (q *Queue) PublishJSON(ctx context.Context, data interface{}, metadata map[string]string) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return q.Publish(ctx, jsonData, metadata)
}

// Consume starts consuming messages with auto-ack mode
func (q *Queue) Consume(handler MessageHandler) error {
	if handler == nil {
		return fmt.Errorf("message handler is required")
	}

	q.handler = handler
	q.wg.Add(1)

	go q.consumeLoop()

	return nil
}

func (q *Queue) consumeLoop() {
	defer q.wg.Done()

	ticker := time.NewTicker(q.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			return
		case <-ticker.C:
			q.processMessages()
			q.claimStuckMessages()
		}
	}
}

func (q *Queue) processMessages() {
	messages, err := q.adapter.XReadGroup(
		q.config.ConsumerGroup,
		q.config.ConsumerName,
		q.config.Name,
		">",
		q.config.BatchSize,
	)

	if err != nil {
		if err != redis.NilError {
			// Log error
		}
		return
	}

	for _, streamMsg := range messages {
		msg := q.streamMessageToMessage(streamMsg)
		msg.queue = q
		q.handleMessage(msg)
	}
}

func (q *Queue) claimStuckMessages() {
	pending, err := q.adapter.XPending(q.config.Name, q.config.ConsumerGroup)
	if err != nil || pending == nil || pending.Count == 0 {
		return
	}

	pendingExt, err := q.adapter.XPendingExt(
		q.config.Name,
		q.config.ConsumerGroup,
		"-",
		"+",
		100,
	)
	if err != nil || len(pendingExt) == 0 {
		return
	}

	var idsToReclaim []string
	for _, msg := range pendingExt {
		if msg.Idle >= q.config.VisibilityTimeout {
			idsToReclaim = append(idsToReclaim, msg.ID)
		}
	}

	if len(idsToReclaim) == 0 {
		return
	}

	messages, err := q.adapter.XClaim(
		q.config.Name,
		q.config.ConsumerGroup,
		q.config.ConsumerName,
		q.config.VisibilityTimeout,
		idsToReclaim...,
	)

	if err != nil {
		return
	}

	for _, streamMsg := range messages {
		msg := q.streamMessageToMessage(streamMsg)
		msg.queue = q
		msg.Attempts++
		q.handleMessage(msg)
	}
}

func (q *Queue) handleMessage(msg *Message) {
	q.mu.Lock()
	q.processing[msg.ID] = msg
	q.mu.Unlock()

	defer func() {
		q.mu.Lock()
		delete(q.processing, msg.ID)
		q.mu.Unlock()
	}()

	// Check if max retries exceeded
	if msg.Attempts >= q.config.MaxRetries {
		q.moveToDeadLetterQueue(msg)
		q.ackMessage(msg.ID)
		return
	}

	ctx, cancel := context.WithTimeout(q.ctx, q.config.VisibilityTimeout)
	defer cancel()

	q.handleMessageAuto(ctx, msg)
}

// handleMessageAuto - automatic ACK on success, NACK on error
func (q *Queue) handleMessageAuto(ctx context.Context, msg *Message) {
	err := q.handler(ctx, msg)
	if err != nil {
		// Error - don't ack, message will be retried
		return
	}

	// Success - acknowledge message
	q.ackMessage(msg.ID)
}

func (q *Queue) ackMessage(messageID string) error {
	return q.adapter.XAck(q.config.Name, q.config.ConsumerGroup, messageID)
}

func (q *Queue) moveToDeadLetterQueue(msg *Message) {
	if !q.config.EnableDLQ {
		return
	}

	dlqName := q.config.Name + ":dlq"

	values := map[string]interface{}{
		"data":           string(msg.Data),
		"original_id":    msg.ID,
		"attempts":       msg.Attempts,
		"failed_at":      time.Now().Unix(),
		"original_queue": q.config.Name,
	}

	for k, v := range msg.Metadata {
		values["meta_"+k] = v
	}

	_, _ = q.adapter.XAdd(dlqName, values)
}

func (q *Queue) streamMessageToMessage(streamMsg redis.StreamMessage) *Message {
	msg := &Message{
		ID:       streamMsg.ID,
		Metadata: make(map[string]string),
	}

	for k, v := range streamMsg.Values {
		switch k {
		case "data":
			if data, ok := v.(string); ok {
				msg.Data = []byte(data)
			}
		case "timestamp":
			if ts, ok := v.(string); ok {
				if unix, err := time.Parse(time.RFC3339, ts); err == nil {
					msg.Timestamp = unix
				}
			}
		case "attempts":
			if attempts, ok := v.(string); ok {
				fmt.Sscanf(attempts, "%d", &msg.Attempts)
			}
		default:
			if len(k) > 5 && k[:5] == "meta_" {
				if val, ok := v.(string); ok {
					msg.Metadata[k[5:]] = val
				}
			}
		}
	}

	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	return msg
}

func (q *Queue) Stop(timeout time.Duration) error {
	q.cancel()

	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for queue to stop")
	}
}

func (q *Queue) GetStats() (*QueueStats, error) {
	totalMessages, err := q.adapter.XLen(q.config.Name)
	if err != nil {
		return nil, err
	}

	pending, err := q.adapter.XPending(q.config.Name, q.config.ConsumerGroup)
	if err != nil {
		pending = nil
	}

	stats := &QueueStats{
		TotalMessages: totalMessages,
	}

	if pending != nil {
		stats.PendingMessages = pending.Count
		stats.ConsumerCount = int64(len(pending.Consumers))
	}

	return stats, nil
}
