package httpapi

import "net/http"

// NewRouter wires every internal ledger route to its handler. cmd/ledger/main.go
// (Task 7) calls this with a *store.Store.
func NewRouter(st Store) http.Handler {
	h := &handlers{store: st}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /accounts", h.createAccount)
	mux.HandleFunc("GET /accounts/{id}/balance", h.getBalance)
	mux.HandleFunc("POST /transfers/{id}/reserve", h.reserve)
	mux.HandleFunc("POST /transfers/{id}/settle", h.settle)
	mux.HandleFunc("GET /transfers/{id}", h.transferStatus)
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /readyz", h.readyz)
	return mux
}
