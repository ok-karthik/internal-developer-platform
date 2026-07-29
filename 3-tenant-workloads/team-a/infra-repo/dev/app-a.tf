locals {
    resource_name_prefix_app_a = "team-a-app-a"
    tags_app_a = {
        Team        = "team-a"
        Service     = "app-a"
        ManagedBy   = "terraform"
        Owner       = "team-a"
    }
}



module "aws-s3-app-a" {
    source    = "git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//1-idp-scaffolder/templates/cloud-services/aws-s3?ref=main"
    team_name = "team-a"
    app_name  = "app-a"
}


