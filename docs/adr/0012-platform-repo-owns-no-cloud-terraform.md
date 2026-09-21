# ADR 0012: The Platform Repo Owns No Cloud Terraform

**Date:** 2026-09-20
**Status:** Accepted

> **Path note (2026-09-21):** `enterprise-aws-infrastructure` later split into `-repo` folders (its ADR 0001): `infrastructure-modules/` is now `iac-modules-repo/`, `infrastructure-live/` is `workloads-live-repo/` (plus `foundation-live-repo/`), and `infrastructure-bootstrap/` is `foundation-live-repo/_bootstrap/`. Paths below use the new names.

## Context

`4-platform-engineering/1-cloud-foundation/aws/` holds Terraform for a VPC, an EKS cluster,
AWS Organizations with SCPs, EKS Access Entries, the S3 state backend, and the IRSA /
Pod Identity seam. It is real Terraform, held to a clean `terraform plan`, never applied.

`enterprise-aws-infrastructure` is a full Terragrunt platform that already does this job
for real: `iac-modules-repo/{network/vpc,compute/eks,identity/human-access}`,
`workloads-live-repo/{dev,prod}/eu-central-1/...`, an `foundation-live-repo/_bootstrap/` day-0
stack with the GitHub OIDC provider and the S3 state bucket, plus OPA policies, tflint,
Checkov, Trivy and Infracost gates.

So the same things are built twice, in two repos, by two different methods, and only one of
them is the real one. A reader comparing `1-cloud-foundation/aws/cluster/` with
`iac-modules-repo/compute/eks/` cannot tell which is authoritative, because nothing
says.

This is also not how the boundary works in a company. A platform team does not own the VPC
or the organization's SCPs. It consumes a cluster and an account layout that a cloud
infrastructure team provides, and the interesting engineering is the *contract* between
them — which is exactly what gets hidden when one repo contains both sides.

## Decision

**This repository owns no cloud Terraform.** Everything under
`4-platform-engineering/1-cloud-foundation/aws/` moves to `enterprise-aws-infrastructure`
and is consumed from there.

| Moves to `enterprise-aws-infrastructure` | Lands as |
|---|---|
| `aws/network/` | merge into `iac-modules-repo/network/vpc` + its live stacks |
| `aws/cluster/` | merge into `iac-modules-repo/compute/eks` + its live stacks |
| `aws/cluster-access/` | `iac-modules-repo/identity/human-access` (EKS Access Entries, IAM Identity Center) |
| `aws/workload-identity/` | `iac-modules-repo/identity/workload-identity` (the IRSA / Pod Identity seam) |
| `aws/organization/` | `iac-modules-repo/governance/organization` + a `_global` live stack (Organizations, OUs, SCPs, ACK cross-account trust) |
| `aws/bootstrap/` | delete — `foundation-live-repo/_bootstrap/` already does this |

**What stays here, and why:**

- `1-cloud-foundation/local/` — the k3d harness. It is a test rig for this repo, not cloud
  infrastructure.
- `2-cluster-services/` — everything ArgoCD manages inside the cluster.
- `4-platform-apis/` — Crossplane XRDs and KRO definitions are Kubernetes APIs.
- `1-platform-catalog/`, `2-idp-scaffolder/`, `3-tenant-repos/` — the platform product.

`1-cloud-foundation/` does not disappear. It becomes the **contract**: a README stating
exactly what this platform needs from the foundation (cluster name, OIDC provider ARN, VPC
and subnet IDs, the ACK cross-account role) and how it reads them — the SSM parameter
service catalog specified in PLAN.md Phase 18.1.

## Rationale

1. **One implementation of each thing.** Two EKS definitions in two repos will drift, and
   the one that never gets applied will drift first and silently.
2. **The seam becomes visible.** With the foundation in another repo, "how does a tenant's
   Terraform learn the VPC ID?" has to be answered explicitly instead of by a relative
   path. That answer — SSM parameters published by the foundation, read by tenant Terraform
   at plan time — is the more interesting piece of engineering, and it only exists as a
   question once the repos are split.
3. **Each repo gets the right CI.** Cloud Terraform belongs where Checkov, tflint,
   Infracost and OPA already run against plan JSON. This repo's CI is about the catalog,
   the two scaffolder engines and tenancy policy.
4. **It matches how the work is actually divided.** A platform engineer is a *customer* of
   the cloud team. Modelling that as a repository boundary is the point of the exercise.

## Consequences

- `4-platform-engineering/` drops from four numbered directories to three, and
  `3-capability-modules/` is deleted as well (ADR 0011). `4-platform-apis/` is renumbered
  to `3-platform-apis/`, so the numbering stays contiguous.
- Several documents describe the foundation as living here and must be rewritten to
  describe it as consumed: `README.md`, `.agents/AGENTS.md`, `docs/runbooks/cluster-upgrade.md`,
  `2-cluster-services/README.md`, `2-cluster-services/security-governance/external-secrets.yaml`.
- Phases 5, 7.2b and 7.2c described work that now lives in the other repo. PLAN.md keeps
  the phase history — it is a changelog of what was done, not a map of where files are —
  but each affected entry gets a line saying where the work now lives.
- The k3d path must keep working end to end with no AWS at all, because that is now the
  only foundation this repo can stand up by itself. `make setup` staying green is the gate.

## Revisit Trigger

Pull cloud Terraform back into this repo only if the platform starts provisioning its own
clusters as a product feature — for example a self-service "give my tenant a dedicated
cluster" golden path. At that point the cluster stops being foundation the platform
consumes and becomes something the platform produces, and the boundary moves.
