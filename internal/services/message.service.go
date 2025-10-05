package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/nimasrn/message-gateway/internal/model"
	"github.com/nimasrn/message-gateway/internal/queue"
	"github.com/nimasrn/message-gateway/internal/repository"
)

var (
	ErrInvalidMobile       = fmt.Errorf("invalid mobile number")
	ErrEmptyContent        = fmt.Errorf("message content cannot be empty")
	ErrContentTooLong      = fmt.Errorf("message content exceeds maximum length")
	ErrInactiveCustomer    = errors.New("customer account is not active")
	ErrInsufficientBalance = errors.New("insufficient balance to send message")
	ErrNotFound            = errors.New("error notfound")
)

type MessageRepository interface {
	Create(ctx context.Context, p *model.Message) (*model.Message, error)
	List(ctx context.Context, f model.MessageFilter) ([]*model.Message, int64, error) // results, totalCount
	GetMessagesWithDeliveryReports(ctx context.Context, f model.MessageFilter) ([]*model.MessageWithDeliveryReports, int64, error)
}

type TransactionRepository interface {
	Create(ctx context.Context, txn *model.Transaction) (*model.Transaction, error)
}

type CustomerRepository interface {
	DeductBalance(ctx context.Context, id int64, amount uint) error
	AddBalance(ctx context.Context, id int64, amount uint) error
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}
type MessageService struct {
	messageRepo       MessageRepository
	customerRepo      CustomerRepository
	transactionRepo   TransactionRepository
	mobileNorm        MobileNormalizer
	maxContentLen     int
	strictTransitions bool
	queue             *queue.Queue
	expressQueue      *queue.Queue
}

func NewMessageService(messageRepo MessageRepository, customerRepo CustomerRepository, transactionRepo TransactionRepository, queue *queue.Queue, expressQueue *queue.Queue) *MessageService {
	return &MessageService{
		messageRepo:     messageRepo,
		customerRepo:    customerRepo,
		transactionRepo: transactionRepo,
		queue:           queue,
		expressQueue:    expressQueue,
	}
}

func (s *MessageService) Create(ctx context.Context, p model.MessageCreateRequest) (*model.Message, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	p.Mobile = strings.TrimSpace(p.Mobile)
	if s.mobileNorm != nil {
		np, err := s.mobileNorm.Normalize(p.Mobile)
		if err != nil || np == "" {
			return nil, ErrInvalidMobile
		}
		p.Mobile = np
	}

	p.Content = strings.TrimSpace(p.Content)
	if p.Content == "" {
		return nil, ErrEmptyContent
	}
	if s.maxContentLen > 0 && utf8.RuneCountInString(p.Content) > s.maxContentLen {
		return nil, ErrContentTooLong
	}

	m := &model.Message{
		CustomerID: p.CustomerID,
		Mobile:     p.Mobile,
		Content:    p.Content,
		Priority:   p.Priority,
	}

	// Start transaction: deduct balance and create message atomically
	var createdMessage *model.Message
	err := s.customerRepo.WithinTransaction(ctx, func(ctx context.Context) error {
		// 1. Deduct balance first (fail fast if insufficient funds)
		if err := s.customerRepo.DeductBalance(ctx, p.CustomerID, 1); err != nil {
			// Map repository errors to service errors
			if errors.Is(err, repository.ErrInsufficientBalance) {
				return ErrInsufficientBalance
			}
			return fmt.Errorf("deduct balance: %w", err)
		}

		// 2. Create message (only if balance deduction succeeded)
		created, err := s.messageRepo.Create(ctx, m)
		if err != nil {
			// Transaction will auto-rollback, refunding the balance
			return fmt.Errorf("create message: %w", err)
		}

		createdMessage = created

		trx := &model.Transaction{
			CustomerID: p.CustomerID,
			Amount:     1,
			Type:       "debit",
			MessageID:  &createdMessage.ID,
		}
		_, err = s.transactionRepo.Create(ctx, trx)
		if err != nil {
			return err
		}

		if createdMessage.Priority == "express" {
			_, err = s.expressQueue.PublishJSON(ctx, created, nil)
			if err != nil {
				return err
			}
			return nil
		}
		_, err = s.queue.PublishJSON(ctx, created, nil)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return createdMessage, nil
}

type MobileNormalizer interface {
	Normalize(s string) (string, error)
}

func NewService(repo MessageRepository) *MessageService {
	s := &MessageService{
		messageRepo:   repo,
		maxContentLen: 160,
	}
	return s
}

func (s *MessageService) List(ctx context.Context, f model.MessageFilter) ([]*model.Message, int64, error) {
	// (Optional) sanitize filter here if needed.
	return s.messageRepo.List(ctx, f)
}

func (s *MessageService) GetMessagesWithDeliveryReports(ctx context.Context, f model.MessageFilter) ([]*model.MessageWithDeliveryReports, int64, error) {
	return s.messageRepo.GetMessagesWithDeliveryReports(ctx, f)
}
