CREATE TABLE accounts (
    id         UUID PRIMARY KEY,
    currency   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- transfers.id is TEXT, not UUID: it is the caller-supplied idempotency key for a
-- transfer (ADR-0003 "Idempotency-Key"), not a server-generated identifier, so it must
-- accept arbitrary client-chosen strings, not just UUID-shaped ones.
CREATE TABLE transfers (
    id              TEXT PRIMARY KEY,
    from_account_id UUID NOT NULL REFERENCES accounts(id),
    to_account_id   UUID NOT NULL REFERENCES accounts(id),
    amount_minor    BIGINT NOT NULL CHECK (amount_minor > 0),
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'settled')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Append-only: application code must never UPDATE or DELETE a row here (ADR-0003).
CREATE TABLE entries (
    id           UUID PRIMARY KEY,
    transfer_id  TEXT NOT NULL REFERENCES transfers(id),
    account_id   UUID NOT NULL REFERENCES accounts(id),
    direction    TEXT NOT NULL CHECK (direction IN ('debit', 'credit')),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX entries_account_id_idx ON entries(account_id);
-- One entry per (transfer, account) direction: reserve writes it once, settle never
-- re-writes entries (it only flips transfers.status and records a settlements row),
-- so balance = SUM(entries) never double-counts a transfer.
CREATE UNIQUE INDEX entries_transfer_account_direction_idx ON entries(transfer_id, account_id, direction);

-- Idempotency guard for the async settle step (ADR-0003: "keyed by a UNIQUE(transfer_id)
-- constraint" — at-least-once redelivery hits this constraint and is a no-op).
CREATE TABLE settlements (
    transfer_id TEXT PRIMARY KEY REFERENCES transfers(id),
    settled_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
