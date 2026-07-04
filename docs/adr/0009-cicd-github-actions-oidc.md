# ADR-0009: CI/CD on GitHub Actions with AWS OIDC (reject GitLab CI, CodePipeline/Jenkins, static keys)

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Related: ADR-0006 (Deployment strategy: CodeDeploy canary), ADR-0007 (Single-account topology), ADR-0008 (Encryption)

## Context

The project needs one CI/CD system to run the DevSecOps gate set (see `CLAUDE.md` and
`docs/SECURITY.md`) on every pull request, and to build, scan, sign, and deploy on merge.
Two things must be true regardless of platform:

- **No long-lived AWS credentials anywhere** — a hard constraint of the design. The pipeline
  must authenticate to AWS with short-lived, federated credentials.
- **Minimal standing cost and operational weight** — this is an ephemeral, spin-up/tear-down
  portfolio project (ADR-0007); a pipeline that requires a running server or control plane
  contradicts that.

The repo is a public GitHub monorepo (portfolio, blog, and interview piece), so the CI
platform is also part of what a reviewer sees first.

Candidates for the CI platform: **GitHub Actions**, **GitLab CI**, **AWS CodePipeline +
CodeBuild**, **self-managed Jenkins**. Candidates for AWS auth: **GitHub OIDC federation**
vs **long-lived IAM access keys stored as CI secrets**.

## Decision

Use **GitHub Actions** as the sole CI/CD platform, authenticating to AWS via the **GitHub
OIDC identity provider** and short-lived role assumption. **No static AWS keys** are stored
in the repo or in GitHub secrets.

- One OIDC provider (`token.actions.githubusercontent.com`) is registered in the account.
- Two roles, split by privilege (least-privilege, ADR-aligned): a read-only **`plan`** role
  assumed by PR workflows, and a scoped **`apply`** role assumed by merge/deploy workflows.
- Role trust policies are constrained on the `sub` claim (repo + branch/environment) and
  `aud`, so only workflows from this repository — and, for `apply`, only from the protected
  branch/environment — can assume them.
- The pipeline stages themselves (gate set on PR; build → scan → SBOM → sign → apply →
  CodeDeploy canary on merge) are defined in ADR-0006 and `CLAUDE.md`; this ADR fixes the
  platform and the auth model, not the stage list.

## Rationale

- **GitHub Actions vs GitLab CI:** the source already lives on GitHub for portfolio
  visibility; GitLab CI would mean either mirroring the repo or hosting elsewhere, adding
  moving parts for no benefit. Actions has first-class, well-documented OIDC-to-AWS support
  and a large public-example surface that makes the pipeline easy for an interviewer to read.
- **GitHub Actions vs CodePipeline/CodeBuild:** keeping CI *out* of AWS is deliberate. It
  keeps AWS-side standing cost near zero, keeps the account boundary clean (CI is an external
  federated identity, not an in-account principal with persistent infrastructure), and makes
  the OIDC trust story the centerpiece rather than an internal detail. CodePipeline also adds
  per-pipeline monthly cost that fights the ephemeral model.
- **GitHub Actions vs Jenkins:** a self-managed Jenkins controller is a standing server to
  patch, secure, and pay for — the exact operational weight this project avoids everywhere
  else. Rejected on cost and toil.
- **OIDC vs static keys:** long-lived IAM keys in CI are the single most common cloud-breach
  root cause and are explicitly forbidden by the design. OIDC issues short-lived credentials
  per run, scoped by trust policy to this repo and branch, with nothing to leak, rotate, or
  scan for. This is the control that `gitleaks` and the "no plaintext secrets" rule exist to
  protect, so removing the secret entirely is strictly better than guarding it.

## Consequences

Positive:
- No AWS secret to store, rotate, or leak; credentials are short-lived and repo-scoped.
- Zero standing CI infrastructure or AWS-side pipeline cost; fits spin-up/tear-down.
- Least-privilege is enforced at the identity boundary via split `plan`/`apply` roles.
- Public, readable workflows double as portfolio evidence of the OIDC pattern.

Negative / risks:
- Ties CI to GitHub as a platform; a move off GitHub would mean re-authoring workflows
  (acceptable — the repo's home is GitHub by design).
- OIDC trust policies must be written carefully: an overly broad `sub` condition (e.g. wildcard
  branch) would let unintended workflows assume the role. Mitigated by pinning `sub` to the
  repo and, for `apply`, to the protected branch/environment.
- GitHub-hosted runners execute build/scan steps; supply-chain trust in the runner and in
  third-party actions is assumed — mitigated by pinning actions to commit SHAs and running
  the Trivy/tfsec/Checkov/gitleaks/cosign gates in-pipeline.

## Production equivalent / when this flips

At organization scale, this pattern holds — GitHub Actions + OIDC is a standard enterprise
choice. What would change: OIDC roles would live in a shared CI/CD or security account and
assume into workload accounts (ADR-0007's multi-account production equivalent); an internal
platform team might add a self-hosted runner fleet (in a private subnet) for network-isolated
builds; and third-party action pinning would be enforced by policy (e.g. `allowed_actions`
plus a pinning check). If the org standardized on GitLab or an internal platform, the CI
front end would change but the OIDC-federation-not-static-keys principle would not.

## Alternatives considered

- **GitLab CI:** viable and OIDC-capable, but the repo lives on GitHub; adds mirroring/hosting
  for no benefit here.
- **AWS CodePipeline + CodeBuild:** pulls CI into the account (standing cost, more account
  surface) and buries the federated-identity story; rejected for the ephemeral, cost-conscious
  model.
- **Self-managed Jenkins:** a standing server to operate and secure; rejected on cost and toil.
- **Static IAM access keys as CI secrets:** rejected outright — long-lived cloud credentials
  are forbidden by the design and are the primary risk OIDC eliminates.
