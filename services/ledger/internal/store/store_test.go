package store_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobleman97/bank-of-the-people/services/ledger/internal/domain"
	"github.com/nobleman97/bank-of-the-people/services/ledger/internal/store"
)

func TestCreateAccount_And_GetBalance_StartsAtZero(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	st := store.New(db)

	acc, err := st.CreateAccount(ctx, "USD")
	require.NoError(t, err)
	assert.NotEmpty(t, acc.ID)
	assert.Equal(t, "USD", acc.Currency)

	balance, err := st.GetBalance(ctx, acc.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), balance)
}

func TestSeedSystemAccount_ExistsAfterMigrate(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	// Query the accounts table directly: GetBalance only sums entries and returns 0 for
	// any id (seeded or not), so it cannot prove the seed row exists. Migration 000002 must.
	var exists bool
	err := db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE id = $1)`, domain.SystemAccountID).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists, "migration 000002 should seed the system account row")
}

func TestPing_Succeeds(t *testing.T) {
	db := newTestDB(t)
	st := store.New(db)
	require.NoError(t, st.Ping(context.Background()))
}

func TestReserve_InsufficientFunds(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	st := store.New(db)

	acc, err := st.CreateAccount(ctx, "USD")
	require.NoError(t, err)

	_, err = st.Reserve(ctx, "t-insufficient", acc.ID, domain.SystemAccountID, 100)
	require.ErrorIs(t, err, store.ErrInsufficientFunds)
}

func TestReserve_IdempotentOnReplay(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	st := store.New(db)

	acc, err := st.CreateAccount(ctx, "USD")
	require.NoError(t, err)
	_, err = st.Reserve(ctx, "t-fund", domain.SystemAccountID, acc.ID, 1000)
	require.NoError(t, err)

	first, err := st.Reserve(ctx, "t-replay", acc.ID, domain.SystemAccountID, 200)
	require.NoError(t, err)
	assert.Equal(t, "reserved", first.Status)

	second, err := st.Reserve(ctx, "t-replay", acc.ID, domain.SystemAccountID, 200)
	require.NoError(t, err)
	assert.Equal(t, "already_pending", second.Status)

	balance, err := st.GetBalance(ctx, acc.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(800), balance, "replaying the same transfer_id must not reserve twice")
}

func TestConcurrentReserve_PreventsOverdraft(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	st := store.New(db)

	acc, err := st.CreateAccount(ctx, "USD")
	require.NoError(t, err)
	_, err = st.Reserve(ctx, "t-fund-concurrent", domain.SystemAccountID, acc.ID, 1000)
	require.NoError(t, err)

	const attempts = 10
	const amount = 600 // two concurrent reserves of 600 cannot both succeed against a balance of 1000

	var wg sync.WaitGroup
	errs := make([]error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := st.Reserve(ctx, fmt.Sprintf("t-concurrent-%d", i), acc.ID, domain.SystemAccountID, amount)
			errs[i] = err
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else {
			assert.ErrorIs(t, err, store.ErrInsufficientFunds)
		}
	}
	assert.Equal(t, 1, successes, "exactly one reservation of 600 should succeed against a balance of 1000")

	balance, err := st.GetBalance(ctx, acc.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(400), balance, "balance must reflect exactly one successful reservation, proving FOR UPDATE serialized the race")
}

// TestConcurrentBidirectionalReserve_NoDeadlock drives many opposite-direction transfers
// (A->B and B->A) concurrently. Locking only the source account would let these deadlock
// (ABBA: SQLSTATE 40P01); canonical sorted lock ordering must serialize them cleanly so
// every reservation succeeds with no deadlock error.
func TestConcurrentBidirectionalReserve_NoDeadlock(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	st := store.New(db)

	a, err := st.CreateAccount(ctx, "USD")
	require.NoError(t, err)
	b, err := st.CreateAccount(ctx, "USD")
	require.NoError(t, err)

	// Fund both generously so insufficient-funds never fires — only a deadlock could fail this.
	_, err = st.Reserve(ctx, "fund-a", domain.SystemAccountID, a.ID, 100000)
	require.NoError(t, err)
	_, err = st.Reserve(ctx, "fund-b", domain.SystemAccountID, b.ID, 100000)
	require.NoError(t, err)

	const pairs = 25
	var wg sync.WaitGroup
	errs := make([]error, pairs*2)
	for i := 0; i < pairs; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			_, e := st.Reserve(ctx, fmt.Sprintf("ab-%d", i), a.ID, b.ID, 10)
			errs[i*2] = e
		}(i)
		go func(i int) {
			defer wg.Done()
			_, e := st.Reserve(ctx, fmt.Sprintf("ba-%d", i), b.ID, a.ID, 10)
			errs[i*2+1] = e
		}(i)
	}
	wg.Wait()

	for _, e := range errs {
		require.NoError(t, e, "canonical lock ordering must prevent ABBA deadlocks on bidirectional transfers")
	}
}

func TestSettle_IdempotentOnReplay(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	st := store.New(db)

	acc, err := st.CreateAccount(ctx, "USD")
	require.NoError(t, err)
	_, err = st.Reserve(ctx, "t-settle", domain.SystemAccountID, acc.ID, 500)
	require.NoError(t, err)

	first, err := st.Settle(ctx, "t-settle")
	require.NoError(t, err)
	assert.Equal(t, "settled", first.Status)

	second, err := st.Settle(ctx, "t-settle")
	require.NoError(t, err)
	assert.Equal(t, "already_settled", second.Status)

	balance, err := st.GetBalance(ctx, acc.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(500), balance, "settling twice must not move funds twice")
}

func TestSettle_UnknownTransfer(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	st := store.New(db)

	_, err := st.Settle(ctx, "does-not-exist")
	assert.ErrorIs(t, err, store.ErrTransferNotFound)
}

func TestTransferStatus_ReflectsLifecycle(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	st := store.New(db)

	acc, err := st.CreateAccount(ctx, "USD")
	require.NoError(t, err)
	_, err = st.Reserve(ctx, "t-status", domain.SystemAccountID, acc.ID, 300)
	require.NoError(t, err)

	status, err := st.TransferStatus(ctx, "t-status")
	require.NoError(t, err)
	assert.Equal(t, "pending", status.Status)

	_, err = st.Settle(ctx, "t-status")
	require.NoError(t, err)

	status, err = st.TransferStatus(ctx, "t-status")
	require.NoError(t, err)
	assert.Equal(t, "settled", status.Status)
}

// TestConcurrentSettle_IdempotentUnderRace fires many Settle calls for the same transfer_id
// at once (at-least-once redelivery, ADR-0003). The SELECT ... FOR UPDATE on the transfer
// row must serialize them so exactly one settles and the rest report already_settled — with
// a single settlements row and no second movement of funds.
func TestConcurrentSettle_IdempotentUnderRace(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	st := store.New(db)

	acc, err := st.CreateAccount(ctx, "USD")
	require.NoError(t, err)
	_, err = st.Reserve(ctx, "t-race-settle", domain.SystemAccountID, acc.ID, 500)
	require.NoError(t, err)

	const attempts = 20
	var wg sync.WaitGroup
	results := make([]store.SettleResult, attempts)
	errs := make([]error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = st.Settle(ctx, "t-race-settle")
		}(i)
	}
	wg.Wait()

	settled, alreadySettled := 0, 0
	for i, e := range errs {
		require.NoError(t, e)
		switch results[i].Status {
		case "settled":
			settled++
		case "already_settled":
			alreadySettled++
		default:
			t.Fatalf("unexpected settle status %q", results[i].Status)
		}
	}
	assert.Equal(t, 1, settled, "exactly one concurrent Settle should finalize the transfer")
	assert.Equal(t, attempts-1, alreadySettled, "every other concurrent Settle should be a no-op")

	var settlementRows int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM settlements WHERE transfer_id = $1`, "t-race-settle").Scan(&settlementRows))
	assert.Equal(t, 1, settlementRows, "duplicate delivery must not create a second settlement row")

	balance, err := st.GetBalance(ctx, acc.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(500), balance, "settling under a race must not move funds more than once")
}
