# ADR 0004: GitOps Environment Boundary

## Status
Accepted

## Context
The GitOps repository structure needs to organize deployment manifests across multiple environments. We must decide how to structure these boundaries to prevent cross-environment pollution and ensure that deployments are deliberately promoted.

## Decision
**GitOps Environment Boundary (Dev/Prod):** The GitOps layer will strictly mirror the Terraform infrastructure layer by adopting an "Environment-as-Folders" promotion model. The CLI will scaffold GitOps manifests exclusively into a `dev/` directory (e.g., `gitops/apps/<app>/dev/`). Production manifests never appear automatically; they require a deliberate PR to promote from `dev/` to `prod/`.

## Consequences
- This physical separation prevents a single shared `Chart.yaml` from triggering premature deployments.
- Perfectly aligns the deployment promotion gate with the infrastructure promotion gate.
- Promotes clarity: if a directory for an environment exists, the application is deployed there.
