# Service Level Objectives — bank-of-the-people

This document defines the SLIs, SLO targets, error budgets, and burn-rate alerting for the
payments platform, and the **measurement path** that makes the numbers honest given the
observability design in [ADR-0004](adr/0004-observability-stack.md).

Targets below are set to be defensible for a portfolio/interview context; each carries the
story behind the number. They are deliberately not "five nines" — realistic, justified
targets are the point.

---

## 1. Where SLIs are measured (and why it matters)

The observability backend is the homelab (VictoriaMetrics/Grafana/Tempo), reached over an
authenticated tunnel with vmagent on-disk buffering (ADR-0004). The ledger is in-region RDS
(ADR-0005). This shapes measurement:

- **Authorization success rate** and **authorization p99 latency** are measured **at the
  `api` service inside AWS**, before any homelab dependency. Because the ledger is in-VPC
  RDS, these SLIs reflect **engineerable in-cloud behavior** and are **not** polluted by
  tunnel jitter or homelab availability.
- **Settlement lag** and **webhook delivery success** are emitted by the `worker` and
  measured from **in-AWS timestamps** (`enqueued_at` → finalize time; webhook attempt
  outcomes).
- Metrics are then remote-written to the homelab for storage, dashboards, and alert
  evaluation. The *measurement point* is in AWS; only the *storage/eval* is in the homelab.

---

## 2. SLIs and SLO targets

| SLI | Definition | Target (SLO) | Rationale |
|-----|------------|--------------|-----------|
| **Authorization success rate** | non-5xx, non-timeout `POST /transfers` auth responses ÷ total auth requests. Business declines (402 insufficient funds) count as **success** — the system worked. | **≥ 99.9%** over 28 days | Auth is the synchronous money path; a 0.1% budget (~43 min/28d) is tight but achievable with in-region RDS and idempotent retries. |
| **Authorization latency** | p99 of `POST /transfers` request duration measured at `api` | **p99 ≤ 250 ms** | In-VPC RDS + single-row-locked reserve should sit well under this; 250 ms leaves headroom for lock contention without being slack. |
| **Settlement lag** | p95 of `enqueued_at → finalize` duration | **p95 ≤ 30 s** | Async settlement is allowed to be slow but bounded; 30 s p95 is generous enough to absorb worker scale-out yet tight enough to catch a stuck backlog. |
| **Webhook delivery success** | webhooks delivered (2xx) within **5** retries ÷ total webhooks | **≥ 99%** | Merchant endpoints are external and flaky; 5 retries with backoff then DLQ. 99% reflects "we did our part reliably"; the 1% is genuinely-unreachable endpoints → DLQ. |

Notes:
- Money is integer minor units; latency SLIs use in-AWS monotonic timing.
- "Success" for auth explicitly **includes** business declines — a correct 402 is the
  system working, not an error. This distinction is called out because it materially
  changes the success-rate math.

---

## 3. Error budgets

For a rolling 28-day window:

| SLI | SLO | Error budget | Budget as time/volume |
|-----|-----|--------------|-----------------------|
| Auth success | 99.9% | 0.1% of requests | ~43 min of full outage-equivalent / 28d |
| Auth latency | 99% of requests ≤ 250 ms (p99) | 1% may exceed 250 ms | — |
| Settlement lag | 95% ≤ 30 s (p95) | 5% may exceed 30 s | — |
| Webhook delivery | 99% within 5 retries | 1% | ~last-resort DLQ rate |

Budget policy (interview-facing): while budget remains, ship features and run chaos
experiments freely; if a burn-rate alert fires or budget is exhausted, freeze risky change
and prioritize reliability work. Chaos exercises (PLAN P10) are intentionally run *against*
the budget to prove the alerting works.

### Gaps are "unknown", not success or failure

A homelab or tunnel outage produces a **metrics gap** that backfills on recovery (vmagent
on-disk buffer, ADR-0004). In error-budget accounting, **gap intervals are treated as
`unknown` and excluded from the denominator** — never counted as automatic success or
automatic failure. This keeps budget math honest: we do not manufacture SLO compliance out
of a monitoring outage, nor do we burn budget for one. This rule is stated explicitly so the
accounting is defensible.

---

## 4. Burn-rate alerting

**Multi-window, multi-burn-rate** alerts, evaluated by **vmalert in the homelab**:

| Alert | Condition (auth success budget) | Meaning |
|-------|--------------------------------|---------|
| Fast burn | consuming **2% of budget in 1 h** (high burn rate, short window + confirmation window) | page — acute regression |
| Slow burn | consuming **5% of budget in 6 h** | ticket — sustained erosion |

- Two windows per alert (long + short confirmation) suppress flapping.
- Thresholds derive from the SLO targets in §2; the same pattern is applied to latency,
  settlement-lag, and webhook SLIs with per-SLI budgets.
- **Alerting-availability caveat (ADR-0004):** these rules evaluate in the homelab. If the
  homelab is down, burn-rate alerting is down for that window. This is an accepted,
  documented limitation; the production equivalent evaluates alerts in a backend that does
  not share a failure domain with the workload (AMP/AMG, Grafana Cloud, or a dedicated
  observability account).

---

## 5. Deploy-gate alarms (the in-cloud seam)

Because burn-rate SLO alerting lives in the homelab, **deployment safety is kept
independent of it** (see [ADR-0006](adr/0006-deployment-strategy-codedeploy-canary.md)).

A **small, separate set of CloudWatch alarms** — evaluated **in AWS** — gates CodeDeploy
canary rollback:

| Deploy-gate alarm | Source | Purpose |
|-------------------|--------|---------|
| ALB/target 5xx rate | CloudWatch (ALB metrics) | roll back a canary that starts erroring |
| Target p99 latency | CloudWatch (ALB target response time) | roll back a canary that regresses latency |

These alarms exist **only** to gate deploys and fire rollback; they are not the primary SLO
observability surface. This means a bad deploy is caught and reverted even during a homelab
outage — deploy safety does not depend on the observability failure domain.

---

## 6. Dashboards (Grafana, homelab)

- **Auth path:** success rate, p50/p99 latency, request volume, decline rate (business vs
  error split).
- **Settlement path:** lag p50/p95, SQS backlog (visible messages, oldest-message age),
  in-flight/worker count, DLQ depth.
- **Webhook:** delivery success, retry distribution, DLQ arrivals.
- **Error budget:** remaining budget per SLI + burn-rate panels, with `unknown` (gap)
  intervals shaded distinctly from success/failure.

Dashboards, an end-to-end trace across SQS, and alert screenshots are captured **during the
session** (the DB and its data do not persist after teardown — see [PLAN.md](PLAN.md)).
