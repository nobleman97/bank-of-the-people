variable "aws_region" {
  description = "AWS region."
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment name (drives naming and tags)."
  type        = string
  default     = "dev"
}

variable "owner" {
  description = "Owner tag applied via provider default_tags."
  type        = string
  default     = "david-omokhodion"
}

variable "cost_center" {
  description = "CostCenter tag applied via provider default_tags."
  type        = string
  default     = "portfolio"
}

variable "vpc_cidr" {
  description = "CIDR block for the dev VPC."
  type        = string
  default     = "10.20.0.0/16"
}

variable "az_count" {
  description = "Number of AZs to span."
  type        = number
  default     = 2
}

variable "egress_mode" {
  description = "Internet egress path: 'instance' (fck-nat, ADR-0010) or 'gateway'."
  type        = string
  default     = "instance"
}

variable "nat_instance_type" {
  description = "fck-nat instance type."
  type        = string
  default     = "t4g.nano"
}

variable "endpoint_az_count" {
  description = "AZs to place interface endpoints in (1 = lowest per-session cost, ADR-0010)."
  type        = number
  default     = 1
}
