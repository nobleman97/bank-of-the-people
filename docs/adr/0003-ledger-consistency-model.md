# ADR-0003: Ledger consistency model — double-entry, append-only, ACID

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Related: ADR-0005 (Ledger datastore placement), ADR-0001 (Messaging)

> This ADR covers the *logical* consistency model of the ledger. ADR-0005 covers where it
> *physically* lives (ephemeral in-VPC RDS PostgreSQL).

## Context

The ledger is the source of truth for money. It sits on the synchronous authorization path
(`api` → `ledger` → RDS) and its correctness is non-negotiable: no double-spend, no lost or
invented funds, even under concurrent transfers, client retries, and at-least-once queue
redelivery. Authorization p99 latency and authorization success rate are headline SLOs, so
the model must be correct *and* cheap to execute.

## Decision

Model the ledger as **double-entry, append-only, with multi-row ACID transactions in
PostgreSQL**.

**Schema (essentials):**
- `accounts(id, currency, …)`.
- `entries(id, transfer_id, account_id, direction, amount_minor, status, created_at)` —
  **append-only**; no updates or deletes. `direction ∈ {debit, credit}`.
- A transfer writes a **balanced** set of entries: total debits = total credits.
- **Balance = `SUM(signed entries)`** for an account (optionally maintained as a
  materialized running balance for read performance, reconciled against the sum).
- Money is stored as **integer minor units** (`amount_minor`), never floats.

**Two-phase lifecycle:**
- **Reserve (authorization, sync):** insert `status = pending` reservation entries inside
  one transaction, after checking funds.
- **Settle (finalization, async):** insert the `status = posted` settlement entry, keyed by
  a **`UNIQUE(transfer_id)`** constraint, and mark the transfer settled.

**Concurrency control:**
- The sufficient-funds check and the reservation insert happen in **one transaction** under
  **Read Committed** isolation with an explicit **`SELECT … FOR UPDATE`** row lock on the
  source account. The row lock serializes concurrent transfers on the same account, which
  is what prevents a double-spend race; other accounts proceed in parallel.
- **Serializable** isolation is noted as the stricter alternative (it would let us drop the
  explicit lock and rely on serialization-failure retries) but is not the default here:
  explicit row locking is more predictable for latency and avoids retry storms on hot
  accounts. The choice is documented so it can be defended either way.

**Idempotency / no double-spend:**
- Sync path: `Idempotency-Key` unique constraint in `api` → a transfer reserves at most
  once per key.
- Async path: `UNIQUE(transfer_id)` on the settlement entry → at-least-once redelivery
  finalizes at most once (duplicate delivery hits the constraint and is a no-op).

## Consequences

Positive:
- Auditable by construction: append-only entries are a natural audit log; balances are
  derivable and verifiable (debits == credits is an invariant you can assert in tests).
- Correctness comes from database constraints and locks, not from queue features or
  application cleverness — the strongest story for money movement.
- Multi-row ACID is exactly what Postgres gives cheaply (the core reason Postgres beats
  DynamoDB here — see ADR-0005).

Negative / risks:
- `SELECT … FOR UPDATE` serializes transfers per source account; a very hot account is a
  contention point. Acceptable at demo scale; the production mitigation is sharding hot
  accounts or moving to serializable-with-retry.
- Running balances, if materialized, must be reconciled against `SUM(entries)` to stay
  honest — covered by an invariant check in tests and a reconciliation runbook.

## Alternatives considered

- **Single-entry / balance-column mutation:** simpler, but loses the audit trail and makes
  correctness proofs weaker; rejected for a money ledger.
- **Serializable isolation, no explicit locks:** correct, but trades predictable latency
  for serialization-failure retries on hot accounts; kept as the documented alternative.
- **Event-sourced ledger on a log (Kafka):** powerful but out of scope and over-weight for
  this workload (see ADR-0001).
