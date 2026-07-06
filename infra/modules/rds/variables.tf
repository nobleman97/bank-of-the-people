variable "name_prefix" {
  description = "Prefix for named resources, e.g. botp-dev-ledger."
  type        = string
}

variable "vpc_id" {
  description = "VPC to place the instance and its subnet group in."
  type        = string
}

variable "private_subnet_ids" {
  description = "Private subnet IDs for the DB subnet group."
  type        = list(string)
}

variable "instance_class" {
  description = "RDS instance class. Smallest viable per ADR-0005."
  type        = string
  default     = "db.t4g.micro"
}

variable "allocated_storage" {
  description = "Allocated storage in GiB."
  type        = number
  default     = 20
}

variable "engine_version" {
  description = "Postgres version — major (\"16\") or major.minor (\"16.4\"); the parameter-group family is derived from the major component."
  type        = string
  default     = "16"
}

variable "database_name" {
  description = "Initial database name RDS creates at bring-up."
  type        = string
  default     = "ledger"
}

variable "master_username" {
  description = "Master DB username (its password is RDS-managed in Secrets Manager)."
  type        = string
  default     = "postgres"
}

variable "skip_final_snapshot" {
  description = "true in dev for a clean destroy (ADR-0005); set false where a session's state must survive."
  type        = bool
  default     = true
}
