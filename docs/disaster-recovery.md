# Disaster Recovery

**Date:** 2026-08-22

Our primary disaster recovery strategy is **re-bootstrap and reconcile from Git**. We do not rely on cluster-wide snapshotting tools like Velero for infrastructure or declarative configuration, because ArgoCD already continuously enforces the state defined in Git. If the cluster is lost, we provision a new EKS cluster and apply the root `bootstrap.yaml` to begin the restoration process.

However, Git does not hold everything. The following state exists only outside Git and requires explicit management.

## What is NOT in Git

| Not in git | Consequence if lost | Mitigation |
|---|---|---|
| PVC data (Prometheus TSDB, Loki chunks, Keycloak DB) | Metrics and log history are lost. | Keycloak realm is fully declarative (Phase 7). For observability data, we accept the data loss in a DR scenario, optimizing for faster RTO over historical metrics retention. |
| Terraform state | The platform's own AWS resources (VPC, IAM roles, EKS cluster) become unmanaged. | State is stored in a versioned S3 bucket (`acme-corp-terraform-state`). We rely on AWS S3 durability and versioning. |
| **Sealed Secrets Private Key** | **Every `SealedSecret` in git becomes permanently undecryptable.** | **CRITICAL:** The Sealed Secrets controller key must be backed up securely off-cluster. |

## Sealed Secrets Controller Backup

The private key used by Sealed Secrets must be extracted and stored securely (e.g., in a secure enterprise vault or 1Password) *before* any disaster occurs. It must not be committed to this repository.

To back up the key:
```bash
kubectl get secret -n kube-system \
  -l sealedsecrets.bitnami.com/sealed-secrets-key -o yaml > sealed-secrets-key.yaml
```

To restore the key into a new cluster (do this *before* installing Sealed Secrets or ArgoCD):
```bash
kubectl apply -f sealed-secrets-key.yaml
```

## Two Secret Systems
We intentionally run two secret management systems:
- **Sealed Secrets:** Used exclusively for "bootstrap" secrets that must exist before any cloud dependencies are resolved (e.g., ArgoCD repository credentials, IdP bootstrap credentials).
- **External Secrets:** Used for everything else (tenant applications, database passwords, API keys) that have a real secret store (e.g., AWS Secrets Manager, HashiCorp Vault) behind them.

## Recovery Objectives
- **RTO (Recovery Time Objective):** ~2 hours to re-provision cloud foundation, apply `bootstrap.yaml`, and allow ArgoCD to fully sync the cluster state.
- **RPO (Recovery Point Objective):**
  - Git configuration: ~0 (continuous)
  - Observability data: N/A (We accept total loss of historical logs and metrics)
