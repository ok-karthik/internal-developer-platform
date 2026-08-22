# Karpenter vs. Fixed Node Groups

**Date:** 2026-08-22
**Status:** Accepted

## Context
The platform currently provisions EKS worker nodes using an `aws_eks_node_group` with a fixed configuration (`max_size = desired + 2`). This is a static approach where capacity and instance types are chosen by a human ahead of time.

An alternative is Karpenter, which dynamically provisions just-in-time compute that directly matches the requirements of pending pods. Building Karpenter would require an `EC2NodeClass`, a `NodePool`, an IAM role via Pod Identity, an SQS interruption queue, and subnet/security-group discovery tags.

## Decision
We will **keep the fixed Node Group approach** for now and defer implementing Karpenter.

## Rationale
1. **Simplicity for the Reference Architecture:** This repository serves as a reference blueprint. A fixed node group is significantly simpler to reason about, test, and document. Karpenter introduces its own set of CRDs, IAM roles, interruption queues, and tag discovery requirements which increase the surface area of the platform considerably.
2. **Current Scaling Needs:** Until tenant workloads exhibit highly variable, unpredictable scaling patterns that exceed the bounds of a reasonably sized static or standard ASG-backed node group, Karpenter's rapid bin-packing and instance-type flexibility are optimizations rather than hard requirements. 
3. **Focus on Workload APIs:** Our immediate focus is on perfecting the GitOps lifecycle, developer experience, and tenancy controls (namespaces, RBAC, IRSA).

## Revisit Trigger
This decision should be revisited when:
- Tenant onboarding scales to the point where manual capacity planning for node groups becomes a bottleneck.
- Workloads begin requesting specialized instance types (e.g., GPUs, high-memory nodes) that would require managing many separate fixed node groups.
- The cost of idle compute during low-traffic periods becomes unacceptable, justifying the investment in Karpenter's tight scaling and consolidation capabilities.
