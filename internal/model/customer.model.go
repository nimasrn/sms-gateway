package model

type Customer struct {
	ID        int64  `json:"id"`
	APIKey    string `json:"api_key"`
	Balance   uint   `json:"balance"`
	RateLimit int    `json:"rate_limit"`
}

func (Customer) TableName() string { return "customers" }
