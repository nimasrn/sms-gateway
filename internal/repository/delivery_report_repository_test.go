package repository

import (
	"context"
	"testing"
	"time"

	"github.com/nimasrn/message-gateway/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliveryReportRepository_Create(t *testing.T) {
	db := setupTestDB(t).DB
	repo := NewDeliveryReportRepository(db)
	ctx := context.Background()

	t.Run("create delivery report with delivered status", func(t *testing.T) {
		deliveredAt := time.Now()
		dr := &model.DeliveryReport{
			MessageID:   1,
			Status:      "DELIVERED",
			DeliveredAt: &deliveredAt,
		}

		created, err := repo.Create(ctx, dr)
		require.NoError(t, err)
		assert.NotZero(t, created.ID)
		assert.Equal(t, dr.MessageID, created.MessageID)
		assert.Equal(t, dr.Status, created.Status)
		assert.NotNil(t, created.DeliveredAt)
		assert.True(t, created.DeliveredAt.Equal(deliveredAt))
	})

	t.Run("create delivery report with pending status", func(t *testing.T) {
		dr := &model.DeliveryReport{
			MessageID:   2,
			Status:      "PENDING",
			DeliveredAt: nil,
		}

		created, err := repo.Create(ctx, dr)
		require.NoError(t, err)
		assert.NotZero(t, created.ID)
		assert.Equal(t, "PENDING", created.Status)
		assert.Nil(t, created.DeliveredAt)
	})

	t.Run("create delivery report with failed status", func(t *testing.T) {
		dr := &model.DeliveryReport{
			MessageID:   3,
			Status:      "FAILED",
			DeliveredAt: nil,
		}

		created, err := repo.Create(ctx, dr)
		require.NoError(t, err)
		assert.NotZero(t, created.ID)
		assert.Equal(t, "FAILED", created.Status)
		assert.Nil(t, created.DeliveredAt)
	})

	t.Run("create multiple delivery reports for same message", func(t *testing.T) {
		messageID := int64(100)

		dr1 := &model.DeliveryReport{
			MessageID:   messageID,
			Status:      "PENDING",
			DeliveredAt: nil,
		}
		created1, err := repo.Create(ctx, dr1)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		deliveredAt := time.Now()
		dr2 := &model.DeliveryReport{
			MessageID:   messageID,
			Status:      "DELIVERED",
			DeliveredAt: &deliveredAt,
		}
		created2, err := repo.Create(ctx, dr2)
		require.NoError(t, err)

		assert.NotEqual(t, created1.ID, created2.ID)
		assert.Equal(t, messageID, created1.MessageID)
		assert.Equal(t, messageID, created2.MessageID)
	})
}
