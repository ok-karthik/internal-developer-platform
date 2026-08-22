## Tenant Idempotent Infrastructure Modules ## - Do not remove these - Danger zone start!
# A per-tenant "aws-vpc" module used to be rendered here, backed by a CIDR the
# Python engine allocated into 3-tenant-workloads/cloud_vpcs_allocated.yaml.
# Removed: it assumed each team would get its own VPC (and implicitly, its
# own cluster) — an earlier design this platform moved away from. What
# actually got built (Phase 1 + Phase 5) is one shared EKS cluster with
# namespace-per-tenant as the soft isolation boundary, and one AWS account per
# ENVIRONMENT (not per team) as the hard one — see README.md's "Two Layers
# of Isolation". The removed module was also already dead in practice: its
# vpc_cidr was hardcoded to "10.0.0.0/16" regardless of team, so the
# allocator's per-tenant CIDR was computed, persisted, and never actually
# substituted into this file — every team would have collided on the same
# block had this ever been applied.

# This is where per-namespace IRSA/Pod Identity trust-policy scoping attaches
# once a real EKS cluster's OIDC issuer exists (4-platform-engineering/1-cloud-foundation/
# is k3d-only today, under local/). See "IRSA / Pod Identity is the missing fourth wall of
# the tenancy model" in .agents/AGENTS.md — it is the AWS-side counterpart to
# the AppProject/NetworkPolicy/RBAC boundary this blueprint already creates.
module "aws-iam" {
    source    = "git::https://github.com/ok-karthik/internal-developer-platform.git//4-platform-engineering/3-capability-modules/aws/iam?ref=v2.0.0"
    team_name = "team-a"
    app_name  = "shared"
}
## Tenant Idempotent Infrastructure Modules ## - Do not remove these - Danger zone end!
