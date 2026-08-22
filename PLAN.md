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

## Phase 11a — Prerequisite: put the scaffolder under CI

**Do this before Phase 11.** It is a single workflow file and it is the safety net that
Phases 11 and 13 both assume already exists.

**What is missing.** `.github/workflows/` holds exactly two workflows — `Platform Policy
Gate` and `Tenant Workloads - CI/CD Pipeline`. **Neither touches `2-idp-scaffolder/`.**
Concretely:

- `go test` never runs in CI, though `internal/catalog/catalog_test.go`,
  `internal/templater/resolve_test.go` and `render_test.go` exist and pass locally.
- `golang/.golangci.yml` is committed and **nothing executes it**.
- The Python engine has **zero tests** and declares no test dependency in
  `pyproject.toml`.
- The two-engine acceptance test is documented in `.agents/AGENTS.md` as a manual
  procedure and is never run automatically.

**Why it blocks Phase 11.** The repository's headline claim is *one catalog, two engines,
identical output* — and only one engine is tested at all. Worse, Phase 11 step 5
**deliberately breaks the coincidence** that currently makes the two engines agree: Python
hardcodes `TENANT_WORKLOADS_DIR / team_name / blueprint_kind` while Go resolves through
`destinations:`. If the Python rewrite is subtly wrong, the failure is silent divergence in
generated output — the single defect this repo is least able to detect today, handed to an
agent that will report success because every command it ran exited zero.

### The workflow

New `.github/workflows/scaffolder-ci.yaml`, on `pull_request` and `push` to `main`, with
`paths: ['2-idp-scaffolder/**', '1-platform-catalog/**']`. Four jobs:

1. **`go`** — `go vet ./...`, `go test ./... -race`, `golangci-lint run` (the config is
   already there).
2. **`python`** — `ruff check`, then `pytest`. Add `pytest` to `pyproject.toml` first.
3. **`acceptance`** — the job that actually matters. Render the same tenant with both
   engines into two temp directories and diff them:

   ```bash
   go run . onboard-team --catalog-root ../../1-platform-catalog --team-name ci-probe
   uv run python main.py onboard-team --team-name ci-probe
   diff -r /tmp/go-out/3-tenant-workloads/ci-probe /tmp/py-out/3-tenant-workloads/ci-probe
   ```

   Follow `AGENTS.md § "The two-engine acceptance test"` for the exact invocation — it
   already specifies the output roots. Do the same for `add-service` across at least one
   `terraform`-provisioned and one `ack`-provisioned capability, so the Phase 3.9 dispatch
   is covered on both paths.

4. **`catalog`** — the destinations-integrity check from Phase 11's Verify block, so a
   destination key pointing at a non-existent directory fails at PR time rather than
   mid-render.

### Minimum Python tests

Not parity with Go — just enough that `pytest` is not an empty run. Port the two checks
that already exist on the Go side and guard real invariants:

- a golden path naming an undeclared runtime is rejected at catalog load
- a runtime directory that exists but is **not** declared in `runtimes:` stays invisible
  (this is deliberate behaviour per AGENTS.md decision 13, and the kind of thing a future
  agent will "fix" unless a test says otherwise)

**Verify:** open a throwaway PR that changes one Python destination path and confirm the
`acceptance` job fails. A gate never seen to fail is not known to work — same rule as
Phase 12.

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

---

## Phase 13 — `team` → `tenant`, but only where the concept is isolation

> **Sequencing.** Land Phase 11 first — both phases edit the same `CODEOWNERS`
> templates and the same `requiredDestinations` list, and doing them in one pass avoids
> two rounds of identical edits. Land this before Phase 14, or the recording shows flags
> that no longer exist.

**Why.** The tree is already incoherent: `3-tenant-workloads/team-a/` says *tenant* at one
level and *team* at the next. And the current model quietly assumes one team == one
isolation boundary, which breaks the moment two squads in the same business unit share a
namespace, an AppProject and a quota.

**This supersedes Decision 9 in `.agents/AGENTS.md`**, which argued the opposite ("teams
are the customers, so use `team`"). That decision was half right — it correctly saw the
two words name different things, then resolved the tension by picking one word for both
jobs. Rewrite it; do not leave both in the file.

### The rule

`tenant` and `team` are **not synonyms**, and a global find-replace is the wrong change.

| Concept | Word | Why |
|---|---|---|
| The isolation unit — one Namespace, one AppProject, one ResourceQuota, one NetworkPolicy, one repo pair | **tenant** | This is what the platform isolates. It may contain more than one team. |
| The human group — a GitHub team in `CODEOWNERS`, an IdP group in an RBAC subject, `spec.owner` in `catalog-info.yaml` | **team** | `@acme-corp/team-a` *is* a GitHub team. Calling it a tenant would be the same error in the other direction. |

### The segregation axis is the tenant, not the platform instance

A platform instance — one hub cluster, one ArgoCD, one catalog version, e.g. `eu` and `us`,
or `staging` and `prod` — is a **deployment target, not an identity**. It must not become a
directory level.

**Why not.** A tenant almost always exists on more than one instance: `payments` runs on
both the EU and the US platform. Segregating by instance first produces `eu/payments/` and
`us/payments/`, which forks one tenant's identity across N trees — N copies of
`CODEOWNERS`, N answers to "who owns payments", and under Phase 11, **N repo pairs per
tenant instead of two**. And with a single instance, which is the case here and in most
organisations, the level carries exactly one value forever: pure cost, zero information.

This repo has already made this mistake once and reversed it. `.agents/AGENTS.md`
Decision 5 removed a `<system>/` directory level for the same reason — it was a second
encoding of a fact better held as metadata. Do not re-introduce the shape under a new name.

**Where instance actually belongs: in the reader, not the data.** Each platform instance
runs its own ArgoCD with its own `ApplicationSet` generator. That generator already decides
which paths the instance syncs, so instance selection is a property of *who is reading the
repository* — nothing in the tenant tree has to change to support a second instance. If a
tenant must be present on some instances and not others, that is one field consumed by the
generator, alongside the `owners:` list above:

```yaml
# 3-tenant-workloads/<tenant>/tenant.yaml
name: payments
owners:    [payments-core, payments-fraud]   # GitHub teams / IdP groups
instances: [eu, us]                          # which platform instances sync this tenant
```

Data, not directories — the same principle as `destinations:` and `provisioner:`.

**The one trigger that separates instances**, and it still is not a directory level: a data
residency rule under which an EU tenant's repository may not replicate to US
infrastructure. That forces a **separate repository**, not a nested folder. Record it as
the trigger and do not pre-pay for it.

**Note on the other reading of the question:** if "platform instance" means *which platform
deployment the CLI is talking to* — which catalog root, which git remote, which backend —
that is scaffolder configuration, not tree structure. It belongs in a config file or an
env var, and it still never appears in a path. Same answer either way.

So: the flag is `--tenant-name`. There is no `--instance-name`.

### Rename to `tenant`

- `3-tenant-workloads/<tenant>/` — the directory level
- `{team}` → `{tenant}` in `catalog.yaml`'s `destinations:` values
- `1-platform-catalog/per-team/` → `per-tenant/` (and the matching `destinations:` keys,
  `requiredDestinations` in `catalog.go`, the `teamBlueprints` table in `render.go`, and
  the loop in `python/cli.py`)
- `TeamName` → `TenantName` in every `.tmpl` and both engines' config structs
- `--team-name` → `--tenant-name` in both CLIs
- `gitops/platform/team/` → `gitops/platform/tenancy/` — six files (appproject, namespace,
  networkpolicy, rbac, policy-exceptions, secretstore) that collectively *are* the tenancy
  boundary; `team/` never named them accurately

### Keep as `team`

- Every `@acme-corp/<team>` owner in `CODEOWNERS` templates
- `RoleBinding.subjects` — the subject is a group, not a tenant
- `catalog-info.yaml`'s `spec.owner`
- Prose that genuinely means a group of humans

### The payoff — make the distinction do work

Once the words are separated, "two teams, one tenant" becomes expressible as data instead
of a redesign. Add a per-tenant declaration rather than deriving ownership from the
directory name:

```yaml
# 3-tenant-workloads/<tenant>/tenant.yaml
name: payments
owners:                      # GitHub teams / IdP groups — one tenant, N teams
  - payments-core
  - payments-fraud
```

Then `CODEOWNERS` renders one line per owner, and `RoleBinding.subjects` renders one
subject per owner. **Do this part only if step one lands cleanly** — it needs a new
scaffolder input, and a rename that half-applies across two engines is worse than no
rename. If you defer it, say so in the AGENTS.md decision so the gap is recorded rather
than implied.

### Backward compatibility

`--team-name` is the CLI's public interface and it appears in the README, both TODO files
and every example. Keep it as a deprecated alias for one release rather than breaking it
silently — Cobra's `MarkDeprecated` prints a warning and still works; do the equivalent in
Typer. Remove it at the next tag.

### Verify

```bash
# No stale vocabulary anywhere in the tree.
grep -rn "TeamName\|per-team\|{team}\|--team-name" \
  1-platform-catalog 2-idp-scaffolder 3-tenant-workloads .agents README.md

# `team` should now survive ONLY in CODEOWNERS owners, RBAC subjects and spec.owner.
grep -rn "\bteam\b" 3-tenant-workloads | grep -viE "CODEOWNERS|subjects|owner"

# Both engines still agree.
# Follow AGENTS.md § "The two-engine acceptance test" verbatim.
```

Tag this one. It changes the CLI's public interface and every rendered path, so old
Terraform `?ref=` pins must keep resolving against the previous tag.

---

## Phase 14 — Prove it runs

**The problem.** Phases 5, 7, 8, 9 and 10 are all explicitly "held to a clean
`terraform plan`", "not a running instance", or "documented, not built". That honesty is a
strength and should stay. But the repo now contains no evidence — no recording, no
screenshot, no transcript — that any of it has ever executed. The ratio of *claimed* to
*demonstrated* is the weakest thing about the repository as an artefact.

Everything below runs on k3d and costs nothing.

**(a) One end-to-end recording, ~90 seconds.** `make setup` → `onboard-team` →
`add-service` → ArgoCD syncs → the app answers an HTTP request. Record with `asciinema`
(text, greppable, small) and embed near the top of the README. A reader watches this;
they do not read 400 lines of `AGENTS.md`.

**(b) Fire a Phase 4 alert on purpose.** Same principle as Phase 12's gate: **an alert
never seen to fire is not known to work.** Drive errors into the sample service until the
multi-window burn-rate rule trips, and capture Prometheus showing the rule as `firing`.

**(c) Close the loop that already exists.** `docs/runbooks/app-a-availability-burn.md` and
`app-a-latency-burn.md` are written for exactly the alerts in (b). Follow one as written,
end to end, and note anything the runbook got wrong — a runbook that has never been walked
is a guess. Alert → runbook → resolution is a complete Incident Response story, and
Incident Response is the highest-demand gap this plan ever identified.

**(d) Commit the artefacts** to `docs/demo/` and link them from the README's Quickstart.

---

## Phase 15 — Promote the decisions into ADRs

`docs/adr/` exists and contains exactly one file. Meanwhile `.agents/AGENTS.md` holds
fifteen numbered architectural decisions with their full reasoning — findable only by an
agent reading a 400-line instruction file.

Extract the ones that carry a genuine "why not the obvious alternative", one file each,
keeping context → alternatives → decision → **revisit trigger**:

| ADR | Source | The question it answers |
|---|---|---|
| Two repos per tenant, not three | Decision 15 | why `apps`+`infra` merge and `gitops` does not |
| One platform-owned chart, rendered manifests | Decision 6 | why CI commits YAML instead of ArgoCD running Helm |
| System is metadata, not a directory | Decision 5 | why a reversal is recorded rather than erased |
| Dev-only scaffolding, gated promotion | Decision 3 | why `prod/` never appears automatically |
| `provisioner:` — Terraform vs ACK | `catalog.yaml` | why one platform uses both, chosen by data |
| k3d is a harness, not the target | Target Environment | why local convenience never picks the production mechanism |
| `tenant` vs `team` | Phase 13 | why they are not synonyms |

Leave the full text in `AGENTS.md` or link out to the ADRs from it — but do not maintain
both copies.

**Why this is worth a phase:** an ADR's structure *is* the structure of a good verbal
answer under pressure. The bottleneck at this point is not the repository.

---

## Phase 16 — Hygiene backlog

Small, independent, no ordering constraints.

- **Move `aws-creds.ini` out of the repo root** to `~/.aws/`. It is untracked, covered by
  `*.ini` in `.gitignore`, and `git log --all -- aws-creds.ini` confirms it was never
  committed — so this is not an incident. It is one `git add -f`, one `.gitignore` edit or
  one `zip -r` of the directory away from becoming one. Zero cost to remove the exposure.
- **Surface `docs/incidents/2026-08-20-traefik-networkpolicy-ingress-blocked.md`** from the
  README. A real postmortem on a bug found and fixed in your own system is rare in a
  reference repo, and it is currently buried two directories deep.
- **`.agents/AGENTS.md`'s decision list skips number 12.** Renumber or insert a stub.
- **`4-platform-engineering/2-cluster-services/otel/`** — check whether this still reads
  as a separate concern from `observability/` after Phase 3.7, or whether the two should
  merge. Two directories for one signal path is the same naming failure Phase 3.8 fixed
  one level up.
