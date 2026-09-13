// Package grpc is the customer context's gRPC delivery adapter. It mirrors
// interfaces/http: it translates requests into the same application
// commands/queries and results/errors into gRPC responses/statuses. It
// contains no business logic.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	customerv1 "github.com/example/myapp/api/proto/customer/v1"
	"github.com/example/myapp/internal/customer/application/commands"
	"github.com/example/myapp/internal/customer/application/dto"
	"github.com/example/myapp/internal/customer/application/queries"
	"github.com/example/myapp/internal/platform/grpcx"
	shareddomain "github.com/example/myapp/internal/shared/domain"
	"github.com/example/myapp/pkg/pagination"
)

// Server implements customerv1.CustomerServiceServer over the application layer.
type Server struct {
	customerv1.UnimplementedCustomerServiceServer

	create      *commands.CreateCustomerHandler
	changeEmail *commands.ChangeCustomerEmailHandler
	get         *queries.GetCustomerHandler
	list        *queries.ListCustomersHandler
}

// NewServer wires the delivery adapter to the application layer.
func NewServer(
	create *commands.CreateCustomerHandler,
	changeEmail *commands.ChangeCustomerEmailHandler,
	get *queries.GetCustomerHandler,
	list *queries.ListCustomersHandler,
) *Server {
	return &Server{create: create, changeEmail: changeEmail, get: get, list: list}
}

func (s *Server) CreateCustomer(ctx context.Context, req *customerv1.CreateCustomerRequest) (*customerv1.CreateCustomerResponse, error) {
	res, err := s.create.Handle(ctx, commands.CreateCustomer{
		Name:  req.GetName(),
		Email: req.GetEmail(),
	})
	if err != nil {
		return nil, mapError(err)
	}
	// gRPC has no notion of "201 Created" or a Location header; these are
	// consumed by grpcx.StatusCodeOption / grpcx.OutgoingHeaderMatcher when
	// this RPC is transcoded to REST by grpc-gateway. Plain gRPC callers never
	// see this metadata.
	_ = grpc.SetHeader(ctx, metadata.Pairs(
		grpcx.HTTPStatusMetadataKey, "201",
		grpcx.LocationMetadataKey, "/api/v1/customers/"+res.CustomerID,
	))
	return &customerv1.CreateCustomerResponse{Id: res.CustomerID}, nil
}

func (s *Server) GetCustomer(ctx context.Context, req *customerv1.GetCustomerRequest) (*customerv1.Customer, error) {
	res, err := s.get.Handle(ctx, queries.GetCustomer{CustomerID: req.GetId()})
	if err != nil {
		return nil, mapError(err)
	}
	return toProto(res), nil
}

func (s *Server) ListCustomers(ctx context.Context, req *customerv1.ListCustomersRequest) (*customerv1.ListCustomersResponse, error) {
	res, err := s.list.Handle(ctx, queries.ListCustomers{
		Page: pagination.New(int(req.GetLimit()), int(req.GetOffset())),
	})
	if err != nil {
		return nil, mapError(err)
	}
	items := make([]*customerv1.Customer, len(res.Items))
	for i, c := range res.Items {
		items[i] = toProto(c)
	}
	return &customerv1.ListCustomersResponse{
		Items:  items,
		Total:  int32(res.Total),
		Limit:  int32(res.Limit),
		Offset: int32(res.Offset),
	}, nil
}

func (s *Server) ChangeCustomerEmail(ctx context.Context, req *customerv1.ChangeCustomerEmailRequest) (*customerv1.ChangeCustomerEmailResponse, error) {
	if _, err := s.changeEmail.Handle(ctx, commands.ChangeCustomerEmail{
		CustomerID: req.GetId(),
		NewEmail:   req.GetEmail(),
	}); err != nil {
		return nil, mapError(err)
	}
	return &customerv1.ChangeCustomerEmailResponse{}, nil
}

func toProto(c dto.Customer) *customerv1.Customer {
	return &customerv1.Customer{
		Id:        c.ID,
		Name:      c.Name,
		Email:     c.Email,
		Status:    c.Status,
		CreatedAt: timestamppb.New(c.CreatedAt),
	}
}

// mapError mirrors interfaces/http/response.go's writeError, translating the
// same domain sentinel errors into gRPC status codes instead of HTTP ones.
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
