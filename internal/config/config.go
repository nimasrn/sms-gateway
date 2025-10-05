package config

import (
	"time"

	"github.com/Netflix/go-env"
	"github.com/joho/godotenv"
	"github.com/nimasrn/message-gateway/pkg/logger"
	"github.com/pkg/errors"
)

const ConfigTagName = "env"
const ConfigDefaultTagName = "default"

var config *Config

// Configuration This struct holds config envs and values
// which are used in the mod_tracking. Only this struct must be used
// to hold any configuration values, no direct access to
// env, ini or any other config source should be made
type Config struct {
	AppEnv              string `env:"APP_ENV" default:"dev"`
	AppName             string `env:"APP_NAME" default:"message_gateway"`
	AppDebug            bool   `env:"APP_DEBUG" default:"1"`
	AppDebugMetricsAddr string `env:"APP_DEBUG_METRIC_ADDR"`
	AppDebugMetricsURI  string `env:"APP_DEBUG_METRIC_URI"`
	AppBaseUrl          string `env:"APP_BASE_URL"`

	HttpListenAddr            string `env:"HTTP_LISTEN_ADDR" validation:"mustExists"`
	HttpBaseRequestUrl        string `env:"HTTP_BASE_REQUEST_URI" validation:"mustExists"`
	HttpServerReadTimeout     int    `env:"HTTP_SERVER_READ_TIMEOUT"`
	HttpServerWriteTimeout    int    `env:"HTTP_SERVER_WRITE_TIMEOUT"`
	HttpServerReadBufferSize  int    `env:"HTTP_SERVER_READ_BUFFER_SIZE"`
	HttpServerWriteBufferSize int    `env:"HTTP_SERVER_WRITE_BUFFER_SIZE"`

	PostgresReadHost     string `env:"POSTGRES_READ_HOST"`
	PostgresReadPort     string `env:"POSTGRES_READ_PORT"`
	PostgresReadUser     string `env:"POSTGRES_READ_USER"`
	PostgresReadPassword string `env:"POSTGRES_READ_PASSWORD"`
	PostgresReadDatabase string `env:"POSTGRES_READ_DBNAME"`

	PostgresWriteHost     string `env:"POSTGRES_WRITE_HOST"`
	PostgresWritePort     string `env:"POSTGRES_WRITE_PORT"`
	PostgresWriteUser     string `env:"POSTGRES_WRITE_USER"`
	PostgresWritePassword string `env:"POSTGRES_WRITE_PASSWORD"`
	PostgresWriteDatabase string `env:"POSTGRES_WRITE_DBNAME"`

	RedisAddr               string `env:"REDIS_ADDR"`
	RedisUsername           string `env:"REDIS_USER"`
	RedisPassword           string `env:"REDIS_PASS"`
	RedisDatabase           int    `env:"REDIS_DATABASE"`
	RedisUniversalKeyPrefix string `env:"REDIS_UNIVERSAL_KEY_PREFIX"`

	PromNamespace string `env:"PROM_NAMESPACE"`

	ProfilerEnable bool `env:"PROFILER_ENABLE"`
	ProfilerPort   int  `env:"PROFILER_PORT"`

	LogLevel []string `env:"LOG_LEVEL"`

	QueueName              string        `env:"QUEUE_NAME"`
	QueueConsumerGroup     string        `env:"QUEUE_CONSUMER_GROUP"`
	QueueConsumerName      string        `env:"QUEUE_CONSUMER_NAME"`
	QueueMaxRetries        int           `env:"QUEUE_MAX_RETRIES"`
	QueueVisibilityTimeout time.Duration `env:"QUEUE_VISIBILITY_TIMEOUT"`
	QueuePollInterval      time.Duration `env:"QUEUE_POLL_INTERVAL"`
	QueueBatchSize         int64         `env:"QUEUE_BATCH_SIZE"`
	QueueMaxLen            int64         `env:"QUEUE_MAX_LEN"`
	QueueEnableDLQ         bool          `env:"QUEUE_ENABLE_DLQ"`

	ProviderPrimaryUrl   string `env:"PROVIDER_PRIMARY_URL"`
	ProviderSecondaryUrl string `env:"PROVIDER_SECONDARY_URL"`
	ProviderBackupUrl    string `env:"PROVIDER_BACKUP_URL"`
}

func Load(path string) error {
	logger.Info("loading configs..", "path", path)
	c := &Config{}
	var err error
	if path != "" {
		logger.Info("trying to publish env from file", "path", path)
		err = godotenv.Load(path)
		if err != nil {
			return errors.New("failed to load configuration file " + path + " error: " + err.Error())
		}
	}

	_, err = env.UnmarshalFromEnviron(c)

	if err != nil {
		return errors.New("failed to map env variables to Configuration object " + " error: " + err.Error())
	}

	config = c
	return nil
}

func Get() *Config {
	if config == nil {
		logger.Panic("Config is not initialized")
	}
	return config
}
