# Cluster Upgrade Runbook

EKS Kubernetes versions age out approximately every 14 months. This runbook describes the procedure for a zero-downtime cluster upgrade.

## 1. Addon Compatibility Check
Before touching the control plane, verify that our core addons support the target Kubernetes version. 
In particular, check:
- `argo-cd`
- `aws-vpc-cni`, `coredns`, `kube-proxy`
- `aws-load-balancer-controller`
- `karpenter` (if we ever adopt it)
- `cert-manager`
- `external-dns`

Upgrade these Helm charts via GitOps to compatible versions *before* upgrading the EKS control plane.

## 2. Control Plane Upgrade
The control plane is upgraded via Terraform:
1. Update the `cluster_version` in `1-cloud-foundation/aws/cluster/main.tf`.
2. Plan and apply.
3. This process takes ~20-40 minutes. The Kubernetes API may experience brief latency spikes, but workloads will continue to run unaffected.

## 3. Node Roll & PodDisruptionBudgets
Once the control plane is upgraded, the worker nodes must be replaced with the new AMI.
If using a managed node group, AWS will cordon and drain the old nodes one by one.

### The Role of PDBs
During the drain process, Kubernetes will evict pods. **PodDisruptionBudgets (PDBs)** are the only mechanism preventing Kubernetes from evicting all replicas of a critical service simultaneously. 
- The platform chart automatically renders a PDB for any service with `replicaCount > 1` (or `minReplicas > 1` for autoscaled services), with `maxUnavailable: 1`. 
- This ensures at least `N-1` replicas remain available to serve traffic while the evicted pod is rescheduled onto the new node.
- **Warning:** A PDB with `minAvailable: 1` (or `maxUnavailable: 0`) on a single-replica deployment will block the node drain indefinitely. Our platform chart explicitly prevents this scenario.

Monitor the node roll using:
```bash
kubectl get nodes -w
```
If a node is stuck in `Ready,SchedulingDisabled` for more than 10 minutes, check for misconfigured PDBs blocking eviction:
```bash
kubectl get evictions -A
```
