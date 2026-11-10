package queries_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/example/myapp/internal/customer/application/dto"
	"github.com/example/myapp/internal/customer/application/mocks"
	"github.com/example/myapp/internal/customer/application/queries"
	shareddomain "github.com/example/myapp/internal/shared/domain"
	"github.com/example/myapp/pkg/pagination"
)

func TestGetCustomer_ReadsFromReadModel(t *testing.T) {
	rm := mocks.NewReadModel(t)
	rm.EXPECT().GetByID(mock.Anything, "c-1").Return(dto.Customer{ID: "c-1", Status: "active"}, nil).Once()
	rm.EXPECT().GetByID(mock.Anything, "nope").Return(dto.Customer{}, shareddomain.ErrNotFound).Once()

	h := queries.NewGetCustomerHandler(queries.Deps{ReadModel: rm})
	got, err := h.Handle(context.Background(), queries.GetCustomer{CustomerID: "c-1"})
	if err != nil || got.ID != "c-1" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := h.Handle(context.Background(), queries.GetCustomer{CustomerID: "nope"}); !errors.Is(err, shareddomain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListCustomers_PassesPageThrough(t *testing.T) {
	page := pagination.New(10, 20)
	want := pagination.Result[dto.Customer]{Items: []dto.Customer{{ID: "a"}}, Total: 21, Limit: 10, Offset: 20}
	rm := mocks.NewReadModel(t)
	rm.EXPECT().List(mock.Anything, page).Return(want, nil).Once()

	got, err := queries.NewListCustomersHandler(queries.Deps{ReadModel: rm}).
		Handle(context.Background(), queries.ListCustomers{Page: page})
	if err != nil || got.Total != 21 || len(got.Items) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
}
