package processor

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	gateway "github.com/nimasrn/message-gateway/internal/gateways"
	"github.com/nimasrn/message-gateway/internal/model"
	"github.com/nimasrn/message-gateway/internal/queue"
	"github.com/nimasrn/message-gateway/pkg/logger"
	"github.com/nimasrn/message-gateway/pkg/prom"
)

type DeliveryReportRepository interface {
	Create(ctx context.Context, dr *model.DeliveryReport) (*model.DeliveryReport, error)
}

type SMSMessageProcessor struct {
	client             *gateway.Client
	deliveryReportRepo DeliveryReportRepository
}

func NewSMSMessageProcessor(client *gateway.Client, deliveryReportRepo DeliveryReportRepository) *SMSMessageProcessor {
	return &SMSMessageProcessor{
		client:             client,
		deliveryReportRepo: deliveryReportRepo,
	}
}

func (p *SMSMessageProcessor) GetType() string {
	return "message"
}

// Process the messages and send to provider
// @TODO: need to collect more metrics
func (p *SMSMessageProcessor) Process(ctx context.Context, queueMessage *queue.Message) error {
	var message model.Message
	err := json.Unmarshal(queueMessage.Data, &message)
	if err != nil {
		// move to dlq for refunding
		// @todo store the errors
		report := &model.DeliveryReport{
			MessageID: message.ID,
			Status:    string(model.MessageStatusFailed),
		}
		_, err := p.deliveryReportRepo.Create(ctx, report)
		if err != nil {
			logger.Error("error creating deliveryReport", "error", err)
			// @TODO retry or use fallback and temporary db
		}
	}
	req := &gateway.SendRequest{
		MessageID:   strconv.FormatInt(message.ID, 10),
		PhoneNumber: message.Mobile,
		Content:     message.Content,
		Priority:    message.Priority,
	}
	res, err := p.client.SendSMS(ctx, req)
	if err != nil {
		return err
	}
	logger.Info("processed completed", "mobile", message.Mobile, "content", message.Content)
	if res.Status == gateway.StatusDelivered {
		prom.AddMessageExpressDeliveryDuration(res.DeliveredAt.Sub(message.CreatedAt).Seconds(), message.Priority)
		report := &model.DeliveryReport{
			MessageID:   message.ID,
			Status:      string(model.MessageStatusDelivered),
			DeliveredAt: res.DeliveredAt,
		}
		_, err := p.deliveryReportRepo.Create(ctx, report)
		if err != nil {
			logger.Error("failed to save delivery report", "error", err)
			// @TODO retry or use fallback and temporary db
		}
		return nil
	}
	return errors.New("failed to send message")
}
