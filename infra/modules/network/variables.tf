variable "name_prefix" {
  description = "Prefix for all named resources, e.g. botp-dev."
  type        = string
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
  default     = "10.20.0.0/16"
}

variable "az_count" {
  description = "Number of AZs to span (public + private subnet per AZ)."
  type        = number
  default     = 2
}

variable "egress_mode" {
  description = "Internet egress path for private subnets: 'instance' (fck-nat, ADR-0010) or 'gateway' (managed NAT Gateway)."
  type        = string
  default     = "instance"

  validation {
    condition     = contains(["instance", "gateway"], var.egress_mode)
    error_message = "egress_mode must be 'instance' or 'gateway'."
  }
}

variable "nat_instance_type" {
  description = "Instance type for the fck-nat NAT instance (egress_mode = instance)."
  type        = string
  default     = "t4g.nano"
}

variable "endpoint_az_count" {
  description = "How many AZs to place interface VPC endpoints in. 1 keeps the per-session cost floor low (ADR-0010); prod uses az_count."
  type        = number
  default     = 1
}

variable "interface_endpoints" {
  description = "Interface VPC endpoint service names (short) to create."
  type        = list(string)
  default     = ["ecr.api", "ecr.dkr", "secretsmanager", "logs", "sqs"]
}
