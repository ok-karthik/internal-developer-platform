# ADR 0007: Values vs Rendered Promotion

## Status
Accepted

## Context
In a GitOps pipeline involving templates (Helm), we must decide what artifacts trigger a deployment promotion from one environment to the next.

## Decision
**Promotion Surface (Values vs Rendered):** Promotion is defined strictly as a PR copying `<tenant>/gitops/apps/<app>/dev/values.yaml` to `prod/values.yaml`. 
- CI reacts to this commit by rendering the chart against the new `values.yaml`.
- CI commits the rendered plain YAML into `manifests/`.
- ArgoCD syncs the rendered YAML.

## Consequences
- Developers only interact with simplified `values.yaml` files, not complex Kubernetes manifests.
- The Git history of `values.yaml` serves as an audit trail of intent, while the Git history of `manifests/` serves as the audit trail of what was actually deployed.
