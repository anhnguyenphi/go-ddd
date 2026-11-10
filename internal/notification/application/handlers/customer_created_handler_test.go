package handlers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/notification/application/handlers"
	"github.com/example/myapp/internal/notification/domain/mocks"
)

func TestCustomerCreated_SendsWelcome(t *testing.T) {
	n := mocks.NewNotificationService(t)
	n.EXPECT().SendWelcome(mock.Anything, "ada@example.com", "Ada").Return(nil).Once()

	err := handlers.NewCustomerCreated(n).Handle(context.Background(), eventbus.Event{
		Name:    handlers.EventCustomerCreated,
		Payload: []byte(`{"id":"c-1","name":"Ada","email":"ada@example.com"}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

func TestCustomerCreated_UndecodablePayloadFailsWithoutSending(t *testing.T) {
	n := mocks.NewNotificationService(t) // no expectations: any call fails the test
	err := handlers.NewCustomerCreated(n).Handle(context.Background(), eventbus.Event{Payload: []byte(`not json`)})
	if err == nil {
		t.Fatal("want an error so the bus retries / dead-letters the message")
	}
}

func TestCustomerCreated_SendErrorPropagates(t *testing.T) {
	sendErr := errors.New("smtp down")
	n := mocks.NewNotificationService(t)
	n.EXPECT().SendWelcome(mock.Anything, mock.Anything, mock.Anything).Return(sendErr).Once()

	err := handlers.NewCustomerCreated(n).Handle(context.Background(), eventbus.Event{
		Payload: []byte(`{"id":"c-1","name":"Ada","email":"ada@example.com"}`),
	})
	if !errors.Is(err, sendErr) {
		t.Fatalf("err = %v, want %v", err, sendErr)
	}
}
