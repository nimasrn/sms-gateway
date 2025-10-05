package repository

import (
	"context"
	"testing"
	"time"

	"github.com/nimasrn/message-gateway/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageRepository_Create(t *testing.T) {
	db := setupTestDB(t).DB
	repo := NewMessageRepository(db)
	ctx := context.Background()

	t.Run("create message successfully", func(t *testing.T) {
		msg := &model.Message{
			CustomerID: 1,
			Mobile:     "+1234567890",
			Content:    "Test message",
			Priority:   "normal",
		}

		created, err := repo.Create(ctx, msg)
		require.NoError(t, err)
		assert.NotZero(t, created.ID)
		assert.Equal(t, msg.CustomerID, created.CustomerID)
		assert.Equal(t, msg.Mobile, created.Mobile)
		assert.Equal(t, msg.Content, created.Content)
		assert.NotZero(t, created.CreatedAt)
	})

	t.Run("create multiple messages", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			msg := &model.Message{
				CustomerID: int64(i + 1),
				Mobile:     "+1234567890",
				Content:    "Test message",
				Priority:   "normal",
			}
			_, err := repo.Create(ctx, msg)
			require.NoError(t, err)
		}
	})
}

func TestMessageRepository_List(t *testing.T) {
	db := setupTestDB(t).DB
	repo := NewMessageRepository(db)
	ctx := context.Background()

	customerID := int64(100)
	for i := 0; i < 5; i++ {
		msg := &model.Message{
			CustomerID: customerID,
			Mobile:     "+1234567890",
			Content:    "Test message",
			Priority:   "normal",
		}
		_, err := repo.Create(ctx, msg)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	t.Run("list all messages", func(t *testing.T) {
		filter := model.MessageFilter{
			CustomerID: &customerID,
			Limit:      10,
			Offset:     0,
		}

		messages, total, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, int64(5), total)
		assert.Len(t, messages, 5)
	})

	t.Run("list with pagination", func(t *testing.T) {
		filter := model.MessageFilter{
			CustomerID: &customerID,
			Limit:      2,
			Offset:     0,
		}

		messages, total, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, int64(5), total)
		assert.Len(t, messages, 2)
	})

	t.Run("list with offset", func(t *testing.T) {
		filter := model.MessageFilter{
			CustomerID: &customerID,
			Limit:      2,
			Offset:     3,
		}

		messages, total, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, int64(5), total)
		assert.Len(t, messages, 2)
	})

	t.Run("list with mobile filter", func(t *testing.T) {
		mobile := "+1234567890"
		filter := model.MessageFilter{
			CustomerID: &customerID,
			Mobile:     &mobile,
			Limit:      10,
			Offset:     0,
		}

		messages, total, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, int64(5), total)
		assert.Len(t, messages, 5)
	})

	t.Run("list with desc order", func(t *testing.T) {
		filter := model.MessageFilter{
			CustomerID: &customerID,
			Limit:      10,
			Offset:     0,
			Desc:       true,
		}

		messages, total, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, int64(5), total)
		assert.Len(t, messages, 5)
		for i := 0; i < len(messages)-1; i++ {
			assert.True(t, messages[i].CreatedAt.After(messages[i+1].CreatedAt) || messages[i].CreatedAt.Equal(messages[i+1].CreatedAt))
		}
	})

	t.Run("list with time range", func(t *testing.T) {
		now := time.Now()
		from := now.Add(-1 * time.Hour)
		to := now.Add(1 * time.Hour)

		filter := model.MessageFilter{
			CustomerID: &customerID,
			From:       &from,
			To:         &to,
			Limit:      10,
			Offset:     0,
		}

		messages, total, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, int64(5), total)
		assert.Len(t, messages, 5)
	})

	t.Run("list with no results", func(t *testing.T) {
		nonExistentID := int64(999)
		filter := model.MessageFilter{
			CustomerID: &nonExistentID,
			Limit:      10,
			Offset:     0,
		}

		messages, total, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Len(t, messages, 0)
	})

	t.Run("list with default limit", func(t *testing.T) {
		filter := model.MessageFilter{
			CustomerID: &customerID,
			Limit:      0,
			Offset:     0,
		}

		messages, total, err := repo.List(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, int64(5), total)
		assert.Len(t, messages, 5)
	})
}

func TestMessageRepository_GetMessagesWithDeliveryReports(t *testing.T) {
	t.Skip("Skipping due to PostgreSQL-specific JSON aggregation functions not supported in SQLite")
}
