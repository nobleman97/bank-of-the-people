data "aws_caller_identity" "current" {}

locals {
  repo           = "${var.github_org}/${var.github_repo}"
  lock_table_arn = "arn:aws:dynamodb:${var.aws_region}:${data.aws_caller_identity.current.account_id}:table/${var.lock_table_name}"
}

module "github_oidc_provider" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-oidc-provider"
  version = "~> 6.6"

  url = "https://token.actions.githubusercontent.com"
}

# `terraform plan` takes a DynamoDB lock, which ReadOnlyAccess does not grant.
data "aws_iam_policy_document" "state_lock" {
  statement {
    sid       = "TerraformStateLock"
    effect    = "Allow"
    actions   = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem"]
    resources = [local.lock_table_arn]
  }
}

resource "aws_iam_policy" "state_lock" {
  name        = "botp-tf-state-lock"
  description = "Terraform S3 backend DynamoDB state locking for GitHub Actions CI roles."
  policy      = data.aws_iam_policy_document.state_lock.json
}

# Read-only: any ref or PR on the repo can plan, never mutate.
module "plan_role" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role"
  version = "~> 6.6"

  name = var.plan_role_name

  enable_github_oidc     = true
  oidc_wildcard_subjects = ["${local.repo}:*"]

  policies = {
    ReadOnlyAccess = "arn:aws:iam::aws:policy/ReadOnlyAccess"
    StateLock      = aws_iam_policy.state_lock.arn
  }

  depends_on = [module.github_oidc_provider]
}

# Write-capable: only main + protected environments (dev, prod) may assume it.
module "apply_role" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role"
  version = "~> 6.6"

  name = var.apply_role_name

  enable_github_oidc = true
  oidc_subjects = [
    "${local.repo}:ref:refs/heads/main",
    "${local.repo}:environment:dev",
    "${local.repo}:environment:prod",
  ]

  policies = merge(
    { for arn in var.apply_role_policy_arns : reverse(split("/", arn))[0] => arn },
    { StateLock = aws_iam_policy.state_lock.arn },
  )

  depends_on = [module.github_oidc_provider]
}
