# ADR-0010: Internet egress via a fck-nat NAT instance (reject NAT Gateway, endpoints-only)

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Related: ADR-0001 (SQS messaging), ADR-0004 (Observability tunnel), ADR-0007 (Single-account topology)

## Context

ECS tasks run in **private subnets with no public IPs** (see `docs/ARCHITECTURE.md` §2).
Most AWS-service traffic is deliberately kept off the internet path via **VPC endpoints**:
interface endpoints for ECR api/dkr, Secrets Manager, CloudWatch Logs, and SQS, plus the
**free S3 gateway endpoint** — which also carries the heavy ECR *layer blobs* (ECR stores
layers in S3), so the largest data mover never touches the egress path.

That leaves exactly **two flows that require genuine internet egress**:

1. **`worker` → merchant webhook endpoints** — outbound HTTPS to arbitrary internet URLs;
   this is a core product feature (HMAC-signed webhook delivery with retry/backoff/DLQ).
2. **`otel-collector` → homelab tunnel** — the metrics/traces remote-write is an *outbound*
   dial from ECS to the homelab (ADR-0004); with no public task IPs it needs a route out.

Neither can be removed without changing the design, so **some egress path is required**.
Both flows are **low-volume and non-critical**: webhook dispatch tolerates retry/backoff/DLQ
(ADR-0001), and a metrics-tunnel gap is a benign, self-healing condition (ADR-0004, with
on-disk buffering). This is a **cost-conscious, ephemeral, single-operator** portfolio stack
(ADR-0007) whose goal is to *demonstrate* engineering judgment, including cost engineering.

Candidates: a **NAT instance** (`fck-nat` on `t4g.nano`), a managed **NAT Gateway** (single,
or one per AZ), or **endpoints-only with no NAT** (eliminate both egress flows).

## Decision

Use a **`fck-nat` NAT instance** as the internet-egress path for the private subnets, with
all AWS-service traffic kept on VPC endpoints so egress carries only webhooks and the tunnel.

- A single `t4g.nano` (ARM/Graviton) running the maintained **`fck-nat`** AMI, in a public
  subnet, as the default route (`0.0.0.0/0`) for the private route table(s).
- **Source/destination check disabled**; an **Elastic IP** for a stable egress address
  (enables allow-listing the egress IP at merchant/homelab endpoints).
- Deployed via an **Auto Scaling Group of min=max=1** so an instance/AZ failure self-heals by
  replacement — cheap resiliency without the per-AZ NAT-Gateway cost.
- Managed by Terraform in the `network` module behind a boolean input
  (`egress_mode = "instance" | "gateway"`), so switching to a managed NAT Gateway — or
  differing per environment — is a one-variable change, not a rewrite.
- The NAT instance is **ephemeral**: created and destroyed with the ECS stack each session
  (`docs/PLAN.md` P1), so it accrues cost only during an active demo/recording.

## Rationale

- **Cost:** a `t4g.nano` is ~10× cheaper hourly than a NAT Gateway and has **no per-GB
  processing fee** (you pay ordinary EC2 data-transfer instead). Because endpoints already
  removed the bulk of data from the egress path, the remaining flows are tiny — so the
  instance's cost floor is effectively just the nano's hourly rate. For a spin-up/tear-down
  stack this is the lowest sensible egress cost that still uses **no public task IPs**.
- **Right-sized risk:** the SPOF and patch surface a NAT instance introduces are acceptable
  here precisely because both egress consumers degrade gracefully (webhook redrive; buffered
  metrics). An ASG(1) turns "instance died" into "instance replaced," covering the common
  failure without per-AZ cost.
- **Why not a managed NAT Gateway as default:** it is zero-ops but carries a fixed hourly
  rate and a per-GB fee; with endpoints already off-loading the bulk traffic, that premium
  buys little here. It remains the **one-variable fallback** (`egress_mode = "gateway"`) and
  the production direction (below).
- **Why not endpoints-only / no NAT:** the homelab tunnel is outbound by design (ADR-0004)
  and realistic webhook delivery targets the public internet. Eliminating egress would
  require an in-VPC *mock* merchant **and** abandoning the outbound tunnel — breaking the
  observability design for a marginal saving. Rejected.
- **Cost subtlety (stated, not hidden):** interface endpoints bill ~$0.01/hr each **per AZ**;
  ~5 endpoints across 2 AZs (~$0.10/hr) can exceed even a NAT Gateway's hourly rate. The
  endpoints earn their place by keeping traffic private and removing the per-GB egress fee on
  image/log traffic — **not** by being unconditionally cheaper to stand up. They may be
  pinned to a single AZ for a lower per-session floor, matching the single-instance posture.
- **Portfolio signal:** choosing — and defending — `fck-nat` over the reflexive managed NAT
  Gateway is a direct demonstration of cost-aware platform engineering, with the boolean
  toggle showing you know exactly when to switch back.

## Consequences

Positive:
- Lowest defensible egress cost for the ephemeral model; no NAT per-GB fee. The heavy
  ECR-layer traffic rides the free S3 gateway endpoint, so egress spend is minimal.
- Stable egress IP (EIP) enables destination allow-listing for webhooks/tunnel.
- One Terraform variable (`egress_mode`) flips to a managed NAT Gateway.

Negative / risks:
- **Operational surface:** the AMI is a managed instance to patch/replace; mitigated by the
  maintained `fck-nat` image and immutable, ephemeral rebuilds each session.
- **Throughput ceiling:** a nano's network/PPS is far below a NAT Gateway's; acceptable for
  two low-volume flows, and the instance type is a one-line change if a flow grows.
- **SPOF within a session:** single instance; mitigated by ASG(1) self-replacement. A true
  AZ-fault-tolerant egress is explicitly out of scope (see production equivalent).
- Egress is not identity-aware; outbound destination control (e.g. webhook SSRF/allow-list
  concerns) is enforced at the application and security-group layer, not by the NAT.

## Production equivalent / when this flips

For a regulated, always-on platform this becomes **NAT Gateways, one per AZ**, for AZ-fault
isolation and managed throughput, fronted by an egress-controls layer (AWS Network Firewall
or a proxy with a domain allow-list for webhook destinations). Flip the `egress_mode` input
back to `"gateway"` and add per-AZ NAT. The trigger is any of: sustained/bursty egress beyond
a small instance, an uptime SLO on egress, or a compliance requirement for managed,
highly-available egress. If the homelab tunnel and webhook delivery were both re-homed onto
private connectivity (PrivateLink to a receiver, pull-based metrics), NAT could be dropped
entirely — but that is a different architecture, not this one.

## Alternatives considered

- **Single NAT Gateway:** zero-ops but a fixed hourly rate and per-GB fee; the premium is
  unjustified for two low-volume, degrade-gracefully flows in an ephemeral stack. Retained as
  the one-variable (`egress_mode = "gateway"`) fallback and the production direction.
- **NAT Gateway per AZ:** AZ-fault isolation at a multiplied hourly cost; the production
  equivalent, not warranted for a single-operator demo.
- **Endpoints-only, no NAT:** would force an in-VPC mock merchant and break the outbound
  homelab tunnel (ADR-0004); rejected as it changes the architecture for a marginal saving.
