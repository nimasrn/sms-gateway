package repository

import (
	"context"
	"testing"

	"github.com/nimasrn/message-gateway/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionRepository_Create(t *testing.T) {
	db := setupTestDB(t).DB
	repo := NewTransactionRepository(db)
	ctx := context.Background()

	t.Run("create debit transaction", func(t *testing.T) {
		txn := &model.Transaction{
			CustomerID: 1,
			Type:       "debit",
			Amount:     100,
			MessageID:  ptr(int64(1)),
		}

		created, err := repo.Create(ctx, txn)
		require.NoError(t, err)
		assert.NotZero(t, created.ID)
		assert.Equal(t, txn.CustomerID, created.CustomerID)
		assert.Equal(t, txn.Type, created.Type)
		assert.Equal(t, txn.Amount, created.Amount)
		assert.Equal(t, *txn.MessageID, *created.MessageID)
	})

	t.Run("create credit transaction", func(t *testing.T) {
		txn := &model.Transaction{
			CustomerID: 2,
			Type:       "credit",
			Amount:     500,
			MessageID:  nil,
		}

		created, err := repo.Create(ctx, txn)
		require.NoError(t, err)
		assert.NotZero(t, created.ID)
		assert.Equal(t, "credit", created.Type)
		assert.Nil(t, created.MessageID)
	})

	t.Run("create multiple transactions", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			txn := &model.Transaction{
				CustomerID: int64(i + 1),
				Type:       "debit",
				Amount:     uint(50 * (i + 1)),
			}
			_, err := repo.Create(ctx, txn)
			require.NoError(t, err)
		}
	})
}

func ptr(i int64) *int64 {
	return &i
}
