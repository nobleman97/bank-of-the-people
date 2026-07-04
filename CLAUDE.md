# CLAUDE.md — working conventions

Conventions for building `bank-of-the-people`. These are binding defaults; deviations must
be justified in an ADR. See `docs/` for architecture, SLOs, security, and the phased plan.

## Project shape

- Monorepo. Go services under `services/{api,ledger,worker}`; static frontend under
  `frontend/`; Terraform under `infra/`; OPA/Conftest policies under `policy/`; CI under
  `.github/workflows/`; docs + ADRs + runbooks under `docs/`.
- Project prefix: **`botp`**. Region: **`us-east-1`**. Single account, single live env
  (see `docs/adr/0007-single-account-single-env-topology.md`).

## Terraform

- **Modules vs live:** reusable, environment-agnostic modules in `infra/modules/<name>`
  (`network`, `ecs-service`, `alb`, `sqs`, `rds`, `waf`, `cloudfront`, `observability`).
  Deployed roots in `infra/live/<scope>/` pass concrete inputs via tfvars and own their state.
  Modules never hardcode env — env comes from inputs. Prefer well-maintained public modules
  over hand-rolled ones where a solid one exists (e.g. OIDC via `terraform-aws-modules/iam`).
- **Scopes:** `<scope>` is `dev`, `prod`, or **`global`**. `global/` holds account-singleton
  resources that belong to no environment — the GitHub OIDC provider and CI `plan`/`apply`
  roles — and is bootstrapped once by a human; envs are then deployed keylessly by those
  roles (see ADR-0007).
- **State:** remote in S3 with **DynamoDB locking**. One **state key per component per scope**
  (e.g. `botp/global/iam-oidc`, `botp/dev/network`, `botp/dev/ecs`). No shared monolithic state.
- **Naming:** `botp-<env>-<resource>` (e.g. `botp-dev-api`, `botp-prod-ledger-rds`).
- **Tagging (provider `default_tags`):** `Project=bank-of-the-people`, `Environment`,
  `Service`, `ManagedBy=terraform`, `Owner`, `CostCenter`. Every resource inherits these.
- **Idempotent + reviewable:** `apply` must be clean; a re-plan is a no-op. Prefer explicit
  resources over clever meta-programming.

## Security rules (non-negotiable)

- **OIDC only.** GitHub Actions → AWS via OIDC. **No long-lived AWS keys** anywhere.
- **Least privilege.** Per-service task roles, resource-scoped ARNs, no data-plane
  wildcards. Split CI roles: read-only `plan`, scoped `apply`.
- **No plaintext secrets.** Secrets in Secrets Manager / SSM; non-secret config in SSM
  Parameter Store. `gitleaks` gates every PR.
- **Encryption everywhere.** Encryption at rest on RDS, SQS, S3, Secrets Manager, and
  CloudWatch Logs using **AWS-managed keys** (not CMKs — see ADR-0008); logs use AWS-owned
  default encryption. TLS in transit; homelab transport over an authenticated tunnel only.
- **Containers:** non-root, read-only root FS where possible, no `:latest` — deploy
  **digest-pinned, cosign-signed** images only.
- Details: `docs/SECURITY.md`.

## CI/CD expectations

- **On PR (read-only `plan` role):** `terraform fmt -check` · `validate` · `tflint` ·
  `tfsec` · `checkov` · `conftest test` · `gitleaks` · `trivy fs` · Go tests/SAST ·
  `terraform plan`. PRs cannot mutate infra.
- **On merge / env-gated (scoped `apply` role):** build image → `trivy image` + ECR
  scan-on-push → Syft SBOM → cosign sign → `terraform apply` → **CodeDeploy canary** with
  alarm-gated rollback.
- **Promotion:** dev→prod promotes the **same image digest** (scanned/signed once), never a
  rebuild.
- Any failing gate blocks the pipeline.

## Local validation

Run before pushing (mirror CI):

```bash
terraform fmt -check -recursive
terraform validate
tflint
tfsec .            # or: checkov -d .
conftest test .    # OPA policies in policy/
gitleaks detect --no-banner
trivy fs .         # and: trivy image <ref> after a build
go test ./...      # per service
```

CI (`.github/workflows/ci.yml`) runs these same commands directly — there is no Makefile
wrapper. Keep the CI steps and this list in sync so local and pipeline runs stay identical.

## Documentation discipline

- One ADR per significant decision in `docs/adr/NNNN-title.md`; never edit an Accepted ADR
  to change its decision — supersede it with a new one.
- Keep `docs/ARCHITECTURE.md`, `docs/API.md`, `docs/SLO.md`, `docs/SECURITY.md`, and
  `docs/PLAN.md` consistent with the ADRs — especially the fixed decisions ADR-0004
  (observability) and ADR-0005 (ledger datastore).
- `docs/API.md` is the `api` HTTP contract and the seam between P3 (`api`) and P5 (frontend);
  a contract change and its implementation land in the **same PR**, with frontend types
  regenerated from it.
- Runbooks live in `docs/runbooks/` and are seeded as their scenarios are implemented
  (settlement DLQ redrive, chaos exercises, incident response).
