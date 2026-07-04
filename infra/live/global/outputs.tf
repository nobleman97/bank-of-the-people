output "github_oidc_provider_arn" {
  description = "ARN of the GitHub Actions OIDC provider."
  value       = module.github_oidc_provider.arn
}

output "plan_role_arn" {
  description = "Read-only CI role ARN. Set as the repo variable AWS_PLAN_ROLE_ARN for PR plans."
  value       = module.plan_role.arn
}

output "apply_role_arn" {
  description = "Write-capable CI role ARN. Set as the repo variable AWS_APPLY_ROLE_ARN for applies."
  value       = module.apply_role.arn
}
