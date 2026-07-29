# Implementation Plan: Fill In the IDP Template Substance

- **Status:** Templates filled in; CLI implementation pending
- **Date:** 2026-07-29
- **Companion to:** `2026-07-29-idp-scaffolder-template-restructuring.md` (the design)
- **Scope:** template *content only* — `1-idp-scaffolder-templates/`. No Go/Python CLI changes.
- **Audience:** an implementer (Gemini) executing task-by-task.

---

## Context: what this plan fixes

The directory restructure is done and correct, but the five principal-level upgrades are hollow. Verified state on disk:

| File(s) | Problem |
|---|---|
| `tenant-foundation/gitops-base/{appproject,namespace,networkpolicy}.yaml.tmpl`, `CODEOWNERS.tmpl` | **empty (0 bytes)** |
| `systems/app-of-apps/applicationset.yaml.tmpl` | **empty (0 bytes)** |
| `components/catalog-info.yaml.tmpl` | **empty (0 bytes)** |
| `components/delivery/values-dev.yaml.tmpl`, `values-prod.yaml.tmpl` | **empty (0 bytes)** |
| `components/infra/postgres.tf.tmpl` vs `s3.tf.tmpl` | **byte-identical duplicates**; both still use generic `[[ .CloudServices ]]` instead of a hardcoded module — so `golden-paths.yaml`'s capability model connects to nothing |
| `infra-base/providers.tf.tmpl` vs `team-iam.tf.tmpl` | **byte-identical duplicates**; both are the old `team-base.tf`; neither declares a `provider "aws"` or a state `backend` |

**Templating rules (must follow):** Go `text/template` delimiters `[[ .Field ]]`; `.tmpl` extension; variables in use are `[[ .TeamName ]]`, `[[ .AppName ]]`, `[[ .SystemName ]]`, `[[ .CloudServices ]]`. Do **not** introduce Jinja. Do **not** touch the CLI code.

**Reuse rule — copy vs move vs reference (read first):** the three reuse targets are handled differently. Do not blanket-copy.

| Existing asset | Action | Rationale |
|---|---|---|
| TF modules (`aws-postgres`/`aws-s3`/`aws-iam`) | **Reference in place** — never copy/move | They are the blessed modules; templates consume them by git URL. Copying forks them. |
| Kyverno controller + cluster policies | **Stay in place; reference only** | Template adds a per-team binding/exception pointing at them. Copying policy bodies forks governance. |
| Cluster-wide `applicationset-tenant-apps.yaml` | **Move + reorganise** into a per-team model (Task C) | A single AppSet with `project: default` undermines the per-team `AppProject` guardrail from Task B. See the decision in Task C. |

---

## Task A — Make infra capabilities real (differentiate the duplicates)

**Why:** `golden-paths.yaml` maps `postgres → aws-postgres` and `s3 → aws-s3`, but both `.tf.tmpl` files are identical generics. The capability model must resolve to a specific blessed module per file.

**A1. `components/infra/postgres.tf.tmpl`** — hardcode the `aws-postgres` module (drop the `[[ .CloudServices ]]` conditional):
```hcl
locals {
  resource_name_prefix = "[[ .TeamName ]]-[[ .AppName ]]"
  tags = {
    Team      = "[[ .TeamName ]]"
    Service   = "[[ .AppName ]]"
    ManagedBy = "terraform"
  }
}

module "postgres" {
  source    = "git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//4-platform-engineering/cloud-services-terraform-modules/aws-postgres?ref=v1.0.2"
  team_name = "[[ .TeamName ]]"
  app_name  = "[[ .AppName ]]"
}
```

**A2. `components/infra/s3.tf.tmpl`** — same shape, but `module "s3"` sourcing `.../aws-s3?ref=v1.0.2`.

**A3.** Add `components/infra/iam.tf.tmpl` sourcing `.../aws-iam?ref=v1.0.2` (the third capability already listed in `golden-paths.yaml`).

**Acceptance:** the three infra files are **not** byte-identical; each hardcodes exactly one module matching the `capabilities:` map in `golden-paths.yaml`; none reference `[[ .CloudServices ]]`.

---

## Task B — Fill the tenancy guardrails (`tenant-foundation/gitops-base/`)

**Why:** this bundle is the highest-signal, most-credible part of the whole design. Empty files here defeat the purpose.

**B1. `appproject.yaml.tmpl`** — an ArgoCD `AppProject` named `[[ .TeamName ]]` that scopes the team's blast radius:
- `spec.sourceRepos` — the team's gitops repo URL (may stay the shared repo URL for the demo)
- `spec.destinations` — server `https://kubernetes.default.svc`, `namespace: [[ .TeamName ]]`
- `spec.clusterResourceWhitelist` — empty or minimal (tenants get no cluster-scoped resources)
- `spec.namespaceResourceWhitelist` — the workload kinds (Deployment, Service, ConfigMap, Ingress, ServiceAccount, HPA)

**B2. `namespace.yaml.tmpl`** — three docs in one file: `Namespace` `[[ .TeamName ]]`; a `ResourceQuota` (cpu/memory requests+limits, pod count); a `LimitRange` (default container requests/limits).

**B3. `networkpolicy.yaml.tmpl`** — a default-deny `NetworkPolicy` in `[[ .TeamName ]]` (deny all ingress; allow intra-namespace + DNS egress).

**B4. `CODEOWNERS.tmpl`** — map the generated tree to `@[[ .TeamName ]]` owners, e.g. `* @acme-corp/[[ .TeamName ]]`.

**B5. (reuse) Kyverno policy binding** — the Kyverno **controller** already exists at `4-platform-engineering/cluster-gitops-argocd-apps/security-governance/kyverno.yaml`, and cluster policies exist alongside it (`disallow-default-namespace.yaml`, `require-encrypted-buckets.yaml`). Add `tenant-foundation/gitops-base/policy-exceptions.yaml.tmpl` — a Kyverno `PolicyException` (or namespace label) scoping those existing policies to `[[ .TeamName ]]`. Copy the label/selector conventions from the existing policy files; do not re-author the policies.

**Acceptance:** all five files non-empty and valid YAML; every hardcoded name replaced with `[[ .TeamName ]]`; B5 references the *existing* Kyverno policies rather than duplicating them.

---

## Task C — Fill the ApplicationSet (`systems/app-of-apps/`) by reusing + fixing the existing one

**Decision — move to a per-team model (not copy).** The existing `4-platform` ApplicationSet is a **single cluster-wide** AppSet that globs *all* tenants with `project: default`. That directly contradicts the per-team `AppProject` blast-radius boundary built in Task B — every app would land in the `default` project and bypass the guardrail. So we **reorganise the discovery model**: generate one AppSet **per team**, scoped to that team's `AppProject`, and demote the cluster-wide file to a thin bootstrap.

- **MOVE** the discovery responsibility into `systems/app-of-apps/applicationset.yaml.tmpl` (per-team, per-system).
- **RECONFIGURE** the `4-platform` cluster-wide file into a bootstrap that discovers each team's generated AppSet (app-of-appsets), OR retire it if `onboard-team` applies the per-team AppSet directly. Pick one and state it in the file's comment; do not leave two overlapping cluster-wide AppSets both globbing tenants.

**Reuse source:** start from the existing `applicationset-tenant-apps.yaml` (it also has a **stale** glob — old `2-tenant-workloads/*/*-gitops-repo/apps/*` path). Copy its structure into the template, then fix it.

**C1. `systems/app-of-apps/applicationset.yaml.tmpl`** — start from the existing file, then:
- Rename metadata to `[[ .TeamName ]]-[[ .SystemName ]]`.
- Change the git generator `directories.path` to the **new** structure and the **current** top-level dir: `3-tenant-workloads/[[ .TeamName ]]/gitops-repo/apps/[[ .SystemName ]]/*`.
- Set `spec.template.spec.project` to `[[ .TeamName ]]` (the AppProject from Task B, not `default`).
- Keep `syncPolicy.automated` (prune + selfHeal) and `CreateNamespace=true`.
- Keep pointing the source `path` at `{{path}}/rendered-manifests` (matches the current render model). Note `{{ }}` here is ArgoCD's own Go-template inside the manifest — to survive the scaffolder's `[[ ]]` pass it can be emitted literally; verify the scaffolder does not choke on `{{ }}` (it uses `[[ ]]` delimiters, so `{{ }}` passes through untouched).

**C2. Fix the source-of-truth copy too:** update `4-platform-engineering/.../applicationset-tenant-apps.yaml` to the `3-tenant-workloads` path so the committed platform manifest isn't stale. (Small, but it's a real bug.)

**Acceptance:** template globs `3-tenant-workloads/.../apps/[[ .SystemName ]]/*`; project is `[[ .TeamName ]]`; the `4-platform` copy no longer references `2-tenant-workloads`.

---

## Task D — Fill delivery values + portal manifest (`components/`)

**D1. `components/delivery/values-dev.yaml.tmpl` / `values-prod.yaml.tmpl`** — env-override values consumed by the `standard-helm` chart render. Minimum differentiation: `replicaCount` (1 dev / 2+ prod), `resources` (smaller dev), ingress host (`[[ .AppName ]]-dev.…` vs `[[ .AppName ]].…`), and image tag strategy. Keys must match `components/delivery/standard-helm/values.yaml.tmpl`.

**D2. `components/catalog-info.yaml.tmpl`** — a Backstage `Component`:
```yaml
apiVersion: backstage.io/v1alpha1
kind: Component
metadata:
  name: [[ .AppName ]]
  annotations:
    github.com/project-slug: acme-corp/[[ .TeamName ]]-[[ .AppName ]]
spec:
  type: service
  lifecycle: production
  owner: team-[[ .TeamName ]]
  system: [[ .SystemName ]]
```

**Acceptance:** value files are non-empty, differ dev-vs-prod, and use keys present in the chart; `catalog-info` sets `owner`, `system`, `name` from template vars.

---

## Task E — Genuinely split `infra-base/`

**Why:** both files are the old `team-base.tf`, and **neither** declares a provider or a remote backend — a real gap for a Terraform root module.

**E1. `infra-base/providers.tf.tmpl`** — NEW content: `terraform { required_providers { aws … } backend "s3" { … } }` + `provider "aws"` block (region, default tags with `[[ .TeamName ]]`). This is the team's TF root config; it currently does not exist anywhere.

**E2. `infra-base/team-iam.tf.tmpl`** — keep the existing team-baseline modules (the `aws-iam` "shared" module, and the `aws-vpc` baseline currently in the file), but **remove** the provider/locals duplication that now belongs in `providers.tf`. Keep the `## Danger zone ##` idempotent-module markers.

**Acceptance:** the two files are no longer identical; `providers.tf.tmpl` owns provider+backend; `team-iam.tf.tmpl` owns the shared IAM/VPC modules.

---

## Task F — Reuse map (copy-from → templatize-to)

| Existing source in `4-platform-engineering/` | Action | Target | Change |
|---|---|---|---|
| `…/gitops-orchestration/applicationset-tenant-apps.yaml` | **MOVE + reorganise** | `systems/app-of-apps/applicationset.yaml.tmpl` (per team) + demote the source file to bootstrap | per-team AppSet; glob `3-tenant-workloads/[[ .TeamName ]]/gitops-repo/apps/[[ .SystemName ]]/*`; project `[[ .TeamName ]]` |
| `…/security-governance/kyverno.yaml` + the two policy `*.yaml` | **REFERENCE** (stay in place) | `tenant-foundation/gitops-base/policy-exceptions.yaml.tmpl` | scope existing policies to `[[ .TeamName ]]`; do not duplicate policy bodies |
| `cloud-services-terraform-modules/{aws-postgres,aws-s3,aws-iam}` | **REFERENCE** (git URL) | `components/infra/{postgres,s3,iam}.tf.tmpl` | one hardcoded `module` block per capability; never copy module source |

---

## Task G — Housekeeping

- Remove the stray `copier.yml` files if the Go scaffolder is now definitive (design D1) — or leave them and note they're Python-only. Confirm intent; do not delete silently.
- After all tasks: regenerate one sample tenant end-to-end into `3-tenant-workloads/` and confirm no `[[ … ]]` markers leak into output.
- Update the design doc status line to "Templates filled in; CLI implementation pending".

---

## Definition of done

1. No 0-byte `.tmpl` files under `1-idp-scaffolder-templates/`.
2. No byte-identical pairs among the infra / infra-base files.
3. `golden-paths.yaml` capabilities each resolve to a real, distinct `components/infra/*.tf.tmpl`.
4. The ApplicationSet template and the `4-platform` source copy both target `3-tenant-workloads`.
5. Every file uses `[[ ]]` delimiters only; a sample render produces valid, marker-free output.
