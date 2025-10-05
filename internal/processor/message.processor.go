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
	idempotency        *IdempotencyService
}

func NewSMSMessageProcessor(client *gateway.Client, deliveryReportRepo DeliveryReportRepository, idempotency *IdempotencyService) *SMSMessageProcessor {
	return &SMSMessageProcessor{
		client:             client,
		deliveryReportRepo: deliveryReportRepo,
		idempotency:        idempotency,
	}
}

func (p *SMSMessageProcessor) GetType() string {
	return "message"
}

// Process the messages and send to provider with idempotency guarantees
func (p *SMSMessageProcessor) Process(ctx context.Context, queueMessage *queue.Message) error {
	// Step 1: Parse message
	var message model.Message
	err := json.Unmarshal(queueMessage.Data, &message)
	if err != nil {
		logger.Error("Failed to unmarshal message", "error", err)
		// Invalid message format - create failed delivery report and move to DLQ
		report := &model.DeliveryReport{
			MessageID: message.ID,
			Status:    string(model.MessageStatusFailed),
		}
		_, _ = p.deliveryReportRepo.Create(ctx, report)
		return err // Return error to trigger DLQ move
	}

	messageID := strconv.FormatInt(message.ID, 10)

	// Step 2: Acquire processing lock and check idempotency
	procCtx, err := p.idempotency.AcquireProcessingLock(ctx, messageID)
	if err != nil {
		if errors.Is(err, ErrAlreadyProcessed) {
			// Message already processed successfully - ACK to remove from queue
			logger.Info("Message already processed, skipping", "message_id", messageID)
			return nil
		}
		if errors.Is(err, ErrMaxRetriesExceeded) {
			// Max retries exceeded - create failed delivery report and ACK
			logger.Error("Max retries exceeded", "message_id", messageID)
			report := &model.DeliveryReport{
				MessageID: message.ID,
				Status:    string(model.MessageStatusFailed),
			}
			_, _ = p.deliveryReportRepo.Create(ctx, report)
			return nil // ACK to move to DLQ
		}
		if errors.Is(err, ErrLockAcquireFailed) {
			// Another consumer is processing - NACK to retry later
			logger.Info("Lock held by another consumer, will retry", "message_id", messageID)
			return errors.New("lock held by another consumer")
		}
		// Unexpected error - NACK to retry
		logger.Error("Failed to acquire lock", "message_id", messageID, "error", err)
		return err
	}

	// Ensure lock is released on exit (if not already marked success/failure)
	defer func() {
		if procCtx.lockAcquired {
			p.idempotency.ReleaseLock(ctx, procCtx)
		}
	}()

	logger.Info("Processing message",
		"message_id", messageID,
		"mobile", message.Mobile,
		"retry_count", procCtx.RetryCount,
		"is_retry", procCtx.IsRetry)

	// Step 3: Send SMS to provider
	req := &gateway.SendRequest{
		MessageID:   messageID,
		PhoneNumber: message.Mobile,
		Content:     message.Content,
		Priority:    message.Priority,
	}

	res, err := p.client.SendSMS(ctx, req)
	if err != nil {
		// Step 4a: Sending failed - mark failure and retry
		logger.Error("Failed to send SMS", "message_id", messageID, "error", err)
		if markErr := p.idempotency.MarkFailure(ctx, procCtx, err); markErr != nil {
			logger.Error("Failed to mark failure", "message_id", messageID, "error", markErr)
		}
		return err // NACK to retry from queue
	}

	// Step 4b: Sending succeeded - save delivery report and mark success
	logger.Info("SMS sent successfully",
		"message_id", messageID,
		"mobile", message.Mobile,
		"status", res.Status,
		"retry_count", procCtx.RetryCount)

	if res.Status == gateway.StatusDelivered {
		// Record delivery metrics
		prom.AddMessageExpressDeliveryDuration(
			res.DeliveredAt.Sub(message.CreatedAt).Seconds(),
			message.Priority,
		)

		// Create delivery report
		report := &model.DeliveryReport{
			MessageID:   message.ID,
			Status:      string(model.MessageStatusDelivered),
			DeliveredAt: res.DeliveredAt,
		}
		_, err := p.deliveryReportRepo.Create(ctx, report)
		if err != nil {
			logger.Error("Failed to save delivery report", "message_id", messageID, "error", err)
			// Continue - we don't want delivery report save failure to trigger retry
		}

		// Mark as successfully processed (sets 24-hour processed marker)
		if markErr := p.idempotency.MarkSuccess(ctx, procCtx); markErr != nil {
			logger.Error("Failed to mark success", "message_id", messageID, "error", markErr)
			// Continue - message was sent successfully
		}

		return nil // ACK message
	}

	// Provider returned non-delivered status - treat as failure
	logger.Warn("SMS not delivered", "message_id", messageID, "status", res.Status)
	if markErr := p.idempotency.MarkFailure(ctx, procCtx, errors.New("provider returned non-delivered status")); markErr != nil {
		logger.Error("Failed to mark failure", "message_id", messageID, "error", markErr)
	}
	return errors.New("failed to send message")
}
