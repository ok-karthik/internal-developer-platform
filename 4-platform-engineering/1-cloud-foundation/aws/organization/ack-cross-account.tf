# Cross-account IAM so ACK controllers running in the HUB account (Phase 5.1's
# EKS cluster, addons in 2-cluster-services/aws-controllers/) can create real
# AWS resources in a SPOKE account (Phase 5.4's per-environment account).
#
# Shown as one file with two aliased providers for readability. In practice
# each spoke account applies only its own `aws_iam_role.ack_spoke` via Account
# Factory (Phase 5.5) — the hub team never holds spoke credentials — and the
# hub applies only `aws_iam_role_policy.hub_ack_assume_spoke`. Splitting this
# into two root modules, one per account, is the production shape; it is one
# file here so the trust relationship reads as a single unit.
#
# THE DIRECTION, stated explicitly because getting it backwards is the most
# common mistake: the SPOKE trusts the HUB, never the reverse. A spoke account
# grants a narrow, revocable door to ITS OWN resources; the hub never widens
# its own trust boundary to let spokes in. This is also why sts:ExternalId is
# on the spoke side's trust policy, not the hub's grant — ExternalId defends
# the party being assumed INTO (the spoke) against the confused-deputy problem
# where a third party tricks the hub's controller into acting on the wrong
# spoke by reusing a role ARN it was never meant to have.

terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 5.0" }
  }
}

variable "hub_account_id" {
  type        = string
  description = "Account ID running the ACK controllers (Phase 5.1's EKS hub)"
}

variable "spoke_account_id" {
  type        = string
  description = "Account ID that owns the actual AWS resources ACK provisions for one team/environment"
}

variable "hub_ack_controller_role_arn" {
  type        = string
  description = "The IAM role the ACK controller pods assume via Pod Identity in the hub (../workload-identity/pod-identity.tf creates the association; the role itself is provisioned alongside the ACK Helm release)."
}

variable "external_id" {
  type        = string
  description = "Shared secret proving the assume-role call is deliberate, not a hub-side ARN confusion. Generate one per spoke, store alongside the account's other vended secrets — never hardcode a shared value across spokes."
  sensitive   = true
}

provider "aws" {
  alias  = "hub"
  region = "eu-central-1"
}

provider "aws" {
  alias  = "spoke"
  region = "eu-central-1"
}

# --- Spoke side: the role ACK assumes INTO this account ----------------------

resource "aws_iam_role" "ack_spoke" {
  provider = aws.spoke
  name     = "ack-hub-controller-access"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { AWS = var.hub_ack_controller_role_arn }
      Action    = "sts:AssumeRole"
      Condition = {
        StringEquals = { "sts:ExternalId" = var.external_id }
      }
    }]
  })

  tags = { ManagedBy = "terraform", Purpose = "ack-cross-account" }

  lifecycle {
    # Catches the copy-paste error this trust relationship is most at risk of:
    # pasting a controller role ARN from the wrong account. var.hub_account_id
    # exists specifically to make this checkable rather than trusted blindly.
    precondition {
      condition     = strcontains(var.hub_ack_controller_role_arn, ":${var.hub_account_id}:")
      error_message = "hub_ack_controller_role_arn does not belong to hub_account_id — refusing to trust a role from an unexpected account."
    }
  }
}

# Scoped to exactly the two ACK controllers this platform runs today (Phase
# 3.2) — S3 and IAM. Widen this only when a third ACK controller is added,
# and widen it here, not by handing the role AdministratorAccess.
resource "aws_iam_role_policy_attachment" "ack_spoke_s3" {
  provider   = aws.spoke
  role       = aws_iam_role.ack_spoke.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonS3FullAccess"
}

resource "aws_iam_role_policy_attachment" "ack_spoke_iam" {
  provider   = aws.spoke
  role       = aws_iam_role.ack_spoke.name
  policy_arn = "arn:aws:iam::aws:policy/IAMFullAccess"
}

# --- Hub side: grant the controller role permission to assume the above -----

resource "aws_iam_role_policy" "hub_ack_assume_spoke" {
  provider = aws.hub
  name     = "assume-spoke-${var.spoke_account_id}"
  # The controller role ARN comes from an existing resource this repo does
  # not create (see the variable's description) — this attaches to it by
  # name, which is what `role =` accepts (not `role_arn =`), so we
  # extract the name from the ARN. Passing the raw ARN here would fail:
  # aws_iam_role_policy.role expects a role name/ID, not an ARN.
  role = split("/", var.hub_ack_controller_role_arn)[length(split("/", var.hub_ack_controller_role_arn)) - 1]

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "sts:AssumeRole"
      Resource = aws_iam_role.ack_spoke.arn
    }]
  })
}

output "spoke_role_arn" {
  description = "Feed this into the ACK CARM ConfigMap (services.k8s.aws/owner-account-id annotation) for the namespace this spoke serves"
  value       = aws_iam_role.ack_spoke.arn
}
