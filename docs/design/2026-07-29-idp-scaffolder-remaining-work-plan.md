# Plan: IDP Templates — Remaining Work & Fixes

- **Status:** Templates filled in; CLI implementation pending
- **Date:** 2026-07-29
- **Supersedes/continues:** `2026-07-29-idp-scaffolder-fill-in-implementation-plan.md` (full task detail lives there)
- **Scope:** template content only — `1-idp-scaffolder-templates/` (+ one stale-path fix in `4-platform-engineering/`). No CLI changes.

---

## Status board

| Task | What | Status |
|---|---|---|
| **B** | Tenancy guardrails (`tenant-foundation/gitops-base/`) | ✅ **Done & verified** — 2 fixes below |
| **A** | Make infra capabilities real (differentiate `postgres`/`s3`/`iam`) | ⬜ Pending |
| **C** | ApplicationSet: move to per-team + fix stale path | ⬜ Pending |
| **D** | Delivery values (dev/prod) + Backstage `catalog-info` | ⬜ Pending |
| **E** | Split `infra-base` (providers/backend vs team IAM) | ⬜ Pending |
| **G** | Housekeeping (`copier.yml`, sample render, status update) | ⬜ Pending |

**Definition of done (unchanged):** no 0-byte `.tmpl`; no byte-identical pairs; every `golden-paths.yaml` capability resolves to a distinct infra template; ApplicationSet + its `4-platform` source both target `3-tenant-workloads`; `[[ ]]` delimiters only; a sample render is marker-free.

---

## B — Fixes to fold in (verification follow-ups)

Task B passed acceptance (files non-empty, distinct, valid, real Kyverno policy/rule names). Two things make the guardrails actually *function*, plus one fidelity nit:

- **B-fix-1 (must-fix — exception is currently inert).** The Kyverno install at `4-platform-engineering/cluster-gitops-argocd-apps/security-governance/kyverno.yaml` (chart `3.7.1`) does **not** enable PolicyExceptions. Kyverno ignores `PolicyException` resources unless started with exceptions enabled. Add the helm parameters to that Application, e.g.:
  ```yaml
  - name: features.policyExceptions.enabled
    value: "true"
  - name: features.policyExceptions.namespace
    value: ""      # "" = allow exceptions in any namespace; or pin to a governance ns
  ```
  (Confirm exact parameter path against the pinned chart; older charts used `--enableException=true` on the admission controller.)
- **B-fix-2 (verify apiVersion).** `policy-exceptions.yaml.tmpl` declares `apiVersion: kyverno.io/v2beta1`. On Kyverno 3.7.x `PolicyException` is generally served as `kyverno.io/v2`. Verify against the pinned chart and update if needed.
- **B-fix-3 (fidelity nit).** The exception file's comment claims Platform/Security approval "via CODEOWNERS," but `CODEOWNERS.tmpl` routes everything to `@acme-corp/[[ .TeamName ]]`. To make that true, add a more specific line so the exceptions file needs security sign-off:
  ```
  * @acme-corp/[[ .TeamName ]]
  /policy-exceptions.yaml @acme-corp/security
  ```

**Acceptance:** Kyverno Application enables exceptions; `PolicyException` apiVersion matches the pinned chart; CODEOWNERS routes `policy-exceptions.yaml` to a security owner.

---

## A — Make infra capabilities real  (`components/infra/`)

`postgres.tf.tmpl` and `s3.tf.tmpl` are currently **byte-identical** and still use the generic `[[ .CloudServices ]]` conditional, so `golden-paths.yaml`'s capability map resolves to nothing.

- **A1** `postgres.tf.tmpl` → hardcode one `module "postgres"` sourcing `.../aws-postgres?ref=v1.0.2`; drop the `[[ .CloudServices ]]` conditional. Keep the `locals`/tags block using `[[ .TeamName ]]` / `[[ .AppName ]]`.
- **A2** `s3.tf.tmpl` → same shape, `module "s3"` → `.../aws-s3?ref=v1.0.2`.
- **A3** add `iam.tf.tmpl` → `module "iam"` → `.../aws-iam?ref=v1.0.2` (third capability in `golden-paths.yaml`).

**Acceptance:** three files, not byte-identical; each hardcodes exactly the module named in `golden-paths.yaml`'s `capabilities:` map; none reference `[[ .CloudServices ]]`.

---

## C — ApplicationSet: move to per-team + fix stale path  (`systems/app-of-apps/`)

**Decision (from prior plan):** a single cluster-wide AppSet with `project: default` bypasses the Task B `AppProject`. Move discovery into a **per-team** AppSet; demote the cluster-wide file.

- **C1** `applicationset.yaml.tmpl`: start from `4-platform-engineering/.../gitops-orchestration/applicationset-tenant-apps.yaml`, then —
  - metadata name `[[ .TeamName ]]-[[ .SystemName ]]`;
  - git generator glob → `3-tenant-workloads/[[ .TeamName ]]/gitops-repo/apps/[[ .SystemName ]]/*`;
  - `spec.template.spec.project: [[ .TeamName ]]` (not `default`);
  - keep `syncPolicy.automated` (prune+selfHeal), `CreateNamespace=true`, and the `{{path}}/rendered-manifests` source (ArgoCD's own `{{ }}` passes through the `[[ ]]` scaffolder untouched — sanity-check one render).
- **C2** Fix the source manifest `4-platform-engineering/.../applicationset-tenant-apps.yaml`: it still globs the **old** `2-tenant-workloads/*/*-gitops-repo/apps/*`. Either update it to `3-tenant-workloads` as a thin bootstrap that discovers per-team AppSets, or retire it if `onboard-team` applies the per-team AppSet directly. **Do not leave two overlapping cluster-wide AppSets globbing tenants.**

**Acceptance:** template globs `3-tenant-workloads/.../apps/[[ .SystemName ]]/*` with project `[[ .TeamName ]]`; no manifest anywhere still references `2-tenant-workloads`.

---

## D — Delivery values + portal manifest  (`components/`)

- **D1** `delivery/values-dev.yaml.tmpl` / `values-prod.yaml.tmpl`: real env overrides consumed by the `standard-helm` chart — `replicaCount` (1 dev / 2+ prod), `resources` (smaller dev), ingress host (`[[ .AppName ]]-dev.…` vs `[[ .AppName ]].…`), image tag strategy. Keys **must** match `delivery/standard-helm/values.yaml.tmpl`.
- **D2** `catalog-info.yaml.tmpl`: Backstage `Component` with `metadata.name: [[ .AppName ]]`, `spec.owner: team-[[ .TeamName ]]`, `spec.system: [[ .SystemName ]]`, `spec.type: service`, `spec.lifecycle: production`.

**Acceptance:** value files non-empty, differ dev-vs-prod, keys exist in the chart; `catalog-info` populates name/owner/system from template vars.

---

## E — Genuinely split `infra-base/`

Both files are the old `team-base.tf`, and **neither declares a `provider` or a state `backend`** — a real gap.

- **E1** `providers.tf.tmpl` → NEW: `terraform { required_providers { aws … } backend "s3" { … } }` + `provider "aws"` (region, default tags with `[[ .TeamName ]]`).
- **E2** `team-iam.tf.tmpl` → keep the shared `aws-iam` (+ baseline `aws-vpc`) modules and the `## Danger zone ##` markers; remove the provider/locals duplication now owned by `providers.tf`.

**Acceptance:** the two files are no longer identical; `providers.tf` owns provider+backend; `team-iam.tf` owns the shared modules.

---

## G — Housekeeping

- Decide the stray `copier.yml` files (root + `components/runtimes/*/`): keep and label Python-only, or remove if the Go scaffolder is definitive. **Confirm intent; don't delete silently.**
- Regenerate one sample tenant end-to-end into `3-tenant-workloads/`; confirm **no `[[ … ]]` markers leak** into output.
- Update the design doc status to "Templates filled in; CLI implementation pending."

---

## Suggested order

**A → E → C → D**, then **B-fixes**, then **G**. Rationale: A and E finish the Terraform capability model together; C is self-contained but touches `4-platform`; D is lowest-risk; B-fixes are trivial; G is the final verification pass. Hand back after each task group for byte-for-byte verification (sizes → duplicates → real content → cross-check against source manifests).
