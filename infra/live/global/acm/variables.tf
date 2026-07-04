variable "aws_region" {
  description = "AWS region. Must be us-east-1 so the same cert can serve CloudFront (P5) as well as the regional ALB."
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment tag. The cert is account-global/shared, so 'global'."
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

variable "domain_name" {
  description = "Primary domain for the public ACM certificate."
  type        = string
  default     = "botp.cognitaid.com"
}

variable "subject_alternative_names" {
  description = "Additional names on the cert. The wildcard covers per-service/per-env hostnames."
  type        = list(string)
  default     = ["*.botp.cognitaid.com"]
}
