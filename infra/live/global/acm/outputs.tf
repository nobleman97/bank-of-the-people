output "certificate_arn" {
  description = "ARN of the ACM certificate (referenced by the ALB and later CloudFront)."
  value       = aws_acm_certificate.this.arn
}

output "certificate_status" {
  description = "Cert status. PENDING_VALIDATION until the Cloudflare CNAME is added, then ISSUED."
  value       = aws_acm_certificate.this.status
}

output "validation_records" {
  description = "CNAME(s) to add in Cloudflare as DNS-only (grey cloud) to validate the cert."
  value = {
    for o in aws_acm_certificate.this.domain_validation_options :
    o.domain_name => {
      name  = o.resource_record_name
      type  = o.resource_record_type
      value = o.resource_record_value
    }
  }
}
