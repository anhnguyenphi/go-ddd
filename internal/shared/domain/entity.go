package domain

// Entity is a marker for objects defined by a thread of identity rather than by
// their attributes. Two entities are equal iff their identities are equal.
//
// It is intentionally minimal: identity types are context-specific value objects
// (e.g. customer.CustomerID), so this interface only pins down the contract that
// generic infrastructure can rely on.
type Entity[ID comparable] interface {
	ID() ID
}
