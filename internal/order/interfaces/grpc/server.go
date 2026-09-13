// Package grpc is the order context's gRPC delivery adapter. It translates
// requests into application commands and results/errors into gRPC
// responses/statuses. It contains no business logic.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/example/myapp/api/proto/order/v1"
	"github.com/example/myapp/internal/order/application/commands"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// Server implements orderv1.OrderServiceServer over the application layer.
type Server struct {
	orderv1.UnimplementedOrderServiceServer

	placeOrder *commands.PlaceOrderHandler
}

// NewServer wires the delivery adapter to the application layer.
func NewServer(placeOrder *commands.PlaceOrderHandler) *Server {
	return &Server{placeOrder: placeOrder}
}

func (s *Server) PlaceOrder(ctx context.Context, req *orderv1.PlaceOrderRequest) (*orderv1.PlaceOrderResponse, error) {
	res, err := s.placeOrder.Handle(ctx, commands.PlaceOrder{CustomerID: req.GetCustomerId()})
	if err != nil {
		return nil, mapError(err)
	}
	return &orderv1.PlaceOrderResponse{Id: res.OrderID}, nil
}

// mapError mirrors customer/interfaces/grpc/server.go's mapError, translating
// the same domain sentinel errors into gRPC status codes. NotFound here means
// the customer the order was placed for does not exist — see
// infrastructure/customerclient.
func mapError(err error) error {
	switch {
	case errors.Is(err, shareddomain.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, shareddomain.ErrConflict):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, shareddomain.ErrValidation):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, shareddomain.ErrPermission):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, "internal server error")
	}
}
