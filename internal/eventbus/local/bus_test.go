package local_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/eventbus/local"
)

func newBus() *local.Bus {
	return local.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestBus_FansOutToAllSubscribers(t *testing.T) {
	b := newBus()
	var a, c int
	_ = b.Subscribe("x", func(context.Context, eventbus.Event) error { a++; return nil })
	_ = b.Subscribe("x", func(context.Context, eventbus.Event) error { c++; return nil })
	_ = b.Subscribe("y", func(context.Context, eventbus.Event) error { t.Fatal("wrong topic"); return nil })

	if err := b.Publish(context.Background(), eventbus.Event{Name: "x"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if a != 1 || c != 1 {
		t.Fatalf("handler counts a=%d c=%d, want 1/1", a, c)
	}
}

func TestBus_JoinsHandlerErrorsAndStillRunsAll(t *testing.T) {
	b := newBus()
	sentinel := errors.New("boom")
	ran := 0
	_ = b.Subscribe("x", func(context.Context, eventbus.Event) error { ran++; return sentinel })
	_ = b.Subscribe("x", func(context.Context, eventbus.Event) error { ran++; return nil })

	err := b.Publish(context.Background(), eventbus.Event{Name: "x"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want to wrap sentinel", err)
	}
	if ran != 2 {
		t.Fatalf("ran = %d, want 2 (one failure must not stop the others)", ran)
	}
}

func TestBus_NoSubscribersIsNotAnError(t *testing.T) {
	if err := newBus().Publish(context.Background(), eventbus.Event{Name: "nobody"}); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}
