package repository

import (
	"github.com/nimasrn/message-gateway/internal/model"
)

type TransactionEntity struct {
	ID         int64  `db:"id"          gorm:"primaryKey;autoIncrement;column:id"`
	CustomerID int64  `db:"customer_id" gorm:"column:customer_id;not null;index"`
	Amount     uint   `db:"amount"      gorm:"column:amount;not null"`
	Type       string `db:"type"        gorm:"column:type;not null"`
	MessageID  *int64 `db:"message_id"  gorm:"column:message_id;index"`
}

func (TransactionEntity) TableName() string {
	return "transactions"
}

func toTransactionEntity(m *model.Transaction) *TransactionEntity {
	if m == nil {
		return nil
	}
	return &TransactionEntity{
		ID:         m.ID,
		CustomerID: m.CustomerID,
		Amount:     m.Amount,
		Type:       m.Type,
		MessageID:  m.MessageID,
	}
}

func toTransactionModel(e *TransactionEntity) *model.Transaction {
	if e == nil {
		return nil
	}
	return &model.Transaction{
		ID:         e.ID,
		CustomerID: e.CustomerID,
		Amount:     e.Amount,
		Type:       e.Type,
		MessageID:  e.MessageID,
	}
}

func toTransactionModels(entities []*TransactionEntity) []*model.Transaction {
	if entities == nil {
		return nil
	}
	models := make([]*model.Transaction, len(entities))
	for i, e := range entities {
		models[i] = toTransactionModel(e)
	}
	return models
}
