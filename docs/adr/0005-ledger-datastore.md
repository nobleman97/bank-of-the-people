# ADR-0005: Ledger datastore on ephemeral AWS RDS PostgreSQL (reject homelab CNPG)

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Related: ADR-0003 (Ledger consistency model), ADR-0004 (Observability stack)

> Note: reconcile the ADR number with the other records once they exist. This
> complements the ledger consistency-model ADR; that one covers the double-entry
> model, this one covers where it physically lives.

## Context

The ledger is double-entry and append-only; a balance is the sum of its
entries. It sits on the **synchronous authorization path**: the API reserves
funds by calling the ledger before it can respond to the caller. Authorization
p99 latency and authorization success rate are headline SLOs.

Cost sensitivity raised the option of running the ledger on **CloudNativePG
(CNPG)** in the homelab K3s cluster to avoid RDS spend. The project's compute
plane is ECS, not Kubernetes.

## Decision

Run the ledger on **AWS RDS PostgreSQL** (or Aurora Serverless v2) in the same
region and VPC as the ECS services. Postgres is chosen over DynamoDB because a
double-entry ledger needs multi-row ACID transactions and constraints, which
Postgres provides directly and cheaply.

Treat the instance as **ephemeral**: provision it via Terraform for demo and
recording sessions, then destroy it, consistent with the project's
spin-up / capture-proof / tear-down lifecycle (see teardown addendum).

Reject homelab CNPG for the ledger.

## Rationale for rejecting homelab CNPG

- **Latency**: every transaction would round-trip AWS to the homelab over the
  internet on the synchronous path, making authorization p99 dominated by
  tunnel jitter rather than anything engineerable. This structurally breaks the
  auth-latency SLO.
- **Reliability inversion**: the source of truth for money would depend on home
  ISP, home power, and a tunnel, the three least reliable links in the system,
  on a project whose entire thesis is production-grade reliability for money
  movement.
- **Security**: PII and financial data would traverse the home network,
  weakening the security narrative exactly where it matters most.
- **Tooling mismatch**: CNPG is a Kubernetes operator. Adopting it forces either
  homelab K8s or a new EKS cluster into what is deliberately an ECS project.

The asymmetry with ADR-0004 is the point: monitoring can live outside the blast
radius because its failure mode is benign; the money ledger must live inside the
reliability and security boundary because its failure mode is not.

## Consequences

Positive:

- Ledger stays inside the AWS reliability and security boundary.
- Protects the authorization p99 SLO from home-network variability.
- Managed durability, automated backups, PITR, and failover for the money store.
- Clean, defensible interview narrative.

Negative:

- Some RDS cost during active sessions.
- Requires disciplined lifecycle so an ephemeral DB is reproducible: automated
  schema migrations and seed data on every bring-up.

Mitigations:

- Small instance (e.g. `db.t4g.micro`) or Aurora Serverless v2 with low minimum
  capacity; provision only during sessions.
- Terraform-managed lifecycle; migrations and seed run automatically on apply.
- `skip_final_snapshot = true` in dev for clean teardown; retain a final
  snapshot only if a session's state must survive.

> Directional on pricing: AWS rates and features (such as Aurora Serverless v2
> minimum capacity behavior) change, so confirm current numbers before sizing.

## Alternatives considered

- **Homelab CNPG**: rejected for the four reasons above.
- **Postgres container on ECS with EBS/EFS persistence**: avoids RDS but gives
  up managed backups, PITR, and failover; a weaker durability story for the
  money store.
- **DynamoDB**: AWS-native default, but multi-row ACID for double-entry
  settlement is awkward and a weaker consistency story than Postgres here.
