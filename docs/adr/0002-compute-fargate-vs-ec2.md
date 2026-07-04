# ADR-0002: ECS on Fargate (note where an EC2 capacity provider would differ)

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Related: ADR-0001 (Messaging), ADR-0007 (Account/env topology)

## Context

The compute plane is ECS. The workload is a small set of stateless Go services (`api`,
`ledger`, `worker`, `otel-collector`) plus a static frontend served off S3/CloudFront.
The project runs on a spin-up / capture-proof / tear-down loop, so clusters exist only
during demo/recording windows. We must choose the ECS launch/capacity model: **Fargate**
vs an **EC2 capacity provider**.

## Decision

Run all ECS services on **Fargate**.

- No EC2 nodes to provision, patch, scale, or bin-pack.
- Per-task sizing (CPU/memory) and per-task IAM roles align cleanly with least-privilege
  and with per-service autoscaling.
- Fast, clean bring-up/tear-down matches the ephemeral lifecycle; nothing lingers to bill.

## Rationale

For a bursty, short-lived, stateless workload, Fargate removes an entire operational axis
(node lifecycle) for a per-second compute premium that is negligible at this scale and
duration. The security story is also stronger: no shared node, no host to harden, task-level
isolation by default.

## Where an EC2 capacity provider would differ (interview-facing)

- **Cost at sustained scale:** for high, steady utilization, reserved/spot EC2 with good
  bin-packing is cheaper per vCPU-hour than Fargate. This workload is neither steady nor
  large, so that advantage does not apply.
- **Daemon workloads:** node-level agents (a per-host log/metrics daemon, security agents)
  map naturally to EC2 daemon services; on Fargate you run sidecars per task instead.
- **Hardware / kernel control:** GPUs, specific instance families, custom AMIs, privileged
  kernel tunables, or larger-than-Fargate task sizes require EC2.
- **Spot at the node level:** EC2 Spot capacity providers give fine-grained interruption
  handling; Fargate Spot exists but with less control.

## Consequences

Positive:
- Zero node management; task-level isolation and task-level IAM; clean ephemeral lifecycle.
- Simpler autoscaling story (per-service Application Auto Scaling, incl. worker-on-backlog).

Negative:
- Higher per-unit compute cost at sustained high utilization (irrelevant here).
- No node-level daemons or custom kernel/hardware access (not needed here).

## Alternatives considered

- **EC2 capacity provider:** better economics only under sustained high load; adds node
  patching/scaling/bin-packing overhead that contradicts the ephemeral design.
- **EKS/Kubernetes:** deliberately out of scope — this is an ECS project (and see ADR-0005
  on why the Kubernetes-native CNPG path was rejected for the ledger).
