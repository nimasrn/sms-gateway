package pg

import (
	"context"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type txContextKey string

const txKey txContextKey = "trx"

type DB struct {
	read  *gorm.DB
	write *gorm.DB
}

func Create(config Config, withDebug bool) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable", config.Host, config.User, config.Password, config.Database, config.Port)),
		&gorm.Config{
			NamingStrategy: schema.NamingStrategy{
				SingularTable: true,
			},
		})
	if err != nil {
		return nil, err
	}

	if withDebug {
		db = db.Debug()
	}
	return db, nil
}

func CreateReadWrite(readConfig Config, writeConfig Config, withDebug bool) (*DB, error) {
	read, err := Create(readConfig, withDebug)
	if err != nil {
		return nil, err
	}
	write, err := Create(writeConfig, withDebug)
	if err != nil {
		return nil, err
	}
	return &DB{read, write}, nil
}

func (r *DB) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return r.write.WithContext(ctx).Debug().Transaction(func(tx *gorm.DB) error {
		ctx = context.WithValue(ctx, txKey, tx)
		return fn(ctx)
	})
}

func (r *DB) Write(ctx context.Context) *gorm.DB {
	tx, ok := ctx.Value(txKey).(*gorm.DB)
	if ok {
		return tx
	}

	tx = r.write.WithContext(ctx)

	return tx
}

func (r *DB) Read(ctx context.Context) *gorm.DB {
	tx, ok := ctx.Value(txKey).(*gorm.DB)
	if ok {
		return tx
	}

	tx = r.read.WithContext(ctx)

	return tx
}
