package application

import "context"

// Query is a request for data that never changes state.
type Query interface{}

// QueryHandler answers exactly one query type with a read model / DTO.
type QueryHandler[Q Query, R any] interface {
	Handle(ctx context.Context, query Q) (R, error)
}
