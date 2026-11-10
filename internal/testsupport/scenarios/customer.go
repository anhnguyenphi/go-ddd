package scenarios

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	customerv1 "github.com/example/myapp/api/proto/customer/v1"
	"github.com/example/myapp/internal/testsupport/api"
)

// Health: liveness and readiness probes answer 200.
func Health(t *testing.T, tg Target) {
	if m := tg.API.Get(t, "/healthz").Expect(t, http.StatusOK).Map(t); m["status"] != "ok" {
		t.Fatalf("/healthz = %v", m)
	}
	if m := tg.API.Get(t, "/readyz").Expect(t, http.StatusOK).Map(t); m["status"] != "ready" {
		t.Fatalf("/readyz = %v", m)
	}
}

// CustomerLifecycle: create -> visible in the read model -> change email ->
// change visible; the old email becomes free and the new one taken.
func CustomerLifecycle(t *testing.T, tg Target) {
	c := tg.API
	id, email := c.CreateCustomer(t, "Lifecycle")

	got := c.AwaitCustomer(t, id, tg.ReadTimeout, nil)
	if got.Email != email || got.Status != "active" || got.Name != "Lifecycle" || got.CreatedAt == "" {
		t.Fatalf("read model = %+v", got)
	}

	newEmail := api.UniqueEmail("changed")
	c.Patch(t, "/api/v1/customers/"+id+"/email", map[string]string{"email": newEmail}).Expect(t, http.StatusOK)
	c.AwaitCustomer(t, id, tg.ReadTimeout, func(cu api.Customer) bool { return cu.Email == newEmail })

	// Same email again is a no-op, not a conflict.
	c.Patch(t, "/api/v1/customers/"+id+"/email", map[string]string{"email": newEmail}).Expect(t, http.StatusOK)

	// The new address is taken; the old one is free again.
	c.Post(t, "/api/v1/customers", map[string]string{"name": "Dup", "email": newEmail}).Expect(t, http.StatusConflict)
	c.Post(t, "/api/v1/customers", map[string]string{"name": "Reuse", "email": email}).Expect(t, http.StatusCreated)
}

// CustomerValidation: bad input is a 400, duplicates are a 409, and nothing
// is created.
func CustomerValidation(t *testing.T, tg Target) {
	c := tg.API
	c.Post(t, "/api/v1/customers", map[string]string{"name": "Bad", "email": "not-an-email"}).Expect(t, http.StatusBadRequest)
	c.Post(t, "/api/v1/customers", map[string]string{}).Expect(t, http.StatusBadRequest)
	c.Post(t, "/api/v1/customers", map[string]string{"name": "", "email": api.UniqueEmail("noname")}).Expect(t, http.StatusBadRequest)

	_, email := c.CreateCustomer(t, "Original")
	c.Post(t, "/api/v1/customers", map[string]string{"name": "Dup", "email": email}).Expect(t, http.StatusConflict)

	id, _ := c.CreateCustomer(t, "Target")
	c.Patch(t, "/api/v1/customers/"+id+"/email", map[string]string{"email": "nope"}).Expect(t, http.StatusBadRequest)
	c.Patch(t, "/api/v1/customers/"+id+"/email", map[string]string{"email": email}).Expect(t, http.StatusConflict)
}

// CustomerNotFound: unknown ids are a 404 on read and write.
func CustomerNotFound(t *testing.T, tg Target) {
	missing := "missing-" + api.Unique()
	tg.API.Get(t, "/api/v1/customers/"+missing).Expect(t, http.StatusNotFound)
	tg.API.Patch(t, "/api/v1/customers/"+missing+"/email", map[string]string{"email": api.UniqueEmail("x")}).
		Expect(t, http.StatusNotFound)
}

// CustomerList: a new customer shows up in the paginated list, and the
// pagination envelope is consistent. Other data may exist (shared envs).
func CustomerList(t *testing.T, tg Target) {
	c := tg.API
	id, _ := c.CreateCustomer(t, "Listed")
	c.AwaitCustomer(t, id, tg.ReadTimeout, nil)

	var page struct {
		Items  []api.Customer `json:"items"`
		Total  int            `json:"total"`
		Limit  int            `json:"limit"`
		Offset int            `json:"offset"`
	}
	c.Get(t, "/api/v1/customers?limit=2&offset=0").Expect(t, http.StatusOK).JSON(t, &page)
	if page.Limit != 2 || page.Offset != 0 || len(page.Items) > 2 || page.Total < 1 {
		t.Fatalf("page envelope = limit %d offset %d items %d total %d", page.Limit, page.Offset, len(page.Items), page.Total)
	}

	// Walk all pages until the new customer is found.
	for offset := 0; offset < page.Total+100; offset += 100 {
		var p struct {
			Items []api.Customer `json:"items"`
		}
		c.Get(t, "/api/v1/customers?limit=100&offset="+strconv.Itoa(offset)).Expect(t, http.StatusOK).JSON(t, &p)
		for _, it := range p.Items {
			if it.ID == id {
				return
			}
		}
		if len(p.Items) == 0 {
			break
		}
	}
	t.Fatalf("customer %s not found in list (total %d)", id, page.Total)
}

// CustomerGRPC: the same contract over gRPC, including status codes.
func CustomerGRPC(t *testing.T, tg Target) {
	if tg.Customers == nil {
		t.Skip("no gRPC target")
	}
	ctx := context.Background()
	email := api.UniqueEmail("grpc")
	created, err := tg.Customers.CreateCustomer(ctx, &customerv1.CreateCustomerRequest{Name: "Grpc", Email: email}, waitForReady)
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	_, err = tg.Customers.CreateCustomer(ctx, &customerv1.CreateCustomerRequest{Name: "Grpc", Email: email})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("duplicate CreateCustomer code = %v, want AlreadyExists", status.Code(err))
	}
	_, err = tg.Customers.CreateCustomer(ctx, &customerv1.CreateCustomerRequest{Name: "Grpc", Email: "bad"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid CreateCustomer code = %v, want InvalidArgument", status.Code(err))
	}

	tg.API.AwaitCustomer(t, created.GetId(), tg.ReadTimeout, nil)
	got, err := tg.Customers.GetCustomer(ctx, &customerv1.GetCustomerRequest{Id: created.GetId()})
	if err != nil || got.GetEmail() != email {
		t.Fatalf("GetCustomer = %v, %v", got, err)
	}
	_, err = tg.Customers.GetCustomer(ctx, &customerv1.GetCustomerRequest{Id: "missing-" + api.Unique()})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("GetCustomer unknown code = %v, want NotFound", status.Code(err))
	}
}

// OpenAPI: the generated OpenAPI document is served and lists every
// context's REST surface.
func OpenAPI(t *testing.T, tg Target) {
	m := tg.API.Get(t, "/openapi.json").Expect(t, http.StatusOK).Map(t)
	paths, ok := m["paths"].(map[string]any)
	if !ok {
		t.Fatalf("/openapi.json has no paths object: %v", m)
	}
	for _, p := range []string{"/api/v1/customers", "/api/v1/orders"} {
		if _, ok := paths[p]; !ok {
			t.Errorf("/openapi.json is missing path %s", p)
		}
	}
}
