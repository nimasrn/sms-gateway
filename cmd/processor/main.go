package main

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nimasrn/message-gateway/internal/config"
	gateway "github.com/nimasrn/message-gateway/internal/gateways"
	"github.com/nimasrn/message-gateway/internal/processor"
	"github.com/nimasrn/message-gateway/internal/repository"
	"github.com/nimasrn/message-gateway/pkg/logger"
	"github.com/nimasrn/message-gateway/pkg/pg"
	"github.com/nimasrn/message-gateway/pkg/prom"
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

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Kill, os.Interrupt, syscall.SIGTERM)
	cfg := &gateway.Config{
		Providers: []gateway.ProviderConfig{
			{Name: "primary", URL: config.Get().ProviderPrimaryUrl, Weight: 100},
			{Name: "secondary", URL: config.Get().ProviderSecondaryUrl, Weight: 80},
			{Name: "backup", URL: config.Get().ProviderBackupUrl, Weight: 60},
		},
		Timeout:                 time.Second * 5,
		MaxRetries:              3,
		RetryDelay:              time.Millisecond * 100,
		MaxConns:                1000,
		ReadBufferSize:          1024 * 4,
		WriteBufferSize:         1024 * 4,
		HealthCheckInterval:     30 * time.Second,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   60 * time.Second,
	}
	client, err := gateway.NewClient(cfg)
	if err != nil {
		logger.Error("failed to create gateway", "error", err)
		return
	}

	deliveryReportRepo := repository.NewDeliveryReportRepository(db)

	// Initialize idempotency service
	idempotencyConfig := processor.DefaultIdempotencyConfig()
	idempotencyService := processor.NewIdempotencyService(redisAdap, idempotencyConfig)

	service, err := processor.NewProcessorService(redisAdap)
	service.RegisterProcessor(processor.NewSMSMessageProcessor(client, deliveryReportRepo, idempotencyService))

	if err != nil {
		logger.Error("failed to run the processor", "error", err)
		return
	}

	var hostname string
	hostname, err = os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	err = prom.Create(hostname, config.Get().AppEnv, config.Get().PromNamespace)
	if err != nil {
		logger.Error("failed to create prometheus metrics", "error", err)
		return
	}

	go func() {
		prom.ListenAndServer(":9100", "/metrics")
	}()

	go func() {
		err := service.Start()
		if err != nil {
			logger.Error("failed to start processor", "error", err)
		}
	}()

	select {
	case <-c:
		service.Stop()
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
