module "network" {
  source = "../../../modules/network"

  name_prefix       = "botp-${var.environment}"
  vpc_cidr          = var.vpc_cidr
  az_count          = var.az_count
  egress_mode       = var.egress_mode
  nat_instance_type = var.nat_instance_type
  endpoint_az_count = var.endpoint_az_count
}
