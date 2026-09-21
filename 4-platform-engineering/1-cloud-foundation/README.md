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
| `/platform/${env}/${region}/eks/cluster_name` | `compute/eks` | tenant Terraform that needs the cluster | same |
| `/platform/${env}/${region}/eks/oidc_provider_arn` | `compute/eks` | IRSA trust policies for tenant workloads | same |
| `/platform/${env}/${region}/ack/cross_account_role_arn` | `governance/organization` | ACK controllers (`2-cluster-services/`) assuming into a tenant's spoke account | ACK reconciles fail with `AccessDenied`; nothing is created |

`env` comes from the scaffolder (`--env`); `region` comes from the AWS provider the tenant's
Terraform runs with (`data "aws_region"`).

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
