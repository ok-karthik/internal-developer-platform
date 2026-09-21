# 1. Cloud Foundation — the contract

This repository owns **no cloud Terraform** ([ADR 0012](../../docs/adr/0012-platform-repo-owns-no-cloud-terraform.md)).
The VPC, EKS cluster, AWS Organizations, human access and workload identity are built and
applied from [`enterprise-aws-infrastructure`](https://github.com/ok-karthik/enterprise-aws-infrastructure).
This directory says what the platform **needs** from that foundation and how it **reads** it.

```
1-cloud-foundation/
├── README.md   # this file: the contract
└── local/      # k3d. A TEST HARNESS — the only foundation this repo can stand up alone.
```

## What the platform needs, and where it comes from

The foundation publishes SSM parameters on every stack apply
(`/platform/${env}/${region}/...`, [PLAN.md Phase 18.1, Pattern A](../../PLAN.md)). Nothing in
this repo hardcodes an AWS ID, and nothing reads another repo's Terraform state — so there
are no cross-repo state locks.

| Parameter | Published by | Read by | If it is missing |
|---|---|---|---|
| `/platform/${env}/${region}/vpc/id` | `network/vpc` | `postgres.tf.tmpl` (database security group) | `terraform plan` fails on the `aws_ssm_parameter` data source — loudly, at plan time |
| `/platform/${env}/${region}/vpc/database_subnets` | `network/vpc` | `postgres.tf.tmpl` (DB subnet group) | same |
| `/platform/${env}/${region}/eks/cluster_name` | `governance/discovery-publisher` | tenant Terraform that needs the cluster | same |
| `/platform/${env}/${region}/eks/oidc_provider_arn` | `governance/discovery-publisher` | IRSA trust policies for tenant workloads | same |
| `/platform/${env}/${region}/ack/cross_account_role_arn` | `governance/organization` | ACK controllers (`2-cluster-services/`) assuming into a tenant's spoke account | ACK reconciles fail with `AccessDenied`; nothing is created |

Every contract parameter is written by exactly one module, `governance/discovery-publisher`
(the `publish_ssm_parameters` switch on `compute/eks` and `network/vpc` is off in live), so
a parameter missing from an account means the publisher did not run there — not that the
module that owns the value forgot to write it.

`env` comes from the scaffolder (`--env`); `region` comes from the AWS provider the tenant's
Terraform runs with (`data "aws_region"`).

## Cluster registration — the fleet contract

Tenant ApplicationSets do not hold cluster endpoints ([PLAN.md Phase 18.2](../../PLAN.md)).
They join each app's env directory to the ArgoCD cluster registry, so **registering a
cluster is what routes workloads to it**. The foundation stack that creates a cluster
writes a Secret in the `argocd` namespace:

| Field | Meaning | Consumed by |
|---|---|---|
| label `argocd.argoproj.io/secret-type: cluster` | makes it a registered cluster | ArgoCD |
| label `environment` (`dev` \| `staging` \| `prod`) | must equal the app's env directory | every tenant ApplicationSet |
| label `region`, label `tier` (`general` \| `pci-compliant`) | informational today; usable as extra selectors | — |
| label `karpenter: enabled` | opt this cluster in to Karpenter | `2-cluster-services/karpenter/` |
| annotation `platform.io/karpenter-node-role` | from SSM `eks/karpenter_node_role` (node IAM role name) | `karpenter-nodes` |
| annotation `platform.io/karpenter-queue` | from SSM `eks/karpenter_queue_name` | `karpenter` |
| `name` | must equal the EKS cluster name (SSM `eks/cluster_name`) | `karpenter` (`settings.clusterName`) |
| `server`, `config.tlsClientConfig.caData` | from SSM `eks/cluster_endpoint`, `eks/cluster_ca_data` | ArgoCD |

The Secret is **built in-cluster from these SSM parameters** (External Secrets), not written
by Terraform: the foundation repo's Terraform never calls the Kubernetes API. That
ExternalSecret does not exist here yet — see the status below.

A cluster with no matching `environment` label receives no tenant apps. `local/cluster-secret.yaml`
is the k3d example (`environment: dev`, no `karpenter` label).

**Status.** The foundation repo is adding `eks/{cluster_endpoint, cluster_ca_data,
karpenter_node_role, karpenter_queue_name}` to `discovery-publisher` (its PLAN 2.10, not
done when this was written), and `network/vpc` does not yet tag subnets with
`karpenter.sh/discovery`, so a Karpenter `EC2NodeClass` would find no subnets. Here, the
ExternalSecret that assembles the cluster Secret is not written: it needs those parameters to
exist, and a `ClusterSecretStore` for Parameter Store (the only store here reads Secrets
Manager). How the hub authenticates to a spoke cluster is unverified. See ADR 0013.

## What stays here

`local/` stays. It is a test rig for this repo, not cloud infrastructure, and after the
move it is the only foundation this repo can stand up on its own — so `make setup` staying
green is a hard gate, not a nicety.

## Where the code went

| Was here | Now in `enterprise-aws-infrastructure` |
|---|---|
| `aws/network` | `iac-modules-repo/network/vpc` |
| `aws/cluster` | `iac-modules-repo/compute/eks` |
| `aws/cluster-access` | `iac-modules-repo/identity/human-access` |
| `aws/workload-identity` | `iac-modules-repo/identity/workload-identity` |
| `aws/organization` (incl. ACK cross-account) | `iac-modules-repo/governance/organization` |
| `aws/bootstrap` | `foundation-live-repo/_bootstrap/` (already existed) |
| capability modules `aws/{postgres,s3,iam}` | `iac-modules-repo/{data/postgres,storage/s3,identity/workload-iam}` |

The old code is in this directory's git history.
