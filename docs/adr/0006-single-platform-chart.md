# ADR 0006: Single Platform Chart

## Status
Accepted

## Context
Deploying services requires Kubernetes manifests. We needed to decide whether to provide templates that are copied into each tenant's repository, or maintain a centralized Helm chart.

## Decision
**Rendered Manifests & Golden Path Delivery:** ONE platform-owned chart at `1-platform-catalog/charts/service/` serves every service.
- The chart sits in `charts/` rather than `per-service/` because it is never *copied* into a tenant repo — it has no `destinations` key, and only its rendered output reaches `3-tenant-workloads/`.
- Per-app identity comes from the Helm release name plus `nameOverride` in the release values.
- Its `values.yaml` holds only genuinely universal defaults (non-root, read-only rootfs, dropped capabilities).
- The CLI scaffolds only per-env `values.yaml` into `<tenant>/gitops/apps/<app>/<env>/`; no Helm packaging leaks into `<tenant>/apps/`.

## Consequences
- A chart fix ships fleet-wide immediately instead of being copy-pasted into N repos.
- CI passes no `--set` overrides: rendered output is a pure function of (chart, values.yaml), so the committed manifests are an honest record of what git says should be running.
- ArgoCD syncs ONLY `manifests/`, reducing complexity in the cluster.
