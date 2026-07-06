package domain

import (
	"errors"
	"time"
)

type Direction string

const (
	DirectionDebit  Direction = "debit"
	DirectionCredit Direction = "credit"
)

// SystemAccountID is the seeded counterparty for demo/seed transfers, so that
// SUM(all debits) == SUM(all credits) holds globally as a testable invariant.
const SystemAccountID = "00000000-0000-0000-0000-000000000001"

type Account struct {
	ID        string    `json:"id"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
}

type Entry struct {
	ID          string    `json:"id"`
	TransferID  string    `json:"transfer_id"`
	AccountID   string    `json:"account_id"`
	Direction   Direction `json:"direction"`
	AmountMinor int64     `json:"amount_minor"`
	CreatedAt   time.Time `json:"created_at"`
}

var ErrUnbalancedEntries = errors.New("unbalanced entries: debits != credits")

// CheckBalanced returns ErrUnbalancedEntries unless the entries' total debits equal
// their total credits (ADR-0003: every transfer writes a balanced set of entries).
func CheckBalanced(entries []Entry) error {
	var debits, credits int64
	for _, e := range entries {
		switch e.Direction {
		case DirectionDebit:
			debits += e.AmountMinor
		case DirectionCredit:
			credits += e.AmountMinor
		}
	}
	if debits != credits {
		return ErrUnbalancedEntries
	}
	return nil
}

// Balance sums signed entries for one account: credits increase it, debits decrease it.
func Balance(entries []Entry) int64 {
	var bal int64
	for _, e := range entries {
		switch e.Direction {
		case DirectionCredit:
			bal += e.AmountMinor
		case DirectionDebit:
			bal -= e.AmountMinor
		}
	}
	return bal
}

// ReservationEntries returns the balanced pending entry pair for a transfer: a debit
// on the source account and a credit on the destination account.
func ReservationEntries(transferID, fromAccountID, toAccountID string, amountMinor int64) []Entry {
	return []Entry{
		{TransferID: transferID, AccountID: fromAccountID, Direction: DirectionDebit, AmountMinor: amountMinor},
		{TransferID: transferID, AccountID: toAccountID, Direction: DirectionCredit, AmountMinor: amountMinor},
	}
}
