package repository

import (
	"context"
	"errors"

	"github.com/nimasrn/message-gateway/internal/model"
	"github.com/nimasrn/message-gateway/pkg/pg"
	"gorm.io/gorm"
)

var (
	// ErrNotFound is returned when a message does not exist.
	ErrNotFound = errors.New("message not found")
)

type MessageRepository struct {
	*pg.DB
}

func NewMessageRepository(db *pg.DB) *MessageRepository {
	return &MessageRepository{
		db,
	}
}

func (r *MessageRepository) Create(ctx context.Context, msg *model.Message) (*model.Message, error) {
	entity := toMessageEntity(msg)

	if err := r.Write(ctx).WithContext(ctx).Create(entity).Error; err != nil {
		return nil, err
	}

	return toMessageModel(entity), nil
}

func (r *MessageRepository) List(ctx context.Context, f model.MessageFilter) ([]*model.Message, int64, error) {
	q := r.Read(ctx).WithContext(ctx).Model(&MessageEntity{})

	if f.CustomerID != nil {
		q = q.Where("customer_id = ?", *f.CustomerID)
	}

	if f.Mobile != nil && *f.Mobile != "" {
		q = q.Where("mobile = ?", *f.Mobile)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at < ?", *f.To)
	}

	// Count before pagination
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Build order clause
	order := "created_at"
	if f.Desc {
		order += " DESC"
	} else {
		order += " ASC"
	}

	// Apply pagination
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	var entities []*MessageEntity
	if err := q.Order(order).Limit(limit).Offset(offset).Find(&entities).Error; err != nil {
		return nil, 0, err
	}

	return toMessageModels(entities), total, nil
}

func (r *MessageRepository) GetMessagesWithDeliveryReports(ctx context.Context, f model.MessageFilter) ([]*model.MessageWithDeliveryReports, int64, error) {
	query := r.buildMessagesWithReportsQuery(ctx)

	// Apply filters
	if f.CustomerID != nil {
		query = query.Where("m.customer_id = ?", *f.CustomerID)
	}
	if f.Mobile != nil && *f.Mobile != "" {
		query = query.Where("m.mobile = ?", *f.Mobile)
	}
	if f.From != nil {
		query = query.Where("m.created_at >= ?", *f.From)
	}
	if f.To != nil {
		query = query.Where("m.created_at < ?", *f.To)
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply ordering
	order := "m.created_at ASC"
	if f.Desc {
		order = "m.created_at DESC"
	}
	query = query.Order(order)

	// Apply pagination
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	query = query.Limit(limit).Offset(offset)

	// Execute query
	var entities []*MessageWithReportsEntity
	if err := query.Find(&entities).Error; err != nil {
		return nil, 0, err
	}

	return toMessageWithReportsModels(entities), total, nil
}

func (r *MessageRepository) buildMessagesWithReportsQuery(ctx context.Context) *gorm.DB {
	return r.Read(ctx).WithContext(ctx).
		Table("messages AS m").
		Select(`
            -- Message fields
            m.id                                    AS id,
            m.customer_id                           AS customer_id,
            m.mobile                                AS mobile,
            m.content                               AS content,
            m.priority                              AS priority,
            m.created_at                            AS created_at,
            
            -- Aggregate delivery reports as JSON
            COALESCE(
                json_agg(
                    json_build_object(
                        'id', dr.id,
                        'message_id', dr.message_id,
                        'status', dr.status,
                        'delivered_at', dr.delivered_at
                    )
                    ORDER BY dr.id DESC
                ) FILTER (WHERE dr.id IS NOT NULL),
                '[]'::json
            )                                       AS delivery_reports
        `).
		Joins("LEFT JOIN delivery_reports AS dr ON dr.message_id = m.id").
		Group(`
            m.id,
            m.customer_id,
            m.mobile,
            m.content,
            m.priority,
            m.created_at
        `)
}
