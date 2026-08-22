# IRSA — the DOCUMENTED FALLBACK, not the platform's default (that is
# pod-identity.tf). Genuinely required in three cases, and knowing them is
# the substance of this file rather than the Terraform itself:
#   1. EKS Fargate — Pod Identity does not run there.
#   2. Workloads outside EKS entirely.
#   3. Any non-AWS cluster — but note IRSA's OIDC-federation MECHANISM is
#      also the portable CONCEPT with real analogues (Azure Workload
#      Identity, GCP Workload Identity Federation), which is exactly what
#      Phase 10.1's portability seam needs. Pod Identity has no such
#      cross-cloud analogue; it is an EKS-only API.
#
# Requires a per-cluster IAM OIDC provider — the setup cost Pod Identity
# exists specifically to remove. Only pay it for a workload that actually
# needs one of the three cases above.
terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 5.0" }
    tls = { source = "hashicorp/tls", version = "~> 4.0" }
  }
}

variable "oidc_issuer_url" {
  type        = string
  description = "From ../cluster/'s output oidc_issuer_url. Only needed for clusters actually using this fallback."
}

variable "irsa_roles" {
  description = "One entry per (namespace, service account) needing the IRSA fallback rather than Pod Identity."
  type = map(object({
    namespace            = string
    service_account_name = string
    policy_arns          = list(string)
  }))
  default = {}
}

data "tls_certificate" "cluster_oidc" {
  url = var.oidc_issuer_url
}

resource "aws_iam_openid_connect_provider" "cluster" {
  url             = var.oidc_issuer_url
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.cluster_oidc.certificates[0].sha1_fingerprint]
}

locals {
  # Strips the scheme so the federated trust policy's StringEquals condition
  # matches the provider's own naming convention (no "https://" prefix).
  oidc_provider_host = replace(var.oidc_issuer_url, "https://", "")
}

resource "aws_iam_role" "irsa" {
  for_each = var.irsa_roles

  name = "irsa-${each.value.namespace}-${each.value.service_account_name}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = aws_iam_openid_connect_provider.cluster.arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${local.oidc_provider_host}:sub" = "system:serviceaccount:${each.value.namespace}:${each.value.service_account_name}"
          "${local.oidc_provider_host}:aud" = "sts.amazonaws.com"
        }
      }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "irsa" {
  for_each = { for pair in flatten([
    for k, v in var.irsa_roles : [
      for arn in v.policy_arns : { key = "${k}-${arn}", role_key = k, policy_arn = arn }
    ]
  ]) : pair.key => pair }

  role       = aws_iam_role.irsa[each.value.role_key].name
  policy_arn = each.value.policy_arn
}

output "irsa_role_arns" {
  value = { for k, v in aws_iam_role.irsa : k => v.arn }
}
