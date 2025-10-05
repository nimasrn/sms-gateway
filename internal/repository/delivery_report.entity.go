package repository

import (
	"time"

	"github.com/nimasrn/message-gateway/internal/model"
)

type DeliveryReportEntity struct {
	ID          int64      `db:"id"           gorm:"primaryKey;autoIncrement;column:id"`
	MessageID   int64      `db:"message_id"   gorm:"column:message_id;not null;index"`
	Status      string     `db:"status"       gorm:"column:status;not null;index"`
	DeliveredAt *time.Time `db:"delivered_at" gorm:"column:delivered_at"`
}

func (DeliveryReportEntity) TableName() string {
	return "delivery_reports"
}

func toDeliveryReportEntity(m *model.DeliveryReport) *DeliveryReportEntity {
	if m == nil {
		return nil
	}
	return &DeliveryReportEntity{
		ID:          m.ID,
		MessageID:   m.MessageID,
		Status:      m.Status,
		DeliveredAt: m.DeliveredAt,
	}
}

func toDeliveryReportModel(e *DeliveryReportEntity) *model.DeliveryReport {
	if e == nil {
		return nil
	}
	return &model.DeliveryReport{
		ID:          e.ID,
		MessageID:   e.MessageID,
		Status:      e.Status,
		DeliveredAt: e.DeliveredAt,
	}
}

func toDeliveryReportModels(entities []*DeliveryReportEntity) []*model.DeliveryReport {
	if entities == nil {
		return nil
	}
	models := make([]*model.DeliveryReport, len(entities))
	for i, e := range entities {
		models[i] = toDeliveryReportModel(e)
	}
	return models
}
