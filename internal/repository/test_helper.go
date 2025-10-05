package repository

import (
	"reflect"
	"testing"

	"github.com/nimasrn/message-gateway/pkg/pg"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testDB struct {
	*pg.DB
	rawDB *gorm.DB
}

func setupTestDB(t *testing.T) *testDB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&CustomerEntity{}, &MessageEntity{}, &TransactionEntity{}, &DeliveryReportEntity{})
	require.NoError(t, err)

	pgDB := &pg.DB{}
	pgDBValue := reflect.ValueOf(pgDB).Elem()

	readField := pgDBValue.FieldByName("read")
	writeField := pgDBValue.FieldByName("write")

	readField = reflect.NewAt(readField.Type(), readField.Addr().UnsafePointer()).Elem()
	writeField = reflect.NewAt(writeField.Type(), writeField.Addr().UnsafePointer()).Elem()

	readField.Set(reflect.ValueOf(db))
	writeField.Set(reflect.ValueOf(db))

	return &testDB{
		DB:    pgDB,
		rawDB: db,
	}
}
