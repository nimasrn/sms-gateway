package services

import (
	"context"
	"testing"

	"github.com/nimasrn/message-gateway/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockMessageRepository struct {
	mock.Mock
}

func (m *MockMessageRepository) Create(ctx context.Context, msg *model.Message) (*model.Message, error) {
	args := m.Called(ctx, msg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Message), args.Error(1)
}

func (m *MockMessageRepository) List(ctx context.Context, f model.MessageFilter) ([]*model.Message, int64, error) {
	args := m.Called(ctx, f)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*model.Message), args.Get(1).(int64), args.Error(2)
}

func (m *MockMessageRepository) GetMessagesWithDeliveryReports(ctx context.Context, f model.MessageFilter) ([]*model.MessageWithDeliveryReports, int64, error) {
	args := m.Called(ctx, f)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*model.MessageWithDeliveryReports), args.Get(1).(int64), args.Error(2)
}

type MockCustomerRepository struct {
	mock.Mock
}

func (m *MockCustomerRepository) DeductBalance(ctx context.Context, id int64, amount uint) error {
	args := m.Called(ctx, id, amount)
	return args.Error(0)
}

func (m *MockCustomerRepository) AddBalance(ctx context.Context, id int64, amount uint) error {
	args := m.Called(ctx, id, amount)
	return args.Error(0)
}

func (m *MockCustomerRepository) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	args := m.Called(ctx, fn)
	if args.Error(0) != nil {
		return args.Error(0)
	}
	return fn(ctx)
}

type MockTransactionRepository struct {
	mock.Mock
}

func (m *MockTransactionRepository) Create(ctx context.Context, txn *model.Transaction) (*model.Transaction, error) {
	args := m.Called(ctx, txn)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Transaction), args.Error(1)
}

func TestMessageService_Create_InsufficientBalance(t *testing.T) {
	msgRepo := new(MockMessageRepository)
	custRepo := new(MockCustomerRepository)
	txnRepo := new(MockTransactionRepository)
	ctx := context.Background()

	service := NewMessageService(msgRepo, custRepo, txnRepo, nil, nil)

	req := model.MessageCreateRequest{
		CustomerID: 1,
		Mobile:     "+1234567890",
		Content:    "Test message",
		Priority:   "normal",
	}

	custRepo.On("WithinTransaction", ctx, mock.AnythingOfType("func(context.Context) error")).
		Return(ErrInsufficientBalance)

	result, err := service.Create(ctx, req)
	assert.ErrorIs(t, err, ErrInsufficientBalance)
	assert.Nil(t, result)

	custRepo.AssertExpectations(t)
}

func TestMessageService_Create_EmptyContent(t *testing.T) {
	msgRepo := new(MockMessageRepository)
	custRepo := new(MockCustomerRepository)
	txnRepo := new(MockTransactionRepository)

	service := NewMessageService(msgRepo, custRepo, txnRepo, nil, nil)

	req := model.MessageCreateRequest{
		CustomerID: 1,
		Mobile:     "+1234567890",
		Content:    "   ",
		Priority:   "normal",
	}

	ctx := context.Background()
	result, err := service.Create(ctx, req)
	assert.ErrorIs(t, err, ErrEmptyContent)
	assert.Nil(t, result)
}

func TestMessageService_Create_InvalidMobile(t *testing.T) {
	msgRepo := new(MockMessageRepository)
	custRepo := new(MockCustomerRepository)
	txnRepo := new(MockTransactionRepository)

	service := NewMessageService(msgRepo, custRepo, txnRepo, nil, nil)

	req := model.MessageCreateRequest{
		CustomerID: 1,
		Mobile:     "",
		Content:    "Test message",
		Priority:   "normal",
	}

	ctx := context.Background()
	result, err := service.Create(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestMessageService_List(t *testing.T) {
	ctx := context.Background()

	t.Run("successful list", func(t *testing.T) {
		msgRepo := new(MockMessageRepository)
		custRepo := new(MockCustomerRepository)
		txnRepo := new(MockTransactionRepository)

		service := NewMessageService(msgRepo, custRepo, txnRepo, nil, nil)

		customerID := int64(1)
		filter := model.MessageFilter{
			CustomerID: &customerID,
			Limit:      10,
			Offset:     0,
		}

		expectedMessages := []*model.Message{
			{ID: 1, CustomerID: 1, Mobile: "+1234567890", Content: "Test 1"},
			{ID: 2, CustomerID: 1, Mobile: "+1234567890", Content: "Test 2"},
		}

		msgRepo.On("List", ctx, filter).Return(expectedMessages, int64(2), nil)

		messages, total, err := service.List(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
		assert.Len(t, messages, 2)

		msgRepo.AssertExpectations(t)
	})

	t.Run("list with no results", func(t *testing.T) {
		msgRepo := new(MockMessageRepository)
		custRepo := new(MockCustomerRepository)
		txnRepo := new(MockTransactionRepository)

		service := NewMessageService(msgRepo, custRepo, txnRepo, nil, nil)

		customerID := int64(999)
		filter := model.MessageFilter{
			CustomerID: &customerID,
			Limit:      10,
			Offset:     0,
		}

		msgRepo.On("List", ctx, filter).Return([]*model.Message{}, int64(0), nil)

		messages, total, err := service.List(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Len(t, messages, 0)

		msgRepo.AssertExpectations(t)
	})
}

func TestMessageService_GetMessagesWithDeliveryReports(t *testing.T) {
	ctx := context.Background()

	t.Run("successful retrieval with delivery reports", func(t *testing.T) {
		msgRepo := new(MockMessageRepository)
		custRepo := new(MockCustomerRepository)
		txnRepo := new(MockTransactionRepository)

		service := NewMessageService(msgRepo, custRepo, txnRepo, nil, nil)

		customerID := int64(1)
		filter := model.MessageFilter{
			CustomerID: &customerID,
			Limit:      10,
			Offset:     0,
		}

		expectedMessages := []*model.MessageWithDeliveryReports{
			{
				ID:         1,
				CustomerID: 1,
				Mobile:     "+1234567890",
				Content:    "Test",
				DeliveryReports: []*model.DeliveryReport{
					{ID: 1, MessageID: 1, Status: "DELIVERED"},
				},
			},
		}

		msgRepo.On("GetMessagesWithDeliveryReports", ctx, filter).Return(expectedMessages, int64(1), nil)

		messages, total, err := service.GetMessagesWithDeliveryReports(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Len(t, messages, 1)
		assert.Len(t, messages[0].DeliveryReports, 1)

		msgRepo.AssertExpectations(t)
	})
}
