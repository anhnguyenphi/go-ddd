package http

import "net/http"

// Register mounts the customer routes on mux. Method+pattern routing needs
// Go 1.22+. The {id} wildcard is read with r.PathValue("id").
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/customers", h.handleCreate)
	mux.HandleFunc("GET /api/v1/customers", h.handleList)
	mux.HandleFunc("GET /api/v1/customers/{id}", h.handleGet)
	mux.HandleFunc("PATCH /api/v1/customers/{id}/email", h.handleChangeEmail)
}
