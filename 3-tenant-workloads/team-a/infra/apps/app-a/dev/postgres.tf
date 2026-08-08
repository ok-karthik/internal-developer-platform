locals {
  resource_name_prefix = "team-a-app-a"
  tags = {
    Team      = "team-a"
    Service   = "app-a"
    ManagedBy = "terraform"
  }
}

module "postgres" {
  source    = "git::https://github.com/ok-karthik/internal-developer-platform.git//4-platform-engineering/cloud-services-terraform-modules/aws-postgres?ref=v1.1.0"
  team_name = "team-a"
  app_name  = "app-a"
}
