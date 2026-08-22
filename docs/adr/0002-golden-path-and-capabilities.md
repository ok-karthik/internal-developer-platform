# ADR 0002: Golden Path and Capabilities

## Status
Accepted

## Context
The platform needs to provide pre-configured, production-ready "paved roads" for common service types, while also allowing teams to selectively include specific infrastructure capabilities like databases or object storage. We needed a consistent way to expose both paradigms through the Scaffolder CLI without creating competing modes of operation.

## Decision
1. **Golden-Path as Seed, Capabilities as Override:** The CLI uses a single unified pipeline rather than two competing modes. 
   - A golden path (`--golden-path`) seeds a default runtime and capability array from `catalog.yaml`.
   - Explicit flags (`--capabilities`) can override or extend that seed array. 
   - This provides the "paved road" defaults while retaining the à la carte "menu" flexibility as an escape hatch.

2. **Unified Vocabulary:** The CLI and templates will standardize on the term **capabilities** (e.g., the `--capabilities` flag) rather than mixing in legacy terms like `--cloud-services`. This keeps the CLI perfectly aligned with `catalog.yaml`.

## Consequences
- Reduces CLI complexity by avoiding mutually exclusive modes.
- Users can start with a golden path and seamlessly transition to a custom capability set as their service evolves.
- Consistent naming (`capabilities`) reduces cognitive load and simplifies catalog definitions.
