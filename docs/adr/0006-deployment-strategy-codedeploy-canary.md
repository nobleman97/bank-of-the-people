# ADR-0006: Progressive delivery via CodeDeploy canary on ECS

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Related: ADR-0004 (Observability stack), SLO.md (deploy-gate alarms)

## Context

We deploy the ECS services frequently and want safe, zero-downtime rollouts with automated
rollback on regression. Options: **ECS rolling update** (native), **CodeDeploy blue/green
all-at-once**, or **CodeDeploy canary/linear** traffic shifting.

There is a wrinkle specific to this project: primary observability and alerting live in the
homelab (VictoriaMetrics/Grafana/vmalert) and are explicitly *outside* the workload's
failure domain, with a known availability caveat (ADR-0004). Deploy safety must **not**
depend on the homelab being up.

## Decision

Use **AWS CodeDeploy blue/green with canary (or linear) traffic shifting** for the ECS
services fronted by the ALB.

- Two target groups per service (blue/green); CodeDeploy shifts a small percentage of
  traffic to green, waits (bake time), then completes or rolls back.
- **Automated rollback is gated on a small set of CloudWatch alarms** — ALB/target 5xx rate
  and target p99 latency — evaluated **in-cloud**. These deploy-gate alarms are deliberately
  separate from the homelab SLO burn-rate alerts.
- The `worker` (no ALB, SQS-driven) uses ECS rolling deployment with min/max healthy
  percentages; blue/green traffic shifting only applies to ALB-fronted services.

## Rationale — the "seam" between homelab SLOs and in-cloud deploy safety

Burn-rate SLO alerting evaluates in the homelab and can be unavailable during a homelab
outage (accepted in ADR-0004). If deployment rollback also depended on the homelab, a
homelab outage would remove our safety net exactly when we might be shipping. So we keep a
**minimal, in-cloud CloudWatch alarm set** whose *only* job is to gate deploys and trigger
CodeDeploy rollback. This is defensible and honest: rich SLO observability is homelab;
deploy safety is self-contained in AWS.

Canary over all-at-once because a canary limits blast radius to a traffic slice and gives
the deploy-gate alarms a real signal window before full cutover — the stronger
progressive-delivery story.

## Consequences

Positive:
- Zero-downtime deploys with automatic, metric-driven rollback independent of the homelab.
- Small, purpose-built CloudWatch alarm set — low cost, clear intent.
- Clean interview narrative connecting deployment safety to failure-domain design.

Negative:
- Blue/green needs duplicate target groups and briefly doubled tasks during a deploy
  (short-lived, minor cost).
- A second, small alerting surface (CloudWatch) exists alongside the homelab one — this is
  intentional and documented, not accidental duplication.

## Alternatives considered

- **ECS rolling update only:** simplest and cheapest, but weakest progressive-delivery
  story and no clean automated-rollback gate.
- **Blue/green all-at-once:** zero-downtime and alarm-gated, but full cutover gives the
  gate alarms no graduated signal window; weaker canary narrative.
