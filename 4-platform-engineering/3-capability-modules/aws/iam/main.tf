terraform {
  required_version = ">= 1.5"

  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 6.0" }
  }
}

variable "team_name" {
  type        = string
  description = "Team/tenant that owns this identity"
}

variable "app_name" {
  type        = string
  description = "Application the identity belongs to"
}

variable "env" {
  type        = string
  description = "Target environment (dev, staging, prod)"
  default     = "dev"
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to every resource, supplied by the scaffolder"
  default     = {}
}

locals {
  role_name = lower("${var.team_name}-${var.app_name}-${var.env}")
}

# NOTE: this capability is still a stub — it declares the module interface the
# scaffolder renders against, but provisions no identity yet.
#
# What it should become, once the cluster's OIDC issuer is known: an IRSA role
# (aws_iam_role with a federated assume-role policy scoped to the workload's
# Kubernetes service account) plus a least-privilege policy composed from the
# OTHER capabilities the service requested — s3 access only if it asked for s3,
# RDS Data API access only if it asked for postgres.
#
# That cross-capability dependency is the interesting design problem here: it
# means capabilities are not independent, and the scaffolder currently renders
# each one in isolation. Resolving it needs either module outputs wired between
# generated files, or an IAM module that reads the service's full capability
# list. Tracked in 2-idp-scaffolder/golang/TODO.md.
#
# Until then, requesting `iam` produces valid Terraform that creates nothing,
# which is honest — as opposed to creating an over-permissive role that looks
# like it works.

output "planned_role_name" {
  description = "Role name this capability will create once implemented"
  value       = local.role_name
}
