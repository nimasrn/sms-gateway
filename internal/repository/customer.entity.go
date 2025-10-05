package repository

import (
	"github.com/nimasrn/message-gateway/internal/model"
)

type CustomerEntity struct {
	ID        int64  `db:"id"         gorm:"primaryKey;autoIncrement;column:id"`
	APIKey    string `db:"api_key"    gorm:"column:api_key;not null;unique"`
	Balance   uint   `db:"balance"    gorm:"column:balance;not null;default:0"`
	RateLimit int    `db:"rate_limit" gorm:"column:rate_limit;not null;default:0"`
}

func (CustomerEntity) TableName() string {
	return "customers"
}

func toCustomerEntity(m *model.Customer) *CustomerEntity {
	if m == nil {
		return nil
	}
	return &CustomerEntity{
		ID:        m.ID,
		APIKey:    m.APIKey,
		Balance:   m.Balance,
		RateLimit: m.RateLimit,
	}
}

func toCustomerModel(e *CustomerEntity) *model.Customer {
	if e == nil {
		return nil
	}
	return &model.Customer{
		ID:        e.ID,
		APIKey:    e.APIKey,
		Balance:   e.Balance,
		RateLimit: e.RateLimit,
	}
}

func toCustomerModels(entities []*CustomerEntity) []*model.Customer {
	if entities == nil {
		return nil
	}
	models := make([]*model.Customer, len(entities))
	for i, e := range entities {
		models[i] = toCustomerModel(e)
	}
	return models
}
