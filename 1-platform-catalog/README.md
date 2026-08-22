# 1. Platform Catalog

The platform's offering. `catalog.yaml` declares the golden paths, the offered runtimes,
the version-pinned capability → Terraform module mapping, and a `destinations:` table
that is the output contract. Three directories sit alongside it, distinguished by *what
happens to the files in them*: `blueprints/` is rendered **once per team** by
`onboard-team`, `building-blocks/` is rendered **once per service** by `add-service`, and
`charts/` is never scaffolded at all — CI renders it and only the output reaches a tenant
repo.

## Render Map — what each directory produces

Answers "what does this directory turn into?" without opening `catalog.yaml`.
`catalog.yaml`'s `destinations:` block remains the **authority**; this table is the
readable projection of it. If they disagree, `catalog.yaml` is right and this table is
stale — fix the table.

Every path on the left is relative to this directory; every path on the right is
relative to `3-tenant-workloads/`.

| You edit this | It renders to | Rendered by | How often |
|---|---|---|---|
| `blueprints/team/apps/` | `{team}/apps/` | `onboard-team` | once per team |
| `blueprints/team/infra/` | `{team}/infra/` | `onboard-team` | once per team |
| `blueprints/team/gitops/` | `{team}/gitops/` | `onboard-team` | once per team |
| `building-blocks/runtimes/<lang>/` | `{team}/apps/{app}/` | `add-service` | once per service — **one** `<lang>` picked by `--runtime` / golden path |
| `building-blocks/service-meta/` | `{team}/apps/{app}/` | `add-service` | once per service, always |
| `building-blocks/capabilities/<cap>.tf.tmpl` | `{team}/infra/apps/{app}/{env}/` | `add-service` | one file per requested capability with `provisioner: terraform` |
| `building-blocks/delivery/release/` | `{team}/gitops/apps/{app}/{env}/` | `add-service` | once per service per env |
| `charts/service/` | **nothing** — never scaffolded | CI, via `helm template` | output only, into `{team}/gitops/apps/{app}/{env}/manifests/` |

Two gaps this table makes visible, both real and both tracked in `PLAN.md`:

- **`building-blocks/capabilities-ack` is a destinations key with no directory.** ACK
  `.yaml.tmpl` templates share `building-blocks/capabilities/` with the Terraform
  `.tf.tmpl` ones, so routing them to `gitops/` instead of `infra/` would require the
  renderer to dispatch on file extension. That dispatch is unbuilt in both engines.
- **`blueprints/` vs `building-blocks/` encode nothing.** The distinction that matters is
  *once per team* vs *once per service*, which is exactly what the two rightmost columns
  above supply. `PLAN.md` §3.9 proposes `per-team/` and `per-service/` with the output
  tree mirrored beneath each — which also removes the phantom key above by making the
  directory the router. **It is blocked**: these names are hardcoded in
  `2-idp-scaffolder/golang/internal/catalog/catalog.go:43-49,127`,
  `2-idp-scaffolder/golang/internal/templater/render.go:181-243`,
  `2-idp-scaffolder/python/cli.py:37-100` and `2-idp-scaffolder/python/api.py:75`, so it
  cannot ship while `2-idp-scaffolder/golang/` is out of scope on the branch this table
  was written from.

See `.agents/AGENTS.md` for the full platform model, code conventions, and execution
commands — this file exists so the render map is visible from inside the directory it
describes, not only from agent instructions.
