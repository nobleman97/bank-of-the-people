output "vpc_id" {
  description = "VPC ID."
  value       = module.network.vpc_id
}

output "vpc_cidr" {
  description = "VPC CIDR block."
  value       = module.network.vpc_cidr
}

output "public_subnet_ids" {
  description = "Public subnet IDs."
  value       = module.network.public_subnet_ids
}

output "private_subnet_ids" {
  description = "Private subnet IDs."
  value       = module.network.private_subnet_ids
}

output "availability_zones" {
  description = "AZs the VPC spans."
  value       = module.network.availability_zones
}

output "endpoint_security_group_id" {
  description = "Interface VPC endpoint security group ID."
  value       = module.network.endpoint_security_group_id
}

output "nat_public_ip" {
  description = "Stable egress EIP of the fck-nat instance."
  value       = module.network.nat_public_ip
}
