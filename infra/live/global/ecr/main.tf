# Shared, persistent ECR repos (global scope, ADR-0007): a digest built once promotes
# dev->prod. IMMUTABLE tags + scan-on-push support the digest-pinned, signed-image rule.
module "ecr" {
  source   = "terraform-aws-modules/ecr/aws"
  version  = "~> 3.0"
  for_each = toset(var.services)

  repository_name                 = "${var.repository_namespace}/${each.value}"
  repository_image_tag_mutability = "IMMUTABLE"
  repository_image_scan_on_push   = true
  repository_encryption_type      = "KMS" # AWS-managed aws/ecr key (ADR-0008)
  repository_force_delete         = false

  create_lifecycle_policy = true
  repository_lifecycle_policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Expire untagged images older than ${var.untagged_expire_days} days"
        selection = {
          tagStatus   = "untagged"
          countType   = "sinceImagePushed"
          countUnit   = "days"
          countNumber = var.untagged_expire_days
        }
        action = { type = "expire" }
      },
    ]
  })

  tags = { Service = each.value }
}
