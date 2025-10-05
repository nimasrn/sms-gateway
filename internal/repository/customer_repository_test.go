package repository

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerRepository_DeductBalance(t *testing.T) {
	db := setupTestDB(t).DB
	repo := NewCustomerRepository(db)
	ctx := context.Background()

	t.Run("successful deduction", func(t *testing.T) {
		customer := &CustomerEntity{
			ID:      1,
			APIKey:  "test-key-1",
			Balance: 1000,
		}
		err := db.Write(ctx).Create(customer).Error
		require.NoError(t, err)

		err = repo.DeductBalance(ctx, 1, 300)
		assert.NoError(t, err)

		balance, err := repo.GetBalance(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, uint(700), balance)
	})

	t.Run("insufficient balance", func(t *testing.T) {
		customer := &CustomerEntity{
			ID:      2,
			APIKey:  "test-key-2",
			Balance: 100,
		}
		err := db.Write(ctx).Create(customer).Error
		require.NoError(t, err)

		err = repo.DeductBalance(ctx, 2, 200)
		assert.ErrorIs(t, err, ErrInsufficientBalance)

		balance, err := repo.GetBalance(ctx, 2)
		require.NoError(t, err)
		assert.Equal(t, uint(100), balance)
	})

	t.Run("customer not found", func(t *testing.T) {
		err := repo.DeductBalance(ctx, 999, 100)
		assert.ErrorIs(t, err, ErrCustomerNotFound)
	})

	t.Run("zero amount deduction", func(t *testing.T) {
		customer := &CustomerEntity{
			ID:      3,
			APIKey:  "test-key-3",
			Balance: 500,
		}
		err := db.Write(ctx).Create(customer).Error
		require.NoError(t, err)

		err = repo.DeductBalance(ctx, 3, 0)
		assert.NoError(t, err)

		balance, err := repo.GetBalance(ctx, 3)
		require.NoError(t, err)
		assert.Equal(t, uint(500), balance)
	})

	t.Run("exact balance deduction", func(t *testing.T) {
		customer := &CustomerEntity{
			ID:      4,
			APIKey:  "test-key-4",
			Balance: 250,
		}
		err := db.Write(ctx).Create(customer).Error
		require.NoError(t, err)

		err = repo.DeductBalance(ctx, 4, 250)
		assert.NoError(t, err)

		balance, err := repo.GetBalance(ctx, 4)
		require.NoError(t, err)
		assert.Equal(t, uint(0), balance)
	})
}

func TestCustomerRepository_AddBalance(t *testing.T) {
	db := setupTestDB(t).DB
	repo := NewCustomerRepository(db)
	ctx := context.Background()

	t.Run("successful addition", func(t *testing.T) {
		customer := &CustomerEntity{
			ID:      1,
			APIKey:  "test-key-1",
			Balance: 500,
		}
		err := db.Write(ctx).Create(customer).Error
		require.NoError(t, err)

		err = repo.AddBalance(ctx, 1, 250)
		assert.NoError(t, err)

		balance, err := repo.GetBalance(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, uint(750), balance)
	})

	t.Run("customer not found", func(t *testing.T) {
		err := repo.AddBalance(ctx, 999, 100)
		assert.ErrorIs(t, err, ErrCustomerNotFound)
	})

	t.Run("zero amount addition", func(t *testing.T) {
		customer := &CustomerEntity{
			ID:      2,
			APIKey:  "test-key-2",
			Balance: 300,
		}
		err := db.Write(ctx).Create(customer).Error
		require.NoError(t, err)

		err = repo.AddBalance(ctx, 2, 0)
		assert.NoError(t, err)

		balance, err := repo.GetBalance(ctx, 2)
		require.NoError(t, err)
		assert.Equal(t, uint(300), balance)
	})

	t.Run("multiple additions", func(t *testing.T) {
		customer := &CustomerEntity{
			ID:      3,
			APIKey:  "test-key-3",
			Balance: 100,
		}
		err := db.Write(ctx).Create(customer).Error
		require.NoError(t, err)

		err = repo.AddBalance(ctx, 3, 50)
		assert.NoError(t, err)

		err = repo.AddBalance(ctx, 3, 75)
		assert.NoError(t, err)

		balance, err := repo.GetBalance(ctx, 3)
		require.NoError(t, err)
		assert.Equal(t, uint(225), balance)
	})
}

func TestCustomerRepository_GetBalance(t *testing.T) {
	db := setupTestDB(t).DB
	repo := NewCustomerRepository(db)
	ctx := context.Background()

	t.Run("get existing balance", func(t *testing.T) {
		customer := &CustomerEntity{
			ID:      1,
			APIKey:  "test-key-1",
			Balance: 1500,
		}
		err := db.Write(ctx).Create(customer).Error
		require.NoError(t, err)

		balance, err := repo.GetBalance(ctx, 1)
		assert.NoError(t, err)
		assert.Equal(t, uint(1500), balance)
	})

	t.Run("customer not found", func(t *testing.T) {
		balance, err := repo.GetBalance(ctx, 999)
		assert.ErrorIs(t, err, ErrCustomerNotFound)
		assert.Equal(t, uint(0), balance)
	})
}

func TestCustomerRepository_ConcurrentDeductions(t *testing.T) {
	t.Skip("Skipping concurrent test - SQLite doesn't handle concurrent writes reliably in tests. Use PostgreSQL for concurrent testing.")

	db := setupTestDB(t).DB
	repo := NewCustomerRepository(db)
	ctx := context.Background()

	customer := &CustomerEntity{
		ID:      1,
		APIKey:  "test-key-1",
		Balance: 1000,
	}
	err := db.Write(ctx).Create(customer).Error
	require.NoError(t, err)

	concurrency := 10
	amountPerDeduction := uint(50)
	var wg sync.WaitGroup
	successCount := int32(0)
	errors := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repo.DeductBalance(ctx, 1, amountPerDeduction)
			if err == nil {
				successCount++
			} else if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// With SQLite, some retries may fail, which is expected
	// The important thing is that the final balance is consistent
	balance, err := repo.GetBalance(ctx, 1)
	require.NoError(t, err)

	// Balance should be initial - (successCount * amount)
	expectedBalance := 1000 - (uint(successCount) * amountPerDeduction)
	assert.Equal(t, expectedBalance, balance)
}

func TestCustomerRepository_ConcurrentAdditions(t *testing.T) {
	t.Skip("Skipping concurrency test - SQLite doesn't handle concurrent writes well")

	db := setupTestDB(t).DB
	repo := NewCustomerRepository(db)
	ctx := context.Background()

	customer := &CustomerEntity{
		ID:      1,
		APIKey:  "test-key-1",
		Balance: 0,
	}
	err := db.Write(ctx).Create(customer).Error
	require.NoError(t, err)

	concurrency := 10
	amountPerAddition := uint(100)
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repo.AddBalance(ctx, 1, amountPerAddition)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		assert.NoError(t, err)
	}

	balance, err := repo.GetBalance(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, uint(1000), balance)
}

func TestCustomerRepository_ContextCancellation(t *testing.T) {
	db := setupTestDB(t).DB
	repo := NewCustomerRepository(db)
	ctx := context.Background()

	customer := &CustomerEntity{
		ID:      1,
		APIKey:  "test-key-1",
		Balance: 1000,
	}
	err := db.Write(ctx).Create(customer).Error
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(ctx)
	cancel()

	err = repo.DeductBalance(ctx, 1, 100)
	assert.Error(t, err)
}

func TestCustomerRepository_MixedOperations(t *testing.T) {
	db := setupTestDB(t).DB
	repo := NewCustomerRepository(db)
	ctx := context.Background()

	customer := &CustomerEntity{
		ID:      1,
		APIKey:  "test-key-1",
		Balance: 500,
	}
	err := db.Write(ctx).Create(customer).Error
	require.NoError(t, err)

	err = repo.DeductBalance(ctx, 1, 200)
	assert.NoError(t, err)

	err = repo.AddBalance(ctx, 1, 300)
	assert.NoError(t, err)

	err = repo.DeductBalance(ctx, 1, 100)
	assert.NoError(t, err)

	balance, err := repo.GetBalance(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, uint(500), balance)
}
