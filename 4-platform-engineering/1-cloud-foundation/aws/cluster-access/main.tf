# Phase 7.2b: the EKS production path for human -> cluster authentication.
# Not a diagram — real Terraform, held to `terraform plan` per the Target
# Environment section. EKS Access Entries replaced the old `aws-auth`
# ConfigMap (a single cluster-wide object where one bad edit locked everyone
# out) with a proper API that has per-entry lifecycle.
#
# THE END-TO-END CHAIN THIS IS ONE LINK OF (see README.md's Phase 7 section
# for the full diagram):
#   Corporate IdP --SAML/SCIM--> IAM Identity Center --Permission Set-->
#   an IAM role vended per account --THIS FILE--> EKS Access Entry -->
#   K8s group "platform:<team>:<tier>" --> RoleBinding (Phase 1.1/7.4, unchanged)
#
# Keycloak (2-cluster-services/identity/) is the LOCAL stand-in for the
# corporate-IdP-through-Identity-Center hop. It is not part of this chain on
# real EKS — Access Entries map an IAM principal, not an OIDC subject.
terraform {
  required_version = ">= 1.10"
  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 5.0" }
  }
  backend "s3" {
    bucket       = "acme-corp-terraform-state"
    key          = "platform/aws/cluster-access/terraform.tfstate"
    region       = "us-east-1"
    use_lockfile = true
    encrypt      = true
  }
}

variable "cluster_name" {
  type        = string
  description = "Must match the cluster created in ../cluster/"
}

variable "team_access" {
  description = <<-EOT
    One entry per (team, tier) that should authenticate to this cluster.
    principal_arn is the IAM role IAM Identity Center vends for that
    team/tier's permission set (see README.md's Phase 5.5 account-vending
    table — this is the same "policy attach" step one plane up). k8s_groups
    follows the Phase 7.0 naming contract exactly, so the RoleBindings this
    maps to (Phase 1.1/7.4) need no separate configuration.
  EOT
  type = map(object({
    principal_arn = string
    k8s_groups    = list(string)
  }))
  default = {
    "team-a-developer" = {
      principal_arn = "arn:aws:iam::123456789012:role/idp-team-a-developer"
      k8s_groups    = ["platform:team-a:developer"]
    }
    "team-a-oncall" = {
      principal_arn = "arn:aws:iam::123456789012:role/idp-team-a-oncall"
      k8s_groups    = ["platform:team-a:oncall"]
    }
  }
}

resource "aws_eks_access_entry" "team" {
  for_each = var.team_access

  cluster_name      = var.cluster_name
  principal_arn     = each.value.principal_arn
  kubernetes_groups = each.value.k8s_groups
  type              = "STANDARD"
}

# Grants a baseline read policy at the CLUSTER scope via the access policy
# association — this is deliberately minimal (view only). The actual
# permissions a developer gets inside their namespace still come from the
# Phase 1.1 RoleBinding matching the k8s_groups above; this association
# exists so `kubectl get nodes`-class cluster-scoped reads do not 403 before
# RBAC is even evaluated.
resource "aws_eks_access_policy_association" "team_view" {
  for_each = var.team_access

  cluster_name  = var.cluster_name
  principal_arn = each.value.principal_arn
  policy_arn    = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSViewPolicy"

  access_scope {
    type = "cluster"
  }
}

output "access_entry_arns" {
  value = { for k, v in aws_eks_access_entry.team : k => v.access_entry_arn }
}
