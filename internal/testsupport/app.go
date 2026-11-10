package testsupport

import (
	"context"
	"errors"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for the Postgres profile
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	customerv1 "github.com/example/myapp/api/proto/customer/v1"
	orderv1 "github.com/example/myapp/api/proto/order/v1"
	"github.com/example/myapp/internal/bootstrap"
	"github.com/example/myapp/internal/platform/config"
	"github.com/example/myapp/internal/testsupport/api"
)

// App is a fully wired application under test plus clients for its HTTP and
// (optionally) gRPC endpoints. Everything is torn down by t.Cleanup.
type App struct {
	*bootstrap.Application

	// HTTP serves app.HTTPHandler() (REST via grpc-gateway, /healthz, ...).
	HTTP *httptest.Server
	// API is a REST client pointed at HTTP.
	API *api.Client

	// Set only with WithGRPC.
	GRPCAddr  string
	Customers customerv1.CustomerServiceClient
	Orders    orderv1.OrderServiceClient
}

type options struct {
	mutate     []func(*config.Config)
	grpc       bool
	background bool
}

// Option configures NewApp.
type Option func(*options)

// WithConfig lets a test tweak the config before Build.
func WithConfig(fn func(*config.Config)) Option {
	return func(o *options) { o.mutate = append(o.mutate, fn) }
}

// WithPostgres switches to the Postgres persistence profile. The database
// must already be migrated (containers.PostgresDSN does that).
func WithPostgres(dsn string) Option {
	return WithConfig(func(c *config.Config) { c.Database.DSN = dsn })
}

// WithKafka switches to the Kafka event bus. Use a unique groupID per test
// (containers.UniqueGroupID) so tests don't steal each other's partitions.
// Retries are made fast so retry/DLQ paths finish within a test timeout.
// It implies WithBackground, since nothing is consumed otherwise.
func WithKafka(brokers []string, groupID string) Option {
	return func(o *options) {
		o.background = true
		o.mutate = append(o.mutate, func(c *config.Config) {
			c.Kafka.Brokers = brokers
			c.Kafka.GroupID = groupID
			c.Kafka.MaxRetries = 1
			c.Kafka.RetryBaseDelay = 100 * time.Millisecond
			c.Kafka.RetryMaxDelay = 100 * time.Millisecond
		})
	}
}

// WithGRPC starts the gRPC server on a free local port and dials clients.
func WithGRPC() Option { return func(o *options) { o.grpc = true } }

// WithBackground runs app.RunBackground (outbox relay + bus consumers), the
// way cmd/api does. Without it, tests drive the relay with DrainOutbox.
func WithBackground() Option { return func(o *options) { o.background = true } }

// NewApp builds the application from config.Default() (never config.Load(),
// which would pick up configs/config.json), applies opts, and serves it.
func NewApp(t testing.TB, opts ...Option) *App {
	t.Helper()

	var o options
	for _, opt := range opts {
		opt(&o)
	}

	cfg := config.Default()
	cfg.Log.Level = "error"
	cfg.Outbox.PollInterval = 50 * time.Millisecond
	for _, fn := range o.mutate {
		fn(&cfg)
	}
	if o.grpc {
		cfg.GRPC.Addr = FreeAddr(t)
	}

	ctx, cancel := context.WithCancel(context.Background())
	app, err := bootstrap.Build(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatalf("bootstrap.Build: %v", err)
	}

	a := &App{Application: app}
	a.HTTP = httptest.NewServer(app.HTTPHandler())
	a.API = api.NewClient(a.HTTP.URL)

	var stops []func()
	stops = append(stops, a.HTTP.Close)

	if o.background {
		stops = append(stops, runLoop(t, "RunBackground", func() error { return app.RunBackground(ctx) }))
	}

	if o.grpc {
		stops = append(stops, runLoop(t, "gRPC server", func() error { return app.GRPCServer().Run(ctx) }))
		conn, err := grpc.NewClient(cfg.GRPC.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("dial %s: %v", cfg.GRPC.Addr, err)
		}
		stops = append(stops, func() { _ = conn.Close() })
		a.GRPCAddr = cfg.GRPC.Addr
		a.Customers = customerv1.NewCustomerServiceClient(conn)
		a.Orders = orderv1.NewOrderServiceClient(conn)
	}

	t.Cleanup(func() {
		cancel()
		for i := len(stops) - 1; i >= 0; i-- {
			stops[i]()
		}
		_ = app.Close()
	})
	return a
}

// runLoop runs fn in a goroutine and returns a func that waits for it to
// return (after ctx is cancelled). Unexpected errors are reported.
func runLoop(t testing.TB, name string, fn func() error) (wait func()) {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	return func() {
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("%s stopped with error: %v", name, err)
			}
		case <-time.After(30 * time.Second):
			t.Errorf("%s did not stop within 30s", name)
		}
	}
}

// FreeAddr asks the OS for a free TCP port and releases it. There is a small,
// accepted race between the release and the server binding it.
func FreeAddr(t testing.TB) string {
	t.Helper()
	var lc net.ListenConfig
	lis, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("FreeAddr: %v", err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()
	return addr
}

// Settle makes the read side consistent: with the in-process bus it drains
// the outbox synchronously; with background loops running it is a no-op and
// callers should poll with api.Eventually instead.
func (a *App) Settle(t testing.TB) {
	t.Helper()
	if a.Config.UseKafka() {
		return
	}
	if _, err := a.DrainOutbox(context.Background()); err != nil {
		t.Fatalf("DrainOutbox: %v", err)
	}
}
