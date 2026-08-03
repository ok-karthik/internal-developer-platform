locals {
  resource_name_prefix = "team-a-app-a"
  tags = {
    Team      = "team-a"
    Service   = "app-a"
    ManagedBy = "terraform"
  }
}

module "postgres" {
  source    = "git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//4-platform-engineering/cloud-services-terraform-modules/