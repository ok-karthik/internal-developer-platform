# Backstage (Phase 8) — scoped deliberately

**7 of 577 postings, 1.2%.** Per `PLAN.md`'s Interlude, Backstage is demoted below Phases
4-6 on keyword grounds. What survives here is the design decision and the config
artifacts that follow from it — not a running Backstage instance.

## Why nothing here is a running app

Scaffolding an actual Backstage instance means creating a second, separate application: a
TypeScript monorepo (`npx @backstage/create-app`), its own `node_modules`, its own backend
process, its own auth wiring — a different category of work from everything else in this
repo, which is Kubernetes manifests, Terraform, and two CLI engines. `PLAN.md` itself
timeboxes this to two days and says explicitly: *"steps 1-2 deliver most of the demo value
in a few hours, and step 3 [wrapping the scaffolder] is where the schedule goes to die...
if you hit the box, ship what works."* Given the phase's own 1.2% weight against Phases
4-7 (all shipped, all keyword-load-bearing), spending the multi-day budget an actually
running Backstage instance needs was the wrong trade here. What ships instead is the part
that is genuinely reusable the day someone *does* run `npx @backstage/create-app`: the
architectural decision, the Software Template, and the config fragment.

## Does Backstage replace the CLI? No — it becomes a client of it.

Catalog validation, golden-path resolution, template rendering, and destination routing
are this platform's **domain logic**, and they stay in Go (`2-idp-scaffolder/golang/`).
Backstage is a **presentation layer** on top. Putting the logic in Backstage TypeScript
instead would mean exactly one client, forever, coupled to a framework this repo does not
control — see `PLAN.md` Phase 8.0 for the full argument, including why a CLI a developer
can run and read is a *better* artefact for a job search than a UI wrapping someone else's
framework.

Three ways to connect them, and the one actually used here:

| | Approach | Cost | This repo |
|---|---|---|---|
| (a) | Reimplement scaffolding as Backstage TS actions | high | **Rejected** — throws away the Go work |
| **(b)** | Backstage shells out to the CLI binary in its own container | ~0 | **`software-template.yaml` below** |
| (c) | CLI logic behind HTTP; Backstage calls it | 2-3 days | Documented as the next step, not built |

## What is actually here

- **`software-template.yaml`** — a real Backstage Software Template using the built-in
  `fetch:template` and `publish:github` actions plus a placeholder custom action
  (`idp:run-cli`) that shells to `2-idp-scaffolder/golang`'s `add-service` command inside
  its own container — approach (b). The custom action itself is a few lines of TypeScript
  once a real Backstage backend exists to host it; it is not written here because there is
  no backend to host it in, and an action file with no plugin around it would be inert.
- **`app-config.fragment.yaml`** — the catalog `locations` entry that would make
  `add-service`'s already-emitted `catalog-info.yaml` (`3-tenant-workloads/*/apps/*/catalog-info.yaml`)
  visible to Backstage with zero new code, plus the OIDC `auth` block pointing at
  Keycloak's `backstage` client (Phase 7.1) — merge this into a real `app-config.yaml` once
  one exists.

## The design rule this protects

`cmd/cli/` in the Go scaffolder stays a thin adapter — no business logic ever moves into
it. The day Backstage (or anything else) forces business logic into `cmd/`, that separation
is broken. This belongs in `2-idp-scaffolder/golang/TODO.md`, which is out of scope to edit
on this branch; recorded here so it is not lost.
