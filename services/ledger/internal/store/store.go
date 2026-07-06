package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/nobleman97/bank-of-the-people/services/ledger/internal/domain"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrTransferNotFound  = errors.New("transfer not found")
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) CreateAccount(ctx context.Context, currency string) (domain.Account, error) {
	id := uuid.NewString()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO accounts (id, currency) VALUES ($1, $2)`, id, currency)
	if err != nil {
		return domain.Account{}, fmt.Errorf("create account: %w", err)
	}
	return domain.Account{ID: id, Currency: currency}, nil
}

func (s *Store) GetBalance(ctx context.Context, accountID string) (int64, error) {
	var balance int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE direction WHEN 'credit' THEN amount_minor ELSE -amount_minor END), 0)
		FROM entries WHERE account_id = $1`, accountID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("get balance: %w", err)
	}
	return balance, nil
}

type ReserveResult struct {
	TransferID string `json:"transfer_id"`
	Status     string `json:"status"`
}

// Reserve inserts a balanced pending entry pair (domain.ReservationEntries) for
// fromAccountID -> toAccountID inside one transaction, after locking the source
// account row with SELECT ... FOR UPDATE (ADR-0003) — this is what serializes
// concurrent transfers on the same account and prevents overdraft. Idempotent on
// transferID: replaying the same transfer_id returns the existing status instead of
// reserving twice.
func (s *Store) Reserve(ctx context.Context, transferID, fromAccountID, toAccountID string, amountMinor int64) (ReserveResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReserveResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Lock BOTH participating account rows FOR UPDATE, in a deterministic (sorted-by-id)
	// order, before the INSERT into transfers below. Two deadlocks are avoided here:
	//   1. Same-row self-upgrade: the transfers INSERT's foreign keys implicitly take a
	//      FOR KEY SHARE lock on both account rows. Acquiring FOR UPDATE first (rather
	//      than after the insert) stops two concurrent same-source transfers from each
	//      holding FOR KEY SHARE and then both blocking to upgrade to FOR UPDATE
	//      (Postgres SQLSTATE 40P01, deadlock detected).
	//   2. Cross-account ABBA: two opposite-direction transfers (A->B and B->A) racing
	//      would otherwise lock their own source then block on the other's row in
	//      opposite orders. Locking both rows in a single global canonical order (sorted
	//      account id) means every transaction takes these locks in the same sequence, so
	//      no cycle can form.
	// The lock on the source row is also the overdraft guard (ADR-0003): the funds check
	// below runs while this exclusive lock is held, serializing same-source reservations.
	lockIDs := []string{fromAccountID, toAccountID}
	sort.Strings(lockIDs)
	for i, id := range lockIDs {
		if i > 0 && id == lockIDs[i-1] {
			continue // self-transfer (from == to): the row is already locked
		}
		var lockedID string
		if err := tx.QueryRowContext(ctx,
			`SELECT id FROM accounts WHERE id = $1 FOR UPDATE`, id,
		).Scan(&lockedID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ReserveResult{}, fmt.Errorf("account not found: %s", id)
			}
			return ReserveResult{}, fmt.Errorf("lock account: %w", err)
		}
	}

	var insertedID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO transfers (id, from_account_id, to_account_id, amount_minor, status)
		VALUES ($1, $2, $3, $4, 'pending')
		ON CONFLICT (id) DO NOTHING
		RETURNING id`, transferID, fromAccountID, toAccountID, amountMinor,
	).Scan(&insertedID)

	if errors.Is(err, sql.ErrNoRows) {
		var status string
		if err := tx.QueryRowContext(ctx,
			`SELECT status FROM transfers WHERE id = $1`, transferID).Scan(&status); err != nil {
			return ReserveResult{}, fmt.Errorf("read existing transfer: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return ReserveResult{}, fmt.Errorf("commit: %w", err)
		}
		return ReserveResult{TransferID: transferID, Status: "already_" + status}, nil
	}
	if err != nil {
		return ReserveResult{}, fmt.Errorf("insert transfer: %w", err)
	}

	// domain.SystemAccountID is the seeded counterparty for demo/seed transfers: it is
	// allowed to go negative (an unlimited external funding source), so it is exempt
	// from the insufficient-funds check that applies to every other account.
	if fromAccountID != domain.SystemAccountID {
		var balance int64
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(CASE direction WHEN 'credit' THEN amount_minor ELSE -amount_minor END), 0)
			FROM entries WHERE account_id = $1`, fromAccountID).Scan(&balance); err != nil {
			return ReserveResult{}, fmt.Errorf("read balance: %w", err)
		}
		if balance < amountMinor {
			return ReserveResult{}, ErrInsufficientFunds
		}
	}

	for _, e := range domain.ReservationEntries(transferID, fromAccountID, toAccountID, amountMinor) {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO entries (id, transfer_id, account_id, direction, amount_minor)
			VALUES ($1, $2, $3, $4, $5)`,
			uuid.NewString(), e.TransferID, e.AccountID, string(e.Direction), e.AmountMinor,
		); err != nil {
			return ReserveResult{}, fmt.Errorf("insert entry: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return ReserveResult{}, fmt.Errorf("commit: %w", err)
	}
	return ReserveResult{TransferID: transferID, Status: "reserved"}, nil
}

type SettleResult struct {
	TransferID string `json:"transfer_id"`
	Status     string `json:"status"`
}

// Settle marks a transfer settled. It never writes new entries — the entries written
// by Reserve are the permanent, append-only ledger record; settlement only records a
// settlements row (the UNIQUE(transfer_id) idempotency guard from ADR-0003) and flips
// transfers.status. Idempotent: replaying an already-settled transfer_id is a no-op.
func (s *Store) Settle(ctx context.Context, transferID string) (SettleResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SettleResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM transfers WHERE id = $1 FOR UPDATE`, transferID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return SettleResult{}, ErrTransferNotFound
	}
	if err != nil {
		return SettleResult{}, fmt.Errorf("read transfer: %w", err)
	}
	if status == "settled" {
		if err := tx.Commit(); err != nil {
			return SettleResult{}, fmt.Errorf("commit: %w", err)
		}
		return SettleResult{TransferID: transferID, Status: "already_settled"}, nil
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO settlements (transfer_id) VALUES ($1) ON CONFLICT (transfer_id) DO NOTHING`,
		transferID,
	); err != nil {
		return SettleResult{}, fmt.Errorf("insert settlement: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE transfers SET status = 'settled' WHERE id = $1`, transferID,
	); err != nil {
		return SettleResult{}, fmt.Errorf("update transfer status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return SettleResult{}, fmt.Errorf("commit: %w", err)
	}
	return SettleResult{TransferID: transferID, Status: "settled"}, nil
}

type TransferStatus struct {
	TransferID string `json:"transfer_id"`
	Status     string `json:"status"`
}

func (s *Store) TransferStatus(ctx context.Context, transferID string) (TransferStatus, error) {
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT status FROM transfers WHERE id = $1`, transferID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return TransferStatus{}, ErrTransferNotFound
	}
	if err != nil {
		return TransferStatus{}, fmt.Errorf("read transfer status: %w", err)
	}
	return TransferStatus{TransferID: transferID, Status: status}, nil
}
