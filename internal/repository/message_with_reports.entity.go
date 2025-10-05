package repository

import (
	"encoding/json"
	"time"

	"github.com/nimasrn/message-gateway/internal/model"
)

type MessageWithReportsEntity struct {
	ID              int64           `gorm:"column:id"`
	CustomerID      int64           `gorm:"column:customer_id"`
	Mobile          string          `gorm:"column:mobile"`
	Content         string          `gorm:"column:content"`
	Priority        string          `gorm:"column:priority"`
	CreatedAt       time.Time       `gorm:"column:created_at"`
	DeliveryReports json.RawMessage `gorm:"column:delivery_reports;type:json"`
}

func toMessageWithReportsModel(e *MessageWithReportsEntity) *model.MessageWithDeliveryReports {
	if e == nil {
		return nil
	}

	msg := &model.MessageWithDeliveryReports{
		ID:         e.ID,
		CustomerID: e.CustomerID,
		Mobile:     e.Mobile,
		Content:    e.Content,
		Priority:   e.Priority,
		CreatedAt:  e.CreatedAt,
	}

	// Parse delivery reports JSON
	var reports []*model.DeliveryReport
	if len(e.DeliveryReports) > 0 && string(e.DeliveryReports) != "[]" {
		if err := json.Unmarshal(e.DeliveryReports, &reports); err == nil {
			msg.DeliveryReports = reports
		} else {
			msg.DeliveryReports = []*model.DeliveryReport{}
		}
	} else {
		msg.DeliveryReports = []*model.DeliveryReport{}
	}

	return msg
}

func toMessageWithReportsModels(entities []*MessageWithReportsEntity) []*model.MessageWithDeliveryReports {
	if entities == nil {
		return nil
	}
	models := make([]*model.MessageWithDeliveryReports, len(entities))
	for i, e := range entities {
		models[i] = toMessageWithReportsModel(e)
	}
	return models
}
