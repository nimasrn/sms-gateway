package repository

import (
	"context"

	"github.com/nimasrn/message-gateway/internal/model"
	"github.com/nimasrn/message-gateway/pkg/pg"
)

type DeliveryReportRepository struct {
	*pg.DB
}

func NewDeliveryReportRepository(db *pg.DB) *DeliveryReportRepository {
	return &DeliveryReportRepository{
		db,
	}
}

func (r *DeliveryReportRepository) Create(ctx context.Context, dr *model.DeliveryReport) (*model.DeliveryReport, error) {
	entity := toDeliveryReportEntity(dr)

	if err := r.Write(ctx).WithContext(ctx).Create(entity).Error; err != nil {
		return nil, err
	}

	return toDeliveryReportModel(entity), nil
}
