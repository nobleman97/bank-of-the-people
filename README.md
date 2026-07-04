# bank-of-the-people

A production-grade payments / money-movement service on AWS ECS, built as a portfolio
piece to demonstrate **DevOps, DevSecOps, and SRE** practice end to end. The application
is deliberately thin — money transfers backed by a double-entry ledger, synchronous
authorization plus asynchronous settlement and signed webhooks; the depth lives in the
**infrastructure, pipeline, and observability**.

> Status: **P0 (Foundations)**. Infrastructure is authored and validated but not yet
> applied — the project runs on a spin-up / capture-proof / tear-down loop (see
> [`docs/PLAN.md`](docs/PLAN.md)).

## What it demonstrates

- **DevOps** — modular Terraform, VPC/ECS Fargate/ALB, ECR, Service Connect, per-service
  autoscaling, dev→prod promotion, CodeDeploy canary.
- **DevSecOps** — GitHub OIDC (no long-lived keys), least-privilege IAM, image/IaC/secret
  scanning, SBOM + cosign signing, OPA policy-as-code, WAF, PCI-DSS/SOC 2 control mapping.
- **SRE** — SLIs/SLOs with burn-rate alerting, structured logging, distributed tracing to a
  self-hosted VictoriaMetrics/Grafana/Tempo stack, runbooks, and chaos exercises.

## Repository layout

```
services/{api,ledger,worker}   Go services (api gateway, double-entry ledger, settlement worker)
frontend/                      Static Next.js UI
infra/
  live/global/                 P0: GitHub OIDC provider + CI plan/apply roles
  live/<env>/<component>/      P1+: per-env, per-component roots
  modules/<name>/              Reusable Terraform modules
policy/                        OPA/Conftest policy-as-code
.github/workflows/             CI/CD (GitHub Actions via AWS OIDC)
docs/                          ARCHITECTURE, SLO, SECURITY, PLAN, API, ADRs, runbooks
```

## Documentation

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — topology, sync/async data flow, idempotency, tracing
- [`docs/API.md`](docs/API.md) — the `api` HTTP contract
- [`docs/SLO.md`](docs/SLO.md) — SLIs, SLOs, error budgets, burn-rate alerting
- [`docs/SECURITY.md`](docs/SECURITY.md) — IAM, secrets, encryption, WAF, compliance mapping
- [`docs/PLAN.md`](docs/PLAN.md) — the phased implementation plan (P0–P11)
- [`docs/adr/`](docs/adr/) — Architecture Decision Records
- [`CLAUDE.md`](CLAUDE.md) — working conventions

## Local validation

Run the same checks CI runs before pushing:

```bash
terraform fmt -check -recursive infra
terraform validate          # per root, after: terraform init -backend=false
tfsec infra                 # or: checkov -d infra
conftest test infra         # OPA policies in policy/
gitleaks detect --no-banner
trivy fs .                  # and: trivy image <ref> after a build
```

See [`CLAUDE.md`](CLAUDE.md) for conventions and the full toolchain.

## Cost

Every costed resource is ephemeral and `terraform destroy`-clean. P0 (OIDC provider +
IAM roles + S3/DynamoDB state) is effectively free. Real spend begins at P1 (NAT
instance, ALB) and is flagged per phase in [`docs/PLAN.md`](docs/PLAN.md).
