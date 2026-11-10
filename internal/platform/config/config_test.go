package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/example/myapp/internal/platform/config"
)

// isolate runs the test in an empty dir (so configs/config.json is not picked
// up) with every config env var cleared.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch {
		case k == "CONFIG_PATH", k == "APP_ENV", k == "DATABASE_URL",
			strings.HasPrefix(k, "HTTP_"), strings.HasPrefix(k, "GRPC_"), strings.HasPrefix(k, "LOG_"),
			strings.HasPrefix(k, "DATABASE_"), strings.HasPrefix(k, "KAFKA_"), strings.HasPrefix(k, "OUTBOX_"):
			t.Setenv(k, "")
		}
	}
	return dir
}

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_DefaultsWithNoFileOrEnv(t *testing.T) {
	isolate(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, config.Default()) {
		t.Fatalf("Load() = %+v, want Default()", cfg)
	}
	if cfg.UseDatabase() || cfg.UseKafka() {
		t.Fatal("defaults must select the in-memory profile")
	}
}

func TestLoad_PicksUpRepoConfigThenEnvOverrides(t *testing.T) {
	dir := isolate(t)
	write(t, dir, "configs/config.json", `{"database":{"dsn":"postgres://file"},"kafka":{"brokers":["file:9092"]}}`)
	t.Setenv("DATABASE_URL", "postgres://env")
	t.Setenv("KAFKA_BROKERS", "a:1,b:2")
	t.Setenv("OUTBOX_POLL_INTERVAL", "250ms")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.DSN != "postgres://env" {
		t.Errorf("DSN = %q, env must win over file", cfg.Database.DSN)
	}
	if !reflect.DeepEqual(cfg.Kafka.Brokers, []string{"a:1", "b:2"}) {
		t.Errorf("brokers = %v", cfg.Kafka.Brokers)
	}
	if cfg.Outbox.PollInterval != 250*time.Millisecond {
		t.Errorf("poll interval = %v", cfg.Outbox.PollInterval)
	}
}

// Documented gotcha (docs/agent/learnings.md): an empty env var does not clear
// a value from the file; CONFIG_PATH must point elsewhere.
func TestLoad_ConfigPathReplacesRepoConfig(t *testing.T) {
	dir := isolate(t)
	write(t, dir, "configs/config.json", `{"database":{"dsn":"postgres://file"}}`)
	t.Setenv("CONFIG_PATH", write(t, dir, "empty.json", `{}`))

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UseDatabase() {
		t.Fatalf("DSN = %q, want empty with CONFIG_PATH -> {}", cfg.Database.DSN)
	}
}

func TestLoad_Rejects(t *testing.T) {
	cases := map[string]string{
		"unknown field":  `{"databse":{}}`,
		"bad log format": `{"log":{"format":"xml"}}`,
		"empty http":     `{"http":{"addr":""}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := isolate(t)
			t.Setenv("CONFIG_PATH", write(t, dir, "c.json", body))
			if _, err := config.Load(); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestLoad_KafkaTopicPrefix(t *testing.T) {
	isolate(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kafka.TopicPrefix != "" {
		t.Errorf("default topic prefix = %q, want empty", cfg.Kafka.TopicPrefix)
	}

	t.Setenv("KAFKA_TOPIC_PREFIX", "sb_feature_a")
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kafka.TopicPrefix != "sb_feature_a" {
		t.Errorf("topic prefix = %q, want sb_feature_a", cfg.Kafka.TopicPrefix)
	}
}
