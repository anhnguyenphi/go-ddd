package bootstrap

import (
	"log/slog"

	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/eventbus/kafka"
	"github.com/example/myapp/internal/eventbus/local"
	"github.com/example/myapp/internal/platform/config"
)

// newBus selects the event bus implementation from config. The in-process bus is
// the default; set KAFKA_BROKERS to switch. Standard middleware (panic recovery,
// structured logging) wraps every handler.
func newBus(cfg config.Config, logger *slog.Logger) eventbus.Bus {
	mw := []eventbus.Middleware{
		eventbus.Recover(logger),
		eventbus.Logging(logger),
	}
	if cfg.UseKafka() {
		logger.Info("event bus: kafka", "brokers", cfg.Kafka.Brokers)
		return kafka.New(kafka.Config{
			Brokers:    cfg.Kafka.Brokers,
			GroupID:    cfg.Kafka.GroupID,
			MaxRetries: cfg.Kafka.MaxRetries,
			Backoff:    kafka.ExponentialBackoff(cfg.Kafka.RetryBaseDelay, cfg.Kafka.RetryMaxDelay),
		}, logger, mw...)
	}
	logger.Info("event bus: in-process")
	return local.New(logger, mw...)
}
