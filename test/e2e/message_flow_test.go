package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/nimasrn/message-gateway/internal/handlers"
	"github.com/nimasrn/message-gateway/internal/model"
	"github.com/nimasrn/message-gateway/internal/processor"
	"github.com/nimasrn/message-gateway/internal/queue"
	"github.com/nimasrn/message-gateway/internal/repository"
	"github.com/nimasrn/message-gateway/internal/services"
	"github.com/nimasrn/message-gateway/pkg/pg"
	"github.com/nimasrn/message-gateway/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testDB = pg.DB

type TestEnvironment struct {
	DB              *pg.DB
	Redis           *miniredis.Miniredis
	RedisAdapter    redis.RedisAdapter
	Queue           *queue.Queue
	CustomerRepo    *repository.CustomerRepository
	MessageRepo     *repository.MessageRepository
	TransactionRepo *repository.TransactionRepository
	DeliveryRepo    *repository.DeliveryReportRepository
	MessageService  *services.MessageService
	MessageHandler  *handlers.MessageHandler
	Processor       *processor.SMSMessageProcessor
}

func setupE2EEnvironment(t *testing.T) *TestEnvironment {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&repository.CustomerEntity{},
		&repository.MessageEntity{},
		&repository.TransactionEntity{},
		&repository.DeliveryReportEntity{},
	)
	require.NoError(t, err)

	pgDB := &testDB{}
	pgDBValue := reflect.ValueOf(pgDB).Elem()

	readField := pgDBValue.FieldByName("read")
	writeField := pgDBValue.FieldByName("write")

	readField = reflect.NewAt(readField.Type(), readField.Addr().UnsafePointer()).Elem()
	writeField = reflect.NewAt(writeField.Type(), writeField.Addr().UnsafePointer()).Elem()

	readField.Set(reflect.ValueOf(db))
	writeField.Set(reflect.ValueOf(db))

	mr, err := miniredis.Run()
	require.NoError(t, err)

	// Use unique connection name per test to avoid global adapter caching issues
	connName := fmt.Sprintf("test-%d", time.Now().UnixNano())
	redisAdapter, err := redis.NewRedisAdapter(connName, "", &goredis.UniversalOptions{
		Addrs: []string{mr.Addr()},
	})
	require.NoError(t, err)

	queueConfig := queue.QueueConfig{
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

	q, err := queue.NewQueue(redisAdapter, queueConfig)
	require.NoError(t, err)

	customerRepo := repository.NewCustomerRepository(pgDB)
	messageRepo := repository.NewMessageRepository(pgDB)
	transactionRepo := repository.NewTransactionRepository(pgDB)
	deliveryRepo := repository.NewDeliveryReportRepository(pgDB)

	messageService := services.NewMessageService(messageRepo, customerRepo, transactionRepo, q, q)
	messageHandler := handlers.NewMessageHandler(messageService)

	return &TestEnvironment{
		DB:              pgDB,
		Redis:           mr,
		RedisAdapter:    redisAdapter,
		Queue:           q,
		CustomerRepo:    customerRepo,
		MessageRepo:     messageRepo,
		TransactionRepo: transactionRepo,
		DeliveryRepo:    deliveryRepo,
		MessageService:  messageService,
		MessageHandler:  messageHandler,
	}
}

func (env *TestEnvironment) Cleanup() {
	// Stop queue first (gracefully drain messages)
	if env.Queue != nil {
		_ = env.Queue.Stop(5 * time.Second)
	}
	// Give time for any in-flight operations to complete
	time.Sleep(100 * time.Millisecond)
	// Then close Redis
	if env.Redis != nil {
		env.Redis.Close()
	}
}

func TestE2E_MessageCreationAndEnqueue(t *testing.T) {
	env := setupE2EEnvironment(t)
	defer env.Cleanup()

	ctx := context.Background()

	customer := &repository.CustomerEntity{
		ID:      1,
		APIKey:  "test-key",
		Balance: 1000,
	}
	err := env.DB.Write(ctx).Create(customer).Error
	require.NoError(t, err)

	req := model.MessageCreateRequest{
		CustomerID: 1,
		Mobile:     "+1234567890",
		Content:    "E2E test message",
		Priority:   "normal",
	}

	msg, err := env.MessageService.Create(ctx, req)
	require.NoError(t, err)
	assert.NotZero(t, msg.ID)
	assert.Equal(t, "+1234567890", msg.Mobile)

	var updatedCustomer repository.CustomerEntity
	err = env.DB.Read(ctx).First(&updatedCustomer, 1).Error
	require.NoError(t, err)
	assert.Equal(t, uint(999), updatedCustomer.Balance)

	var txn repository.TransactionEntity
	err = env.DB.Read(ctx).Where("customer_id = ? AND type = ?", 1, "debit").First(&txn).Error
	require.NoError(t, err)
	assert.Equal(t, uint(1), txn.Amount)

	stats, err := env.Queue.GetStats()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, stats.TotalMessages, int64(1))
}

func TestE2E_InsufficientBalance(t *testing.T) {
	env := setupE2EEnvironment(t)
	defer env.Cleanup()

	ctx := context.Background()

	customer := &repository.CustomerEntity{
		ID:      2,
		APIKey:  "test-key-2",
		Balance: 0,
	}
	err := env.DB.Write(ctx).Create(customer).Error
	require.NoError(t, err)

	req := model.MessageCreateRequest{
		CustomerID: 2,
		Mobile:     "+1234567890",
		Content:    "Test message",
		Priority:   "normal",
	}

	msg, err := env.MessageService.Create(ctx, req)
	assert.ErrorIs(t, err, services.ErrInsufficientBalance)
	assert.Nil(t, msg)

	var count int64
	env.DB.Read(ctx).Model(&repository.MessageEntity{}).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestE2E_MessageConsumption(t *testing.T) {
	env := setupE2EEnvironment(t)
	defer env.Cleanup()

	ctx := context.Background()

	customer := &repository.CustomerEntity{
		ID:      3,
		APIKey:  "test-key-3",
		Balance: 100,
	}
	err := env.DB.Write(ctx).Create(customer).Error
	require.NoError(t, err)

	req := model.MessageCreateRequest{
		CustomerID: 3,
		Mobile:     "+9876543210",
		Content:    "Consumer test message",
		Priority:   "normal",
	}

	msg, err := env.MessageService.Create(ctx, req)
	require.NoError(t, err)

	received := make(chan bool, 1)
	handler := func(ctx context.Context, qMsg *queue.Message) error {
		var data map[string]interface{}
		err := json.Unmarshal(qMsg.Data, &data)
		assert.NoError(t, err)
		assert.Equal(t, float64(msg.ID), data["id"])
		assert.Equal(t, "+9876543210", data["mobile"])
		received <- true
		return nil
	}

	err = env.Queue.Consume(handler)
	require.NoError(t, err)

	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("message not consumed within timeout")
	}
}

func TestE2E_ExpressQueue(t *testing.T) {
	env := setupE2EEnvironment(t)
	defer env.Cleanup()

	ctx := context.Background()

	customer := &repository.CustomerEntity{
		ID:      4,
		APIKey:  "test-key-4",
		Balance: 50,
	}
	err := env.DB.Write(ctx).Create(customer).Error
	require.NoError(t, err)

	req := model.MessageCreateRequest{
		CustomerID: 4,
		Mobile:     "+1112223333",
		Content:    "Express message",
		Priority:   "express",
	}

	msg, err := env.MessageService.Create(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "express", msg.Priority)

	var savedMsg repository.MessageEntity
	err = env.DB.Read(ctx).Where("id = ?", msg.ID).First(&savedMsg).Error
	require.NoError(t, err)
	assert.Equal(t, "express", savedMsg.Priority)
}

func TestE2E_ConcurrentMessageCreation(t *testing.T) {
	t.Skip("Skipping concurrency test - SQLite doesn't handle concurrent writes well")

	env := setupE2EEnvironment(t)
	defer env.Cleanup()

	ctx := context.Background()

	customer := &repository.CustomerEntity{
		ID:      5,
		APIKey:  "test-key-5",
		Balance: 100,
	}
	err := env.DB.Write(ctx).Create(customer).Error
	require.NoError(t, err)

	concurrency := 10
	done := make(chan bool, concurrency)
	successCount := make(chan int, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer func() { done <- true }()

			req := model.MessageCreateRequest{
				CustomerID: 5,
				Mobile:     fmt.Sprintf("+123456%04d", id),
				Content:    fmt.Sprintf("Concurrent message %d", id),
				Priority:   "normal",
			}

			_, err := env.MessageService.Create(ctx, req)
			if err == nil {
				successCount <- 1
			}
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		<-done
	}
	close(successCount)

	total := 0
	for range successCount {
		total++
	}

	assert.LessOrEqual(t, total, 100)

	balance, err := env.CustomerRepo.GetBalance(ctx, 5)
	require.NoError(t, err)
	assert.Equal(t, uint(100-total), balance)
}

func TestE2E_TransactionRollback(t *testing.T) {
	env := setupE2EEnvironment(t)
	defer env.Cleanup()

	ctx := context.Background()

	customer := &repository.CustomerEntity{
		ID:      6,
		APIKey:  "test-key-6",
		Balance: 10,
	}
	err := env.DB.Write(ctx).Create(customer).Error
	require.NoError(t, err)

	req := model.MessageCreateRequest{
		CustomerID: 6,
		Mobile:     "",
		Content:    "Test message",
		Priority:   "normal",
	}

	_, err = env.MessageService.Create(ctx, req)
	assert.Error(t, err)

	balance, err := env.CustomerRepo.GetBalance(ctx, 6)
	require.NoError(t, err)
	assert.Equal(t, uint(10), balance)

	var count int64
	env.DB.Read(ctx).Model(&repository.MessageEntity{}).Where("customer_id = ?", 6).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestE2E_ListMessages(t *testing.T) {
	env := setupE2EEnvironment(t)
	defer env.Cleanup()

	ctx := context.Background()

	customer := &repository.CustomerEntity{
		ID:      7,
		APIKey:  "test-key-7",
		Balance: 100,
	}
	err := env.DB.Write(ctx).Create(customer).Error
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		req := model.MessageCreateRequest{
			CustomerID: 7,
			Mobile:     "+1234567890",
			Content:    fmt.Sprintf("Message %d", i),
			Priority:   "normal",
		}
		_, err := env.MessageService.Create(ctx, req)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	customerID := int64(7)
	filter := model.MessageFilter{
		CustomerID: &customerID,
		Limit:      10,
		Offset:     0,
	}

	messages, total, err := env.MessageService.List(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, messages, 5)
}

func TestE2E_DeliveryReportCreation(t *testing.T) {
	t.Skip("Skipping test that requires PostgreSQL JSON functions - not compatible with SQLite")

	env := setupE2EEnvironment(t)
	defer env.Cleanup()

	ctx := context.Background()

	customer := &repository.CustomerEntity{
		ID:      8,
		APIKey:  "test-key-8",
		Balance: 50,
	}
	err := env.DB.Write(ctx).Create(customer).Error
	require.NoError(t, err)

	req := model.MessageCreateRequest{
		CustomerID: 8,
		Mobile:     "+5551234567",
		Content:    "Delivery report test",
		Priority:   "normal",
	}

	msg, err := env.MessageService.Create(ctx, req)
	require.NoError(t, err)

	deliveredAt := time.Now()
	dr := &model.DeliveryReport{
		MessageID:   msg.ID,
		Status:      "DELIVERED",
		DeliveredAt: &deliveredAt,
	}

	createdDR, err := env.DeliveryRepo.Create(ctx, dr)
	require.NoError(t, err)
	assert.NotZero(t, createdDR.ID)
	assert.Equal(t, msg.ID, createdDR.MessageID)
	assert.Equal(t, "DELIVERED", createdDR.Status)

	customerID := int64(8)
	filter := model.MessageFilter{
		CustomerID: &customerID,
		Limit:      10,
		Offset:     0,
	}

	messagesWithReports, total, err := env.MessageService.GetMessagesWithDeliveryReports(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, messagesWithReports, 1)
	assert.Len(t, messagesWithReports[0].DeliveryReports, 1)
	assert.Equal(t, "DELIVERED", messagesWithReports[0].DeliveryReports[0].Status)
}
