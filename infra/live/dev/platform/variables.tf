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

variable "state_bucket" {
  description = "S3 bucket holding remote state (to read the network component's outputs)."
  type        = string
  default     = "devopsroyale-state-files-ccsji365i"
}

variable "enable_https" {
  description = "Attach a 443 HTTPS listener (with 80->443 redirect) using the ACM cert. Set false to fall back to plain HTTP:80 if the cert is not yet ISSUED."
  type        = bool
  default     = true
}

variable "acm_domain" {
  description = "Primary domain of the ISSUED ACM cert to attach to the HTTPS listener (from the global/acm root)."
  type        = string
  default     = "botp.cognitaid.com"
}
