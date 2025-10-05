package main

import (
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/nimasrn/message-gateway/internal/config"
	"github.com/nimasrn/message-gateway/internal/handlers"
	"github.com/nimasrn/message-gateway/internal/queue"
	"github.com/nimasrn/message-gateway/internal/repository"
	"github.com/nimasrn/message-gateway/internal/services"
	xhttp "github.com/nimasrn/message-gateway/pkg/http"
	"github.com/nimasrn/message-gateway/pkg/logger"
	"github.com/nimasrn/message-gateway/pkg/pg"
	"github.com/nimasrn/message-gateway/pkg/redis"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {

	err := config.Load(argContainsEnvPath())
	if err != nil {
		logger.Error("failed to load config", "error", err)
		return
	}

	// transport (tcp for now)
	s := xhttp.NewServer(xhttp.DefaultServerOption)
	s.Server.ReadBufferSize = 1024 * 16
	s.Server.WriteBufferSize = 1024 * 16
	s.Use(xhttp.CompressMiddleware(6))
	s.Use(xhttp.TimeoutMiddleware(time.Second * 5))
	s.Use(xhttp.RequestLoggerMiddleware)
	s.Use(xhttp.RecoverMiddleware)
	s.Router = xhttp.CreateDefaultRouter()

	readConf := pg.Config{
		User:     config.Get().PostgresReadUser,
		Host:     config.Get().PostgresReadHost,
		Port:     config.Get().PostgresReadPort,
		Password: config.Get().PostgresReadPassword,
		Database: config.Get().PostgresReadDatabase,
	}
	writeConf := pg.Config{
		User:     config.Get().PostgresWriteUser,
		Host:     config.Get().PostgresWriteHost,
		Port:     config.Get().PostgresWritePort,
		Password: config.Get().PostgresWritePassword,
		Database: config.Get().PostgresWriteDatabase,
	}

	pgDebug := false
	if config.Get().AppEnv == "dev" {
		pgDebug = true
	}
	db, err := pg.CreateReadWrite(readConf, writeConf, pgDebug)
	if err != nil {
		logger.Error("failed connecting to pg", "error", err)
		return
	}

	redisAdap, err := redis.NewRedisAdapter("default", config.Get().RedisUniversalKeyPrefix, &redis.Options{
		Addrs:      []string{config.Get().RedisAddr},
		ClientName: "default",
		DB:         config.Get().RedisDatabase,
		Username:   config.Get().RedisUsername,
		Password:   config.Get().RedisPassword,
	})
	if err != nil {
		logger.Error("failed connecting to redis", "error", err)
		return
	}

	q, err := queue.NewQueue(redisAdap, queue.QueueConfig{
		Name:              config.Get().QueueName,
		ConsumerGroup:     config.Get().QueueConsumerGroup,
		ConsumerName:      config.Get().QueueConsumerName,
		MaxRetries:        config.Get().QueueMaxRetries,
		VisibilityTimeout: config.Get().QueueVisibilityTimeout,
		PollInterval:      config.Get().QueuePollInterval,
		BatchSize:         config.Get().QueueBatchSize,
		MaxLen:            config.Get().QueueMaxLen,
		EnableDLQ:         config.Get().QueueEnableDLQ,
	})
	if err != nil {
		logger.Error("failed creating queue", "error", err)
	}

	expressQ, err := queue.NewQueue(redisAdap, queue.QueueConfig{
		Name:              config.Get().QueueName + ":express",
		ConsumerGroup:     config.Get().QueueConsumerGroup,
		ConsumerName:      config.Get().QueueConsumerName,
		MaxRetries:        config.Get().QueueMaxRetries,
		VisibilityTimeout: config.Get().QueueVisibilityTimeout,
		PollInterval:      config.Get().QueuePollInterval,
		BatchSize:         config.Get().QueueBatchSize,
		MaxLen:            config.Get().QueueMaxLen,
		EnableDLQ:         config.Get().QueueEnableDLQ,
	})
	if err != nil {
		logger.Error("failed creating queue", "error", err)
	}

	messageRepo := repository.NewMessageRepository(db)
	transactionRepo := repository.NewTransactionRepository(db)
	customerRepo := repository.NewCustomerRepository(db)

	// services
	messageService := services.NewMessageService(messageRepo, customerRepo, transactionRepo, q, expressQ)
	healthService := services.NewHealthService()

	// v1 handlers
	messageHandler := handlers.NewMessageHandler(messageService)
	healthHandler := handlers.NewHealthHandler(healthService)

	g := s.Router.Group("/api/v1")
	handlers.RegisterMessageRoutes(g, messageHandler)
	handlers.RegisterHealthRoutes(g, healthHandler)

	// Create new server
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Kill)

	go func() {
		var err = s.ListenAndServe(config.Get().HttpListenAddr)
		if err != nil {
			logger.Error("error in running http-server", "error", err)
		}
	}()

	select {
	case <-c:
		s.Shutdown()
	}
}

func argContainsEnvPath() string {
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
	return ""
}
