package domain

import "testing"

func TestCheckBalanced_Balanced(t *testing.T) {
	entries := []Entry{
		{TransferID: "t1", AccountID: "a", Direction: DirectionDebit, AmountMinor: 500},
		{TransferID: "t1", AccountID: "b", Direction: DirectionCredit, AmountMinor: 500},
	}
	if err := CheckBalanced(entries); err != nil {
		t.Fatalf("expected balanced entries to pass, got %v", err)
	}
}

func TestCheckBalanced_Unbalanced(t *testing.T) {
	entries := []Entry{
		{TransferID: "t1", AccountID: "a", Direction: DirectionDebit, AmountMinor: 500},
		{TransferID: "t1", AccountID: "b", Direction: DirectionCredit, AmountMinor: 400},
	}
	err := CheckBalanced(entries)
	if err == nil {
		t.Fatal("expected unbalanced entries to fail")
	}
	if err != ErrUnbalancedEntries {
		t.Fatalf("expected ErrUnbalancedEntries, got %v", err)
	}
}

func TestBalance_CreditsMinusDebits(t *testing.T) {
	entries := []Entry{
		{AccountID: "a", Direction: DirectionCredit, AmountMinor: 1000},
		{AccountID: "a", Direction: DirectionDebit, AmountMinor: 300},
	}
	got := Balance(entries)
	if got != 700 {
		t.Fatalf("expected balance 700, got %d", got)
	}
}

func TestReservationEntries_IsBalanced(t *testing.T) {
	entries := ReservationEntries("t1", "from", "to", 250)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if err := CheckBalanced(entries); err != nil {
		t.Fatalf("expected ReservationEntries to be balanced, got %v", err)
	}
	if entries[0].Direction != DirectionDebit || entries[0].AccountID != "from" {
		t.Fatalf("expected first entry to be a debit on the source account, got %+v", entries[0])
	}
	if entries[1].Direction != DirectionCredit || entries[1].AccountID != "to" {
		t.Fatalf("expected second entry to be a credit on the destination account, got %+v", entries[1])
	}
}
