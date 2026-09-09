// Package queries contains the customer context's query handlers. They read from
// the ReadModel port only — never from the aggregate repository.
package queries

import "github.com/example/myapp/internal/customer/application"

// Deps is the shared dependency set for every query handler in this context.
type Deps struct {
	ReadModel application.ReadModel
}
