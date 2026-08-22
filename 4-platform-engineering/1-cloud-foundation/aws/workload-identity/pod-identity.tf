# Phase 7.2c: EKS Pod Identity is the platform's workload-identity mechanism
# on EKS — AWS's recommended path since 2023, chosen over IRSA specifically
# because it removes the per-cluster OIDC provider and collapses the N
# clusters x M roles trust-policy problem IRSA creates the moment there is
# more than one cluster, which Phase 5's hub/spoke design guarantees. Needs
# the eks-pod-identity-agent add-on (../cluster/main.tf already installs it).
#
# THE SEAM, not a pick: the tenant-facing contract is identical either way —
# a namespace gets a ServiceAccount that maps to exactly one role. This file
# is the EKS-target implementation; irsa.tf in this same directory is the
# documented fallback for the three cases that genuinely need it (EKS
# Fargate, workloads outside EKS, non-AWS clusters).
#
# (terraform {} / required_providers declared once for this directory, in
# irsa.tf — Terraform merges provider requirements across files in one
# module, so it is not repeated here.)

variable "cluster_name" {
  type = string
}

variable "workload_identities" {
  description = <<-EOT
    One entry per (namespace, service account) that needs AWS credentials —
    in this platform, one per team's ACK-adjacent workloads plus the ACK
    controllers themselves. role_arn is provisioned elsewhere (e.g. the
    aws-iam capability module, or ../organization/ack-cross-account.tf for
    the ACK controller's own role) and passed in rather than created here,
    so this file's only job is the (namespace, SA) -> role ASSOCIATION,
    matching the "blueprint renders the ServiceAccount; the environment
    decides how it is bound" split.
  EOT
  type = map(object({
    namespace            = string
    service_account_name = string
    role_arn             = string
  }))
  default = {}
}

resource "aws_eks_pod_identity_association" "this" {
  for_each = var.workload_identities

  cluster_name    = var.cluster_name
  namespace       = each.value.namespace
  service_account = each.value.service_account_name
  role_arn        = each.value.role_arn
}

output "associations" {
  value = { for k, v in aws_eks_pod_identity_association.this : k => v.association_id }
}
