# ADR 0008: Platform as a Product

## Status
Accepted

## Context
Internal Developer Platforms can easily devolve into unstructured collections of scripts and templates if not guided by a core philosophy.

## Decision
**Platform as a Product Philosophy:** The platform is treated as a product where development teams are the customers. This is enacted through platform behavior, not just folder names:
- **Golden Paths** are the UX.
- The **Scaffolder CLI** is the self-service portal.
- **Version-pinned Terraform modules** are the stable API.

## Consequences
- Requires a high bar for documentation, stability, and versioning.
- Changes to Terraform modules or the scaffolder must be treated as API updates with deprecation cycles.
- The GitOps reference architecture acts as a cohesive product rather than a toolkit.
