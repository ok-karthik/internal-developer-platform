# ADR 0013: Karpenter, Split Across the Repo Boundary

**Date:** 2026-09-21
**Status:** Accepted — supersedes [ADR 0009](0009-karpenter-vs-fixed-nodes.md)

## Context

ADR 0009 kept fixed node groups and deferred Karpenter, on the grounds that Karpenter needs
IAM roles, an SQS interruption queue, discovery tags and its own CRDs — surface area a
reference architecture did not need yet.

Two things changed. The AWS prerequisites are no longer this repo's surface area: since
[ADR 0012](0012-platform-repo-owns-no-cloud-terraform.md) they are built by `compute/eks` in
`enterprise-aws-infrastructure` (`enable_karpenter = true`, the default), which already
creates the controller Pod Identity association, the node role, the interruption queue with
its EventBridge rules, and the `karpenter.sh/discovery` tag on the node security group. And
the platform has an FinOps story ([PLAN.md Phase 18.3](../../PLAN.md)) in which idle compute
is a line item, which is the third revisit trigger ADR 0009 named.

## Decision

Adopt Karpenter, split at the repository boundary:

| Half | Lives in | Why |
|---|---|---|
| IAM, SQS, EventBridge, discovery tags | `enterprise-aws-infrastructure` (`compute/eks`) | Cloud Terraform (ADR 0012) |
| Controller Helm release, `EC2NodeClass`, `NodePool` | this repo, `2-cluster-services/karpenter/` and `1-platform-catalog/charts/karpenter-nodes/` | Kubernetes state, reconciled by ArgoCD |

There is no capability-module URL for the in-cluster half. Capability modules (ADR 0011) are
Terraform that a *tenant* claims; Karpenter is cluster infrastructure the *platform* runs,
and its source is Kubernetes YAML, so there is nothing for a `?ref=` pin to point at.

Karpenter is installed **per registered cluster that opts in**, using the fleet contract
from Phase 18.2: an ArgoCD cluster Secret labelled `karpenter: enabled`. The cluster name
and the two per-cluster facts the AWS half produces (node role name, interruption queue)
travel on that Secret as the annotations `platform.io/karpenter-node-role` and
`platform.io/karpenter-queue`. Terraform in the foundation repo does not call the Kubernetes
API, so it cannot write the Secret; instead `governance/discovery-publisher` publishes the
values to SSM and the Secret is assembled in-cluster from them (External Secrets, not yet
written here). The local k3d cluster is not labelled and never receives it.

## Consequences

- Upgrading Karpenter or tuning a `NodePool` is a PR here with no Terraform state change.
- The Karpenter chart is pinned (`1.12.0`) in `karpenter.yaml`; Renovate does not manage it
  yet.
- **Open gaps in the foundation repo**, found while writing this and not fixed here:
  the node role, queue name, cluster endpoint and CA are not yet in the SSM contract (being
  added to `discovery-publisher`, which is the only module that writes contract parameters);
  and `network/vpc` does not tag subnets with `karpenter.sh/discovery` (only the node
  security group gets it), so the `EC2NodeClass` subnet selector matches nothing until it
  does.
- **Not written here yet:** the ExternalSecret that builds the ArgoCD cluster Secret. It
  waits on those parameters, and on a Parameter Store `ClusterSecretStore`. How the hub
  authenticates to a spoke cluster is unverified.
- Nothing here has run against a real cluster. It is verified only to the extent that both
  charts render with the values the ApplicationSets pass.

## Revisit Trigger

Revert to managed node groups if Karpenter's disruption behaviour (consolidation evicting
pods) proves incompatible with tenant PodDisruptionBudgets in practice.
