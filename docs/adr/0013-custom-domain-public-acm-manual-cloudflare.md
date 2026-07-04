# ADR-0013: Custom domain + free public ACM on the ALB, validated via manual Cloudflare DNS

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Supersedes: the "default CloudFront domain, no ACM, HTTP origin" posture of ADR-0012 (the
  parts about edge certificate and TLS placement; ADR-0012's rejection of the Cloudflare
  *proxy* still stands)
- Related: ADR-0007 (single-account topology), ADR-0010 (egress), ADR-0012 (edge TLS)

## Context

ADR-0012 deferred a custom domain + ACM as "cosmetic for an ephemeral demo, additive later",
and accepted a plaintext window: with the CloudFront default domain and an HTTP-only ALB,
there is **no TLS anywhere until CloudFront exists (P5)**, and even then the CloudFront→ALB
origin hop is HTTP (not end-to-end TLS).

Two facts change the calculus:

- **Public ACM certificates are free.** The only requirement is a domain the operator
  controls. The operator owns **`cognitaid.com`, managed on Cloudflare** — so the "bring a
  domain" precondition is already met at zero cost.
- The plaintext-public window on the ALB (P1–P4) is avoidable cheaply by terminating real TLS
  at the ALB now, rather than waiting for the CloudFront edge.

## Decision

Adopt a **custom subdomain with a free public ACM certificate**, terminating real TLS at the
ALB immediately, and validate it with **manual Cloudflare DNS (DNS-only / grey cloud)**.

- **Certificate:** one public ACM cert for **`botp.cognitaid.com` + `*.botp.cognitaid.com`**,
  DNS-validated, in **`us-east-1`** (so the same cert also serves CloudFront in P5). It lives
  in the **`global/` scope** (`infra/live/global/acm`) — persistent, issued/validated once,
  reused across every ephemeral session and both dev/prod, exactly like the shared ECR
  (ADR-0007).
- **Validation is manual and out-of-band.** The Terraform root creates the cert and **outputs
  the validation CNAME**; the operator adds it in Cloudflare once as **DNS-only**. No
  `aws_acm_certificate_validation` resource, so `apply` never blocks waiting on DNS. The
  validation record is permanent for the life of the cert.
- **ALB gets a real 443 HTTPS listener** using the cert, with **80 → 443 redirect**
  (`infra/live/dev/platform`, guarded by an `enable_https` toggle so the stack still applies
  before the cert reaches ISSUED). The ALB looks the cert up by domain
  (`data.aws_acm_certificate`, `statuses = ["ISSUED"]`), so it stays decoupled from the ACM
  root's state.
- **The per-session app record is manual too:** the operator points
  `botp.cognitaid.com → <alb-dns-name>` in Cloudflare (DNS-only) each session, because the
  ephemeral ALB hostname rotates on every rebuild. Chosen over Route53 alias automation to
  keep the path **free** and on the existing Cloudflare zone (no Route53 hosted-zone charge,
  no Cloudflare API token/provider).

## Rationale

- **Free and stronger posture.** Real ACM TLS at the ALB removes the P1–P4 plaintext window at
  zero dollar cost, and the same cert enables **end-to-end TLS to the origin** once CloudFront
  is added (CloudFront→ALB over HTTPS against `botp.cognitaid.com`), improving on ADR-0012's
  HTTP origin hop.
- **Better portfolio signal.** "Real domain + free public ACM + end-to-end TLS" is a stronger
  interview story than the default `*.cloudfront.net` cert.
- **Manual DNS is an acceptable trade for an ephemeral, single-operator stack.** Applies are
  already run by hand per session; adding/re-pointing one CNAME is marginal. It avoids a
  Route53 hosted-zone charge and avoids introducing a long-lived Cloudflare API token (which
  would cut against the OIDC/no-long-lived-secrets posture).
- **Cloudflare stays DNS-only.** This does **not** revive the Cloudflare *proxy* path ADR-0012
  rejected — grey-cloud records only; CloudFront + AWS WAF remain the edge.

## Consequences

Positive:
- TLS from P1 onward at no cost; end-to-end TLS unlocked for P5; cert issued once and reused.
- No Route53 charge, no third-party provider, no Cloudflare token to manage.

Negative / risks:
- **Manual per-session DNS:** the app CNAME must be re-pointed to the new ALB hostname each
  session (the cert and its validation record are stable; only the A/CNAME to the rotating
  ALB changes). A forgotten update means the hostname resolves to a dead ALB until fixed.
- **Human step in the loop:** validation and app records are added by hand in Cloudflare, not
  captured in Terraform state — documented in the demo runbook rather than codified.
- Cert lifecycle (renewal) is AWS-managed via DNS as long as the validation CNAME remains.

## Production equivalent / when this flips

A real deployment automates DNS: either move the zone (or delegate a subdomain) to **Route53**
for Terraform-managed **alias** records that track the load balancer automatically, or drive
Cloudflare via its Terraform provider with a scoped API token. At that point the manual CNAME
steps disappear and the app record tracks the ALB/CloudFront without operator action.

## Alternatives considered

- **Route53 subdomain delegation:** Terraform-managed alias records that auto-track the
  ephemeral ALB; cleanest automation, but a $0.50/mo hosted-zone charge and a one-time NS
  delegation. Deferred to keep the path free; named as the production direction.
- **Cloudflare Terraform provider:** fully free and keeps DNS on Cloudflare, but introduces a
  long-lived Cloudflare API token (secret) and a third-party provider. Rejected for now to
  avoid the standing credential.
- **Stay on ADR-0012 (default CloudFront domain, HTTP ALB):** zero DNS work, but keeps the
  plaintext window and forgoes free end-to-end TLS now that a domain is available. Superseded.
