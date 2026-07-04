terraform {
  backend "s3" {
    bucket         = "devopsroyale-state-files-ccsji365i"
    key            = "botp/global/ecr.tfstate"
    region         = "us-east-1"
    dynamodb_table = "terraform-locks"
    encrypt        = true
  }
}
