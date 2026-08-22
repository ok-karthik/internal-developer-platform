# AWS Organizations: accounts, Organizational Units (OUs), and Service Control
# Policies (SCPs). Applied from the ORGANIZATION MANAGEMENT account — a
# different account from both `hub` and `spoke` in ack-cross-account.tf, so
# this file uses the default (unaliased) provider rather than either alias.
#
# THE MENTAL MODEL THAT MATTERS: an SCP is a permission CEILING, not a grant.
# It cannot give any principal — including the account root user — a single
# permission. It can only cap what IAM policies in the account are allowed to
# grant. That inversion is what makes an account boundary HARD where a
# Kubernetes NetworkPolicy is SOFT (Phase 1.6): a NetworkPolicy is enforced by
# a controller a privileged pod could in principle disable; an SCP is enforced
# by AWS itself, outside the account entirely, and nothing inside the account
# — not even an admin — can turn it off.
#
# (terraform {} / required_providers is declared once for this whole
# directory, in ack-cross-account.tf — Terraform merges provider requirements
# across files in one module, so it is not repeated here.)

variable "root_id" {
  type        = string
  description = "AWS Organizations root ID (looks like r-xxxx). Not fetched via data source so this module stays terraform-validate-able with no org to query."
}

# --- Organizational Units -----------------------------------------------------
# Phase 5.4's answer to "one account per team or one account per environment?"
# is environment — Production and NonProduction, each holding every team's
# spoke account for that environment. Team boundaries stay soft (namespaces,
# Phase 1); environment boundaries go hard (accounts, this file).

resource "aws_organizations_organizational_unit" "production" {
  name      = "Production"
  parent_id = var.root_id
}

resource "aws_organizations_organizational_unit" "non_production" {
  name      = "NonProduction"
  parent_id = var.root_id
}

# --- Service Control Policies --------------------------------------------------

resource "aws_organizations_policy" "deny_leave_org" {
  name        = "deny-leave-organization"
  description = "No account may remove itself from the organization, which would strip every other SCP here along with it."
  type        = "SERVICE_CONTROL_POLICY"

  content = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid      = "DenyLeaveOrganization"
      Effect   = "Deny"
      Action   = "organizations:LeaveOrganization"
      Resource = "*"
    }]
  })
}

resource "aws_organizations_policy" "deny_disable_cloudtrail" {
  name        = "deny-disable-cloudtrail"
  description = "No principal, including the account root, may stop or delete the org-wide audit trail. Phase 4/7 both depend on CloudTrail (and the EKS audit log) actually existing when someone needs it."
  type        = "SERVICE_CONTROL_POLICY"

  content = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid    = "DenyCloudTrailTampering"
      Effect = "Deny"
      Action = [
        "cloudtrail:StopLogging",
        "cloudtrail:DeleteTrail",
        "cloudtrail:UpdateTrail",
      ]
      Resource = "*"
    }]
  })
}

resource "aws_organizations_policy" "deny_outside_eu_central_1" {
  name        = "deny-region-outside-eu-central-1"
  description = "Restricts every account under the OUs below to eu-central-1. This is a DATA RESIDENCY control, not an availability one — see PLAN.md Phase 10.3 (STACKIT/EU sovereignty) for why region choice is a compliance question here, not just a latency one. Global services (IAM, Organizations, CloudFront, Route53) are exempted, or nothing in the account could function."
  type        = "SERVICE_CONTROL_POLICY"

  content = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid    = "DenyOutsideEuCentral1"
      Effect = "Deny"
      NotAction = [
        "iam:*",
        "organizations:*",
        "route53:*",
        "cloudfront:*",
        "support:*",
        "sts:*",
      ]
      Resource = "*"
      Condition = {
        StringNotEquals = { "aws:RequestedRegion" = "eu-central-1" }
      }
    }]
  })
}

resource "aws_organizations_policy_attachment" "production_guardrails" {
  for_each = toset([
    aws_organizations_policy.deny_leave_org.id,
    aws_organizations_policy.deny_disable_cloudtrail.id,
    aws_organizations_policy.deny_outside_eu_central_1.id,
  ])
  policy_id = each.value
  target_id = aws_organizations_organizational_unit.production.id
}

resource "aws_organizations_policy_attachment" "non_production_guardrails" {
  for_each = toset([
    aws_organizations_policy.deny_leave_org.id,
    aws_organizations_policy.deny_disable_cloudtrail.id,
    aws_organizations_policy.deny_outside_eu_central_1.id,
  ])
  policy_id = each.value
  target_id = aws_organizations_organizational_unit.non_production.id
}
