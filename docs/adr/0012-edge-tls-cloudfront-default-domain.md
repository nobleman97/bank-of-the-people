# ADR-0012: Public edge & TLS via CloudFront default domain (reject custom domain/ACM, reject Cloudflare proxy — for now)

- Status: Accepted
- Date: 2026-07-04
- Deciders: David Omokhodion
- Related: ADR-0007 (Single-account topology), ADR-0010 (egress), ADR-0011 (auth)

## Context

The public surface is a static frontend plus the `api` under `/api/*`, both fronted at the
edge (ARCHITECTURE §2; `API.md` §1). We need TLS and a CDN/WAF edge without standing cost or
DNS/cert lifecycle management, for an **ephemeral, spin-up/tear-down** demo (ADR-0007).

Options weighed: a **custom domain** (Route53 or an existing Cloudflare domain) with **ACM**
certs; **Cloudflare orange-cloud proxying** in front of the ALB (Cloudflare does CDN/TLS/WAF);
or the **CloudFront default `*.cloudfront.net` domain** with its built-in certificate.

The portfolio's explicit goal is to demonstrate the **AWS-native edge** — CloudFront + AWS
WAF + ACM — on a public payment endpoint (PLAN P5 "proves"). That biases toward keeping the
AWS edge rather than replacing it with a third-party proxy.

## Decision

Use the **CloudFront default domain** (`*.cloudfront.net`) with its bundled certificate for
the demo. No custom domain, no Route53, no ACM public certificate, no Cloudflare in the path.

- CloudFront terminates viewer TLS at the edge with the default certificate.
- CloudFront serves the static site from S3 (OAC) and path-routes `/api/*` to the ALB origin
  (same-origin, no CORS — `API.md` §1).
- **CloudFront → ALB origin runs over HTTP** inside AWS: an ALB has no certificate-eligible
  hostname without a custom domain, and CloudFront will not trust a self-signed origin cert.
  The public hop (viewer → CloudFront) is HTTPS; the AWS-internal hop is HTTP. This is a
  **documented demo simplification**, acceptable because it is internal to the VPC/AWS
  backbone and the payload is already behind edge TLS + WAF.
- **AWS WAF** is attached at the CloudFront distribution (and regionally at the ALB), so the
  edge-WAF-on-payments demonstration is preserved.

## Rationale

- **Why default domain:** zero DNS/cert lifecycle, zero domain cost, nothing to validate or
  renew, and it tears down cleanly with the stack — the right fit for an ephemeral demo, and
  it keeps CloudFront + AWS WAF (the skills being showcased) fully in the picture.
- **Why not a custom domain + ACM now:** it adds cert issuance/validation and a DNS record to
  manage for cosmetic benefit (a pretty hostname). Easy to add later — it is an ACM cert plus
  a CloudFront alias and one DNS record — so nothing here forecloses it.
- **Why not Cloudflare proxy:** Cloudflare doing CDN/TLS/WAF would make CloudFront + AWS WAF
  redundant and swap an AWS-platform demonstration for a third-party one, plus require locking
  the ALB to Cloudflare IPs to prevent origin bypass. Good for a real SaaS; wrong for this
  AWS-centric portfolio piece. Using an existing Cloudflare *domain* as DNS-only (grey-cloud)
  remains available if a custom hostname is later wanted, without changing the AWS edge.

## Consequences

Positive:
- No domain/DNS/ACM cost or lifecycle; cleanest possible ephemeral edge.
- Preserves the CloudFront + AWS WAF demonstration on the public payment path.
- Adding a custom domain later is additive (ACM cert + alias + DNS record), not a redesign.

Negative / risks:
- The CloudFront→ALB origin hop is **HTTP**, not end-to-end TLS — a demo simplification, noted
  openly. Mitigated by it being AWS-internal and behind edge TLS + WAF.
- `*.cloudfront.net` URLs are not brandable and rotate per distribution — fine for a demo,
  not for a real product.
- No custom-domain email/DNS features (irrelevant to this scope).

## Production equivalent / when this flips

Production uses a **custom domain with ACM end-to-end TLS**: an ACM cert on CloudFront (viewer)
and a domain cert on the ALB so the origin hop is HTTPS too, with Route53 (or Cloudflare
DNS-only) records and, where required, `aws:SourceArn`/origin-lock so only CloudFront reaches
the ALB. Flip the trigger: any real user-facing launch, a brand requirement, or a compliance
control mandating end-to-end TLS to the origin.

## Alternatives considered

- **Custom domain + ACM (Route53 or Cloudflare DNS-only):** the production shape; deferred as
  cosmetic for an ephemeral demo and additive later.
- **Cloudflare orange-cloud proxy → ALB:** simpler/cheaper edge but drops CloudFront + AWS WAF
  and needs origin lockdown; trades the AWS-edge demonstration away. Rejected for this piece.
- **CloudFront default domain (chosen):** lowest-friction edge that keeps the AWS demonstration.
