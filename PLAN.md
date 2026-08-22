# PLAN.md — Phase History

What each phase of this platform's build-out actually did. Kept short on purpose — this is
a changelog, not the design doc. The original 1,780-line planning document (with the full
reasoning behind each decision) is retrievable from git history: `git log --all -- PLAN.md`.

Phases 0 through 10 below are tagged **`v2.0.0`** — also the git tag every Terraform module
source in `catalog.yaml` resolves against (`?ref=v2.0.0`), so the tag isn't just a label,
it's the actual pinned version tenants build against.

---

**Phase 0 — Truth-up.** Fixed two real `NetworkPolicy` bugs (Traefik couldn't reach tenant
pods; egress didn't allow traffic to RDS/AWS). Replaced one default-deny blob with 5 named
policies. Corrected the README's Component Matrix (it claimed Crossplane; nothing deployed
it). Deleted `archived/`. Documented that infrastructure provisioning was a text file, not
a closed loop.

**Phase 1 — Multi-tenancy controls.** Added RBAC (`Role`/`RoleBinding`, read+logs only, no
write, no `pods/exec`). Added Pod Security Admission at `restricted`. Extended
`ResourceQuota` (capped load balancers, NodePorts, PVCs, secrets). Added `LimitRange`
max/min so one pod can't eat the whole quota. Documented namespaces as a *cooperative*
boundary, not a security one.

**Phase 2 — Structural cleanup.** Reshaped `4-platform-engineering/` into
clusters/addons/apis/modules. Made Argo Rollouts real (a `Rollout` template, gated by a
values flag) instead of an installed-but-unused controller. Demonstrated the promotion gate
with a real `prod/values.yaml`. Removed an over-broad "multi-cloud" roadmap claim.

**Phase 3 — Close the infrastructure loop.** Added a `provisioner:` field to
`catalog.yaml` (terraform vs. ack). Installed ACK controllers for S3 and IAM. Added ACK
capability templates. **3.7** gave every addon file an explicit namespace and sync-wave.
**3.8** renamed `4-platform-engineering/` into `1-cloud-foundation` / `2-cluster-services`
/ `3-capability-modules` / `4-platform-apis`. **3.9** renamed `1-platform-catalog/`
(`blueprints`→`per-team`, `building-blocks`→`per-service`) and wired real
provisioner-based dispatch in both scaffolder engines — this closed a live bug where the
ACK path was declared but never rendered.

**Phase 4 — Operate observability.** SLOs with error budgets for the sample service.
Multi-window, multi-burn-rate `PrometheusRule` (not a static threshold alert). Alertmanager
routing with page/ticket severities and an inhibition rule. Runbooks per alert. A worked
postmortem on the Phase 0 NetworkPolicy bug.

**Phase 5 — Hub/spoke AWS foundation.** Real Terraform for a VPC, an EKS cluster (private
endpoint, IMDSv2, control-plane logging), AWS Organizations (OUs + SCPs), and cross-account
IAM so ACK controllers in a hub account can provision into a tenant's spoke account. Not
applied to a real account — held to a clean `terraform plan`.

**Phase 6 — DORA metrics.** A dashboard with real queries for deployment frequency and
change failure rate against ArgoCD's own sync metrics. Lead time and MTTR are marked as
open gaps (no data source exists yet) rather than faked.

**Phase 7 — Identity.** Keycloak as the IdP, seeded declaratively. EKS Access Entries for
real `kubectl` auth. A Pod Identity / IRSA seam for workload-to-AWS credentials. ArgoCD's
own login and sync permissions wired to the same groups. Break-glass access as a separate,
audited role. External Secrets Operator alongside Sealed Secrets. Full write-up:
[`docs/identity-and-sso.md`](docs/identity-and-sso.md).

**Phase 8 — Backstage.** Deliberately not a running instance — a Software Template and a
config fragment showing how Backstage would call the existing CLI rather than reimplement
it. Standing up a real instance is a separate, larger piece of work.

**Phase 9 — Composition engine.** Restored an earlier Crossplane experiment from git
history and modernized it to Crossplane v2 (namespaced resources, no claims). Demonstrates
composing multiple AWS resources from one custom object, which ACK alone can't do.

**Phase 10 — Cloud portability seam.** Added `azure/postgres` as a second-provider module
with the exact same input/output contract as `aws/postgres`, proving that switching
providers is a one-line catalog change. `catalog.yaml` still points at AWS — this proves
the seam works, it doesn't move the platform off AWS.

---

# Next — specified, not yet executed

Everything above shipped. Everything below is a spec written to be executed by a fresh
agent. It is more verbose than the changelog above on purpose: a one-line summary is
enough to *record* a decision, and not enough to *perform* one.

---

## Phase 11 — Collapse the tenant split from three repos to two

> **Scope note.** This phase edits `2-idp-scaffolder/golang/`. That tree is normally
> off-limits; it is authorised for *this change only* — two one-line edits in the
> destination wiring. Do not refactor anything else while you are in there.

**The decision.** Each of `team-a/{apps,infra,gitops}` is currently a would-be repo root,
so a team costs three repos — sixty at twenty teams. Collapse to **two**: `apps` + `infra`
become one repo, `gitops` stays its own. Full rationale, the trigger for ever going back
to three, and the extraction commands are **Decision 15 in `.agents/AGENTS.md`** — read it
before touching anything. In one line: *split on who or what writes a directory, not on
what technology is in it.*

**The directory tree does not change.** `3-tenant-workloads/team-a/` keeps all three
directories. What moves is `CODEOWNERS`, because GitHub honours it only at a repository
root — and the merged repo's root maps to `{team}/`, not `{team}/apps/`.

### Steps

1. **Create `1-platform-catalog/per-team/root/CODEOWNERS.tmpl`** covering both halves of
   the merged repo. Merge the two existing templates; last matching pattern wins, so the
   general rule goes first:

   ```
   # CODEOWNERS for <org>/[[ .TeamName ]]
   #
   # Root of the merged application + infrastructure repository. In this monorepo that
   # root is 3-tenant-workloads/[[ .TeamName ]]/; it becomes the real repository root
   # when the tree is extracted (AGENTS.md Decision 15 has the filter-repo command).
   #
   # apps/ and infra/ live together because a service and the infrastructure it claims
   # are one unit of change: "this service now needs a bucket" is one PR, one review,
   # one revert. Ownership is separated by path, not by repository.

   *                  @acme-corp/[[ .TeamName ]]

   # Provider config, backend state wiring and the team IAM role decide what the team's
   # Terraform is *able* to do, so they stay platform-owned.
   /infra/platform/   @acme-corp/platform-engineering
   ```

2. **`catalog.yaml`** — add the new key, delete the now-empty one:
   ```yaml
   per-team/root:   "{team}/"        # merged apps+infra repo root (CODEOWNERS)
   per-team/infra:  "{team}/infra/"
   per-team/gitops: "{team}/gitops/"
   ```
   `per-team/apps` goes away entirely — `CODEOWNERS.tmpl` was its only file and it has
   moved up a level.

3. **`golang/internal/catalog/catalog.go`** — in `requiredDestinations`, replace
   `"per-team/apps"` with `"per-team/root"`.

4. **`golang/internal/templater/render.go`** — in `RenderTenantFoundation`'s
   `teamBlueprints`, replace the `per-team/apps` entry with:
   ```go
   {src: "per-team/root", destKey: "per-team/root"}, // CODEOWNERS for the merged apps+infra repo
   ```

5. **`python/cli.py` — fix a real engine drift while you are here.**
   `onboard_team_workload` loops `for blueprint_kind in ["apps","infra","gitops"]` and
   builds `dst_dir = TENANT_WORKLOADS_DIR / team_name / blueprint_kind`. It **never reads
   the `destinations:` map**, while the Go engine resolves every destination through it.
   Both produce identical paths today by coincidence, which is why the two-engine
   acceptance test has never caught it — and it is precisely the drift that test exists
   to catch. This phase breaks the coincidence, because `per-team/root` maps to `{team}/`
   and no longer matches `team_name / blueprint_kind`.

   Do not add a special case. Reshape the loop to resolve through `destinations`, the way
   the service path already does:
   ```python
   for dest_key in ["per-team/root", "per-team/infra", "per-team/gitops"]:
       src_dir = catalog_dir / dest_key
       dst_dir = render.TENANT_WORKLOADS_DIR / cat.destinations[dest_key].format(team=team_name)
   ```
   Confirm the exact accessor against `catalog.py`'s pydantic model before writing it.

6. **Delete** `per-team/apps/CODEOWNERS.tmpl` and `per-team/infra/CODEOWNERS.tmpl`, then
   `rmdir 1-platform-catalog/per-team/apps`.

7. **Re-render `team-a`.** Delete the two stale rendered files
   (`3-tenant-workloads/team-a/apps/CODEOWNERS`, `.../infra/CODEOWNERS`); a new
   `3-tenant-workloads/team-a/CODEOWNERS` should appear. `team-a/gitops/CODEOWNERS` is
   untouched.

8. **Update the header comment in `per-team/gitops/CODEOWNERS.tmpl`** — it describes a
   three-repo world. It should say this is the *second of two* repos and why it is
   separate: ArgoCD reads it and CI writes to it, so it is the one repo where automation
   holds credentials.

9. **Docs:** the directory tree and the Render Map in `.agents/AGENTS.md`, plus any README
   passage describing three tenant repos.

### Verify

```bash
# 1. Both engines still agree — the invariant step 5 protects.
#    Follow AGENTS.md § "The two-engine acceptance test" verbatim.

# 2. CODEOWNERS lands at the merged repo root, not inside apps/.
test -f 3-tenant-workloads/team-a/CODEOWNERS
test ! -f 3-tenant-workloads/team-a/apps/CODEOWNERS
test ! -f 3-tenant-workloads/team-a/infra/CODEOWNERS
test -f 3-tenant-workloads/team-a/gitops/CODEOWNERS

# 3. No destination key points at a directory that does not exist.
python3 -c "
import yaml, pathlib
c = yaml.safe_load(open('1-platform-catalog/catalog.yaml'))
missing = [k for k in c['destinations'] if not pathlib.Path('1-platform-catalog', k).is_dir()]
assert not missing, missing
print('destinations OK')"
```

**Do not run `git filter-repo`.** Extraction is a one-time future operation; the command
lives in AGENTS.md so the design is verifiable, not so it is performed now.

---

## Phase 12 — Enforce the tenancy boundary in CI, not in review

**The problem.** `team-a/gitops/CODEOWNERS` claims *"a team cannot widen its own
AppProject, relax its NetworkPolicy, or repoint its ApplicationSet."* In this monorepo
that claim enforces **nothing**: GitHub reads `CODEOWNERS` only from a repository root,
`.github/`, or `docs/`, and `3-tenant-workloads/team-a/gitops/` is none of those. The
control switches on only after the extraction in Phase 11 — which has not happened and
may never happen.

**Do not fix this with a root `.github/CODEOWNERS`.** Its owners (`@acme-corp/team-a`) do
not exist in this GitHub org, so GitHub flags them as unresolvable and the file enforces
nothing either. A broken control that *looks* like a control is worse than an honest gap.

**Enforce the invariant instead of the review.** A CI gate on the outcome needs no GitHub
teams, no branch protection and no repository split — and it keeps working unchanged after
Phase 11's extraction, because it runs against paths.

### (a) Run the policies you already have against the tenant tree

Kyverno is already installed with two `ClusterPolicy` resources in
`4-platform-engineering/2-cluster-services/security-governance/`
(`disallow-default-namespace.yaml`, `require-encrypted-buckets.yaml`). The same policies
run offline against files — this is the shift-left of a control you already own:

```bash
kyverno apply 4-platform-engineering/2-cluster-services/security-governance/ \
  --resource 3-tenant-workloads/ --policy-report
```

Point `--resource` at Kubernetes YAML **only**. The tenant tree also holds `.tf` files and
`values.yaml`, which are not Kubernetes resources; feeding them to `kyverno apply`
produces parse noise, and a noisy gate is an ignored gate. Glob
`3-tenant-workloads/*/gitops/` and exclude `values.yaml`.

### (b) Write the policies that actually express the tenancy boundary

(a) is the quick win, but neither existing policy asserts anything about tenancy. These
are new `ClusterPolicy` files and they are the real deliverable — each turns a sentence
from the README into a check:

| Invariant | Rule |
|---|---|
| A team cannot widen its AppProject | `AppProject.spec.destinations[].namespace` must equal the team's namespace; `sourceRepos` must stay under the team's own path |
| Default-deny stays default-deny | every team `NetworkPolicy` carries `policyTypes: [Ingress, Egress]` with an empty default rule |
| Pod Security Admission is not downgraded | `Namespace` must carry `pod-security.kubernetes.io/enforce: restricted` |
| ResourceQuota is not removed | a `ResourceQuota` must exist in each team namespace |
| An ApplicationSet is not repointed | `ApplicationSet.spec.generators[].git.directories` must stay under the team's own path |

Write them as `validate` rules with `failureAction: Enforce`, in
`2-cluster-services/security-governance/tenancy/`. The same files then serve both the
in-cluster admission path and the CI gate — **one definition, two enforcement points**,
which is the entire argument for policy-as-code and the thing worth being able to say out
loud in an interview.

### (c) Wire it as a workflow

New `.github/workflows/platform-policy-gate.yaml`, triggered on `pull_request` with
`paths: ['3-tenant-workloads/**']`. Install the Kyverno CLI, run (a) and (b), fail on any
policy failure. Keep it **separate** from `tenant-workloads-ci-cd.yaml`: that one runs on
`push` to `main` and writes rendered manifests back, whereas a gate has to run on the pull
request, before merge, or it is not a gate.

### (d) While you are in there — narrow the CI trigger

`tenant-workloads-ci-cd.yaml` triggers on `3-tenant-workloads/**`, so editing
`infra/apps/app-a/dev/postgres.tf` fires an image build and a manifest render. That is the
"a monorepo triggers everything" complaint that repository splits are usually reached for
— sitting unfixed in the repo used as evidence against needing the split. Narrow to:

```yaml
paths:
  - '3-tenant-workloads/*/apps/**'
  - '3-tenant-workloads/*/gitops/apps/**/values.yaml'
  - '1-platform-catalog/charts/service/**'
  - '.github/workflows/tenant-workloads-ci-cd.yaml'
```

### Verify

Break something deliberately and watch the gate catch it — **a gate never seen to fail is
not known to work**:

```bash
# Widen the AppProject, confirm a non-zero exit naming the offending resource, then revert.
kyverno apply 4-platform-engineering/2-cluster-services/security-governance/tenancy/ \
  --resource 3-tenant-workloads/team-a/gitops/platform/ --policy-report
```

Put both the passing and the failing transcript in the PR description. That is the
artefact worth showing: a green build demonstrates only that nothing was tested.
