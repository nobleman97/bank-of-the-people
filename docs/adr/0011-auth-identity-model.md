# ADR-0011: Authentication & identity — `auth_mode` toggle: seeded JWT (default) or Cognito signup (reject Clerk, reject hand-rolled auth)

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Related: ADR-0003 (Ledger consistency), ADR-0007 (Single-account topology), ADR-0009 (CI/CD OIDC), ADR-0010 (egress toggle precedent)

## Context

The frontend must answer "**who am I, and which accounts are mine?**" before it can show
balances, history, or initiate a transfer. Identity is not a frontend feature: it drives
authn middleware in `api`, account-scoping on every ledger read/write, and the secrets/IAM
story. It must be settled before the API contract (`docs/API.md`) and the frontend (PLAN P5).

Two demo shapes are both legitimate for this portfolio:

1. **Pure infra demo** — a hardcoded/seeded user is enough to exercise money movement and show
   the whole infra/pipeline/observability story cheaply, with nothing standing between
   sessions. This is the cost-conscious, ephemeral default (ADR-0007).
2. **Realistic product demo** — real **user signup** (registration, email verification, MFA,
   session management) so the piece doesn't read as a toy.

Rather than pick one and lose the other, we parameterize it — the same pattern used for
network egress in [ADR-0010](adr/0010-nat-egress.md) (`egress_mode`). The constant across both
shapes is that **`api` scopes all account access to the authenticated subject (`sub`)**; only
**token issuance/validation** differs. The project constraint — "keep the application code
deliberately thin; sophistication lives in infra, pipeline, observability" — means auth must
be *real enough to demonstrate an authn boundary and account-scoped authorization* without
becoming an identity product, and without hand-rolling security-critical code.

Candidates for the realistic mode: **AWS Cognito**, **Clerk / Auth0 (hosted SaaS IdP)**, or a
**hand-built auth service**.

## Decision

Introduce a Terraform/`api` input **`auth_mode = "seeded" | "cognito"`**. `api`'s
account-scoping authorization is identical in both modes; only the identity provider changes.

**`auth_mode = "seeded"` (default — pure infra demo):**
- The ledger seed step (PLAN P2) creates 1–2 demo users owning specific `account` rows;
  credentials live in **Secrets Manager**, not code (bcrypt/argon2 hashes, never plaintext).
- `POST /api/auth/login` returns a short-lived **HS256 JWT** (`sub = user_id`, ~15 min) signed
  with a Secrets Manager key (`botp-<env>-jwt-signing-key`). No signup.

**`auth_mode = "cognito"` (realistic demo — enables signups):**
- An **AWS Cognito user pool** (Terraform-managed, created/destroyed with the stack) provides
  **signup, email verification, MFA, and password reset** out of the box.
- `api` exposes `POST /api/auth/signup`, `POST /api/auth/verify`, `POST /api/auth/login`
  backed by Cognito; on first signup `api` provisions the user's ledger `account` row(s).
- Tokens are **Cognito-issued JWTs validated by `api` against the pool's JWKS (RS256)** —
  asymmetric, rotated by AWS, no shared signing secret in `api`.
- Optionally, the **ALB OIDC authenticate action** fronts the pool so unauthenticated requests
  never reach `api` (defense in depth; ties into the OIDC theme of ADR-0009).

**Invariant (both modes):** every non-public route requires `Authorization: Bearer <jwt>`;
`api` validates it and scopes account access to `sub`. A transfer from a non-owned account is
`403`; an unauthenticated call is `401`. This is unrelated to workload identity (ECS task
roles) and CI identity (GitHub OIDC, ADR-0009).

## Rationale

- **Why a toggle, not a single choice:** the two demo shapes serve different audiences (raw
  infra depth vs. product realism). A one-variable switch preserves the cheap, zero-dependency
  ephemeral demo *and* a full managed-IdP-with-signup story, without forking the codebase — the
  account-scoping seam in `api` is written once and is mode-agnostic.
- **Why Cognito over Clerk for the realistic mode:** Clerk is excellent DX for a Next.js app,
  but it is an **external, persistent SaaS** — user PII lives outside the account and outside
  the per-session teardown, it is not Terraform-managed like the rest of the stack, and to a
  platform/SRE interviewer "integrated an auth widget" is a weaker signal than "stood up a
  user pool in Terraform with JWKS validation and ALB OIDC." Cognito keeps identity
  **AWS-native, in-account, Terraform-managed, and ephemeral**, consistent with every other
  pillar of the project, and simplifies the PCI/SOC 2 data-flow story (no third-party PII
  processor). Clerk/Auth0 would be the right call for a real speed-to-market SaaS — not for
  this AWS-infra-centric portfolio piece.
- **Why not hand-build auth:** signup, verification, reset, session revocation, lockout,
  login rate-limiting, and MFA are large, security-critical surfaces — the canonical
  "don't roll your own auth." It is the most code for the least portfolio value, off-thesis for
  an SRE/platform role, and the highest liability. Rejected.
- **Why seeded stays the default:** it needs no IdP at all, spins up and tears down with zero
  external state, and keeps the infra demo the cheapest possible — the right default for a
  spin-up/record/destroy session.

## Consequences

Positive:
- One authn/authorization seam in `api`, two identity backends behind a single variable.
- `seeded`: zero identity infrastructure, fully self-contained, cheapest ephemeral demo.
- `cognito`: real signup/verification/MFA and **asymmetric JWKS validation** with no signing
  secret in `api` — a genuine production-grade identity integration, still in-account and
  Terraform-managed, still torn down per session.

Negative / risks:
- **Two code paths for token validation** (HS256 local vs RS256/JWKS) — bounded by keeping
  everything after validation (account scoping) shared; covered by tests on both modes.
- `seeded` mode retains the small self-minted-token crypto surface (alg pinning, short expiry,
  Secrets-Manager key) — acceptable for a demo default, superseded entirely in `cognito` mode.
- `cognito` mode adds a user pool to provision and, if ALB OIDC is enabled, listener/redirect
  wiring — the cost of realism, gated behind the toggle so the default demo stays simple.
- Cognito user data is ephemeral (destroyed on teardown); re-signup or seed each session.

## Production equivalent / when this flips

`auth_mode = "cognito"` **is** the production shape (managed IdP, asymmetric tokens, MFA,
verification), optionally hardened with ALB-enforced OIDC so unauthenticated traffic never
reaches `api`, refresh-token rotation, and adaptive/risk-based auth. An enterprise might swap
Cognito for another OIDC IdP; the account-scoping in `api` is unchanged — only the JWKS/issuer
config moves. `seeded` mode is demo-only and never ships. The trigger to default to `cognito`:
any real user base, multi-tenant access, or a compliance requirement (SOC 2 access controls,
PCI 8.x).

## Alternatives considered

- **Single mode, seeded only:** simplest, but cannot demonstrate signup/verification/MFA and
  reads as a toy for the product-realism audience. Kept as the default, not the only option.
- **Single mode, Cognito only:** production-correct but makes every ephemeral infra demo stand
  up a user pool and re-provision users — unnecessary weight for the raw-infra story. Offered
  via the toggle instead of forced.
- **Clerk / Auth0 (hosted SaaS):** great Next.js DX, but external, persistent, non-Terraform,
  PII off-account, and a frontend-dev signal rather than a platform one. Wrong fit for this
  project; right fit for a real SaaS.
- **Hand-built auth service:** large security-critical surface, off-thesis, highest liability.
  Rejected outright.
- **No authentication:** cannot demonstrate account scoping; indefensible for money movement.
