# Backstage (Phase 8) — scoped deliberately

The Backstage app lives in its own repo (`backstage/dev-portal`). Its deployment will live under
`4-platform-engineering/2-cluster-services/` later. This folder holds what this repo owns: the Software
Template, a config fragment, and the workflow Backstage calls.

## Does Backstage replace the CLI? No — it becomes a client of it.

Catalog validation, golden-path resolution, template rendering, and destination routing
are this platform's **domain logic**, and they stay in Go (`2-idp-scaffolder/`).
Backstage is a **presentation layer** on top. Putting the logic in Backstage TypeScript
instead would mean exactly one client, forever, coupled to a framework this repo does not
control — see `PLAN.md` Phase 8.0 for the full argument, including why a CLI a developer
can run and read is a *better* artefact for a job search than a UI wrapping someone else's
framework.

Four ways to connect them, and the one actually used here:

| | Approach | Cost | This repo |
|---|---|---|---|
| (a) | Reimplement scaffolding as Backstage TypeScript actions | high | **Rejected** — throws away the Go work |
| (b) | Custom action (`idp:run-cli`) runs the CLI inside Backstage | medium | **Replaced** — needs TypeScript, Go in the Backstage image, and push credentials in Backstage |
| (c) | CLI logic behind an HTTP API; Backstage calls it | 2-3 days | Not built (ADR 0015) |
| **(d)** | **`github:actions:dispatch` → `scaffold-service.yaml` → PR** | low | **Used** (ADR 0016) |

## How the pieces connect

```
Developer fills the Backstage form (tenant, app, golden path, capabilities, system)
        │  step: github:actions:dispatch   (built-in Backstage action, no TypeScript)
        ▼
GitHub Actions: .github/workflows/scaffold-service.yaml   (workflow_dispatch)
        │  1. validate inputs          3. go run . add-service --output-root ../3-tenant-repos
        │  2. tenant exists, app new   4. commit on branch scaffold/<tenant>-<app>, open a PR
        ▼
Pull request with the new service under 3-tenant-repos/<tenant>/...
        │  human review + merge
        ▼
tenant-repos-ci-cd.yaml (already exists) builds the image and renders manifests → ArgoCD syncs
```

The workflow writes exactly what `make demo-add-service` writes today — `workloads-repo/services/<app>/`,
`workloads-repo/infra/services/<app>/<env>/` and `gitops-repo/services/<app>/<env>/` — but on a new
branch, delivered as a PR instead of a local change. It never pushes to `main`.

## What is here

- **`software-template.yaml`** — the Backstage form. Its one step dispatches the workflow.
- **`app-config.fragment.yaml`** — the catalog `locations` entry that would make
  `add-service`'s already-emitted `catalog-info.yaml` (`3-tenant-repos/*/apps/*/catalog-info.yaml`)
  visible to Backstage with zero new code, plus the OIDC `auth` block pointing at
  Keycloak's `backstage` client (Phase 7.1) — merge this into a real `app-config.yaml`.
- **`LEARNING.md`** — plain-language guide to how Backstage works.
- **`.github/workflows/scaffold-service.yaml`** — the workflow that runs the CLI and opens the PR.

## The design rule this protects

`cmd/cli/` in the Go scaffolder stays a thin adapter — no business logic ever moves into
it. The day Backstage (or anything else) forces business logic into `cmd/`, that separation
is broken. This belongs in a `2-idp-scaffolder/TODO.md` (which does not exist yet); recorded here so it is not lost.
