output "vpc_id" {
  description = "VPC ID."
  value       = module.vpc.vpc_id
}

output "vpc_cidr" {
  description = "VPC CIDR block."
  value       = var.vpc_cidr
}

output "public_subnet_ids" {
  description = "Public subnet IDs (ALB, NAT instance)."
  value       = module.vpc.public_subnets
}

output "private_subnet_ids" {
  description = "Private subnet IDs (ECS tasks, RDS)."
  value       = module.vpc.private_subnets
}

output "private_route_table_ids" {
  description = "Private route table IDs."
  value       = module.vpc.private_route_table_ids
}

output "availability_zones" {
  description = "AZs the VPC spans."
  value       = local.azs
}

output "endpoint_security_group_id" {
  description = "Security group guarding the interface VPC endpoints."
  value       = aws_security_group.endpoints.id
}

output "nat_public_ip" {
  description = "Elastic IP of the fck-nat instance (null when egress_mode = gateway)."
  value       = local.use_nat_instance ? aws_eip.nat[0].public_ip : null
}
