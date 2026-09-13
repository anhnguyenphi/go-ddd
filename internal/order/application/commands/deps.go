// Package commands contains the order context's command handlers — one
// use-case per file. Handlers orchestrate; they hold no business rules (those
// belong to the aggregate) and no transport concerns.
package commands

import (
	"github.com/example/myapp/internal/order/application"
	"github.com/example/myapp/internal/order/domain"
	sharedapp "github.com/example/myapp/internal/shared/application"
)

// Deps is the shared dependency set for every command handler in this
// context. Every field is a port (interface) — concrete adapters are chosen
// in the bootstrap, never referenced here.
type Deps struct {
	Repo     domain.Repository
	Clock    sharedapp.Clock
	IDs      sharedapp.IDGenerator
	Customer application.CustomerVerifier
}
