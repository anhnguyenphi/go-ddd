package httpx

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/example/myapp/internal/shared/infrastructure/id"
	"github.com/example/myapp/internal/shared/infrastructure/logging"
)

// RequestIDHeader is echoed back and attached to the context logger.
const RequestIDHeader = "X-Request-ID"

// Middleware is the standard net/http decorator shape.
type Middleware func(http.Handler) http.Handler

// Apply wraps h with mw so mw[0] runs first.
func Apply(h http.Handler, mw ...Middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// RequestID ensures every request carries an id and puts it on the context
// logger so downstream layers log it automatically.
func RequestID(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := r.Header.Get(RequestIDHeader)
			if rid == "" {
				rid = id.New()
			}
			w.Header().Set(RequestIDHeader, rid)
			ctx := logging.Into(r.Context(), logger.With("request_id", rid))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Recover turns a panic into a 500 and keeps the server alive.
func Recover() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if p := recover(); p != nil {
					logging.From(r.Context()).Error("panic serving request",
						"method", r.Method, "path", r.URL.Path, "panic", p)
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":{"code":"internal","message":"internal server error"}}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// AccessLog logs one line per request with status and latency.
func AccessLog() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			logging.From(r.Context()).Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"bytes", sw.written,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status  int
	written int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.written += n
	return n, err
}
