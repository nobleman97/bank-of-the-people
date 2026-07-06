package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobleman97/bank-of-the-people/services/ledger/internal/domain"
	"github.com/nobleman97/bank-of-the-people/services/ledger/internal/httpapi"
	"github.com/nobleman97/bank-of-the-people/services/ledger/internal/store"
)

type fakeStore struct {
	createAccountFn func(ctx context.Context, currency string) (domain.Account, error)
	getBalanceFn    func(ctx context.Context, accountID string) (int64, error)
	reserveFn       func(ctx context.Context, transferID, from, to string, amount int64) (store.ReserveResult, error)
	settleFn        func(ctx context.Context, transferID string) (store.SettleResult, error)
	transferStatusFn func(ctx context.Context, transferID string) (store.TransferStatus, error)
	pingFn          func(ctx context.Context) error
}

func (f *fakeStore) Ping(ctx context.Context) error { return f.pingFn(ctx) }
func (f *fakeStore) CreateAccount(ctx context.Context, currency string) (domain.Account, error) {
	return f.createAccountFn(ctx, currency)
}
func (f *fakeStore) GetBalance(ctx context.Context, accountID string) (int64, error) {
	return f.getBalanceFn(ctx, accountID)
}
func (f *fakeStore) Reserve(ctx context.Context, transferID, from, to string, amount int64) (store.ReserveResult, error) {
	return f.reserveFn(ctx, transferID, from, to, amount)
}
func (f *fakeStore) Settle(ctx context.Context, transferID string) (store.SettleResult, error) {
	return f.settleFn(ctx, transferID)
}
func (f *fakeStore) TransferStatus(ctx context.Context, transferID string) (store.TransferStatus, error) {
	return f.transferStatusFn(ctx, transferID)
}

func TestCreateAccount_Success(t *testing.T) {
	fs := &fakeStore{
		createAccountFn: func(ctx context.Context, currency string) (domain.Account, error) {
			return domain.Account{ID: "acc-1", Currency: currency}, nil
		},
	}
	router := httpapi.NewRouter(fs)

	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(`{"currency":"USD"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var got domain.Account
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "acc-1", got.ID)
}

func TestCreateAccount_MissingCurrency(t *testing.T) {
	fs := &fakeStore{}
	router := httpapi.NewRouter(fs)

	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReserve_InsufficientFunds_Returns422(t *testing.T) {
	fs := &fakeStore{
		reserveFn: func(ctx context.Context, transferID, from, to string, amount int64) (store.ReserveResult, error) {
			return store.ReserveResult{}, store.ErrInsufficientFunds
		},
	}
	router := httpapi.NewRouter(fs)

	body := `{"from_account_id":"a","to_account_id":"b","amount_minor":100}`
	req := httptest.NewRequest(http.MethodPost, "/transfers/t1/reserve", strings.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestReserve_Success_Returns201(t *testing.T) {
	fs := &fakeStore{
		reserveFn: func(ctx context.Context, transferID, from, to string, amount int64) (store.ReserveResult, error) {
			return store.ReserveResult{TransferID: transferID, Status: "reserved"}, nil
		},
	}
	router := httpapi.NewRouter(fs)

	body := `{"from_account_id":"a","to_account_id":"b","amount_minor":100}`
	req := httptest.NewRequest(http.MethodPost, "/transfers/t1/reserve", strings.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	// The response body must use the project's snake_case keys (the wire contract the
	// api service codes against in P3), not Go's PascalCase field names.
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "t1", got["transfer_id"])
	assert.Equal(t, "reserved", got["status"])
}

func TestSettle_UnknownTransfer_Returns404(t *testing.T) {
	fs := &fakeStore{
		settleFn: func(ctx context.Context, transferID string) (store.SettleResult, error) {
			return store.SettleResult{}, store.ErrTransferNotFound
		},
	}
	router := httpapi.NewRouter(fs)

	req := httptest.NewRequest(http.MethodPost, "/transfers/missing/settle", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestReadyz_DependencyUnavailable_Returns503(t *testing.T) {
	fs := &fakeStore{
		pingFn: func(ctx context.Context) error { return errors.New("connection refused") },
	}
	router := httpapi.NewRouter(fs)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHealthz_AlwaysReturns200(t *testing.T) {
	router := httpapi.NewRouter(&fakeStore{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}
