package model

import "time"

type MessageWithDeliveryReports struct {
	ID              int64             `json:"id"`
	CustomerID      int64             `json:"customer_id"`
	Mobile          string            `json:"mobile"`
	Content         string            `json:"content"`
	Priority        string            `json:"priority"`
	CreatedAt       time.Time         `json:"created_at"`
	DeliveryReports []*DeliveryReport `json:"delivery_reports"`
}
