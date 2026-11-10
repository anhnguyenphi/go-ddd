package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// Unique returns a short random suffix. Every test that writes data uses it,
// so suites can run in parallel and re-run against shared environments.
func Unique() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// UniqueEmail returns an address no other test will use.
func UniqueEmail(prefix string) string {
	return prefix + "+" + Unique() + "@example.test"
}

// Customer is the REST representation of customer.v1.Customer.
type Customer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// CreateCustomer POSTs a customer with a unique email, expects 201 and
// returns the new id.
func (c *Client) CreateCustomer(t testing.TB, name string) (id, email string) {
	t.Helper()
	email = UniqueEmail("cust")
	res := c.Post(t, "/api/v1/customers", map[string]string{"name": name, "email": email}).
		Expect(t, http.StatusCreated)
	var out struct {
		ID string `json:"id"`
	}
	res.JSON(t, &out)
	if out.ID == "" {
		t.Fatalf("create customer: no id in %s", res.Body)
	}
	return out.ID, email
}

// AwaitCustomer polls the read model until the customer is visible and
// matches (when match is non-nil).
func (c *Client) AwaitCustomer(t testing.TB, id string, timeout time.Duration, match func(Customer) bool) Customer {
	t.Helper()
	var got Customer
	Eventually(t, timeout, func() (bool, string) {
		res, err := c.Try(http.MethodGet, "/api/v1/customers/"+id, nil)
		if err != nil {
			return false, err.Error()
		}
		if res.Status != http.StatusOK {
			return false, "status " + http.StatusText(res.Status) + ": " + string(res.Body)
		}
		got = Customer{}
		if err := json.Unmarshal(res.Body, &got); err != nil {
			return false, err.Error()
		}
		if match != nil && !match(got) {
			return false, "not matched yet: " + string(res.Body)
		}
		return true, ""
	})
	return got
}
