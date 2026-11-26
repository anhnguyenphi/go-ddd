package domain

import (
	"strings"

	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// ProductName is what an order buys, as free text (there is no product
// catalogue). It is trimmed and never blank; build one with NewProductName.
type ProductName struct{ value string }

// NewProductName trims v and rejects a blank name.
func NewProductName(v string) (ProductName, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return ProductName{}, shareddomain.Invalid("order requires a product name")
	}
	return ProductName{value: v}, nil
}

// String returns the name.
func (n ProductName) String() string { return n.value }
