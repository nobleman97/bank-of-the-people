output "repository_urls" {
  description = "Map of service -> ECR repository URL (push/pull target)."
  value       = { for svc, m in module.ecr : svc => m.repository_url }
}

output "repository_arns" {
  description = "Map of service -> ECR repository ARN."
  value       = { for svc, m in module.ecr : svc => m.repository_arn }
}
