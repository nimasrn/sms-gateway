package model

import "time"

type DeliveryReport struct {
	ID          int64      `json:"id"           db:"id"            gorm:"primaryKey;autoIncrement;column:id"`
	MessageID   int64      `json:"message_id"   db:"message_id"    gorm:"column:message_id;not null;index"`
	SMS         *Message   `json:"-"                                 gorm:"foreignKey:MessageID;references:ID;constraint:OnDelete:CASCADE"`
	Status      string     `json:"status"       db:"status"        gorm:"column:status;not null;index"`
	DeliveredAt *time.Time `json:"delivered_at" db:"delivered_at"  gorm:"column:delivered_at"` // nullable
}

func (DeliveryReport) TableName() string { return "delivery_reports" }
