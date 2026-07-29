locals {
    team_name = "team-b"
    tags = {
        Team        = "team-b"
        ManagedBy   = "terraform"
        Owner       = "team-b"
    }
}

## Tenant Idempotent Infrastructure Modules ## - Do not remove these - Danger zone start!
module "aws-vpc" {
    source    = "git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//1-idp-scaffolder/templates/cloud-services/aws-networking?ref=main"
    team_name = "team-b"
    app_name  = "shared"
    vpc_cidr  = "10.1.0.0/16"
}

module "aws-iam" {
    source    = "git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//1-idp-scaffolder/templates/cloud-services/aws-iam?ref=main"
    team_name = "team-b"
    app_name  = "shared"
}
## Tenant Idempotent Infrastructure Modules ## - Do not remove these - Danger zone end!
