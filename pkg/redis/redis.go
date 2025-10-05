package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var NilError = goredis.Nil

type Options = goredis.UniversalOptions

// StreamMessage represents a message in Redis Stream
type StreamMessage struct {
	ID     string
	Values map[string]interface{}
}

type RedisAdapter interface {
	// Basic operations
	Set(key string, value []byte, ttl time.Duration) error
	SetNX(key string, value []byte, ttl time.Duration) (bool, error)
	Get(key string) ([]byte, error)
	Del(key string) error
	SMembers(key string) ([]string, error)
	SAdd(key string, value ...interface{}) error
	HGet(key string, field string) ([]byte, error)
	HGetAll(key string) (map[string]string, error)
	HScan(key string, cursor uint64, match string, count int64) ([]string, uint64, error)
	SScan(key string, cursor uint64, match string, count int64) ([]string, uint64, error)
	HGetMultiple(keys ...string) (map[string]map[string]string, error)
	HSetIfNotExists(key string, field string, value interface{}) error
	HSet(key string, field string, value interface{}) error
	Exist(key string) (int64, error)
	HIncrement(key string, field string, value int64) error
	HIncrementBatch(coreName, keySuffix string, fieldAndValues map[string]int64, ttl time.Duration) error
	TxPipelined(fn func(goredis.Pipeliner) error) ([]goredis.Cmder, error)
	Client() goredis.UniversalClient

	// Stream operations
	XAdd(key string, values map[string]interface{}) (string, error)
	XAddWithID(key string, id string, values map[string]interface{}) (string, error)
	XRead(key string, id string, count int64) ([]StreamMessage, error)
	XReadGroup(group, consumer, key, id string, count int64) ([]StreamMessage, error)
	XAck(key, group string, ids ...string) error
	XGroupCreate(key, group, start string) error
	XGroupCreateMkStream(key, group, start string) error
	XLen(key string) (int64, error)
	XDel(key string, ids ...string) error
	XTrim(key string, maxLen int64) error
	XTrimApprox(key string, maxLen int64) error
	XPending(key, group string) (*goredis.XPending, error)
	XPendingExt(key, group string, start, end string, count int64) ([]goredis.XPendingExt, error)
	XClaim(key, group, consumer string, minIdle time.Duration, ids ...string) ([]StreamMessage, error)
}

type redisAdapter struct {
	prefix   string
	Conn     goredis.UniversalClient
	ConnName string
}

var redisLock = &sync.RWMutex{}
var redisInstance map[string]RedisAdapter

func NewRedisAdapter(connName string, keysPrefix string, opts *goredis.UniversalOptions) (RedisAdapter, error) {
	// First check if adapter already exists (with read lock)
	redisLock.RLock()
	if redisInstance != nil {
		if adapter, ok := redisInstance[connName]; ok {
			redisLock.RUnlock()
			return adapter, nil
		}
	}
	redisLock.RUnlock()

	redisLock.Lock()
	if redisInstance == nil {
		redisInstance = make(map[string]RedisAdapter)
	}
	if adapter, ok := redisInstance[connName]; ok {
		redisLock.Unlock()
		return adapter, nil
	}
	redisLock.Unlock()

	c := goredis.NewUniversalClient(opts)
	if cmd := c.Ping(context.Background()); cmd.Err() != nil {
		return nil, cmd.Err()
	}

	adapter := &redisAdapter{
		Conn:     c,
		prefix:   keysPrefix,
		ConnName: connName,
	}

	redisLock.Lock()
	redisInstance[connName] = adapter
	redisLock.Unlock()

	return adapter, nil
}

func GetRedis(connName ...string) RedisAdapter {
	redisLock.RLock()
	defer redisLock.RUnlock()

	name := "default"
	if len(connName) > 0 && connName[0] != "" {
		name = connName[0]
	}

	if adapter, ok := redisInstance[name]; ok {
		return adapter
	}

	// Fallback to default
	return redisInstance["default"]
}

// Basic operations (existing methods)
func (r *redisAdapter) Set(key string, value []byte, ttl time.Duration) error {
	st := r.Conn.Set(context.Background(), r.prefix+key, value, ttl)
	return st.Err()
}

func (r *redisAdapter) SetNX(key string, value []byte, ttl time.Duration) (bool, error) {
	cmd := r.Conn.SetNX(context.Background(), r.prefix+key, value, ttl)
	if err := cmd.Err(); err != nil {
		return false, err
	}
	return cmd.Val(), nil
}

func (r *redisAdapter) Get(key string) ([]byte, error) {
	st := r.Conn.Get(context.Background(), r.prefix+key)
	if err := st.Err(); err != nil {
		return nil, err
	}
	return st.Bytes()
}

func (r *redisAdapter) Del(key string) error {
	cmd := r.Conn.Del(context.Background(), r.prefix+key)
	return cmd.Err()
}

func (r *redisAdapter) SMembers(key string) ([]string, error) {
	st := r.Conn.SMembers(context.Background(), r.prefix+key)
	if st.Err() != nil {
		return nil, st.Err()
	}
	return st.Val(), nil
}

func (r *redisAdapter) SAdd(key string, value ...interface{}) error {
	st := r.Conn.SAdd(context.Background(), r.prefix+key, value...)
	return st.Err()
}

func (r *redisAdapter) HGet(key string, field string) ([]byte, error) {
	st := r.Conn.HGet(context.Background(), r.prefix+key, field)
	b, err := st.Bytes()
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r *redisAdapter) HGetAll(key string) (map[string]string, error) {
	st := r.Conn.HGetAll(context.Background(), r.prefix+key)
	if st.Err() != nil {
		return nil, st.Err()
	}
	return st.Val(), nil
}

func (r *redisAdapter) HScan(key string, cursor uint64, match string, count int64) ([]string, uint64, error) {
	st := r.Conn.HScan(context.Background(), r.prefix+key, cursor, match, count)
	if st.Err() != nil {
		return nil, 0, nil
	}
	return st.Result()
}

func (r *redisAdapter) SScan(key string, cursor uint64, match string, count int64) ([]string, uint64, error) {
	st := r.Conn.SScan(context.Background(), r.prefix+key, cursor, match, count)
	if st.Err() != nil {
		return nil, 0, nil
	}
	return st.Result()
}

func (r *redisAdapter) HGetMultiple(keys ...string) (map[string]map[string]string, error) {
	response := make(map[string]map[string]string)
	for _, rk := range keys {
		st := r.Conn.HGetAll(context.Background(), r.prefix+rk)
		if st.Err() != nil {
			return nil, st.Err()
		}
		response[rk] = st.Val()
	}
	return response, nil
}

func (r *redisAdapter) HSetIfNotExists(key string, field string, value interface{}) error {
	cmd := r.Conn.HSetNX(context.Background(), r.prefix+key, field, value)
	if cmd.Err() != nil {
		return cmd.Err()
	}
	return nil
}

func (r *redisAdapter) HSet(key string, field string, value interface{}) error {
	cmd := r.Conn.HSet(context.Background(), r.prefix+key, field, value)
	if cmd.Err() != nil {
		return cmd.Err()
	}
	return nil
}

func (r *redisAdapter) HIncrement(key string, field string, value int64) error {
	cmd := r.Conn.HIncrBy(context.Background(), r.prefix+key, field, value)
	if cmd.Err() != nil {
		return cmd.Err()
	}
	return nil
}

func (r *redisAdapter) Exist(key string) (int64, error) {
	res, err := r.Conn.Exists(context.Background(), r.prefix+key).Result()
	return res, err
}

func (r *redisAdapter) Client() goredis.UniversalClient {
	return r.Conn
}

func (r *redisAdapter) TxPipelined(fn func(goredis.Pipeliner) error) ([]goredis.Cmder, error) {
	pipelined, err := r.Conn.TxPipelined(context.Background(), fn)
	if err != nil {
		return nil, err
	}
	return pipelined, nil
}

func (r *redisAdapter) HIncrementBatch(coreName, keySuffix string, fieldAndValues map[string]int64, ttl time.Duration) error {
	cmd, err := r.Conn.Pipelined(context.Background(), func(pipeliner goredis.Pipeliner) error {
		mainKey := r.prefix + coreName + keySuffix
		for field, v := range fieldAndValues {
			pipeliner.HIncrBy(context.Background(), r.prefix+coreName+keySuffix, field, v)
			pipeliner.Expire(context.Background(), mainKey, ttl)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create pipelined HIncrBy: %w", err)
	}
	if cmd != nil && len(cmd) != 0 {
		for _, v := range cmd {
			if v != nil && v.Err() != nil {
				return fmt.Errorf("pipelined HIncr error: %w", v.Err())
			}
		}
	}
	return nil
}

// Stream operations implementation

func (r *redisAdapter) XAdd(key string, values map[string]interface{}) (string, error) {
	return r.XAddWithID(key, "*", values)
}

func (r *redisAdapter) XAddWithID(key string, id string, values map[string]interface{}) (string, error) {
	cmd := r.Conn.XAdd(context.Background(), &goredis.XAddArgs{
		Stream: r.prefix + key,
		ID:     id,
		Values: values,
	})
	if cmd.Err() != nil {
		return "", cmd.Err()
	}
	return cmd.Val(), nil
}

func (r *redisAdapter) XRead(key string, id string, count int64) ([]StreamMessage, error) {
	streams := r.Conn.XRead(context.Background(), &goredis.XReadArgs{
		Streams: []string{r.prefix + key, id},
		Count:   count,
		Block:   0,
	})

	if streams.Err() != nil {
		return nil, streams.Err()
	}

	var messages []StreamMessage
	for _, stream := range streams.Val() {
		for _, msg := range stream.Messages {
			messages = append(messages, StreamMessage{
				ID:     msg.ID,
				Values: msg.Values,
			})
		}
	}
	return messages, nil
}

func (r *redisAdapter) XReadGroup(group, consumer, key, id string, count int64) ([]StreamMessage, error) {
	streams := r.Conn.XReadGroup(context.Background(), &goredis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{r.prefix + key, id},
		Count:    count,
		Block:    0,
	})

	if streams.Err() != nil {
		return nil, streams.Err()
	}

	var messages []StreamMessage
	for _, stream := range streams.Val() {
		for _, msg := range stream.Messages {
			messages = append(messages, StreamMessage{
				ID:     msg.ID,
				Values: msg.Values,
			})
		}
	}
	return messages, nil
}

func (r *redisAdapter) XAck(key, group string, ids ...string) error {
	cmd := r.Conn.XAck(context.Background(), r.prefix+key, group, ids...)
	return cmd.Err()
}

func (r *redisAdapter) XGroupCreate(key, group, start string) error {
	cmd := r.Conn.XGroupCreate(context.Background(), r.prefix+key, group, start)
	return cmd.Err()
}

func (r *redisAdapter) XGroupCreateMkStream(key, group, start string) error {
	cmd := r.Conn.XGroupCreateMkStream(context.Background(), r.prefix+key, group, start)
	return cmd.Err()
}

func (r *redisAdapter) XLen(key string) (int64, error) {
	cmd := r.Conn.XLen(context.Background(), r.prefix+key)
	if cmd.Err() != nil {
		return 0, cmd.Err()
	}
	return cmd.Val(), nil
}

func (r *redisAdapter) XDel(key string, ids ...string) error {
	cmd := r.Conn.XDel(context.Background(), r.prefix+key, ids...)
	return cmd.Err()
}

func (r *redisAdapter) XTrim(key string, maxLen int64) error {
	cmd := r.Conn.XTrimMaxLen(context.Background(), r.prefix+key, maxLen)
	return cmd.Err()
}

func (r *redisAdapter) XTrimApprox(key string, maxLen int64) error {
	cmd := r.Conn.XTrimMaxLenApprox(context.Background(), r.prefix+key, maxLen, 0)
	return cmd.Err()
}

func (r *redisAdapter) XPending(key, group string) (*goredis.XPending, error) {
	cmd := r.Conn.XPending(context.Background(), r.prefix+key, group)
	if cmd.Err() != nil {
		return nil, cmd.Err()
	}
	result := cmd.Val()
	return result, nil
}

func (r *redisAdapter) XPendingExt(key, group string, start, end string, count int64) ([]goredis.XPendingExt, error) {
	cmd := r.Conn.XPendingExt(context.Background(), &goredis.XPendingExtArgs{
		Stream: r.prefix + key,
		Group:  group,
		Start:  start,
		End:    end,
		Count:  count,
	})
	if cmd.Err() != nil {
		return nil, cmd.Err()
	}
	return cmd.Val(), nil
}

func (r *redisAdapter) XClaim(key, group, consumer string, minIdle time.Duration, ids ...string) ([]StreamMessage, error) {
	cmd := r.Conn.XClaim(context.Background(), &goredis.XClaimArgs{
		Stream:   r.prefix + key,
		Group:    group,
		Consumer: consumer,
		MinIdle:  minIdle,
		Messages: ids,
	})

	if cmd.Err() != nil {
		return nil, cmd.Err()
	}

	var messages []StreamMessage
	for _, msg := range cmd.Val() {
		messages = append(messages, StreamMessage{
			ID:     msg.ID,
			Values: msg.Values,
		})
	}
	return messages, nil
}
