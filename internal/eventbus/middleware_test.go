package eventbus_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/example/myapp/internal/eventbus"
)

func TestChain_AppliesMiddlewareOutermostFirst(t *testing.T) {
	var order []string
	mw := func(name string) eventbus.Middleware {
		return func(next eventbus.Handler) eventbus.Handler {
			return func(ctx context.Context, e eventbus.Event) error {
				order = append(order, name)
				return next(ctx, e)
			}
		}
	}
	h := eventbus.Chain(func(context.Context, eventbus.Event) error {
		order = append(order, "handler")
		return nil
	}, mw("a"), mw("b"))

	if err := h(context.Background(), eventbus.Event{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(order, ","); got != "a,b,handler" {
		t.Fatalf("order = %s, want a,b,handler", got)
	}
}

func TestRecover_TurnsPanicIntoError(t *testing.T) {
	var logs bytes.Buffer
	h := eventbus.Recover(slog.New(slog.NewTextHandler(&logs, nil)))(
		func(context.Context, eventbus.Event) error { panic("kaboom") },
	)
	err := h(context.Background(), eventbus.Event{Name: "x.v1.y"})
	if err == nil || !strings.Contains(err.Error(), "kaboom") {
		t.Fatalf("err = %v, want one mentioning the panic", err)
	}
	if !strings.Contains(logs.String(), "event handler panicked") {
		t.Fatalf("panic not logged: %s", logs.String())
	}
}

func TestLogging_PassesErrorThroughAndLogsIt(t *testing.T) {
	var logs bytes.Buffer
	want := errors.New("handler failed")
	h := eventbus.Logging(slog.New(slog.NewTextHandler(&logs, nil)))(
		func(context.Context, eventbus.Event) error { return want },
	)
	if err := h(context.Background(), eventbus.Event{Name: "x.v1.y", ID: "e-1"}); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if out := logs.String(); !strings.Contains(out, "event handler failed") || !strings.Contains(out, "event_id=e-1") {
		t.Fatalf("log = %s", out)
	}
}
