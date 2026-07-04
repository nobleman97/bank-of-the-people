# infra/live — root configurations

Each directory here is a **separate Terraform root** with its own remote-state key
(`botp/<scope>/<component>.tfstate`) — no shared monolithic state (CLAUDE.md).

| Root | State key | Scope | Introduced |
|------|-----------|-------|------------|
| `global/` | `botp/global/iam-oidc` | Account-global: GitHub OIDC provider + CI `plan`/`apply` roles. One per account. | **P0** |
| `dev/<component>/` | `botp/dev/<component>` | Per-component dev roots (network, ecs, rds, …). | P1+ |
| `prod/<component>/` | `botp/prod/<component>` | Per-component prod roots; same modules, prod tfvars. | P1+ |

Reusable modules live in `infra/modules/<name>` and are consumed by these roots.
For P0 the only root is `global/`, which references the two **public**
`terraform-aws-modules/iam/aws` submodules rather than a hand-rolled OIDC module.

## Backend

All roots use the S3 backend with DynamoDB locking:

- Bucket: `devopsroyale-state-files-ccsji365i` (pre-existing, referenced)
- Lock table: `terraform-locks` (pre-existing, referenced)
- Region: `us-east-1`, `encrypt = true`

## Bootstrap ordering (one-time)

`global/` is applied **once, by a human with credentials**, to create the OIDC
provider and CI roles. Every later root is then planned/applied by GitHub Actions
via those roles — no static keys anywhere after bootstrap.
