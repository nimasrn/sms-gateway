package pg

import (
	_ "github.com/lib/pq"
	"github.com/nimasrn/message-gateway/pkg/logger"
	"github.com/pressly/goose/v3"
)

func Migrate(cfg Config, dir string) error {
	if err := goose.SetDialect("postgres"); err != nil {
		logger.Fatal(err)
	}

	db, err := newSqlConnection(cfg)
	if err != nil {
		return err
	}
	if err = goose.Up(db, dir); err != nil {
		logger.Fatal(err)
	}

	return nil
}
