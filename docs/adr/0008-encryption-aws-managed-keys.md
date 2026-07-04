# ADR-0008: Encryption at rest via AWS-managed keys (reject customer-managed CMKs)

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Related: ADR-0005 (Ledger datastore), ADR-0007 (Account/env topology), SECURITY.md

## Context

Every data store in the platform must be encrypted at rest: RDS (the ledger), SQS (main +
DLQ), S3 (frontend assets, state, artifacts, logs), Secrets Manager, and CloudWatch Logs.
KMS offers three key types:

- **Customer-managed keys (CMKs):** full control of key policy, rotation cadence, and
  cross-account grants; ~$1/key/month plus usage.
- **AWS-managed keys** (`aws/rds`, `aws/sqs`, `aws/s3`, `aws/secretsmanager`): per-account,
  per-service keys; AWS owns the policy and rotates them (annually); no monthly key charge,
  usage only.
- **AWS-owned keys:** invisible service-side keys (e.g. default log-group / SSE-S3
  encryption); no configuration, no charge.

The project is a cost-conscious, ephemeral, single-account portfolio stack
(ADR-0007) that spins up and tears down per session.

## Decision

Encrypt every data store at rest, but use **AWS-managed keys** rather than customer-managed
CMKs.

| Domain | Key |
|--------|-----|
| RDS Postgres | AWS-managed `aws/rds` |
| SQS (main + DLQ) | SSE-KMS with AWS-managed `aws/sqs` |
| S3 | SSE-KMS with AWS-managed `aws/s3` |
| Secrets Manager | AWS-managed `aws/secretsmanager` |
| CloudWatch Logs | **AWS-owned default encryption** (a specific key would require a CMK; none is attached) |

Task-role KMS permissions (`kms:Decrypt` / `kms:GenerateDataKey`) are scoped with the
`kms:ViaService` condition so a role can only use a key through its owning service.

## Rationale

- **Cost / simplicity fit the lifecycle.** CMKs carry a per-key monthly charge and add key
  policies and rotation config to maintain, for infrastructure that exists only during demo
  windows. Encryption-at-rest is still fully enforced with AWS-managed keys at no key charge.
- **The control that matters here is "data is encrypted at rest", and it is met.** What we
  give up — owning key policy, custom rotation, cross-account grants — is not exercised by a
  single-account ephemeral stack.
- **Honesty about the taxonomy.** CloudWatch Logs cannot attach an AWS-*managed* key; it is
  either AWS-owned default encryption or a CMK. We use the default. Stating this precisely is
  itself the point — it shows the AWS-owned vs AWS-managed vs customer-managed distinction is
  understood, not glossed.

## Consequences

Positive:
- No per-key monthly spend; less Terraform and no key-policy surface to get wrong.
- Encryption at rest everywhere; automatic AWS rotation of the managed keys.

Negative / risks:
- No control over key policy or rotation cadence, and no cross-account key sharing — a real
  gap versus regulated fintech practice, called out in SECURITY.md §7.
- Weaker "key management" demonstration for PCI-DSS 3.6–3.7 (satisfied by AWS as provider
  rather than demonstrated by us).

## Production equivalent

A regulated deployment would use **customer-managed CMKs** per data domain with explicit key
policies, defined rotation, and (in a multi-account Organization) cross-account grants and a
central key-management account. This is a straightforward swap — the modules encrypt with a
key reference, so pointing them at CMKs is a variable change, not a redesign.

## Alternatives considered

- **Customer-managed CMKs (original design):** the stronger control and the production
  answer, but per-key monthly cost and management overhead disproportionate to an ephemeral
  single-account portfolio stack.
- **AWS-owned everywhere (SSE-S3 / SSE-SQS / default):** even simpler, but the resources
  don't surface a named key in the account, which reads as a weaker (if functionally similar)
  encryption story; AWS-managed keys are the better-legible middle ground.
