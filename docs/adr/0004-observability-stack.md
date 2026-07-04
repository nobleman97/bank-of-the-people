# ADR-0004: Observability via self-hosted VictoriaMetrics + Grafana, remote-write from ECS

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Related: ADR-0005 (Ledger datastore placement)

> Note: reconcile the ADR number with the other records once they exist. This
> assumes the observability-stack slot from the planned ADR set.

## Context

The payments platform runs on AWS ECS and needs metrics, dashboards, tracing,
and alerting. Candidate backends:

- **CloudWatch-native** (metrics, Logs, X-Ray).
- **AWS managed** Prometheus + Grafana (AMP/AMG).
- **Self-hosted** Prometheus/Grafana on Fargate.
- **Existing homelab** VictoriaMetrics + Grafana (K3s on Proxmox), already
  running with vmagent and kube-prometheus-stack.

Two properties of monitoring make it different from the workload it observes:
it sits off the user's synchronous critical path, and it tolerates short gaps
without user-visible harm. That means its failure mode is benign, so it can
safely live outside the workload's blast radius, including off-cloud.

Cost sensitivity is a stated project constraint, and reusing the homelab stack
demonstrates skills directly relevant to the target SRE/Platform roles.

## Decision

Use the existing homelab **VictoriaMetrics + Grafana** as the observability
backend for the project.

Collection and transport:

- Run **vmagent** (or an OpenTelemetry Collector) in ECS that scrapes/receives
  app metrics and **remote-writes** to VictoriaMetrics.
- The remote-write path is **authenticated and encrypted** over a private link
  (Cloudflare Tunnel or Tailscale/WireGuard). No unauthenticated ingestion
  endpoint is exposed to the public internet.
- Enable vmagent **on-disk buffering** (`-remoteWrite.tmpDataPath`,
  `-remoteWrite.maxDiskUsagePerURL`) so a homelab outage produces a
  backfillable gap on recovery rather than lost samples.

Traces:

- The OpenTelemetry Collector in ECS exports spans to a tracing backend in the
  homelab (Grafana Tempo). Trace context is propagated end to end (HTTP headers
  on the sync path, SQS message attributes across the queue).

Alerting:

- vmalert + Alertmanager (or Grafana alerting) evaluate rules **in the
  homelab**. This is accepted with eyes open: homelab downtime means alerting
  downtime for the duration (see Consequences).

## Consequences

Positive:

- Near-zero incremental cost; reuses a proven, already-operated stack.
- Monitoring lives outside the workload's failure domain.
- On-disk buffering makes metric continuity resilient to short homelab outages.

Negative / risks:

- Homelab availability affects metric continuity and, more importantly, alert
  evaluation. A single home site provides no HA for observability.
- Requires and depends on a secure tunnel between AWS and the homelab.

Mitigations:

- vmagent on-disk buffer + backfill covers short outages without data loss.
- Authenticated, encrypted transport; rotate the tunnel credential and treat it
  as a secret in Secrets Manager / the homelab secret store.
- Gaps in SLI series are treated as **unknown** in error-budget accounting, not
  as success or failure (see SLO addendum).

## Production equivalent

In a real production setting this homelab backend stands in for a central
observability platform: AMP/AMG, Grafana Cloud, or a dedicated observability
AWS account with its own availability guarantees and alerting that does not
share a failure domain with either the workload or a single home site. Stating
this explicitly keeps the choice legible as an intentional, cost-driven
portfolio decision rather than a gap.

## Alternatives considered

- **CloudWatch-native**: cheapest to wire inside AWS and always-on, but weakest
  demonstration of the target skills and per-metric/per-log costs accumulate.
- **AMP/AMG**: strong production answer, but ongoing spend and little
  differentiation versus operating the stack directly.
- **Self-hosted on Fargate**: more spend and setup for capability the homelab
  already provides.
