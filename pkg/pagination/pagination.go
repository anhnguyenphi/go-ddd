// Package pagination holds transport-agnostic paging primitives. It lives under
// pkg/ because it carries no business meaning and is safe to import anywhere,
// including from outside this module.
package pagination

import (
	"net/url"
	"strconv"
)

// Defaults and bounds for page size.
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Page is a validated limit/offset window.
type Page struct {
	Limit  int
	Offset int
}

// New clamps limit to [1, MaxLimit] and offset to >= 0.
func New(limit, offset int) Page {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return Page{Limit: limit, Offset: offset}
}

// FromQuery reads ?limit= and ?offset= from a URL query.
func FromQuery(q url.Values) Page {
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	return New(limit, offset)
}

// Result wraps a page of items with the total count.
type Result[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}
