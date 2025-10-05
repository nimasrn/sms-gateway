package helpers

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/nimasrn/message-gateway/internal/repository"
	"github.com/nimasrn/message-gateway/pkg/pg"
	"github.com/nimasrn/message-gateway/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func SetupTestDB(t *testing.T) *pg.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&repository.CustomerEntity{},
		&repository.MessageEntity{},
		&repository.TransactionEntity{},
		&repository.DeliveryReportEntity{},
	)
	require.NoError(t, err)

	pgDB := &pg.DB{}
	pgDBValue := reflect.ValueOf(pgDB).Elem()

	readField := pgDBValue.FieldByName("read")
	writeField := pgDBValue.FieldByName("write")

	readField = reflect.NewAt(readField.Type(), readField.Addr().UnsafePointer()).Elem()
	writeField = reflect.NewAt(writeField.Type(), writeField.Addr().UnsafePointer()).Elem()

	readField.Set(reflect.ValueOf(db))
	writeField.Set(reflect.ValueOf(db))

	return pgDB
}

func SetupTestRedis(t *testing.T) (*miniredis.Miniredis, redis.RedisAdapter) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	adapter, err := redis.NewRedisAdapter("test", "", &goredis.UniversalOptions{
		Addrs: []string{mr.Addr()},
	})
	require.NoError(t, err)

	return mr, adapter
}

func CreateTestCustomer(t *testing.T, db *pg.DB, id int64, balance uint) *repository.CustomerEntity {
	ctx := context.Background()
	customer := &repository.CustomerEntity{
		ID:      id,
		APIKey:  RandomAPIKey(),
		Balance: balance,
	}
	err := db.Write(ctx).Create(customer).Error
	require.NoError(t, err)
	return customer
}

func CreateTestMessage(t *testing.T, db *pg.DB, customerID int64, mobile, content string) *repository.MessageEntity {
	ctx := context.Background()
	msg := &repository.MessageEntity{
		CustomerID: customerID,
		Mobile:     mobile,
		Content:    content,
		Priority:   "normal",
		CreatedAt:  time.Now(),
	}
	err := db.Write(ctx).Create(msg).Error
	require.NoError(t, err)
	return msg
}

func CreateTestTransaction(t *testing.T, db *pg.DB, customerID int64, amount uint, txnType string, messageID *int64) *repository.TransactionEntity {
	ctx := context.Background()
	txn := &repository.TransactionEntity{
		CustomerID: customerID,
		Amount:     amount,
		Type:       txnType,
		MessageID:  messageID,
	}
	err := db.Write(ctx).Create(txn).Error
	require.NoError(t, err)
	return txn
}

func CreateTestDeliveryReport(t *testing.T, db *pg.DB, messageID int64, status string) *repository.DeliveryReportEntity {
	ctx := context.Background()
	deliveredAt := time.Now()
	dr := &repository.DeliveryReportEntity{
		MessageID:   messageID,
		Status:      status,
		DeliveredAt: &deliveredAt,
	}
	err := db.Write(ctx).Create(dr).Error
	require.NoError(t, err)
	return dr
}

func WaitForCondition(t *testing.T, timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func AssertEventually(t *testing.T, timeout time.Duration, condition func() bool, msg string) {
	if !WaitForCondition(t, timeout, condition) {
		t.Fatal(msg)
	}
}

func ContextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

func RandomAPIKey() string {
	return "test-api-key-" + time.Now().Format("20060102150405")
}

func Ptr[T any](v T) *T {
	return &v
}
