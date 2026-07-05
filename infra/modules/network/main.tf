data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  azs              = slice(data.aws_availability_zones.available.names, 0, var.az_count)
  use_nat_instance = var.egress_mode == "instance"
  endpoint_subnets = slice(module.vpc.private_subnets, 0, var.endpoint_az_count)
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 6.0"

  name = var.name_prefix
  cidr = var.vpc_cidr
  azs  = local.azs

  public_subnets  = [for i in range(var.az_count) : cidrsubnet(var.vpc_cidr, 8, i)]
  private_subnets = [for i in range(var.az_count) : cidrsubnet(var.vpc_cidr, 4, i + 1)]

  enable_dns_hostnames    = true
  enable_dns_support      = true
  map_public_ip_on_launch = false

  # Managed NAT Gateway only when egress_mode = "gateway"; otherwise fck-nat below.
  enable_nat_gateway = !local.use_nat_instance
  single_nat_gateway = true

  manage_default_security_group  = true
  default_security_group_ingress = []
  default_security_group_egress  = []
}

# fck-nat NAT instance as the default route for private subnets (ADR-0010).
# ha_mode drives an ASG that self-heals by replacement; a pre-allocated EIP on the
# static ENI gives the stable egress address ADR-0010 calls for.
resource "aws_eip" "nat" {
  #checkov:skip=CKV2_AWS_19:Associated to the fck-nat static ENI inside the RaJiska/fck-nat module (external, not scanned under download-external-modules=false).
  count  = local.use_nat_instance ? 1 : 0
  domain = "vpc"
  tags   = { Name = "${var.name_prefix}-nat" }
}

module "fck_nat" {
  count   = local.use_nat_instance ? 1 : 0
  source  = "RaJiska/fck-nat/aws"
  version = "~> 1.6"

  name          = "${var.name_prefix}-nat"
  vpc_id        = module.vpc.vpc_id
  subnet_id     = module.vpc.public_subnets[0]
  instance_type = var.nat_instance_type

  ha_mode            = true
  eip_allocation_ids = [aws_eip.nat[0].allocation_id]

  update_route_tables = true
  route_tables_ids    = { for idx, rt in module.vpc.private_route_table_ids : "private-${idx}" => rt }
}

resource "aws_security_group" "endpoints" {
  #checkov:skip=CKV2_AWS_5:Attached to the interface VPC endpoints via the vpc-endpoints module's security_group_ids (external, not scanned under download-external-modules=false).
  name_prefix = "${var.name_prefix}-vpce-"
  description = "HTTPS from within the VPC to interface endpoints."
  vpc_id      = module.vpc.vpc_id

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_ingress_rule" "endpoints_https" {
  security_group_id = aws_security_group.endpoints.id
  description       = "HTTPS from VPC CIDR"
  cidr_ipv4         = var.vpc_cidr
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
}

module "vpc_endpoints" {
  source  = "terraform-aws-modules/vpc/aws//modules/vpc-endpoints"
  version = "~> 6.0"

  vpc_id             = module.vpc.vpc_id
  security_group_ids = [aws_security_group.endpoints.id]

  endpoints = merge(
    {
      s3 = {
        service         = "s3"
        service_type    = "Gateway"
        route_table_ids = module.vpc.private_route_table_ids
        tags            = { Name = "${var.name_prefix}-s3" }
      }
    },
    {
      for name in var.interface_endpoints : name => {
        service             = name
        private_dns_enabled = true
        subnet_ids          = local.endpoint_subnets
        tags                = { Name = "${var.name_prefix}-${name}" }
      }
    },
  )
}
