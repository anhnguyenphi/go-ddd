package http

import (
	"net/http"

	"github.com/example/myapp/internal/customer/application/commands"
	"github.com/example/myapp/internal/customer/application/queries"
	"github.com/example/myapp/pkg/pagination"
)

// Handler holds the application entrypoints this adapter drives.
type Handler struct {
	create      *commands.CreateCustomerHandler
	changeEmail *commands.ChangeCustomerEmailHandler
	get         *queries.GetCustomerHandler
	list        *queries.ListCustomersHandler
}

// NewHandler wires the delivery adapter to the application layer.
func NewHandler(
	create *commands.CreateCustomerHandler,
	changeEmail *commands.ChangeCustomerEmailHandler,
	get *queries.GetCustomerHandler,
	list *queries.ListCustomersHandler,
) *Handler {
	return &Handler{create: create, changeEmail: changeEmail, get: get, list: list}
}

type createCustomerRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type createCustomerResponse struct {
	ID string `json:"id"`
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createCustomerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := h.create.Handle(r.Context(), commands.CreateCustomer{
		Name:  req.Name,
		Email: req.Email,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/customers/"+res.CustomerID)
	writeJSON(w, http.StatusCreated, createCustomerResponse{ID: res.CustomerID})
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	res, err := h.get.Handle(r.Context(), queries.GetCustomer{
		CustomerID: r.PathValue("id"),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	res, err := h.list.Handle(r.Context(), queries.ListCustomers{
		Page: pagination.FromQuery(r.URL.Query()),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type changeEmailRequest struct {
	Email string `json:"email"`
}

func (h *Handler) handleChangeEmail(w http.ResponseWriter, r *http.Request) {
	var req changeEmailRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if _, err := h.changeEmail.Handle(r.Context(), commands.ChangeCustomerEmail{
		CustomerID: r.PathValue("id"),
		NewEmail:   req.Email,
	}); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
