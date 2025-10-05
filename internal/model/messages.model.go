package model

import (
	"errors"
	"time"
)

// Status is the lifecycle state of a message.
type MessageStatus string

const (
	MessageStatusQueued    MessageStatus = "queued"
	MessageStatusSent      MessageStatus = "sent"
	MessageStatusDelivered MessageStatus = "delivered"
	MessageStatusFailed    MessageStatus = "failed"
)

type Message struct {
	ID         int64     `json:"id"          db:"id"           gorm:"primaryKey;autoIncrement;column:id"`
	CustomerID int64     `json:"customer_id" db:"customer_id"  gorm:"column:customer_id;not null;index"`
	Customer   *Customer `json:"-"                              gorm:"foreignKey:CustomerID;references:ID;constraint:OnDelete:CASCADE"`
	Mobile     string    `json:"mobile"       db:"mobile"        gorm:"column:mobile;not null"` // consider E.164 normalization
	Content    string    `json:"content"     db:"content"      gorm:"column:content;not null"`
	Priority   string    `json:"priority"    db:"priority"     gorm:"column:priority;not null;default:0"`
	CreatedAt  time.Time `json:"created_at"  db:"created_at"   gorm:"column:created_at;autoCreateTime"`
}

func (Message) TableName() string { return "messages" }

// CreateParams is the input for creating a message.
type MessageCreateRequest struct {
	CustomerID int64
	Mobile     string
	Content    string
	Priority   string
}

func (p MessageCreateRequest) Validate() error {
	if p.CustomerID == 0 {
		return errors.New("customer_id is required")
	}
	if p.Mobile == "" {
		return errors.New("mobile is required")
	}
	if p.Content == "" {
		return errors.New("content is required")
	}
	return nil
}

// MessageFilter controls List queries.
type MessageFilter struct {
	CustomerID *int64          // equals
	Statuses   []MessageStatus // IN (...)
	Mobile     *string         // equals (use normalized format)
	From       *time.Time
	To         *time.Time
	Limit      int  // default 50
	Offset     int  // for pagination
	Desc       bool // order by created_at
}
