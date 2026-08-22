# 1. Platform Catalog

The platform's offering. `catalog.yaml` declares the golden paths, the offered runtimes,
the version-pinned capability → module mapping, and a `destinations:` table that is the
output contract. Three directories sit alongside it, and their names carry the fact that
matters — *how often each one renders*:

- **`per-team/`** — rendered **once per team**, by `onboard-team`.
- **`per-service/`** — rendered **once per service** (or once per requested capability), by
  `add-service`.
- **`charts/`** — never scaffolded at all. CI renders it and only the output reaches a
  tenant repo.

## Render Map — what each directory produces

Answers "what does this directory turn into?" without opening `catalog.yaml`.
`catalog.yaml`'s `destinations:` block remains the **authority**; this table is the
readable projection of it. If they disagree, `catalog.yaml` is right and this table is
stale — fix the table.

Every path on the left is relative to this directory; every path on the right is
relative to `3-tenant-workloads/`.

| You edit this | It renders to | Rendered by | How often |
|---|---|---|---|
| `per-team/apps/` | `{team}/apps/` | `onboard-team` | once per team |
| `per-team/infra/` | `{team}/infra/` | `onboard-team` | once per team |
| `per-team/gitops/` | `{team}/gitops/` | `onboard-team` | once per team |
| `per-service/apps/runtimes/<lang>/` | `{team}/apps/{app}/` | `add-service` | once per service — **one** `<lang>` picked by `--runtime` / golden path |
| `per-service/apps/service-meta/` | `{team}/apps/{app}/` | `add-service` | once per service, always |
| `per-service/infra/capabilities/<cap>.tf.tmpl` | `{team}/infra/apps/{app}/{env}/` | `add-service` | one file per requested capability with `provisioner: terraform` |
| `per-service/gitops/capabilities/<cap>.yaml.tmpl` | `{team}/gitops/apps/{app}/{env}/` | `add-service` | one file per requested capability with `provisioner: ack` |
| `per-service/gitops/release/` | `{team}/gitops/apps/{app}/{env}/` | `add-service` | once per service per env |
| `charts/service/` | **nothing** — never scaffolded | CI, via `helm template` | output only, into `{team}/gitops/apps/{app}/{env}/manifests/` |

**Why the directory names carry cardinality.** The prefix (`per-team` / `per-service`)
tells you *how often* something renders; the path underneath tells you *where it lands*.
That is a readable projection of the two rightmost columns above, available without
opening this table at all — which is the whole point of the rename this directory used to
lack.

**Why `per-service/` splits into `infra/` and `gitops/` subtrees.** A capability's
`provisioner:` in `catalog.yaml` decides which one its template lives in:
`provisioner: terraform` → `per-service/infra/capabilities/<cap>.tf.tmpl`, applied by a
Terraform run; `provisioner: ack` → `per-service/gitops/capabilities/<cap>.yaml.tmpl`,
applied by ArgoCD reconciling a Kubernetes-native CRD. The directory a template lives in
**is** the routing decision — both scaffolder engines look in the directory that matches
a capability's declared provisioner, so there is no file-extension sniffing and no
destinations key pointing at a directory that doesn't exist.

See `.agents/AGENTS.md` for the full platform model, code conventions, and execution
commands — this file exists so the render map is visible from inside the directory it
describes, not only from agent instructions.
