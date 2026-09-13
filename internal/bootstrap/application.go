// Package bootstrap is the application's composition root. It is the only place
// where concrete implementations are chosen and wired; every other package
// depends on interfaces. cmd/* entrypoints call Build and then run the pieces
// they need.
package bootstrap

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/example/myapp/api/openapi"
	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/eventbus/outbox"
	"github.com/example/myapp/internal/platform/config"
	"github.com/example/myapp/internal/platform/grpcx"
	"github.com/example/myapp/internal/platform/httpx"
	"github.com/example/myapp/internal/platform/observability"
)

// Application is the fully wired system. Not every field is used by every
// process: the API serves Handler; the worker runs RunBackground.
type Application struct {
	Config config.Config
	Logger *slog.Logger

	handler     http.Handler
	grpcServer  *grpcx.Server
	bus         eventbus.Bus
	relay       *outbox.Relay
	persistence persistence
}

// Build constructs the application graph from configuration.
func Build(ctx context.Context, cfg config.Config) (*Application, error) {
	logger := observability.NewLogger(cfg.Log)
	logger.Info("building application", "env", cfg.Env)

	bus := newBus(cfg, logger)

	// Transactional outbox: domain-event publication writes here (inside the
	// request transaction); the relay forwards to the bus at-least-once.
	outboxStore := outbox.NewMemoryStore()
	outboxPub := outbox.NewPublisher(outboxStore)
	relay := outbox.NewRelay(outboxStore, bus, logger, outbox.Options{
		Interval:  cfg.Outbox.PollInterval,
		BatchSize: cfg.Outbox.BatchSize,
	})

	p, err := newPersistence(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	mods := newModules(logger, p, outboxPub)

	// The gateway calls the gRPC service objects directly in-process (no
	// dial, no second network hop). It's the actual REST implementation —
	// there is no separate hand-written HTTP adapter — so its JSON wire
	// format is pinned to the proto's own field names (snake_case) to match
	// the generated OpenAPI doc (json_names_for_fields=false in buf.gen.yaml).
	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions:   protojson.MarshalOptions{UseProtoNames: true},
			UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
		}),
		runtime.WithForwardResponseOption(grpcx.StatusCodeOption),
		runtime.WithOutgoingHeaderMatcher(grpcx.OutgoingHeaderMatcher),
	)
	if err := mods.registerGateway(ctx, gwMux); err != nil {
		_ = p.close()
		return nil, err
	}

	mux := http.NewServeMux()
	registerOps(mux, p)
	registerDocs(mux)
	mux.Handle("/api/v1/", gwMux)
	if err := mods.registerSubscriptions(bus); err != nil {
		_ = p.close()
		return nil, err
	}

	handler := httpx.Apply(mux,
		httpx.RequestID(logger),
		httpx.AccessLog(),
		httpx.Recover(),
	)

	grpcSrv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcx.Recovery(logger), grpcx.AccessLog(logger)),
	)
	mods.registerGRPC(grpcSrv)
	reflection.Register(grpcSrv) // lets grpcurl/grpcui introspect without the .proto files

	return &Application{
		Config:      cfg,
		Logger:      logger,
		handler:     handler,
		grpcServer:  grpcx.New(cfg.GRPC, grpcSrv, logger),
		bus:         bus,
		relay:       relay,
		persistence: p,
	}, nil
}

// HTTPHandler is the root handler for the API process.
func (a *Application) HTTPHandler() http.Handler { return a.handler }

// GRPCServer is the gRPC endpoint for the API process.
func (a *Application) GRPCServer() *grpcx.Server { return a.grpcServer }

// runner is implemented by any bus that owns a background loop (the Kafka
// adapter's consumer readers; the in-process bus has none). Detected via an
// optional interface, the same pattern Close already uses, so swapping in a
// future bus implementation needs no change here.
type runner interface {
	Run(ctx context.Context) error
}

// RunBackground runs the outbox relay and, if the wired bus owns one (Kafka
// does; the in-process bus does not), its consumer loop — concurrently, until
// ctx is cancelled or either returns a hard error. The API runs this in a
// goroutine; the worker runs it as its main loop.
func (a *Application) RunBackground(ctx context.Context) error {
	r, ok := a.bus.(runner)
	if !ok {
		return a.relay.Run(ctx)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)
	go func() { errCh <- a.relay.Run(runCtx) }()
	go func() { errCh <- r.Run(runCtx) }()

	err := <-errCh
	cancel()
	<-errCh // wait for the second loop to unwind before returning
	return err
}

// DrainOutbox performs a single synchronous relay pass. Useful in tests and to
// flush pending events on shutdown.
func (a *Application) DrainOutbox(ctx context.Context) (int, error) {
	return a.relay.Drain(ctx)
}

// Close releases resources (DB pool, bus connections).
func (a *Application) Close() error {
	if c, ok := a.bus.(interface{ Close() error }); ok {
		_ = c.Close()
	}
	return a.persistence.close()
}

// registerOps mounts liveness/readiness probes.
func registerOps(mux *http.ServeMux, p persistence) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if p.db != nil {
			if err := p.db.PingContext(r.Context()); err != nil {
				writeJSON(w, http.StatusServiceUnavailable,
					map[string]string{"status": "unavailable", "detail": err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
}

// registerDocs mounts the embedded OpenAPI spec (generated from the proto —
// see api/openapi/openapi.go) and a Redoc viewer for it.
func registerDocs(mux *http.ServeMux) {
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(openapi.MyAppV1)
	})
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(docsHTML))
	})
}

const docsHTML = `<!doctype html>
<html>
  <head>
    <title>myapp API docs</title>
    <meta charset="utf-8"/>
  </head>
  <body>
    <redoc spec-url="/openapi.json"></redoc>
    <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
  </body>
</html>`

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
