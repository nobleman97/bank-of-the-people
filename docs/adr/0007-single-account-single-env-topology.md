# ADR-0007: Single AWS account, single live environment (env-parameterized modules)

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Related: ADR-0002 (Compute), ADR-0005 (Ledger datastore), SECURITY.md

## Context

The original brief called for "at least two environments (dev, prod)". The project is also a
cost-conscious, ephemeral portfolio piece run on a spin-up / capture-proof / tear-down loop,
by a single operator. Standing up two permanent environments (or a multi-account
Organization landing zone) multiplies fixed cost — NAT Gateways, ALBs, RDS, CloudFront — and
setup effort, for little incremental demonstration value at this scale.

We must reconcile the "two environments / dev→prod promotion" requirement with the cost and
lifecycle constraints.

## Decision

Run a **single AWS account** with a **single live environment at a time**, while keeping all
Terraform **modules env-parameterized** so the multi-environment promotion path is fully
demonstrable in code.

- `infra/modules/*` are environment-agnostic **definitions**; `infra/live/<scope>/` holds the
  deployed **instances** — thin roots that call modules with concrete inputs and own their
  state. `dev` and `prod` call the same modules with different tfvars, so promotion is a
  config change, not a rewrite.
- A third scope, **`infra/live/global/`**, holds **account-singleton** resources that belong to
  no single environment: the **GitHub OIDC provider** (AWS permits exactly one per account per
  URL) and the CI **`plan`/`apply` roles** (which *create* the environments, so they cannot be
  owned by one). Putting these under `dev/` would make `prod`'s pipeline depend on `dev`'s
  state and give teardown a cross-env blast radius. `global/` is bootstrapped once by a human;
  everything else is then deployed keylessly by those roles. Future `global/` residents: shared
  ECR repositories (a digest built once promotes across envs).
- The CI/CD pipeline supports `dev → prod` promotion (same immutable image, promoted by
  environment-scoped apply with approval gates), even though only one environment is
  **materialized** during a given demo/recording session to control cost.
- Environment isolation within the account is enforced by naming (`botp-<env>-*`), tags,
  distinct state keys (`botp/<scope>/<component>`), and scoped IAM — not by account boundaries.

## Rationale

The requirement's *intent* — reproducible environments, a real promotion path, no
snowflake prod — is satisfied by env-parameterized modules and a promotion-capable pipeline.
What we consciously drop is running two environments **simultaneously and permanently**,
which is a pure cost decision, not an architectural one. Keeping the code multi-env means
"stand up a second environment" is a tfvars/workspace change, not a rewrite.

## Consequences

Positive:
- Minimal fixed cost; only one environment bills at a time; clean teardown.
- Promotion mechanics (image immutability, env-scoped apply, approval gates) are real and
  demonstrable.
- Modules stay honestly reusable — the second environment is a config away.

Negative / risks:
- No account-boundary separation-of-duties (a real control in regulated fintech). This is
  the main gap versus production and is called out explicitly, not hidden.
- Single account means a broad blast radius for a misconfigured credential; mitigated by
  least-privilege IAM, OIDC-only access, and scoped state (see SECURITY.md).

## Production equivalent

In a regulated setting this would be a **multi-account AWS Organization**: separate dev and
prod accounts (often per-workload), a management/shared-services account for state, ECR,
and OIDC, SCP guardrails, and centralized logging. The `global/` scope maps directly onto
that shared-services account — the same "account-singleton, deploys the environments" role,
promoted from a state-key convention to a hard account boundary. Stating this keeps the
single-account choice legible as an intentional, cost-driven portfolio decision rather than
a blind spot.

## Alternatives considered

- **Two permanent environments in one account:** closer to the letter of the brief, but
  doubles fixed cost with little added demonstration value at this scale.
- **Multi-account Organization (2–3 accounts):** the strongest production/SOC 2 story, but
  the most setup and cross-account IAM plumbing — disproportionate for an ephemeral solo
  portfolio project.
