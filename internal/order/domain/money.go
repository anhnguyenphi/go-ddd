package domain

import (
	"regexp"

	shareddomain "github.com/example/myapp/internal/shared/domain"
)

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

// Money is an amount in the minor units of a currency (1999 EUR = 19.99 EUR).
// Integer minor units, never floats, so sums and comparisons are exact. The
// zero value is not valid money; build one with NewMoney.
type Money struct {
	amount   int64
	currency string
}

// NewMoney validates a positive amount and an ISO 4217 currency code (three
// uppercase letters). An order is never placed for nothing, so zero is
// rejected along with negative amounts.
func NewMoney(amount int64, currency string) (Money, error) {
	if amount <= 0 {
		return Money{}, shareddomain.Invalid("amount must be greater than zero")
	}
	if !currencyCode.MatchString(currency) {
		return Money{}, shareddomain.Invalid("currency must be a 3-letter ISO 4217 code, e.g. EUR")
	}
	return Money{amount: amount, currency: currency}, nil
}

// Amount returns the value in minor units.
func (m Money) Amount() int64 { return m.amount }

// Currency returns the ISO 4217 code.
func (m Money) Currency() string { return m.currency }

// IsZero reports whether m is the unbuilt zero value.
func (m Money) IsZero() bool { return m == Money{} }
