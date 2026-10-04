# ADR 0015: Go Is the Only Scaffolder Engine

**Date:** 2026-10-04
**Status:** Accepted

## Context

Two scaffolder engines read one catalog: Go (1,594 lines) and Python (789 lines). Every template field change needed edits in both, and a CI parity job diffed their output byte for byte. Backstage, the Makefile demo and CI already used the Go engine. The only thing Python offered that Go did not was a FastAPI server, and nothing called it.

## Decision

Go is the only engine, and it lives at `2-idp-scaffolder/`. The Python engine is deleted; recover it from git history (the last commit that has it is `0688f58`).

Consumers call the CLI:
- Developers run it directly.
- Backstage calls it through a custom scaffolder action (`idp:run-cli`) that runs the binary.
- CI runs it (`smoke-test-go-cli` scaffolds a real tenant and service on every push to `main` and every PR that touches the scaffolder or catalog).

There is no REST API for now.

### When to add a REST API

Add `cmd/api/` (using `net/http`) when a second consumer cannot run a binary, for example Backstage moving to a hosted backend without the CLI in its image, or a non-Backstage portal. `templater.Resolve` is already pure, so a handler can reuse it.

## Consequences

- We lose the cross-language check on the catalog. It is replaced by load-time validation in `internal/catalog`, golden-file tests in `internal/templater/testdata/`, and the CI smoke run.
- There is one place to change per template field.
- This supersedes the two-engine reasoning in the README's former "One Catalog, Two Engines" section.
