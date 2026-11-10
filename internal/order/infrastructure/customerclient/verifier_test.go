package customerclient

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	customerv1 "github.com/example/myapp/api/proto/customer/v1"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

type fakeCustomers struct{ err error }

func (f fakeCustomers) GetCustomer(context.Context, *customerv1.GetCustomerRequest) (*customerv1.Customer, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &customerv1.Customer{Id: "c-1"}, nil
}

func TestVerify(t *testing.T) {
	unavailable := status.Error(codes.Unavailable, "down")
	plain := errors.New("boom")
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"exists", nil, nil},
		{"NotFound maps to domain ErrNotFound", status.Error(codes.NotFound, "x"), shareddomain.ErrNotFound},
		{"other gRPC errors pass through", unavailable, unavailable},
		{"non-gRPC errors pass through", plain, plain},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := New(fakeCustomers{err: c.err}).Verify(context.Background(), "c-1")
			if !errors.Is(err, c.want) || (c.want == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}
