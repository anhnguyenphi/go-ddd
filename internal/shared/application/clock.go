package application

import "time"

// Clock is the application layer's port for "now". Implementations live in
// shared/infrastructure/clock (System for production, Fixed for tests).
type Clock interface {
	Now() time.Time
}

// IDGenerator is the port for minting new aggregate identifiers.
// Implementation: shared/infrastructure/id (UUID for production, Fixed for tests).
type IDGenerator interface {
	NewID() string
}
