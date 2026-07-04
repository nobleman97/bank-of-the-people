variable "aws_region" {
  description = "AWS region for the provider and derived ARNs."
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment tag for these account-global resources."
  type        = string
  default     = "global"
}

variable "owner" {
  description = "Owner tag applied to every resource via provider default_tags."
  type        = string
  default     = "david-omokhodion"
}

variable "cost_center" {
  description = "CostCenter tag applied to every resource via provider default_tags."
  type        = string
  default     = "portfolio"
}

variable "github_org" {
  description = "GitHub org/user that owns the repository (the OIDC subject prefix)."
  type        = string
  default     = "nobleman97"
}

variable "github_repo" {
  description = "GitHub repository name, without the owner."
  type        = string
  default     = "bank-of-the-people"
}

variable "lock_table_name" {
  description = "Existing DynamoDB table used for Terraform state locking (referenced, not created)."
  type        = string
  default     = "terraform-locks"
}

variable "plan_role_name" {
  description = "Name of the read-only CI role used for PR plans."
  type        = string
  default     = "botp-github-actions-plan"
}

variable "apply_role_name" {
  description = "Name of the write-capable CI role used for applies on main/protected environments."
  type        = string
  default     = "botp-github-actions-apply"
}

variable "apply_role_policy_arns" {
  description = "Managed policy ARNs on the apply role. AdministratorAccess by default so CI can provision the whole stack; narrow later without a module change."
  type        = list(string)
  default     = ["arn:aws:iam::aws:policy/AdministratorAccess"]
}
