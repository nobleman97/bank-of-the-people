provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "bank-of-the-people"
      Environment = var.environment
      ManagedBy   = "terraform"
      Owner       = var.owner
      CostCenter  = var.cost_center
    }
  }
}
