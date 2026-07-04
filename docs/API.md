# API contract — `api` (payments gateway)

The public HTTP surface of the `api` service. This is the contract the **frontend** (PLAN P5)
is built against and the `api` (PLAN P3) implements; keep both sides in sync with this file.
Money movement semantics mirror `docs/ARCHITECTURE.md` §3–§7; identity mirrors
[ADR-0011](adr/0011-auth-identity-model.md).

> Status: contract draft for review. A machine-readable `openapi.yaml` may be generated from
> this later; prose is the source of truth for now.

---

## 1. Conventions

- **Base path / origin:** the API is served under **`/api/*`** on the **same CloudFront
  domain** as the static frontend (CloudFront path-routes `/api/*` to the ALB origin). This
  makes browser calls **same-origin — no CORS** — and puts the edge WAF in front of the API
  too. There is no separate API hostname.
- **Transport:** HTTPS only (TLS via ACM). HTTP is redirected/refused.
- **Money:** amounts are **`amount_minor`**, an **integer in minor units** (e.g. cents).
  Never floats. `currency` is an ISO-4217 code; the demo is single-currency **USD** unless a
  request says otherwise.
- **Time:** all timestamps are **RFC 3339 / ISO 8601** UTC strings.
- **IDs:** `transfer_id`, `account_id`, `user_id` are **UUIDv4** strings.
- **Tracing:** clients SHOULD send **`traceparent`** (W3C trace context); `api` extracts or
  creates it and propagates it end-to-end (ARCHITECTURE §8). `api` returns an **`X-Request-Id`**
  on every response for correlation.
- **Auth:** all routes except `POST /api/auth/login` and `GET /healthz` require
  **`Authorization: Bearer <jwt>`** (ADR-0011). Account access is scoped to the token `sub`.
- **Rate limiting:** enforced at the edge by **WAF** (rate-based rules), not per-endpoint in
  `api`. Exceeding it yields WAF `403`/`429` before the request reaches `api`.

### 1.1 Error envelope

Every non-2xx response uses one shape:

```json
{
  "error": {
    "code": "insufficient_funds",
    "message": "human-readable, safe to surface",
    "request_id": "uuid",
    "details": {}
  }
}
```

`code` is a stable machine string (snake_case); `message` is display-safe; `details` is
optional and endpoint-specific. Clients branch on `code`, never on `message` text.

| HTTP | `code` examples | Meaning |
|------|-----------------|---------|
| 400 | `validation_error` | Malformed body / missing field / bad currency. |
| 401 | `unauthenticated` | Missing/invalid/expired bearer token. |
| 402 | `insufficient_funds` | Authorization declined by the ledger. |
| 403 | `forbidden_account` | Authenticated, but the account is not owned by the caller. |
| 404 | `not_found` | Unknown `transfer_id`/`account_id` (or not owned — see note). |
| 409 | `idempotency_conflict` | Same `Idempotency-Key`, different request fingerprint. |
| 422 | `unprocessable` | Well-formed but semantically invalid (e.g. `from == to`). |
| 429 | `rate_limited` | Throttled (typically surfaced by WAF at the edge). |
| 500 | `internal` | Unexpected server error. |
| 503 | `dependency_unavailable` | Ledger/RDS unreachable; safe to retry. |

> Note: for owned-vs-unknown resources the API returns `404` rather than `403` where leaking
> existence would be a privacy issue; `403 forbidden_account` is used only where the account
> id is a legitimate input the caller supplied for their own transfer.

---

## 2. Authentication

Auth has two modes behind **`auth_mode`** ([ADR-0011](adr/0011-auth-identity-model.md)). The
**request/response shapes below are identical across modes** — only token issuance/validation
differs, and `signup`/`verify` exist only in `cognito` mode. Everything after the token
(account-scoping on `sub`) is mode-independent.

- **`auth_mode = "seeded"` (default):** seeded demo user; `login` returns an `api`-minted
  **HS256** JWT. No signup.
- **`auth_mode = "cognito"`:** AWS Cognito user pool; `signup` + `verify` enabled; tokens are
  **Cognito-issued and validated against the pool JWKS (RS256)**.

### `POST /api/auth/signup`  *(cognito mode only)*
Public. Registers a new user and triggers email verification. Returns `404 not_found` when
`auth_mode = "seeded"`.

Request:
```json
{ "email": "user@example.com", "password": "••••••", "name": "Ada L." }
```

`202 Accepted`:
```json
{ "user_id": "uuid", "status": "pending_verification" }
```

On success `api` provisions the user's ledger `account` row(s). Errors: `400 validation_error`
(weak password / bad email per pool policy), `409 conflict` (email already registered).

### `POST /api/auth/verify`  *(cognito mode only)*
Public. Confirms a signup with the emailed code. Returns `404 not_found` in `seeded` mode.

Request:
```json
{ "email": "user@example.com", "code": "123456" }
```

`200 OK` → `{ "user_id": "uuid", "status": "verified" }`. Errors: `400 validation_error`,
`410 gone` (code expired).

### `POST /api/auth/login`
Public. Exchanges credentials for a short-lived JWT (ADR-0011). In `seeded` mode the
credentials are the seeded demo user; in `cognito` mode they are a verified pool user.

Request:
```json
{ "username": "demo", "password": "••••••" }
```

`200 OK`:
```json
{
  "access_token": "<jwt>",
  "token_type": "Bearer",
  "expires_in": 900,
  "user_id": "uuid"
}
```

Errors: `400 validation_error`, `401 unauthenticated` (bad credentials). No account
enumeration: invalid username and bad password both return `401` with the same message.

The token has `sub = user_id` and a short expiry (~15 min). In `seeded` mode it is an
`api`-minted **HS256** JWT; in `cognito` mode it is a **Cognito-issued** JWT that `api`
validates against the pool **JWKS (RS256)**. Clients send it as `Authorization: Bearer <jwt>`
on every subsequent call — the header is identical in both modes. `seeded` mode has no refresh
endpoint (re-login on expiry); `cognito` mode may use Cognito refresh tokens.

---

## 3. Accounts & balances

### `GET /api/accounts`
Lists the accounts owned by the authenticated user.

`200 OK`:
```json
{
  "accounts": [
    { "account_id": "uuid", "name": "Checking", "currency": "USD",
      "balance_minor": 250000, "as_of": "2026-07-04T12:00:00Z" }
  ]
}
```

### `GET /api/accounts/{account_id}/balance`
Balance for one owned account. `balance_minor = SUM(entries)` over the ledger
(ARCHITECTURE §3; ADR-0003), so **pending reservations are already reflected** (an authorized
but unsettled transfer has reduced the available balance).

`200 OK`:
```json
{ "account_id": "uuid", "currency": "USD", "balance_minor": 250000,
  "pending_minor": -100000, "available_minor": 150000, "as_of": "2026-07-04T12:00:00Z" }
```

Errors: `403 forbidden_account`, `404 not_found`.

### `GET /api/accounts/{account_id}/transactions`
Transaction history for one owned account, newest first. **Cursor-paginated.**

Query params: `limit` (default 25, max 100), `cursor` (opaque, from a previous response).

`200 OK`:
```json
{
  "transactions": [
    { "transfer_id": "uuid", "direction": "debit", "amount_minor": 100000,
      "currency": "USD", "status": "settled", "counterparty_account": "uuid",
      "created_at": "2026-07-04T12:00:00Z", "settled_at": "2026-07-04T12:00:03Z" }
  ],
  "next_cursor": "opaque-or-null"
}
```

`next_cursor` is `null` when there are no more pages.

---

## 4. Transfers

### `POST /api/transfers`
Initiate a transfer. **Synchronous authorization** — returns a definitive authorized/declined
answer after the ledger reserves funds; settlement is asynchronous (ARCHITECTURE §3–§4).

Required headers:
- `Authorization: Bearer <jwt>`
- **`Idempotency-Key: <client-generated unique string>`** (required; the client generates one
  per logical transfer attempt and reuses it on retry).
- `Content-Type: application/json`

Request:
```json
{
  "from_account": "uuid",
  "to_account": "uuid",
  "amount_minor": 100000,
  "currency": "USD",
  "reference": "optional free-text, <=140 chars"
}
```

`from_account` **must be owned** by the caller (else `403 forbidden_account`). `to_account`
may be any valid account. `from_account == to_account` → `422 unprocessable`.

Responses:

- **`201 Created`** — authorized and reserved; settlement enqueued.
  ```json
  { "transfer_id": "uuid", "status": "pending",
    "from_account": "uuid", "to_account": "uuid",
    "amount_minor": 100000, "currency": "USD",
    "created_at": "2026-07-04T12:00:00Z" }
  ```
- **`200 OK`** — idempotent replay: same `Idempotency-Key` **and** same request fingerprint;
  the **stored** response is returned verbatim, no second reservation (ARCHITECTURE §6).
- **`402 insufficient_funds`** — ledger declined; no reservation, no settlement job.
- **`409 idempotency_conflict`** — same key, **different** request body fingerprint.
- **`400` / `401` / `403` / `422` / `503`** per the error table.

> Idempotency semantics are authoritative in ARCHITECTURE §6. The `Idempotency-Key` is stored
> with a `UNIQUE` constraint; correctness under client/network retries comes from the ledger,
> not from the client behaving.

### `GET /api/transfers/{transfer_id}`
Fetch one transfer's current state — this is what the frontend **polls to watch settlement**.
Only transfers involving an account owned by the caller are visible.

`200 OK`:
```json
{
  "transfer_id": "uuid",
  "status": "settled",
  "from_account": "uuid",
  "to_account": "uuid",
  "amount_minor": 100000,
  "currency": "USD",
  "reference": "invoice-42",
  "created_at": "2026-07-04T12:00:00Z",
  "settled_at": "2026-07-04T12:00:03Z",
  "failure_reason": null
}
```

Errors: `401`, `404 not_found`.

#### Settlement status model

The frontend watches a transfer by **polling `GET /api/transfers/{transfer_id}`** on a short
interval (e.g. 1–2 s with backoff) until the status is terminal. Polling is chosen over
WebSocket/SSE for demo simplicity and because it keeps the whole flow inside one trace
(ARCHITECTURE §8); server-push is named as the "when it flips" upgrade.

| `status` | Meaning | Terminal? |
|----------|---------|-----------|
| `pending` | Authorized: funds reserved in the ledger, settlement enqueued to SQS. | no |
| `settled` | Worker finalized the transaction; entries moved from pending to settled. | yes |
| `failed` | Settlement exhausted retries and went to the DLQ; reservation reversed. | yes |

Transitions: `pending → settled` (happy path) or `pending → failed` (poison message to DLQ,
ARCHITECTURE §7). There is no `pending → pending` mutation; `settled`/`failed` are immutable.
When `status = failed`, `failure_reason` is a short machine string.

---

## 5. Operational endpoints

### `GET /healthz`
Public, unauthenticated liveness/readiness probe used by the **ALB target group** health
check. Returns `200` with a small body when the process is up and its ledger/DB dependency is
reachable; `503 dependency_unavailable` otherwise. Not under `/api` and not behind auth.

```json
{ "status": "ok", "version": "git-sha", "checks": { "ledger": "ok" } }
```

---

## 6. Versioning & change control

- The path prefix `/api` is unversioned for the demo; a breaking change would introduce
  `/api/v2` rather than mutating shapes in place.
- The **async settlement message** carries its own `schema_version` (ARCHITECTURE §5); this
  HTTP contract and that message contract version independently.
- Any change here must land in the **same PR** as the `api` change that implements it, and the
  frontend types regenerated from it — the contract is the seam between P3 and P5.
