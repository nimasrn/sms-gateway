package logger

import (
	"os"

	"go.uber.org/zap"
)

type Logger interface {
	Info(msg string, values ...any)
	Warn(msg string, values ...any)
	Error(msg string, values ...any)
	Debug(msg string, values ...any)
	Panic(message string, values ...any)
	Fatal(error error, values ...any)
	Printf(format string, args ...interface{})
}

func init() {
	var config zap.Config

	env := os.Getenv("LOG_ENV")
	if env == "production" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
	}

	_, err := NewLogger(config)
	if err != nil {
		panic(err)
	}
}

func Info(msg string, values ...any) {
	GetLogger().Info(msg, values...)
}

func Warn(msg string, values ...any) {
	GetLogger().Warn(msg, values...)
}

func Error(msg string, values ...any) {
	GetLogger().Error(msg, values...)
}

func Debug(msg string, values ...any) {
	GetLogger().Debug(msg, values...)
}

func Panic(msg string, values ...any) {
	GetLogger().Panic(msg, values...)
}

func Fatal(error error, values ...any) {
	GetLogger().Fatal(error, values...)
}
