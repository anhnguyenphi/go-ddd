package application

import "context"

// Command is a request to change state. It is a plain data struct named in the
// imperative ("CreateCustomer"); it carries no behaviour.
type Command interface{}

// CommandHandler executes exactly one command type. R is the result payload
// (often just an identifier, or application.None for fire-and-forget).
type CommandHandler[C Command, R any] interface {
	Handle(ctx context.Context, cmd C) (R, error)
}

// None is the result type for commands that return nothing meaningful.
type None struct{}
