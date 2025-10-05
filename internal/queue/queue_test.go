package queue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/nimasrn/message-gateway/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) (*miniredis.Miniredis, redis.RedisAdapter) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	// Use unique connection name per test to avoid global adapter caching issues
	connName := t.Name() + "-" + mr.Addr()
	adapter, err := redis.NewRedisAdapter(connName, "", &goredis.UniversalOptions{
		Addrs: []string{mr.Addr()},
	})
	require.NoError(t, err)

	return mr, adapter
}

func TestQueue_PublishAndConsume(t *testing.T) {
	mr, adapter := setupTestRedis(t)
	defer mr.Close()

	config := QueueConfig{
		Name:              "test:queue",
		ConsumerGroup:     "test-group",
		ConsumerName:      "test-consumer",
		MaxRetries:        3,
		VisibilityTimeout: 5 * time.Second,
		PollInterval:      100 * time.Millisecond,
		BatchSize:         10,
		MaxLen:            1000,
		EnableDLQ:         true,
	}

	queue, err := NewQueue(adapter, config)
	require.NoError(t, err)

	t.Run("publish and consume message", func(t *testing.T) {
		ctx := context.Background()
		testData := map[string]string{"key": "value"}

		_, err := queue.PublishJSON(ctx, testData, map[string]string{"type": "test"})
		require.NoError(t, err)

		received := make(chan bool, 1)
		handler := func(ctx context.Context, msg *Message) error {
			var data map[string]string
			err := json.Unmarshal(msg.Data, &data)
			assert.NoError(t, err)
			assert.Equal(t, "value", data["key"])
			assert.Equal(t, "test", msg.Metadata["type"])
			received <- true
			return nil
		}

		err = queue.Consume(handler)
		require.NoError(t, err)

		select {
		case <-received:
		case <-time.After(2 * time.Second):
			t.Fatal("message not received")
		}

		queue.Stop(time.Second)
	})
}

func TestQueue_PublishJSON(t *testing.T) {
	mr, adapter := setupTestRedis(t)
	defer mr.Close()

	config := QueueConfig{
		Name:              "test:json:queue",
		ConsumerGroup:     "test-group",
		ConsumerName:      "test-consumer",
		MaxRetries:        3,
		VisibilityTimeout: 5 * time.Second,
		PollInterval:      100 * time.Millisecond,
		BatchSize:         10,
	}

	queue, err := NewQueue(adapter, config)
	require.NoError(t, err)
	defer queue.Stop(time.Second)

	ctx := context.Background()
	testObj := struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}{
		ID:   123,
		Name: "test",
	}

	_, err = queue.PublishJSON(ctx, testObj, map[string]string{"source": "test"})
	assert.NoError(t, err)
}

func TestQueue_RetryMechanism(t *testing.T) {
	mr, adapter := setupTestRedis(t)
	defer mr.Close()

	config := QueueConfig{
		Name:              "test:retry:queue",
		ConsumerGroup:     "test-group",
		ConsumerName:      "test-consumer",
		MaxRetries:        2,
		VisibilityTimeout: 1 * time.Second,
		PollInterval:      100 * time.Millisecond,
		BatchSize:         10,
		EnableDLQ:         true,
	}

	queue, err := NewQueue(adapter, config)
	require.NoError(t, err)
	defer queue.Stop(time.Second)

	ctx := context.Background()
	_, err = queue.PublishJSON(ctx, map[string]string{"test": "retry"}, nil)
	require.NoError(t, err)

	attempts := 0
	handler := func(ctx context.Context, msg *Message) error {
		attempts++
		if attempts <= 2 {
			return assert.AnError
		}
		return nil
	}

	err = queue.Consume(handler)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)
	assert.GreaterOrEqual(t, attempts, 1)
}

func TestQueue_GetStats(t *testing.T) {
	mr, adapter := setupTestRedis(t)
	defer mr.Close()

	config := QueueConfig{
		Name:              "test:stats:queue",
		ConsumerGroup:     "test-group",
		ConsumerName:      "test-consumer",
		MaxRetries:        3,
		VisibilityTimeout: 5 * time.Second,
		PollInterval:      100 * time.Millisecond,
		BatchSize:         10,
	}

	queue, err := NewQueue(adapter, config)
	require.NoError(t, err)
	defer queue.Stop(time.Second)

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, err := queue.PublishJSON(ctx, map[string]int{"count": i}, nil)
		require.NoError(t, err)
	}

	stats, err := queue.GetStats()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, stats.TotalMessages, int64(5))
}

func TestMessage_AckNack(t *testing.T) {
	mr, adapter := setupTestRedis(t)
	defer mr.Close()

	config := QueueConfig{
		Name:              "test:ack:queue",
		ConsumerGroup:     "test-group",
		ConsumerName:      "test-consumer",
		MaxRetries:        3,
		VisibilityTimeout: 5 * time.Second,
		PollInterval:      100 * time.Millisecond,
		BatchSize:         10,
	}

	queue, err := NewQueue(adapter, config)
	require.NoError(t, err)
	defer queue.Stop(time.Second)

	t.Run("ack marks message as processed", func(t *testing.T) {
		// Publish a real message to get a valid ID
		msgID, err := queue.Publish(context.Background(), []byte(`{"test":"data"}`), map[string]string{})
		require.NoError(t, err)

		msg := &Message{
			ID:       msgID,
			Data:     []byte(`{"test":"data"}`),
			Metadata: map[string]string{},
			acked:    false,
			nacked:   false,
			queue:    queue,
		}

		err = msg.Ack()
		assert.NoError(t, err)
		assert.True(t, msg.acked)
		assert.False(t, msg.nacked)
	})

	t.Run("nack marks message for retry", func(t *testing.T) {
		msg := &Message{
			ID:       "test-2",
			Data:     []byte(`{"test":"data"}`),
			Metadata: map[string]string{},
			acked:    false,
			nacked:   false,
			queue:    queue,
		}

		err := msg.Nack()
		assert.NoError(t, err)
		assert.False(t, msg.acked)
		assert.True(t, msg.nacked)
	})

	t.Run("cannot ack already acked message", func(t *testing.T) {
		msg := &Message{
			ID:       "test-3",
			Data:     []byte(`{"test":"data"}`),
			Metadata: map[string]string{},
			acked:    true,
			nacked:   false,
		}

		err := msg.Ack()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already acknowledged")
	})

	t.Run("cannot nack already nacked message", func(t *testing.T) {
		msg := &Message{
			ID:       "test-4",
			Data:     []byte(`{"test":"data"}`),
			Metadata: map[string]string{},
			acked:    false,
			nacked:   true,
		}

		err := msg.Nack()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already rejected")
	})
}

func TestQueueConfig_Validation(t *testing.T) {
	_, adapter := setupTestRedis(t)

	t.Run("valid config creates queue", func(t *testing.T) {
		config := QueueConfig{
			Name:              "valid:queue",
			ConsumerGroup:     "valid-group",
			ConsumerName:      "valid-consumer",
			MaxRetries:        3,
			VisibilityTimeout: 5 * time.Second,
			PollInterval:      100 * time.Millisecond,
			BatchSize:         10,
		}

		queue, err := NewQueue(adapter, config)
		assert.NoError(t, err)
		assert.NotNil(t, queue)
		queue.Stop(time.Second)
	})
}

func TestQueue_ConcurrentPublish(t *testing.T) {
	mr, adapter := setupTestRedis(t)
	defer mr.Close()

	config := QueueConfig{
		Name:              "test:concurrent:queue",
		ConsumerGroup:     "test-group",
		ConsumerName:      "test-consumer",
		MaxRetries:        3,
		VisibilityTimeout: 5 * time.Second,
		PollInterval:      100 * time.Millisecond,
		BatchSize:         10,
	}

	queue, err := NewQueue(adapter, config)
	require.NoError(t, err)
	defer queue.Stop(time.Second)

	ctx := context.Background()
	numGoroutines := 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			_, err := queue.PublishJSON(ctx, map[string]int{"id": id}, nil)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	stats, err := queue.GetStats()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, stats.TotalMessages, int64(numGoroutines))
}

func TestQueue_Stop(t *testing.T) {
	mr, adapter := setupTestRedis(t)
	defer mr.Close()

	config := QueueConfig{
		Name:              "test:stop:queue",
		ConsumerGroup:     "test-group",
		ConsumerName:      "test-consumer",
		MaxRetries:        3,
		VisibilityTimeout: 5 * time.Second,
		PollInterval:      100 * time.Millisecond,
		BatchSize:         10,
	}

	queue, err := NewQueue(adapter, config)
	require.NoError(t, err)

	handler := func(ctx context.Context, msg *Message) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	err = queue.Consume(handler)
	require.NoError(t, err)

	err = queue.Stop(2 * time.Second)
	assert.NoError(t, err)
}
