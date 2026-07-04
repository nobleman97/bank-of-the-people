# infra/live — root configurations

Each directory here is a **separate Terraform root** with its own remote-state key
(`botp/<scope>/<component>.tfstate`) — no shared monolithic state (CLAUDE.md).

| Root | State key | Scope | Introduced |
|------|-----------|-------|------------|
| `global/` | `botp/global/iam-oidc` | Account-global: GitHub OIDC provider + CI `plan`/`apply` roles. One per account. | **P0** |
| `global/ecr/` | `botp/global/ecr` | Account-global: shared **ECR** repos (api, ledger, worker). Persistent — a digest built once promotes across envs (ADR-0007). | **P1** |
| `global/acm/` | `botp/global/acm` | Account-global: free public **ACM cert** (`botp.cognitaid.com` + wildcard). Persistent — issued/validated once (manual Cloudflare DNS), reused by the ALB and CloudFront (ADR-0013). | **P1** |
| `dev/network/` | `botp/dev/network` | VPC, subnets, fck-nat egress, VPC endpoints. | **P1** |
| `dev/platform/` | `botp/dev/platform` | ECS Fargate cluster + public ALB (reads `dev/network` via remote state). | **P1** |
| `dev/<component>/` | `botp/dev/<component>` | Further per-component dev roots (rds, messaging, services, …). | P2+ |
| `prod/<component>/` | `botp/prod/<component>` | Per-component prod roots; same modules, prod tfvars. | P1+ |

Reusable modules live in `infra/modules/<name>` and are consumed by these roots. Each root
is a separate Terraform root even when nested (e.g. `global/` and `global/ecr/` are two
independent roots with distinct state keys — Terraform reads only the `.tf` in the current
directory, not recursively). The `global/ecr/` component was added nested rather than
migrating the already-applied P0 OIDC state.

Most roots compose **well-maintained public modules** (`terraform-aws-modules/*`,
`RaJiska/fck-nat`) rather than hand-rolled resources.

## Inter-component wiring

Downstream roots read upstream outputs via `terraform_remote_state` against the S3 backend
(e.g. `dev/platform` reads `botp/dev/network`). Apply order within an env: `network` →
`platform` → (later) data/services.

## Backend

All roots use the S3 backend with DynamoDB locking:

- Bucket: `devopsroyale-state-files-ccsji365i` (pre-existing, referenced)
- Lock table: `terraform-locks` (pre-existing, referenced)
- Region: `us-east-1`, `encrypt = true`

## Bootstrap ordering (one-time)

`global/` is applied **once, by a human with credentials**, to create the OIDC
provider and CI roles. Every later root is then planned/applied by GitHub Actions
via those roles — no static keys anywhere after bootstrap.
