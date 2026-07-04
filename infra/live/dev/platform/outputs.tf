output "cluster_arn" {
  description = "ECS cluster ARN."
  value       = module.ecs_cluster.arn
}

output "cluster_name" {
  description = "ECS cluster name."
  value       = module.ecs_cluster.name
}

output "alb_dns_name" {
  description = "Public DNS name of the ALB (curl this for the P1 health check)."
  value       = module.alb.dns_name
}

output "alb_arn" {
  description = "ALB ARN."
  value       = module.alb.arn
}

output "alb_security_group_id" {
  description = "ALB security group ID (targets allow ingress from this in later phases)."
  value       = module.alb.security_group_id
}
