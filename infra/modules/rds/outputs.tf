output "security_group_id" {
  description = "SG guarding the RDS instance. Consumer roots attach their own ingress rule to this."
  value       = aws_security_group.this.id
}

output "endpoint" {
  description = "host:port connection endpoint."
  value       = module.rds.db_instance_endpoint
}

output "address" {
  description = "Bare hostname (no port)."
  value       = module.rds.db_instance_address
}

output "port" {
  value = module.rds.db_instance_port
}

output "master_user_secret_arn" {
  description = "Secrets Manager ARN of the RDS-managed master password. Only Terraform (via the postgresql provider) reads this — no application ever does."
  value       = module.rds.db_instance_master_user_secret_arn
}

output "database_name" {
  value = var.database_name
}
