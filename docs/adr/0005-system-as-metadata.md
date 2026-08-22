# ADR 0005: System is Metadata, Not a Directory

## Status
Accepted

## Context
An earlier revision carried a `<system>/` directory level under `apps/` to group related applications. This was initially justified as an ArgoCD anchor and a Terraform blast-radius boundary.

## Decision
**System is Metadata, Not a Directory (reversed):** The physical `<system>/` directory level has been removed. Neither of the original justifications held up: 
- Terraform blast radius is set by where `apply` runs (`<app>/<env>/`).
- A team-level AppSet globbing `apps/*/*` discovers apps perfectly well without an intermediate system directory.

Backstage models System as a *relation*, so it now lives only in `catalog-info.yaml` (`spec.system`, via the optional `--system` flag) where it cannot drift from a second encoding in the path.

## Consequences
- Flatter, simpler directory structure (`apps/<app>`).
- Reintroduce a grouping level only when a single team outgrows a flat app list, which can be accomplished via a YAML edit since `destinations:` is driven by data.
