package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nimasrn/message-gateway/internal/model"
	xhttp "github.com/nimasrn/message-gateway/pkg/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

type MockMessageService struct {
	mock.Mock
}

func (m *MockMessageService) Create(ctx context.Context, p model.MessageCreateRequest) (*model.Message, error) {
	args := m.Called(ctx, p)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Message), args.Error(1)
}

func (m *MockMessageService) List(ctx context.Context, f model.MessageFilter) ([]*model.Message, int64, error) {
	args := m.Called(ctx, f)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*model.Message), args.Get(1).(int64), args.Error(2)
}

func (m *MockMessageService) GetMessagesWithDeliveryReports(ctx context.Context, f model.MessageFilter) ([]*model.MessageWithDeliveryReports, int64, error) {
	args := m.Called(ctx, f)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*model.MessageWithDeliveryReports), args.Get(1).(int64), args.Error(2)
}

func setupTestContext(method, path string, body []byte) *xhttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(method)
	ctx.Request.SetRequestURI(path)
	if body != nil {
		ctx.Request.SetBody(body)
	}
	return ctx
}

func TestMessageHandler_CreateMessage(t *testing.T) {
	t.Run("successful message creation", func(t *testing.T) {
		svc := new(MockMessageService)
		handler := NewMessageHandler(svc)

		reqBody := createMessageRequest{
			CustomerID: 1,
			Mobile:     "+1234567890",
			Content:    "Test message",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		expectedMsg := &model.Message{
			ID:         123,
			CustomerID: 1,
			Mobile:     "+1234567890",
			Content:    "Test message",
			Priority:   "normal",
		}

		svc.On("Create", mock.Anything, mock.MatchedBy(func(p model.MessageCreateRequest) bool {
			return p.CustomerID == 1 && p.Mobile == "+1234567890" && p.Priority == "normal"
		})).Return(expectedMsg, nil)

		ctx := setupTestContext("POST", "/messages", bodyBytes)
		handler.CreateMessage(ctx)

		assert.Equal(t, 201, ctx.Response.StatusCode())

		var response model.Message
		err := json.Unmarshal(ctx.Response.Body(), &response)
		require.NoError(t, err)
		assert.Equal(t, int64(123), response.ID)
		assert.Equal(t, "normal", response.Priority)

		svc.AssertExpectations(t)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		svc := new(MockMessageService)
		handler := NewMessageHandler(svc)

		ctx := setupTestContext("POST", "/messages", []byte("invalid json"))
		handler.CreateMessage(ctx)

		assert.Equal(t, 400, ctx.Response.StatusCode())

		var response map[string]string
		err := json.Unmarshal(ctx.Response.Body(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "invalid JSON")
	})

	t.Run("service error", func(t *testing.T) {
		svc := new(MockMessageService)
		handler := NewMessageHandler(svc)

		reqBody := createMessageRequest{
			CustomerID: 1,
			Mobile:     "+1234567890",
			Content:    "Test message",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		svc.On("Create", mock.Anything, mock.Anything).Return(nil, errors.New("service error"))

		ctx := setupTestContext("POST", "/messages", bodyBytes)
		handler.CreateMessage(ctx)

		assert.Equal(t, 400, ctx.Response.StatusCode())

		var response map[string]string
		err := json.Unmarshal(ctx.Response.Body(), &response)
		require.NoError(t, err)
		assert.Equal(t, "service error", response["error"])

		svc.AssertExpectations(t)
	})
}

func TestMessageHandler_CreateExpressMessage(t *testing.T) {
	t.Run("successful express message creation", func(t *testing.T) {
		svc := new(MockMessageService)
		handler := NewMessageHandler(svc)

		reqBody := createMessageRequest{
			CustomerID: 1,
			Mobile:     "+1234567890",
			Content:    "Urgent message",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		expectedMsg := &model.Message{
			ID:         456,
			CustomerID: 1,
			Mobile:     "+1234567890",
			Content:    "Urgent message",
			Priority:   "express",
		}

		svc.On("Create", mock.Anything, mock.MatchedBy(func(p model.MessageCreateRequest) bool {
			return p.CustomerID == 1 && p.Mobile == "+1234567890" && p.Priority == "express"
		})).Return(expectedMsg, nil)

		ctx := setupTestContext("POST", "/messages/express", bodyBytes)
		handler.CreateExpressMessage(ctx)

		assert.Equal(t, 201, ctx.Response.StatusCode())

		var response model.Message
		err := json.Unmarshal(ctx.Response.Body(), &response)
		require.NoError(t, err)
		assert.Equal(t, int64(456), response.ID)
		assert.Equal(t, "express", response.Priority)

		svc.AssertExpectations(t)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		svc := new(MockMessageService)
		handler := NewMessageHandler(svc)

		ctx := setupTestContext("POST", "/messages/express", []byte("invalid"))
		handler.CreateExpressMessage(ctx)

		assert.Equal(t, 400, ctx.Response.StatusCode())
	})
}

func TestMessageHandler_ListMessages(t *testing.T) {
	t.Run("successful list", func(t *testing.T) {
		svc := new(MockMessageService)
		handler := NewMessageHandler(svc)

		expectedMessages := []*model.Message{
			{ID: 1, CustomerID: 1, Mobile: "+1234567890", Content: "Test 1"},
			{ID: 2, CustomerID: 1, Mobile: "+1234567890", Content: "Test 2"},
		}

		svc.On("List", mock.Anything, mock.AnythingOfType("model.MessageFilter")).
			Return(expectedMessages, int64(2), nil)

		ctx := setupTestContext("GET", "/messages?customer_id=1&limit=10&offset=0", nil)
		handler.ListMessages(ctx)

		assert.Equal(t, 200, ctx.Response.StatusCode())

		var response listResponse
		err := json.Unmarshal(ctx.Response.Body(), &response)
		require.NoError(t, err)
		assert.Equal(t, int64(2), response.Total)
		assert.Len(t, response.Items, 2)

		svc.AssertExpectations(t)
	})

	t.Run("list with pagination", func(t *testing.T) {
		svc := new(MockMessageService)
		handler := NewMessageHandler(svc)

		svc.On("List", mock.Anything, mock.MatchedBy(func(f model.MessageFilter) bool {
			return f.Limit == 5 && f.Offset == 10
		})).Return([]*model.Message{}, int64(0), nil)

		ctx := setupTestContext("GET", "/messages?limit=5&offset=10", nil)
		handler.ListMessages(ctx)

		assert.Equal(t, 200, ctx.Response.StatusCode())
		svc.AssertExpectations(t)
	})

	t.Run("list with desc order", func(t *testing.T) {
		svc := new(MockMessageService)
		handler := NewMessageHandler(svc)

		svc.On("List", mock.Anything, mock.MatchedBy(func(f model.MessageFilter) bool {
			return f.Desc == true
		})).Return([]*model.Message{}, int64(0), nil)

		ctx := setupTestContext("GET", "/messages?order=desc", nil)
		handler.ListMessages(ctx)

		assert.Equal(t, 200, ctx.Response.StatusCode())
		svc.AssertExpectations(t)
	})

	t.Run("service error", func(t *testing.T) {
		svc := new(MockMessageService)
		handler := NewMessageHandler(svc)

		svc.On("List", mock.Anything, mock.Anything).
			Return(nil, int64(0), errors.New("database error"))

		ctx := setupTestContext("GET", "/messages", nil)
		handler.ListMessages(ctx)

		assert.Equal(t, 400, ctx.Response.StatusCode())

		var response map[string]string
		err := json.Unmarshal(ctx.Response.Body(), &response)
		require.NoError(t, err)
		assert.Equal(t, "database error", response["error"])

		svc.AssertExpectations(t)
	})
}

func TestMessageHandler_ListMessagesWithDeliveryReports(t *testing.T) {
	t.Run("successful list with delivery reports", func(t *testing.T) {
		svc := new(MockMessageService)
		handler := NewMessageHandler(svc)

		deliveredAt := time.Now()
		expectedMessages := []*model.MessageWithDeliveryReports{
			{
				ID:         1,
				CustomerID: 1,
				Mobile:     "+1234567890",
				Content:    "Test",
				DeliveryReports: []*model.DeliveryReport{
					{ID: 1, MessageID: 1, Status: "DELIVERED", DeliveredAt: &deliveredAt},
				},
			},
		}

		svc.On("GetMessagesWithDeliveryReports", mock.Anything, mock.AnythingOfType("model.MessageFilter")).
			Return(expectedMessages, int64(1), nil)

		ctx := setupTestContext("GET", "/messages/delivery-reports?customer_id=1", nil)
		handler.ListMessagesWithDeliveryReports(ctx)

		assert.Equal(t, 200, ctx.Response.StatusCode())

		var response listWithReportResponse
		err := json.Unmarshal(ctx.Response.Body(), &response)
		require.NoError(t, err)
		assert.Equal(t, int64(1), response.Total)
		assert.Len(t, response.Items, 1)
		assert.Len(t, response.Items[0].DeliveryReports, 1)

		svc.AssertExpectations(t)
	})

	t.Run("list with time range", func(t *testing.T) {
		svc := new(MockMessageService)
		handler := NewMessageHandler(svc)

		svc.On("GetMessagesWithDeliveryReports", mock.Anything, mock.MatchedBy(func(f model.MessageFilter) bool {
			return f.From != nil && f.To != nil
		})).Return([]*model.MessageWithDeliveryReports{}, int64(0), nil)

		ctx := setupTestContext("GET", "/messages/delivery-reports?from=2024-01-01&to=2024-12-31", nil)
		handler.ListMessagesWithDeliveryReports(ctx)

		assert.Equal(t, 200, ctx.Response.StatusCode())
		svc.AssertExpectations(t)
	})

	t.Run("service error", func(t *testing.T) {
		svc := new(MockMessageService)
		handler := NewMessageHandler(svc)

		svc.On("GetMessagesWithDeliveryReports", mock.Anything, mock.Anything).
			Return(nil, int64(0), errors.New("query error"))

		ctx := setupTestContext("GET", "/messages/delivery-reports", nil)
		handler.ListMessagesWithDeliveryReports(ctx)

		assert.Equal(t, 400, ctx.Response.StatusCode())
		svc.AssertExpectations(t)
	})
}

func TestHelperFunctions(t *testing.T) {
	t.Run("readJSON", func(t *testing.T) {
		data := map[string]string{"key": "value"}
		bodyBytes, _ := json.Marshal(data)
		ctx := setupTestContext("POST", "/", bodyBytes)

		var result map[string]string
		err := readJSON(ctx, &result)
		require.NoError(t, err)
		assert.Equal(t, "value", result["key"])
	})

	t.Run("writeJSON", func(t *testing.T) {
		ctx := setupTestContext("GET", "/", nil)
		data := map[string]string{"message": "test"}

		writeJSON(ctx, 200, data)

		assert.Equal(t, 200, ctx.Response.StatusCode())
		assert.Contains(t, string(ctx.Response.Header.Peek("Content-Type")), "application/json")

		var result map[string]string
		err := json.Unmarshal(ctx.Response.Body(), &result)
		require.NoError(t, err)
		assert.Equal(t, "test", result["message"])
	})

	t.Run("writeError", func(t *testing.T) {
		ctx := setupTestContext("GET", "/", nil)
		writeError(ctx, 404, "not found")

		assert.Equal(t, 404, ctx.Response.StatusCode())

		var result map[string]string
		err := json.Unmarshal(ctx.Response.Body(), &result)
		require.NoError(t, err)
		assert.Equal(t, "not found", result["error"])
	})

	t.Run("parseTime RFC3339", func(t *testing.T) {
		timeStr := "2024-01-01T12:00:00Z"
		parsed, err := parseTime(timeStr)
		require.NoError(t, err)
		assert.Equal(t, 2024, parsed.Year())
	})

	t.Run("parseTime date only", func(t *testing.T) {
		timeStr := "2024-01-01"
		parsed, err := parseTime(timeStr)
		require.NoError(t, err)
		assert.Equal(t, 2024, parsed.Year())
		assert.Equal(t, time.Month(1), parsed.Month())
		assert.Equal(t, 1, parsed.Day())
	})

	t.Run("parseTime invalid", func(t *testing.T) {
		timeStr := "invalid"
		_, err := parseTime(timeStr)
		assert.Error(t, err)
	})
}
