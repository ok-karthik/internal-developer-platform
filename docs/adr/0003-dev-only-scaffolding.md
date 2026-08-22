# ADR 0003: Dev-Only Scaffolding

## Status
Accepted

## Context
When a new service is onboarded, infrastructure definitions (such as Terraform modules for RDS or S3) are scaffolded into the tenant's repository. We needed to decide whether to scaffold these definitions for all target environments (e.g., dev, staging, prod) simultaneously, or adopt a more progressive approach.

## Decision
**Dev-Only Scaffolding & Gated Promotion:** When scaffolding infrastructure (e.g., `postgres.tf`), the CLI writes **only** to the `dev/` directory. Production infrastructure never appears automatically. 

## Consequences
- Prevents premature or accidental provisioning of production infrastructure.
- Forces teams to undergo a deliberate, gated promotion step (such as a pull request that copies the `dev/` configurations to `prod/`).
- Enforces an "Environment-as-Folders" promotion model where the presence of files acts as the declaration of intent for an environment.
