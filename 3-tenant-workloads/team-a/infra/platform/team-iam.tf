## Tenant Idempotent Infrastructure Modules ## - Do not remove these - Danger zone start!
module "aws-vpc" {
  source    = "git::https://github.com/ok-karthik/internal-developer-platform.git//4-platform-engineering/cloud-services-terraform-modules/aws-networking?ref=v1.1.0"
  team_name = "team-a"
  app_name  = "shared"
  vpc_cidr  = "10.0.0.0/16"
}

module "aws-iam" {
  source    = "git::https://github.com/ok-karthik/internal-developer-platform.git//4-platform-engineering/cloud-services-terraform-modules/aws-iam?ref=v1.1.0"
  team_name = "team-a"
  app_name  = "shared"
}
## Tenant Idempotent Infrastructure Modules ## - Do not remove these - Danger zone end!
