# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Conventions for building `bank-of-the-people`. These are binding defaults; deviations must
be justified in an ADR. See `docs/` for architecture, SLOs, security, and the phased plan.

## Current state (as of P0)

Only [P0](docs/PLAN.md) is implemented: `infra/live/global` (GitHub OIDC provider + CI
`plan`/`apply` roles, via the public `terraform-aws-modules/iam/aws` submodules — see
`infra/live/README.md`). Everything else is scaffolded but empty:

- `services/{api,ledger,worker}` and `frontend/` contain only `.gitkeep` — no Go modules,
  no application code yet. `go test ./...` etc. have nothing to run until P2/P3/P4 land.
- `policy/` (OPA/Conftest) is empty; `conftest test` is a no-op until policies exist.
- `infra/modules/` doesn't exist yet — it's created starting P1, when the first reusable
  module (`network`) is written; `infra/live/dev|prod` roots also start at P1.

Don't assume services, modules, or policies exist — check before referencing paths from
the plan below; most of `docs/PLAN.md`'s phases (P1–P11) describe work not yet started.

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
tfsec .            # or: checkov -d . --config-file=.checkov.yaml
conftest test .    # OPA policies in policy/
gitleaks detect --no-banner
trivy fs .         # and: trivy image <ref> after a build
go test ./...      # per service
```

CI runs across two workflows — `.github/workflows/ci-terraform.yml` (fmt/validate/tflint/
checkov, then `plan` on the `global` root via the read-only OIDC role) and
`.github/workflows/ci-security.yml` (gitleaks, `trivy fs`) — there is no Makefile wrapper.
Keep the CI steps and this list in sync so local and pipeline runs stay identical.

`.pre-commit-config.yaml` mirrors this same gate set locally in two stages: `pre-commit`
(fast — fmt, gitleaks, generic hygiene) and `pre-push` (heavier — validate, tflint, tfsec,
checkov, conftest, trivy fs). Install once with `pre-commit install`; the Go hooks
(`go vet`/`go test`) are present but commented out until a service has a `go.mod`.

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
