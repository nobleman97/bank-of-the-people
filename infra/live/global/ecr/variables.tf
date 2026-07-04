variable "aws_region" {
  description = "AWS region for the provider."
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment tag. ECR is account-global/shared, so this is 'global'."
  type        = string
  default     = "global"
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

variable "repository_namespace" {
  description = "ECR namespace prefix for repositories (e.g. botp/api)."
  type        = string
  default     = "botp"
}

variable "services" {
  description = "Services that get a container image / ECR repo. Frontend is static (S3), so it is excluded."
  type        = list(string)
  default     = ["api", "ledger", "worker"]
}

variable "untagged_expire_days" {
  description = "Expire untagged images older than this many days (cost hygiene; deploys are digest-pinned)."
  type        = number
  default     = 14
}
