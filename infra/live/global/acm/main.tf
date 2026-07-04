# Public ACM cert (free), DNS-validated. Persistent/global (ADR-0013): issue and validate
# once, reuse across every ephemeral session and both the ALB and CloudFront (P5).
# No aws_acm_certificate_validation resource: validation is manual via Cloudflare (DNS-only),
# so apply must not block waiting for it. Add the CNAME from `validation_records`, then the
# cert moves to ISSUED out-of-band.
resource "aws_acm_certificate" "this" {
  #checkov:skip=CKV2_AWS_71:Wildcard SAN is intentional (ADR-0013) — one cert/one manual validation CNAME covers every per-env/per-service subdomain; blast radius accepted for an ephemeral single-operator stack.
  domain_name               = var.domain_name
  subject_alternative_names = var.subject_alternative_names
  validation_method         = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}
