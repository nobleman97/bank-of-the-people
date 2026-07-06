# Ingress is intentionally not declared here — it's attached by consumer roots (e.g.
# infra/live/dev/services/ledger) to avoid a circular security-group reference between
# this module and the service that's allowed to reach it.
resource "aws_security_group" "this" {
  #checkov:skip=CKV2_AWS_5:Ingress rules are attached by consumer roots (see comment above); this SG is still referenced (as an egress target) by every service that talks to this database.
  name_prefix = "${var.name_prefix}-rds-"
  description = "RDS Postgres for ${var.name_prefix}. Ingress attached by consumer roots."
  vpc_id      = var.vpc_id

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_db_subnet_group" "this" {
  name_prefix = "${var.name_prefix}-subnets-"
  subnet_ids  = var.private_subnet_ids

  lifecycle {
    create_before_destroy = true
  }
}

module "rds" {
  source  = "terraform-aws-modules/rds/aws"
  version = "~> 6.0"

  identifier = "${var.name_prefix}-rds"

  engine         = "postgres"
  engine_version = var.engine_version
  # Derive the parameter-group family from the engine's major version so it can never
  # drift from engine_version if a consumer overrides it (e.g. "17" -> "postgres17").
  family            = "postgres${split(".", var.engine_version)[0]}"
  instance_class    = var.instance_class
  allocated_storage = var.allocated_storage
  storage_encrypted = true # AWS-managed key (ADR-0008)

  db_name                     = var.database_name
  username                    = var.master_username
  manage_master_user_password = true

  multi_az               = false
  publicly_accessible    = false
  create_db_subnet_group = false
  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.this.id]

  skip_final_snapshot    = var.skip_final_snapshot
  deletion_protection    = false
  create_monitoring_role = false
}
