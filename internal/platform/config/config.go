// Package config loads application configuration.
//
// Precedence (low -> high): built-in defaults < JSON file (CONFIG_PATH or
// ./configs/config.json) < environment variables. Env vars win so containers and
// secret managers can override anything without a rebuild.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the whole application's configuration tree.
type Config struct {
	Env      string         `json:"env"` // dev | staging | prod
	HTTP     HTTPConfig     `json:"http"`
	GRPC     GRPCConfig     `json:"grpc"`
	Log      LogConfig      `json:"log"`
	Database DatabaseConfig `json:"database"`
	Kafka    KafkaConfig    `json:"kafka"`
	Outbox   OutboxConfig   `json:"outbox"`
}

type HTTPConfig struct {
	Addr            string        `json:"addr"`
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
}

type GRPCConfig struct {
	Addr string `json:"addr"`
}

type LogConfig struct {
	Level  string `json:"level"`  // debug | info | warn | error
	Format string `json:"format"` // json | text
}

type DatabaseConfig struct {
	// DSN is empty by default, which selects the in-memory persistence wiring.
	DSN             string        `json:"dsn"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
}

type KafkaConfig struct {
	// Brokers empty => the in-process bus is used instead of Kafka.
	Brokers []string `json:"brokers"`
	GroupID string   `json:"group_id"`

	// MaxRetries is how many retry-topic hops a failed message gets before it
	// is dead-lettered.
	MaxRetries int `json:"max_retries"`
	// RetryBaseDelay/RetryMaxDelay bound the exponential backoff between hops.
	RetryBaseDelay time.Duration `json:"retry_base_delay"`
	RetryMaxDelay  time.Duration `json:"retry_max_delay"`
}

type OutboxConfig struct {
	PollInterval time.Duration `json:"poll_interval"`
	BatchSize    int           `json:"batch_size"`
}

// Default returns a fully populated, all-in-memory configuration.
func Default() Config {
	return Config{
		Env: "dev",
		HTTP: HTTPConfig{
			Addr:            ":8080",
			ReadTimeout:     10 * time.Second,
			WriteTimeout:    15 * time.Second,
			ShutdownTimeout: 20 * time.Second,
		},
		GRPC: GRPCConfig{Addr: ":9090"},
		Log:  LogConfig{Level: "info", Format: "text"},
		Database: DatabaseConfig{
			MaxOpenConns:    20,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
		},
		Kafka: KafkaConfig{
			GroupID:        "myapp",
			MaxRetries:     3,
			RetryBaseDelay: time.Second,
			RetryMaxDelay:  30 * time.Second,
		},
		Outbox: OutboxConfig{PollInterval: time.Second, BatchSize: 100},
	}
}

// Load builds the effective configuration.
func Load() (Config, error) {
	cfg := Default()

	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		if _, err := os.Stat("configs/config.json"); err == nil {
			path = "configs/config.json"
		}
	}
	if path != "" {
		if err := loadFile(path, &cfg); err != nil {
			return Config{}, err
		}
	}

	applyEnv(&cfg)

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadFile(path string, cfg *Config) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// Duration fields in the JSON file are nanosecond integers (stdlib
	// encoding/json semantics). Prefer env vars ("1s", "30m") for readability.
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return errors.New("config: parsing " + path + ": " + err.Error())
	}
	return nil
}

func applyEnv(cfg *Config) {
	setStr(&cfg.Env, "APP_ENV")
	setStr(&cfg.HTTP.Addr, "HTTP_ADDR")
	setDur(&cfg.HTTP.ReadTimeout, "HTTP_READ_TIMEOUT")
	setDur(&cfg.HTTP.WriteTimeout, "HTTP_WRITE_TIMEOUT")
	setDur(&cfg.HTTP.ShutdownTimeout, "HTTP_SHUTDOWN_TIMEOUT")
	setStr(&cfg.GRPC.Addr, "GRPC_ADDR")
	setStr(&cfg.Log.Level, "LOG_LEVEL")
	setStr(&cfg.Log.Format, "LOG_FORMAT")
	setStr(&cfg.Database.DSN, "DATABASE_URL")
	setInt(&cfg.Database.MaxOpenConns, "DATABASE_MAX_OPEN_CONNS")
	setInt(&cfg.Database.MaxIdleConns, "DATABASE_MAX_IDLE_CONNS")
	setDur(&cfg.Database.ConnMaxLifetime, "DATABASE_CONN_MAX_LIFETIME")
	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		cfg.Kafka.Brokers = strings.Split(v, ",")
	}
	setStr(&cfg.Kafka.GroupID, "KAFKA_GROUP_ID")
	setInt(&cfg.Kafka.MaxRetries, "KAFKA_MAX_RETRIES")
	setDur(&cfg.Kafka.RetryBaseDelay, "KAFKA_RETRY_BASE_DELAY")
	setDur(&cfg.Kafka.RetryMaxDelay, "KAFKA_RETRY_MAX_DELAY")
	setDur(&cfg.Outbox.PollInterval, "OUTBOX_POLL_INTERVAL")
	setInt(&cfg.Outbox.BatchSize, "OUTBOX_BATCH_SIZE")
}

func (c Config) validate() error {
	if c.HTTP.Addr == "" {
		return errors.New("config: http.addr is required")
	}
	if c.GRPC.Addr == "" {
		return errors.New("config: grpc.addr is required")
	}
	switch c.Log.Format {
	case "json", "text":
	default:
		return errors.New("config: log.format must be json or text")
	}
	return nil
}

// UseDatabase reports whether real persistence is configured.
func (c Config) UseDatabase() bool { return c.Database.DSN != "" }

// UseKafka reports whether the Kafka bus should be used.
func (c Config) UseKafka() bool { return len(c.Kafka.Brokers) > 0 }

func setStr(dst *string, env string) {
	if v := os.Getenv(env); v != "" {
		*dst = v
	}
}

func setInt(dst *int, env string) {
	if v := os.Getenv(env); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}

func setDur(dst *time.Duration, env string) {
	if v := os.Getenv(env); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			*dst = d
		}
	}
}
