# Implementation Plan — bank-of-the-people

A phased plan. **Each phase leaves the system in a working, demoable state**, and each
carries: **Objective · Deliverables · Verification/acceptance · What this proves in an
interview · Cost & teardown.**

The project runs on a **spin-up / capture-proof / tear-down** loop. Costed resources exist
only during demo/recording windows. Everything below is `terraform destroy`-clean.

> **Lifecycle rule of thumb per session:** `apply infra → run migrations + seed → generate
> load → capture SLO dashboards + an end-to-end trace across SQS + a chaos exercise →
> destroy infra`. Capture proof *during* the session — the ephemeral DB and its data do not
> persist after teardown.

---

## Cost & teardown model (applies to every phase)

**Persistent (kept, ~free):** the homelab observability stack (VictoriaMetrics/Grafana/
Tempo) is the central platform and is **not** torn down between sessions.

**Ephemeral, created/destroyed with the ECS stack:** VPC + fck-nat NAT instance, ALB, ECS Fargate
tasks, **RDS Postgres**, SQS, CloudFront, WAF, the in-ECS OTel Collector/vmagent transport,
and FIS templates.

**Real-spend flags** (watch these): **fck-nat NAT instance** (`t4g.nano` hourly, no per-GB
fee — ADR-0010), **ALB** (hourly + LCU), **RDS** (instance-hours — smallest viable, e.g.
`db.t4g.micro` or Aurora Serverless v2 low-min), **CloudFront** (requests/egress), **FIS**
(per-action, P10 only).

**Teardown specifics:**
- Ledger DB: `terraform destroy`; **`skip_final_snapshot = true`** in dev for a clean
  destroy — retain a final snapshot only if a session's state must survive.
- Observability transport: destroyed with the ECS stack; the **tunnel credential** is a
  rotated secret so a torn-down stack cannot be impersonated on the remote-write endpoint.

---

## P0 — Foundations
- **Objective:** secure, reproducible baseline before any workload exists.
- **Deliverables:** monorepo scaffold; Terraform **remote state** (S3 bucket + DynamoDB
  lock table); **GitHub OIDC** provider + `plan`/`apply` roles (see
  [ADR-0009](adr/0009-cicd-github-actions-oidc.md)); tagging strategy and module
  conventions (see `CLAUDE.md`); CI skeleton (fmt/validate + scanners wired, no deploy yet).
- **Verification:** a PR runs `terraform plan` via the read-only OIDC role with **no static
  keys**; state locks/unlocks; scanners execute.
- **Proves:** you bootstrap securely — OIDC, remote state with locking, least-privilege from
  line one.
- **Cost & teardown:** negligible (S3 + DynamoDB pay-per-use); nothing to tear down nightly.

## P1 — Network & platform
- **Objective:** the VPC and shared platform that everything runs on.
- **Deliverables:** VPC across 2 AZs (public/private subnets), **fck-nat NAT instance**
  (`t4g.nano`, ASG(1), EIP — ADR-0010), **VPC endpoints**
  (ECR, Secrets Manager, CloudWatch Logs, SQS, S3), **ECR** repos, **encryption at rest via
  AWS-managed keys** per domain (ADR-0008), ECS Fargate cluster, ALB + HTTPS listener (ACM).
- **Verification:** `terraform apply` is clean and idempotent (second plan is a no-op); ALB
  serves a health endpoint; image push to ECR works; endpoints resolve privately.
- **Proves:** production networking with least-cost egress (endpoints over NAT) and
  encryption-by-default.
- **Cost & teardown:** **NAT instance + ALB accrue** — destroy after the session.

## P2 — Ledger + ephemeral RDS
- **Objective:** the strongly-consistent double-entry core.
- **Deliverables:** `ledger` service (internal via Service Connect); **RDS Postgres**
  (ephemeral, ADR-0005); **automated schema migrations + seed** on bring-up; double-entry
  model per [ADR-0003](adr/0003-ledger-consistency-model.md).
- **Verification:** create accounts, post balanced entries, query balance = `SUM(entries)`;
  invariant test asserts debits == credits; concurrent-transfer test shows the
  `FOR UPDATE` lock prevents overdraft.
- **Proves:** ACID double-entry correctness + a reproducible ephemeral DB lifecycle
  (migrations/seed on every apply).
- **Cost & teardown:** **RDS accrues** — smallest viable instance; `skip_final_snapshot`
  destroy.

## P3 — API gateway + idempotency
- **Objective:** the synchronous authorization path.
- **Deliverables:** `api` service behind ALB target group; **Service Connect** to `ledger`;
  `Idempotency-Key` handling with a unique-constraint store; reserve-funds call; **auth behind
  `auth_mode`** — `seeded` (JWT login, default) or `cognito` (user pool + signup/verify/MFA,
  JWKS validation) — with **per-request account scoping** identical across modes
  ([ADR-0011](adr/0011-auth-identity-model.md)); full HTTP surface per [`API.md`](API.md)
  (auth, transfers, accounts, balances, history, status).
- **Verification:** `POST /api/transfers` reserves funds and returns `201 pending`; **replaying
  the same key returns the stored response** (no second reservation); different body + same
  key → `409`; insufficient funds → `402`; **unauthenticated → `401`, non-owned `from_account`
  → `403`**; `GET /api/transfers/{id}` reflects status; in `cognito` mode a **signup → verify →
  login** round-trip issues a JWKS-validated token.
- **Proves:** synchronous authorization + idempotency under client/network retries, behind an
  authn boundary with account-scoped authorization.
- **Cost & teardown:** as P1/P2.

## P4 — Async settlement + webhooks + DLQ
- **Objective:** the asynchronous settlement path with reliable delivery.
- **Deliverables:** SQS **standard** queue + **DLQ** (redrive `maxReceiveCount`); `worker`
  consuming SQS, **idempotent finalize** (`UNIQUE(transfer_id)`); **HMAC-SHA256 webhooks**
  with bounded retry/backoff; DLQ redrive runbook (seed `docs/runbooks/`).
- **Verification:** a transfer settles asynchronously; webhook delivered + signature
  verifies; a forced-500 merchant proves retry/backoff → DLQ; **redelivery does not
  double-spend** (settlement unique constraint holds).
- **Proves:** at-least-once reliability, idempotent consumers, poison handling — no
  double-spend under retries.
- **Cost & teardown:** SQS pay-per-request (trivial); destroy with stack.

## P5 — Frontend (S3 + CloudFront + WAF)
- **Objective:** the thin UI and edge protection.
- **Deliverables:** static Next export on **S3**; **CloudFront** distribution (ACM, OAC) with
  a **`/api/*` behavior routing to the ALB origin** (same-origin API, no CORS); typed client
  against [`API.md`](API.md); **CloudFront WAF** + **regional WAF on the ALB** (managed rules +
  rate limit on `/api/transfers`).
- **Verification:** UI initiates a transfer and shows balances, history, and live settlement
  status; WAF blocks a scripted bad request / trips the rate limit on `/transfers`.
- **Proves:** CDN + edge WAF on a public payment endpoint; end-to-end user flow.
- **Cost & teardown:** **CloudFront accrues** on requests/egress — destroy with stack.

## P6 — Observability transport
- **Objective:** get metrics and traces to the homelab, resiliently.
- **Deliverables:** in-ECS **OTel Collector** (+ vmagent for remote-write); **authenticated
  tunnel** to homelab; **remote-write with on-disk buffering**; Grafana dashboards (auth,
  settlement, webhook, error-budget); Tempo receiving spans.
- **Verification:** dashboards populate from live load; **one unbroken end-to-end trace**
  spans api → ledger → SQS → worker → webhook; a simulated tunnel blip produces a
  **backfilled gap**, not lost samples.
- **Proves:** self-hosted observability from cloud workloads; benign-failure design; trace
  context across an async queue hop.
- **Cost & teardown:** transport destroyed with the stack; homelab persists; rotate tunnel
  credential.

## P7 — SLOs + alerting
- **Objective:** turn telemetry into objectives with alerting.
- **Deliverables:** vmalert **multi-window multi-burn-rate** rules per [SLO.md](SLO.md);
  **gaps-as-unknown** accounting; the in-cloud **deploy-gate CloudWatch alarms** (ALB 5xx,
  target p99).
- **Verification:** driving errors past a threshold fires the fast-burn alert; a metrics gap
  is excluded from the budget denominator (not counted as pass/fail); deploy-gate alarms
  evaluate independently of the homelab.
- **Proves:** SLO/error-budget engineering, honest gap accounting, and deploy safety that
  does not share the observability failure domain.
- **Cost & teardown:** CloudWatch alarms (minimal); destroy with stack.

## P8 — Progressive delivery + supply chain
- **Objective:** ship safely through a hardened pipeline.
- **Deliverables:** **CodeDeploy canary** (blue/green target groups, linear/canary shift,
  alarm-gated rollback — [ADR-0006](adr/0006-deployment-strategy-codedeploy-canary.md));
  full DevSecOps gate set wired into GitHub Actions via OIDC — Trivy, tfsec, Checkov,
  gitleaks, Syft (SBOM), cosign (signing), Conftest (policy); **digest-based dev→prod
  promotion**.
- **Verification:** a green deploy shifts traffic and completes; a deliberately-bad deploy
  trips a deploy-gate alarm and **auto-rolls back**; the pipeline **blocks** on a planted
  secret / high-CVE image / policy violation; only signed images deploy.
- **Proves:** end-to-end DevSecOps + safe progressive delivery + artifact integrity.
- **Cost & teardown:** brief doubled tasks during canary (minor); destroy with stack.

## P9 — Worker autoscaling on backlog
- **Objective:** scale settlement on demand, correctly.
- **Deliverables:** Application Auto Scaling on the `worker` using an **SQS
  backlog-per-task** target (derived from `ApproximateNumberOfMessagesVisible` and
  `ApproximateAgeOfOldestMessage`), **not CPU**.
- **Verification:** a burst of transfers grows the backlog; worker task count scales **out**,
  drains the queue, then scales **in**; latency of settlement recovers within SLO.
- **Proves:** you autoscale on the signal that actually reflects load for a queue worker, not
  a proxy (CPU).
- **Cost & teardown:** scales to a small floor; destroy with stack.

## P10 — Chaos + runbooks + incident response
- **Objective:** prove resilience and operational readiness.
- **Deliverables:** **app-level fault injection** flags — inject ledger latency, kill the
  worker, force webhook 500s; **one AWS FIS experiment** template (e.g. task kill / injected
  latency); runbooks (`docs/runbooks/`) and a documented **incident-response flow**.
- **Verification (the three named exercises):**
  1. Inject ledger latency → watch auth p99 rise and (if past threshold) the burn-rate alert
     fire.
  2. Kill the worker → watch SQS backlog + oldest-message age grow, then autoscaling/restart
     drain it.
  3. Make webhooks return 500 → prove retry/backoff → DLQ, and **no double-spend** on
     redelivery.
- **Proves:** failure-injection discipline, SLO-driven detection, and correctness under
  fault — the SRE core.
- **Cost & teardown:** **FIS per-action spend** (small); destroy templates with stack.

## P11 — Teardown + proof capture
- **Objective:** close the loop cleanly and cheaply.
- **Deliverables:** demo runbook executing the full loop; captured artifacts (dashboards,
  an end-to-end trace, chaos + rollback screenshots); a short **cost accounting** of the
  session.
- **Verification:** `terraform destroy` removes all ephemeral resources (`skip_final_snapshot`
  in dev); a follow-up plan shows nothing left billing; homelab retains the captured metrics
  history (backfilled).
- **Proves:** cost-disciplined, reproducible lifecycle — spin up, prove it, tear down.
- **Cost & teardown:** returns spend to ~zero (homelab only).

---

## Phase dependency summary

```
P0 → P1 → P2 → P3 → P4 → P5
                     ↘ P6 → P7 → P8 → P9 → P10 → P11
```
P6 depends on services emitting telemetry (P2–P5); P7 depends on P6; P8 can begin once P1's
ALB/ECR exist but is demoed after P7 so deploy-gate alarms are in place; P9/P10 depend on
the full async path + observability.
