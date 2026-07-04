# Security — bank-of-the-people

Security model for the payments platform: identity and access, secrets, data protection,
edge protection, supply-chain assurance, and a mapping of controls to PCI-DSS / SOC 2
requirements.

**Scope caveat (read first):** this is a portfolio demonstration. It does **not** process
real cardholder data (no PAN), is **not** a real Cardholder Data Environment (CDE), and
runs ephemerally in a single account ([ADR-0007](adr/0007-single-account-single-env-topology.md)).
The PCI-DSS / SOC 2 mapping below shows *how each control would satisfy a requirement*, not a
claim of certification.

---

## 1. Identity & access (IAM)

### 1.1 CI/CD → AWS: OIDC only, no long-lived keys

Platform and auth-model rationale (GitHub Actions vs GitLab/CodePipeline/Jenkins; OIDC vs
static keys) is recorded in [ADR-0009](adr/0009-cicd-github-actions-oidc.md).

- GitHub Actions authenticates to AWS via **OIDC federation** (GitHub OIDC provider +
  `sts:AssumeRoleWithWebIdentity`). **No long-lived AWS access keys exist anywhere** — not
  in secrets, not in the repo, not on a runner.
- Trust policies are scoped by `sub` (repo + branch/environment) and `aud`, so only the
  intended workflow on the intended ref can assume a role.
- **Two roles, split by privilege:**
  - `plan` role — **read-only** (plus state read/lock); used on pull requests to run
    `terraform plan` and all scanners. Cannot mutate infrastructure.
  - `apply` role — scoped write; used only on protected-branch/merge or environment-gated
    jobs to `terraform apply` and drive CodeDeploy.

### 1.2 Runtime: least-privilege per service

Each ECS service has its **own task role** with only the permissions it needs:

| Service | Task role permissions (illustrative, resource-scoped) |
|---------|-------------------------------------------------------|
| `api` | `sqs:SendMessage` on the **one** settlement queue; `secretsmanager:GetSecretValue` on its own secret; `kms:Decrypt`/`kms:GenerateDataKey` (via `kms:ViaService`) on the relevant AWS-managed key. No receive/delete. |
| `ledger` | RDS/IAM DB auth or DB-credential secret read; `kms:Decrypt`; no SQS, no S3. |
| `worker` | `sqs:ReceiveMessage`/`DeleteMessage`/`GetQueueAttributes` on main + DLQ; webhook-signing secret read; `kms:Decrypt`. No `SendMessage` to arbitrary queues. |
| `otel-collector` | tunnel-credential secret read; `kms:Decrypt`. No data-plane access. |

- A separate **task execution role** (shared, minimal) handles ECR image pull and
  CloudWatch Logs — distinct from the task role that grants app permissions.
- Policies are resource-scoped (specific queue ARNs, specific secret ARNs) and constrain
  KMS use to the relevant service via `kms:ViaService` — no wildcards on data-plane actions.

---

## 2. Secrets management

- **AWS Secrets Manager** holds: RDS credentials (with rotation enabled), the webhook
  **HMAC signing key**, and the **homelab tunnel credential**. Each is encrypted at rest
  with the AWS-managed `aws/secretsmanager` key and read at runtime via the service's task
  role.
- **SSM Parameter Store** holds non-secret configuration (feature flags, endpoints, tuning).
- **Never in code, never in plaintext:** no secrets in env files, task-definition literals,
  or the repo. `gitleaks` runs in CI to enforce this (see §5).
- Tunnel credential is rotated and treated as a first-class secret so a torn-down stack
  cannot be impersonated on the remote-write endpoint (ADR-0004 lifecycle note).

---

## 3. Data protection

### 3.1 Encryption at rest — AWS-managed keys

Encryption at rest is enabled everywhere, using **AWS-managed KMS keys** (the per-service
`aws/*` keys) rather than customer-managed keys (CMKs). This is a deliberate cost/simplicity
choice for an ephemeral portfolio stack — encryption is still enforced on every data store;
we simply do not own the key policy or rotation schedule. Rationale and trade-offs are in
[ADR-0008](adr/0008-encryption-aws-managed-keys.md).

| Domain | Encryption |
|--------|-----------|
| RDS Postgres (ledger) | storage encrypted with the AWS-managed `aws/rds` key; automated backups/snapshots inherit it |
| SQS (main + DLQ) | SSE-KMS with the AWS-managed `aws/sqs` key |
| S3 (frontend assets, state, artifacts, logs) | SSE-KMS with the AWS-managed `aws/s3` key |
| Secrets Manager | AWS-managed `aws/secretsmanager` key |
| CloudWatch Logs | **AWS-owned default encryption** — log groups are encrypted at rest by default; associating a *specific* key would require a CMK, which we intentionally do not use, so no key is attached |

### 3.2 Encryption in transit

- **ACM** certificates on the ALB (HTTPS) and CloudFront; HTTP→HTTPS redirect.
- TLS from services to RDS.
- Homelab remote-write/OTLP travels an **authenticated, encrypted tunnel** (Cloudflare
  Tunnel or Tailscale/WireGuard) — no unauthenticated ingestion endpoint on the internet
  (ADR-0004).

### 3.3 Network isolation

- ECS tasks and RDS in **private subnets**, no public IPs. Only the ALB (API) and CloudFront
  (frontend) are internet-facing.
- Security groups are least-privilege and reference-based (ALB SG → api SG → ledger SG → RDS
  SG); the ledger accepts traffic only from `api`'s SG, RDS only from `ledger`'s SG.
- VPC endpoints keep ECR/Secrets/Logs/SQS/S3 traffic on the AWS network.

---

## 4. Edge protection — WAF

- **Regional WAF web ACL on the ALB** (the public payment endpoint): AWS Managed Rules
  (Common Rule Set, SQLi, Known Bad Inputs) **plus a rate-based rule on `POST /transfers`**
  to blunt abuse/enumeration of the money endpoint.
- **CloudFront WAF web ACL** for the static frontend (managed rule baseline).
- WAF logs go to an encrypted destination for later inspection.

---

## 5. Supply-chain assurance (DevSecOps gates in CI)

Every change passes through automated gates before it can deploy:

| Stage | Tool | Gate |
|-------|------|------|
| Secret scanning | **gitleaks** | fail on any detected secret |
| IaC scanning | **tfsec** + **Checkov** | fail on high-severity misconfig |
| Policy-as-code | **OPA / Conftest** | deny `:latest` images, require non-root containers, require encryption + required tags, require least-priv patterns |
| Image vuln scanning | **Trivy** + **ECR scan-on-push** | fail on high/critical CVEs |
| SBOM | **Syft** | generate SBOM per image, publish as build artifact |
| Image signing | **cosign** | sign images; deploy only signed images |
| SAST | Go static analysis (`go vet`, staticcheck) + CodeQL | fail on findings above threshold |

- Images are **immutable** (digest-pinned) and promoted across environments by digest, not
  rebuilt — the artifact that was scanned/signed is the artifact that runs.
- **CloudTrail** is enabled for an account-level audit trail; access logs (ALB, CloudFront,
  WAF) are retained encrypted.

---

## 6. PCI-DSS / SOC 2 control mapping

How each implemented control maps to a requirement. (Demonstration mapping — see scope
caveat; no real CDE / no PAN.)

| Control (this project) | PCI-DSS v4.0 | SOC 2 (TSC) |
|------------------------|--------------|-------------|
| WAF on public payment endpoint + rate limiting | 6.4.1 / 6.4.2 (public-facing web app protection) | CC6.6 (boundary protection) |
| Least-privilege IAM, per-service task roles | 7.2 (need-to-know), 7.3 | CC6.1 / CC6.3 (logical access) |
| OIDC federation, **no long-lived keys**, scoped trust | 8.2 / 8.3 (identify & authenticate access) | CC6.1 (credentials) |
| Encryption at rest via AWS-managed keys (RDS, SQS, S3, Secrets; AWS-owned default for Logs) | 3.5 (protect stored data); key lifecycle managed by AWS (3.6–3.7 satisfied by the provider, not demonstrated by us) | CC6.7 (data at rest) |
| TLS/ACM in transit + authenticated tunnel | 4.2.1 (strong crypto in transit) | CC6.7 (data in transit) |
| Secrets Manager + rotation, no plaintext secrets | 3.6 / 8.3.9 (credential mgmt) | CC6.1 |
| Image vuln scanning (Trivy/ECR) | 6.3.1 / 6.3.2 (identify vulns) / 11.3 | CC7.1 (detect vulnerabilities) |
| SAST + IaC scan + policy-as-code in pipeline | 6.2 / 6.3 (secure SDLC) | CC8.1 (change management) |
| SBOM + cosign signing, immutable digests | 6.3.2 / 6.5 (integrity of changes) | CC7.1 / CC8.1 |
| CloudTrail + ALB/CloudFront/WAF access logs | 10.2 / 10.3 (audit logging) | CC7.2 (monitoring) |
| CodeDeploy canary + deploy-gate rollback | 6.5 (change control, safe deploys) | CC8.1 |
| Network segmentation (private subnets, SGs, endpoints) | 1.2 / 1.3 (network controls) | CC6.6 |
| SLOs + burn-rate alerting + IR runbook | 10.4 / 12.10 (monitoring & incident response) | CC7.3 / CC7.4 (incident response) |

---

## 7. Known limitations (stated, not hidden)

- **Single account** — no account-boundary separation-of-duties; production equivalent is a
  multi-account Organization (ADR-0007).
- **Homelab alerting availability** — burn-rate alerts evaluate in the homelab and share its
  uptime; deploy-gate alarms are the in-cloud mitigation (ADR-0004, ADR-0006).
- **Ephemeral by design** — infrastructure exists only during sessions; long-term audit-log
  retention would be added for a real deployment.
- **AWS-managed keys, not CMKs** — encryption at rest is on everywhere, but we do not own key
  policy, rotation cadence, or cross-account grants; a regulated deployment would use
  customer-managed keys for explicit key-management controls (ADR-0008).
