package repository

import (
	"context"

	"github.com/nimasrn/message-gateway/internal/model"
	"github.com/nimasrn/message-gateway/pkg/pg"
)

type TransactionRepository struct {
	*pg.DB
}

func NewTransactionRepository(db *pg.DB) *TransactionRepository {
	return &TransactionRepository{
		db,
	}
}

func (r *TransactionRepository) Create(ctx context.Context, txn *model.Transaction) (*model.Transaction, error) {
	entity := toTransactionEntity(txn)

	if err := r.Write(ctx).WithContext(ctx).Create(entity).Error; err != nil {
		return nil, err
	}

	return toTransactionModel(entity), nil
}

type TransactionFilter struct {
	CustomerID *int64
	Type       *string // "debit" or "credit"
	MessageID  *int64
	Limit      int
	Offset     int
}
