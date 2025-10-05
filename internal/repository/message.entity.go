package repository

import (
	"time"

	"github.com/lib/pq"
	"github.com/nimasrn/message-gateway/internal/model"
)

type MessageEntity struct {
	ID              int64                   `db:"id"          gorm:"primaryKey;autoIncrement;column:id"`
	CustomerID      int64                   `db:"customer_id" gorm:"column:customer_id;not null;index"`
	Mobile          string                  `db:"mobile"      gorm:"column:mobile;not null"`
	Content         string                  `db:"content"     gorm:"column:content;not null"`
	Priority        string                  `db:"priority"    gorm:"column:priority;not null;default:0"`
	CreatedAt       time.Time               `db:"created_at"  gorm:"column:created_at;autoCreateTime"`
	DeliveryReports []*DeliveryReportEntity `gorm:"foreignKey:MessageID"`
}

func (MessageEntity) TableName() string {
	return "messages"
}

func toMessageEntity(m *model.Message) *MessageEntity {
	if m == nil {
		return nil
	}
	return &MessageEntity{
		ID:         m.ID,
		CustomerID: m.CustomerID,
		Mobile:     m.Mobile,
		Content:    m.Content,
		Priority:   m.Priority,
		CreatedAt:  m.CreatedAt,
	}
}

func toMessageModel(e *MessageEntity) *model.Message {
	if e == nil {
		return nil
	}
	return &model.Message{
		ID:         e.ID,
		CustomerID: e.CustomerID,
		Mobile:     e.Mobile,
		Content:    e.Content,
		Priority:   e.Priority,
		CreatedAt:  e.CreatedAt,
	}
}

func toMessageModels(entities []*MessageEntity) []*model.Message {
	if entities == nil {
		return nil
	}
	models := make([]*model.Message, len(entities))
	for i, e := range entities {
		models[i] = toMessageModel(e)
	}
	return models
}

type MessageWithReports struct {
	ID                int64          `gorm:"column:id"`
	CustomerID        int64          `gorm:"column:customer_id"`
	Mobile            string         `gorm:"column:mobile"`
	Content           string         `gorm:"column:content"`
	Priority          string         `gorm:"column:priority"`
	CreatedAt         time.Time      `gorm:"column:created_at"`
	DeliveryReportIDs pq.Int64Array  `gorm:"column:delivery_report_ids;type:bigint[]"`
	DeliveryStatuses  pq.StringArray `gorm:"column:delivery_statuses;type:text[]"`
	DeliveredAts      pq.StringArray `gorm:"column:delivered_ats;type:text[]"`
}
