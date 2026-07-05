data "terraform_remote_state" "network" {
  backend = "s3"
  config = {
    bucket = var.state_bucket
    key    = "botp/dev/network.tfstate"
    region = var.aws_region
  }
}

data "aws_acm_certificate" "edge" {
  count       = var.enable_https ? 1 : 0
  domain      = var.acm_domain
  statuses    = ["ISSUED"]
  most_recent = true
}

locals {
  name_prefix = "botp-${var.environment}"
  network     = data.terraform_remote_state.network.outputs
  cert_arn    = one(data.aws_acm_certificate.edge[*].arn)

  ok_response = {
    content_type = "text/plain"
    message_body = "ok"
    status_code  = "200"
  }

  # Heterogeneous listener shapes can't unify under a ternary, so each is an independently
  # guarded single-entry map merged together. HTTPS: 80 redirects to 443, 443 serves the
  # fixed 200 (targets arrive in P3). Fallback (enable_https=false): plain HTTP:80.
  alb_listeners = merge(
    { for k, v in {
      http_redirect = {
        port     = 80
        protocol = "HTTP"
        redirect = { port = "443", protocol = "HTTPS", status_code = "HTTP_301" }
      }
    } : k => v if var.enable_https },
    { for k, v in {
      https = {
        port            = 443
        protocol        = "HTTPS"
        certificate_arn = local.cert_arn
        fixed_response  = local.ok_response
      }
    } : k => v if var.enable_https },
    { for k, v in {
      http = {
        port           = 80
        protocol       = "HTTP"
        fixed_response = local.ok_response
      }
    } : k => v if !var.enable_https },
  )
}

module "ecs_cluster" {
  source  = "terraform-aws-modules/ecs/aws//modules/cluster"
  version = "~> 7.0"

  name = "${local.name_prefix}-cluster"

  cluster_capacity_providers = ["FARGATE", "FARGATE_SPOT"]
  default_capacity_provider_strategy = {
    FARGATE      = { weight = 1, base = 1 }
    FARGATE_SPOT = { weight = 0 }
  }
}

# Public ALB. Serves a fixed 200 so the endpoint is verifiable before any service exists
# (targets arrive in P3). Real edge TLS via the ACM cert on the 443 listener (ADR-0013);
# 80 redirects to 443. Ingress is public for the health check and is locked to the
# CloudFront managed prefix list in P5.
module "alb" {
  source  = "terraform-aws-modules/alb/aws"
  version = "~> 10.0"

  name    = "${local.name_prefix}-alb"
  vpc_id  = local.network.vpc_id
  subnets = local.network.public_subnet_ids

  internal                   = false
  enable_deletion_protection = false

  security_group_ingress_rules = {
    http = {
      from_port   = 80
      to_port     = 80
      ip_protocol = "tcp"
      description = "HTTP from internet (redirect to 443; locked to CloudFront in P5)"
      cidr_ipv4   = "0.0.0.0/0"
    }
    https = {
      from_port   = 443
      to_port     = 443
      ip_protocol = "tcp"
      description = "HTTPS from internet (locked to CloudFront in P5)"
      cidr_ipv4   = "0.0.0.0/0"
    }
  }
  security_group_egress_rules = {
    to_vpc = {
      ip_protocol = "-1"
      cidr_ipv4   = local.network.vpc_cidr
      description = "To in-VPC targets"
    }
  }

  listeners = local.alb_listeners
}
