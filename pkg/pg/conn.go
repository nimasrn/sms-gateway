package pg

import (
	"database/sql"
	"fmt"
)

type Config struct {
	User     string `env:"USER"`
	Host     string `env:"HOST"`
	Port     string `env:"PORT"`
	Password string `env:"PASSWORD"`
	Database string `env:"DBNAME"`
}

func newSqlConnection(config Config) (*sql.DB, error) {
	return sql.Open("postgres", fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable", config.Host, config.User, config.Password, config.Database, config.Port))
}
