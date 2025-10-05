package fixtures

import (
	"time"

	"github.com/nimasrn/message-gateway/internal/model"
)

var (
	TestCustomer1 = model.Customer{
		ID:        1,
		APIKey:    "test-api-key-1",
		Balance:   1000,
		RateLimit: 100,
	}

	TestCustomer2 = model.Customer{
		ID:        2,
		APIKey:    "test-api-key-2",
		Balance:   500,
		RateLimit: 50,
	}

	TestCustomerLowBalance = model.Customer{
		ID:        3,
		APIKey:    "test-api-key-3",
		Balance:   1,
		RateLimit: 10,
	}

	TestCustomerZeroBalance = model.Customer{
		ID:        4,
		APIKey:    "test-api-key-4",
		Balance:   0,
		RateLimit: 10,
	}
)

func NewTestMessage(customerID int64, mobile, content, priority string) *model.Message {
	return &model.Message{
		ID:         0,
		CustomerID: customerID,
		Mobile:     mobile,
		Content:    content,
		Priority:   priority,
		CreatedAt:  time.Now(),
	}
}

func NewTestMessageCreateRequest(customerID int64, mobile, content, priority string) model.MessageCreateRequest {
	return model.MessageCreateRequest{
		CustomerID: customerID,
		Mobile:     mobile,
		Content:    content,
		Priority:   priority,
	}
}

func NewTestTransaction(customerID int64, amount uint, txnType string, messageID *int64) *model.Transaction {
	return &model.Transaction{
		ID:         0,
		CustomerID: customerID,
		Amount:     amount,
		Type:       txnType,
		MessageID:  messageID,
	}
}

func NewTestDeliveryReport(messageID int64, status string) *model.DeliveryReport {
	now := time.Now()
	return &model.DeliveryReport{
		ID:          0,
		MessageID:   messageID,
		Status:      status,
		DeliveredAt: &now,
	}
}

var (
	ValidMobileNumbers = []string{
		"+1234567890",
		"+9876543210",
		"+4412345678",
		"+33123456789",
		"+81312345678",
	}

	InvalidMobileNumbers = []string{
		"",
		"123",
		"invalid",
		"+",
		"abc123",
	}

	ValidMessageContents = []string{
		"Hello World",
		"Test message",
		"Short",
		"This is a longer message with more content for testing purposes",
	}

	InvalidMessageContents = []string{
		"",
		"   ",
		"\n\t",
	}
)

func MessageWithID(id int64) *model.Message {
	msg := NewTestMessage(1, "+1234567890", "Test", "normal")
	msg.ID = id
	return msg
}

func MessageCreateRequestNormal() model.MessageCreateRequest {
	return NewTestMessageCreateRequest(1, "+1234567890", "Normal priority message", "normal")
}

func MessageCreateRequestExpress() model.MessageCreateRequest {
	return NewTestMessageCreateRequest(1, "+1234567890", "Express priority message", "express")
}

func MessageCreateRequestInvalidMobile() model.MessageCreateRequest {
	return NewTestMessageCreateRequest(1, "", "Test message", "normal")
}

func MessageCreateRequestEmptyContent() model.MessageCreateRequest {
	return NewTestMessageCreateRequest(1, "+1234567890", "", "normal")
}

func MessageFilterByCustomer(customerID int64) model.MessageFilter {
	return model.MessageFilter{
		CustomerID: &customerID,
		Limit:      50,
		Offset:     0,
		Desc:       false,
	}
}

func MessageFilterWithPagination(customerID int64, limit, offset int) model.MessageFilter {
	return model.MessageFilter{
		CustomerID: &customerID,
		Limit:      limit,
		Offset:     offset,
		Desc:       false,
	}
}

func MessageFilterByMobile(customerID int64, mobile string) model.MessageFilter {
	return model.MessageFilter{
		CustomerID: &customerID,
		Mobile:     &mobile,
		Limit:      50,
		Offset:     0,
		Desc:       false,
	}
}

func MessageFilterByTimeRange(customerID int64, from, to time.Time) model.MessageFilter {
	return model.MessageFilter{
		CustomerID: &customerID,
		From:       &from,
		To:         &to,
		Limit:      50,
		Offset:     0,
		Desc:       false,
	}
}
