// Package identity is a placeholder bounded context (see internal/order/doc.go
// for the conventions). It would own users, credentials, sessions and access
// policy, exposing authN/authZ to the delivery layer and publishing
// identity.v1.* events (e.g. identity.v1.user_registered).
package identity
