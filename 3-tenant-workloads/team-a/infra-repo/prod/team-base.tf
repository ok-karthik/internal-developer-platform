locals {
    team_name = "team-a"
    tags = {
        Team        = "team-a"
        ManagedBy   = "terraform"
        Owner       = "team-a"
    }
}

## Tenant Idempotent Infrastructure Modules ## - Do not remove these - Danger zone start!
module "aws-vpc" {
    source    = "git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//1-idp-scaffolder/templates/cloud-services/aws-networking?ref=main"
    team_name = "team-a"
    app_name  = "shared"
    vpc_cidr  = "10.0.0.0/16"
}

module "aws-iam" {
    source    = "git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//1-idp-scaffolder/templates/cloud-services/aws-iam?ref=main"
    team_name = "team-a"
    app_name  = "shared"
}
## Tenant Idempotent Infrastructure Modules ## - Do not remove these - Danger zone end!
