package model

type Transaction struct {
	ID         int64     `json:"id"          db:"id"           gorm:"primaryKey;autoIncrement;column:id"`
	CustomerID int64     `json:"customer_id" db:"customer_id"  gorm:"column:customer_id;not null;index"`
	Customer   *Customer `json:"-"                              gorm:"foreignKey:CustomerID;references:ID;constraint:OnDelete:CASCADE"`
	Amount     uint      `json:"amount"      db:"amount"       gorm:"column:amount;not null"`
	Type       string    `json:"type"        db:"type"         gorm:"column:type;not null"`    // e.g. "debit" | "credit"
	MessageID  *int64    `json:"message_id"  db:"message_id"   gorm:"column:message_id;index"` // nullable (ON DELETE SET NULL)
	SMS        *Message  `json:"-"                              gorm:"foreignKey:MessageID;references:ID;constraint:OnDelete:SET NULL"`
}

func (Transaction) TableName() string { return "transactions" }
