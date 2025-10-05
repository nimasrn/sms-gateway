package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nimasrn/message-gateway/pkg/pg"
	"gorm.io/gorm"
)

var (
	ErrCustomerNotFound    = errors.New("customer not found")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrInactiveAccount     = errors.New("account is not active")
	ErrConcurrentUpdate    = errors.New("concurrent update detected")
	ErrMaxRetriesExceeded  = errors.New("max retries exceeded")
	ErrDuplicateAPIKey     = errors.New("api key already exists")
)

type CustomerRepository struct {
	*pg.DB
}

func NewCustomerRepository(db *pg.DB) *CustomerRepository {
	return &CustomerRepository{
		db,
	}
}

// DeductBalance performs atomic balance deduction with automatic retry.
// This is used for charges/debits (e.g., sending an SMS).
func (r *CustomerRepository) DeductBalance(ctx context.Context, customerID int64, amount uint) error {
	const maxRetries = 3
	const baseDelay = 2 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := r.deductBalanceAttempt(ctx, customerID, amount)

		if err == nil {
			return nil // Success!
		}

		// Don't retry on permanent errors
		if errors.Is(err, ErrCustomerNotFound) ||
			errors.Is(err, ErrInsufficientBalance) ||
			errors.Is(err, ErrInactiveAccount) {
			return err
		}

		// Retry on transient errors
		if attempt < maxRetries {
			delay := baseDelay * time.Duration(1<<attempt) // Exponential backoff: 2ms, 4ms, 8ms
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}
	}

	return fmt.Errorf("%w: failed after %d attempts", ErrMaxRetriesExceeded, maxRetries+1)
}

// deductBalanceAttempt performs a single deduction attempt.
func (r *CustomerRepository) deductBalanceAttempt(ctx context.Context, customerID int64, amount uint) error {
	// Atomic UPDATE with all conditions in WHERE clause
	result := r.Write(ctx).WithContext(ctx).
		Model(&CustomerEntity{}).
		Where("id = ? AND balance >= ?",
			customerID,
			amount,
		).
		Update("balance", gorm.Expr("balance - ?", amount))

	if result.Error != nil {
		return result.Error
	}

	// Update succeeded
	if result.RowsAffected > 0 {
		return nil
	}

	// No rows updated - determine why
	return r.checkDeductionFailureReason(ctx, customerID, amount)
}

// checkDeductionFailureReason determines why the deduction failed.
func (r *CustomerRepository) checkDeductionFailureReason(ctx context.Context, customerID int64, amount uint) error {
	var entity CustomerEntity
	err := r.Read(ctx).WithContext(ctx).
		Where("id = ?", customerID).
		First(&entity).
		Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCustomerNotFound
		}
		return err
	}

	if entity.Balance < amount {
		return ErrInsufficientBalance
	}

	// Balance was sufficient but update failed - likely concurrent modification
	return ErrConcurrentUpdate
}

// AddBalance performs atomic balance addition with automatic retry.
// This is used for credits/deposits (e.g., customer top-up).
func (r *CustomerRepository) AddBalance(ctx context.Context, customerID int64, amount uint) error {
	const maxRetries = 3
	const baseDelay = 2 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		result := r.Write(ctx).WithContext(ctx).
			Model(&CustomerEntity{}).
			Where("id = ?", customerID).
			Update("balance", gorm.Expr("balance + ?", amount))

		if result.Error != nil {
			if attempt < maxRetries {
				delay := baseDelay * time.Duration(1<<attempt)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
					continue
				}
			}
			return result.Error
		}

		if result.RowsAffected == 0 {
			return ErrCustomerNotFound
		}

		return nil
	}

	return ErrMaxRetriesExceeded
}

func (r *CustomerRepository) GetBalance(ctx context.Context, customerID int64) (uint, error) {
	var entity CustomerEntity
	err := r.Read(ctx).WithContext(ctx).
		Select("balance").
		Where("id = ?", customerID).
		First(&entity).
		Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrCustomerNotFound
		}
		return 0, err
	}

	return entity.Balance, nil
}
