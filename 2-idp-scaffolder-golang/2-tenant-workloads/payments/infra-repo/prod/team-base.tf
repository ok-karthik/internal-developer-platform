locals {
    team_name = "payments"
    tags = {
        Team        = "payments"
        ManagedBy   = "terraform"
        Owner       = "payments"
    }
}

## Tenant Idempotent Infrastructure Modules ## - Do not remove these - Danger zone start!
module "aws-vpc" {
    source    = "git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//4-platform-engineering/cloud-services-terraform-modules/aws-networking?ref=v1.0.2"
    team_name = "payments"
    app_name  = "shared"
    vpc_cidr  = "10.0.0.0/16"
}

module "aws-iam" {
    source    = "git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//4-platform-engineering/cloud-services-terraform-modules/aws-iam?ref=v1.0.2"
    team_name = "payments"
    app_name  = "shared"
}
## Tenant Idempotent Infrastructure Modules ## - Do not remove these - Danger zone end!
