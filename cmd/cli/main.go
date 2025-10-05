package main

import (
	"os"
	"strings"

	"github.com/nimasrn/message-gateway/internal/config"
	"github.com/nimasrn/message-gateway/pkg/logger"
	"github.com/nimasrn/message-gateway/pkg/pg"
)

func main() {
	err := config.Load(getEnvPath())
	if err != nil {
		logger.Error("failed to load config", "error", err)
	}
	// main.go --dir=./migrations
	pgConf := pg.Config{
		User:     config.Get().PostgresWriteUser,
		Host:     config.Get().PostgresWriteHost,
		Port:     config.Get().PostgresWritePort,
		Password: config.Get().PostgresWritePassword,
		Database: config.Get().PostgresWriteDatabase,
	}
	err = pg.Migrate(pgConf, getMigrationPath())
	if err != nil {
		logger.Error("migration: error running migrations", "error", err)
	}
}

func getEnvPath() string {
	for _, v := range os.Args {
		if strings.Contains(v, "--env=") {
			s := strings.Split(v, "=")
			if _, err := os.Open(s[1]); err != nil {
				logger.Error("failed to open the passed env file, got error" + err.Error())
				return ""
			}
			return s[1]
		}
	}
	if _, err := os.Open(".env"); err != nil {
		logger.Error("failed to open the passed env file, got error" + err.Error())
		return ""
	}
	return ".env"
}

func getMigrationPath() string {
	for _, v := range os.Args {
		if strings.Contains(v, "--dir=") {
			s := strings.Split(v, "=")
			if _, err := os.Open(s[1]); err != nil {
				logger.Error("failed to open the passed env file, got error" + err.Error())
				return ""
			}
			return s[1]
		}
	}
	if _, err := os.Open("./migrations"); err != nil {
		logger.Error("failed to open the passed env file, got error" + err.Error())
		return ""
	}
	return "./migrations"
}
