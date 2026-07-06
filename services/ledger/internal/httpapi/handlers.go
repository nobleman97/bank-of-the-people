package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/nobleman97/bank-of-the-people/services/ledger/internal/domain"
	"github.com/nobleman97/bank-of-the-people/services/ledger/internal/store"
)

// Store is the dependency httpapi needs from the ledger's Postgres store. *store.Store
// (services/ledger/internal/store) satisfies it; tests use a fake.
type Store interface {
	Ping(ctx context.Context) error
	CreateAccount(ctx context.Context, currency string) (domain.Account, error)
	GetBalance(ctx context.Context, accountID string) (int64, error)
	Reserve(ctx context.Context, transferID, fromAccountID, toAccountID string, amountMinor int64) (store.ReserveResult, error)
	Settle(ctx context.Context, transferID string) (store.SettleResult, error)
	TransferStatus(ctx context.Context, transferID string) (store.TransferStatus, error)
}

type handlers struct {
	store Store
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}

type createAccountRequest struct {
	Currency string `json:"currency"`
}

func (h *handlers) createAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Currency == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "currency is required")
		return
	}
	acc, err := h.store.CreateAccount(r.Context(), req.Currency)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, acc)
}

func (h *handlers) getBalance(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	balance, err := h.store.GetBalance(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account_id": accountID, "balance_minor": balance})
}

type reserveRequest struct {
	FromAccountID string `json:"from_account_id"`
	ToAccountID   string `json:"to_account_id"`
	AmountMinor   int64  `json:"amount_minor"`
}

func (h *handlers) reserve(w http.ResponseWriter, r *http.Request) {
	transferID := r.PathValue("id")
	var req reserveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.FromAccountID == "" || req.ToAccountID == "" || req.AmountMinor <= 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"from_account_id, to_account_id, and a positive amount_minor are required")
		return
	}
	result, err := h.store.Reserve(r.Context(), transferID, req.FromAccountID, req.ToAccountID, req.AmountMinor)
	if errors.Is(err, store.ErrInsufficientFunds) {
		writeError(w, http.StatusUnprocessableEntity, "insufficient_funds", "source account balance is too low")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *handlers) settle(w http.ResponseWriter, r *http.Request) {
	transferID := r.PathValue("id")
	result, err := h.store.Settle(r.Context(), transferID)
	if errors.Is(err, store.ErrTransferNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "transfer not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handlers) transferStatus(w http.ResponseWriter, r *http.Request) {
	transferID := r.PathValue("id")
	status, err := h.store.TransferStatus(r.Context(), transferID)
	if errors.Is(err, store.ErrTransferNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "transfer not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *handlers) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) readyz(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "database unreachable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
