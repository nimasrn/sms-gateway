package logger

import "go.uber.org/zap"

type ZapLogger struct {
	log *zap.SugaredLogger
}

var zapLogger *ZapLogger

func NewLogger(config zap.Config) (*ZapLogger, error) {
	logger, err := config.Build()
	if err != nil {
		return nil, err
	}
	defer logger.Sync() //nolint
	logger = logger.WithOptions(zap.AddCallerSkip(2))
	zapLogger = &ZapLogger{log: logger.Sugar()}
	return zapLogger, nil
}

func GetLogger() *ZapLogger {
	if zapLogger == nil {
		panic("logger not initialized")
	}
	return zapLogger
}

func (l *ZapLogger) Panic(message string, values ...any) {
	l.log.Panicw(message, values...)
}

func (l *ZapLogger) Fatal(error error, values ...any) {
	l.log.Fatalw(error.Error(), values...)
}

func (l *ZapLogger) Info(message string, values ...any) {
	l.log.Infow(message, values...)
}

func (l *ZapLogger) Warn(message string, values ...any) {
	l.log.Warnw(message, values...)
}

func (l *ZapLogger) Error(message string, values ...any) {
	l.log.Errorw(message, values...)
}

func (l *ZapLogger) Debug(message string, values ...any) {
	l.log.Debugw(message, values...)
}

func (l *ZapLogger) Printf(format string, args ...interface{}) {
	l.log.Infof(format, args...)
}
