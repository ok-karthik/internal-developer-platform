# 1. Platform Catalog

The platform's offering. `catalog.yaml` declares the golden paths, the offered runtimes,
the version-pinned capability → module mapping, and a `destinations:` table that is the
output contract. Three directories sit alongside it, and their names carry the fact that
matters — *how often each one renders*:

- **`per-tenant/`** — rendered **once per tenant**, by `onboard-tenant`.
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
relative to `3-tenant-repos/`. Each tenant is two directories — `workloads-repo/` and `gitops-repo/` — one per real repository (ADR 0010).

| You edit this | It renders to | Rendered by | How often |
|---|---|---|---|
| `per-tenant/root/` | `{tenant}/workloads-repo/` | `onboard-tenant` | once per tenant |
| `per-tenant/infra/` | `{tenant}/workloads-repo/infra/` | `onboard-tenant` | once per tenant |
| `per-tenant/gitops/` | `{tenant}/gitops-repo/` | `onboard-tenant` | once per tenant |
| `per-service/apps/runtimes/<lang>/` | `{tenant}/workloads-repo/services/{app}/` | `add-service` | once per service — **one** `<lang>` picked by `--runtime` / golden path |
| `per-service/apps/service-meta/` | `{tenant}/workloads-repo/services/{app}/` | `add-service` | once per service, always |
| `per-service/infra/capabilities/<cap>.tf.tmpl` | `{tenant}/workloads-repo/infra/services/{app}/{env}/` | `add-service` | one file per requested capability with `provisioner: terraform` |
| `per-service/gitops/capabilities/<cap>.yaml.tmpl` | `{tenant}/gitops-repo/services/{app}/{env}/` | `add-service` | one file per requested capability with `provisioner: ack` |
| `per-service/gitops/release/` | `{tenant}/gitops-repo/services/{app}/{env}/` | `add-service` | once per service per env |
| `charts/service/` | **nothing** — never scaffolded | CI, via `helm template` | output only, into `{tenant}/gitops-repo/services/{app}/{env}/manifests/` |

**Why the directory names carry cardinality.** The prefix (`per-tenant` / `per-service`)
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
