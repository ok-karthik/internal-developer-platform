# PLAN.md — Phase History

What each phase of this platform's build-out actually did. Kept short on purpose — this is
a changelog, not the design doc. The original 1,780-line planning document (with the full
reasoning behind each decision) is retrievable from git history: `git log --all -- PLAN.md`.

Phases 0 through 10 were tagged **`v2.0.0`**; Phases 11 through 13 were tagged **`v2.2.0`**
(the tag Terraform module sources in `catalog.yaml` used to resolve against; since Phase 19.3 they resolve
against module-scoped tags in `enterprise-aws-infrastructure`, e.g. `?ref=postgres-v2.0.0`);
Phases 15 through 17 completed ADR extraction, repo hygiene, and Day-2 operational controls.

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
applied to a real account — held to a clean `terraform plan`. *(This work now lives in
`enterprise-aws-infrastructure`; see Phase 19.5.)*

**Phase 6 — DORA metrics.** A dashboard with real queries for deployment frequency and
change failure rate against ArgoCD's own sync metrics. Lead time and MTTR are marked as
open gaps (no data source exists yet) rather than faked.

**Phase 7 — Identity.** Keycloak as the IdP, seeded declaratively. EKS Access Entries for
real `kubectl` auth. A Pod Identity / IRSA seam for workload-to-AWS credentials. ArgoCD's
own login and sync permissions wired to the same groups. Break-glass access as a separate,
audited role. External Secrets Operator alongside Sealed Secrets. Full write-up:
[`docs/identity-and-sso.md`](docs/identity-and-sso.md). *(The Access Entries and Pod Identity / IRSA
Terraform, 7.2b and 7.2c, now live in `enterprise-aws-infrastructure`; see Phase 19.5.)*

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

**Phase 11a — Scaffolder CI & Parity Gate.** Added `.github/workflows/ci.yaml` running Go
tests with race detection, Python catalog validation, automated byte-for-byte engine parity
checks (`diff -r`) between Go and Python scaffolders across capabilities, and offline
Kyverno tenancy validation.

**Phase 11 — Two-Repo Collapse.** Collapsed tenant repository model from three repos to two
(`apps` + `infra` merged, `gitops` separate) based on writer ownership. Created
`per-tenant/root/CODEOWNERS.tmpl`, updated `catalog.yaml` destinations, and moved root
ownership to `{tenant}/CODEOWNERS`.

**Phase 12 — Tenancy Boundary CI Enforcement.** Shifted tenancy governance left into CI
(`validate-tenancy` job). Added 5 Kyverno `ClusterPolicy` rules in
`2-cluster-services/security-governance/tenancy/` enforcing AppProject bounds, default-deny
NetworkPolicies, Pod Security `restricted`, ResourceQuota presence, and ApplicationSet paths.
Narrowed workload CI triggers.

**Phase 13 — `team` → `tenant` Boundary Alignment.** Separated the isolation boundary
(`tenant`) from human groups (`team`). Renamed catalog keys to `per-tenant/`, updated
destinations to `{tenant}/`, renamed CLI flags to `--tenant-name`, moved tenancy platform
manifests to `gitops/platform/tenancy/`, and cut tag `v2.2.0`.

**Phase 15 — Architectural Decision Records (ADRs).** Extracted core platform decisions
into formal ADR documents in `docs/adr/` (`0001` through `0009`), codifying rationale,
alternatives, and revisit triggers for tool choices, golden paths, dev-only scaffolding,
GitOps boundaries, single platform chart, rendered manifests, and Karpenter node provisioning.

**Phase 16 — Operational Hygiene.** Cleaned untracked local credentials (`aws-creds.ini`),
surfaced the Traefik/NetworkPolicy postmortem in `README.md`, restored Decision 12 in
`.agents/AGENTS.md`, and merged OpenTelemetry manifests into `observability/`.

**Phase 17 — Day-2 Platform Operations.** Added S3 remote state bootstrap with native
`use_lockfile = true` for platform Terraform (17.1). Made ArgoCD self-managed via GitOps
`Application` and shared `values.yaml` (17.2). Closed the supply-chain security loop with
Trivy scans, Syft SBOM generation, Cosign keyless image signing, and Kyverno image
verification admission policy (17.3). Added `PodDisruptionBudget` (`maxUnavailable: 1`,
`replicaCount > 1`) and HPA with `metrics-server` to the platform Helm chart (17.4, 17.5).
Decided on fixed node groups with Karpenter migration criteria in ADR 0009 (17.6). Added
`external-dns` with Pod Identity seam (17.7). Documented disaster recovery RTO/RPO and
backup of unrecoverable Sealed Secrets keys in `docs/disaster-recovery.md` (17.8). Documented
cluster upgrade runbook and dual secrets strategy (17.9).

---

# Next — specified, not yet executed

Everything above shipped. Everything below is a spec written to be executed by a fresh
agent. It is more verbose than the changelog above on purpose: a one-line summary is
enough to *record* a decision, and not enough to *perform* one.

---

## Phase 14 — Prove it runs

**The problem.** Phases 5, 7, 8, 9 and 10 are all explicitly "held to a clean
`terraform plan`", "not a running instance", or "documented, not built". That honesty is a
strength and should stay. But the repo now contains no evidence — no recording, no
screenshot, no transcript — that any of it has ever executed. The ratio of *claimed* to
*demonstrated* is the weakest thing about the repository as an artefact.

Everything below runs on k3d and costs nothing.

**(a) One end-to-end recording, ~90 seconds.** `make setup` → `onboard-tenant` →
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

## Phase 18 — Enterprise Topology, Contract Seams & Multi-Cluster Fleet Orchestration

Defines the contract boundaries between the Cloud Foundation (`enterprise-aws-infrastructure`)
and this Developer Platform (`internal-developer-platform`), and formalizes how tenant workloads
dynamically discover infrastructure and fan out across multi-cluster fleets.

### 18.1 — The Discovery Contract: How Tenants Discover VPC & Cluster Context

Tenant capability modules in `3-tenant-repos/<tenant>/workloads-repo/infra/services/<app>/<env>/` must never
hardcode AWS IDs (VPC IDs, Subnet IDs, OIDC ARNs, KMS keys).

#### The 3 Contract Seams:
1. **Pattern A: AWS SSM Parameter Store Service Catalog (Recommended for Terraform claims)**
   - The Cloud Foundation repo publishes standard SSM parameters on every stack apply:
     - `/platform/${env}/${region}/vpc/id`
     - `/platform/${env}/${region}/vpc/database_subnets`
     - `/platform/${env}/${region}/eks/cluster_name`
     - `/platform/${env}/${region}/eks/oidc_provider_arn`
   - Tenant Terraform modules ingest parameters at plan time via `data "aws_ssm_parameter"`.
   - **Why this is the enterprise standard:** Decouples state backends entirely. Zero cross-repo state locks.
2. **Pattern B: Tag-Based Dynamic Queries (`data "aws_vpc"` / `data "aws_subnets"`)**
   - Tenant Terraform queries AWS APIs using standardized tags (`Platform:Environment = var.env`).
   - Fallback pattern when SSM parameters are not provisioned.
3. **Pattern C: Kubernetes CRD / Cloud Control Plane (ACK / Crossplane)**
   - Tenant developers declare resources as Kubernetes Custom Resources (CRDs). The in-cluster
     controller (pre-configured with VPC and Pod Identity) handles AWS provisioning directly.

### 18.2 — Multi-Cluster Fleet Orchestration & Label Contracts

How ArgoCD dynamically routes tenant applications across multiple EKS clusters (multi-region,
multi-account, PCI-DSS compliance isolation) without the scaffolder needing cluster endpoints.

```text
[ Cluster Provisioning in Repo 1 ]
       │  Registers Cluster as Secret in ArgoCD with Labels:
       ▼  (environment: prod, region: eu-central-1, tier: pci-compliant)
[ ArgoCD Cluster Secret Registry ]
       ▲
       │  ApplicationSet Generator matches label intent:
       │  matchLabels: { environment: prod }
[ Scaffolder Renders ApplicationSet in Repo 2 ]
```

1. **Cluster Registration (Platform SRE / Day-0):**
   - EKS clusters are registered in ArgoCD as Kubernetes Secrets with `argocd.argoproj.io/secret-type: cluster`
   - Standard labels: `environment` (`dev` | `prod` | `staging`), `region` (`eu-central-1` | `us-east-1`), `tier` (`general` | `pci-compliant`).
2. **Scaffolder Intent Rendering:**
   - The Scaffolder CLI/API reads `--env` from `catalog.yaml` and renders an `ApplicationSet` with a cluster generator.
   - The scaffolder does not hold cluster IP/DNS state; ArgoCD dynamically binds the application to all matching clusters.

### 18.3 — Holistic FinOps & Cost Attribution Seam

Establishes the three-tier FinOps model connecting foundational platform costs with tenant claims:
- **Baseline Foundation:** EKS control plane, NAT Gateways, Transit Gateway attachments (Platform Cost Center).
- **Direct AWS Resources:** Tenant RDS, S3, DynamoDB tagged with mandatory `Tenant`, `Service`, `CostCenter` via OPA gates.
- **In-Cluster Compute:** Kubecost / OpenCost namespace attribution for shared pod CPU/Memory and egress.

### 18.4 — Karpenter Autoscaling & Spot Interruption Architecture

Decouples Karpenter into two clean halves across the platform boundary: AWS prerequisites live in
`enterprise-aws-infrastructure`, while the runtime controller and node CRDs live in this repo
under `4-platform-engineering/2-cluster-services/karpenter/` managed by ArgoCD.

```text
========================================================================================================
                                 KARPENTER CROSS-REPO ARCHITECTURE SEAM
========================================================================================================

  [ CLOUD FOUNDATION: enterprise-aws-infrastructure ]
  ├── 1. Controller IAM Role (EKS Pod Identity association)
  ├── 2. Node IAM Role & EKS Access Entry (AmazonEKSWorkerNodePolicy, ContainerRegistryReadOnly)
  ├── 3. SQS Queue: karpenter-interruption-queue (20s retention, server-side encrypted)
  ├── 4. EventBridge Rules:
  │      ├── aws.ec2 Spot Instance Interruption Warning -> SQS
  │      ├── aws.ec2 EC2 Instance Rebalance Recommendation -> SQS
  │      ├── aws.ec2 EC2 Instance State-change Notification -> SQS
  │      └── aws.health AWS Health Event (Scheduled Maintenance) -> SQS
  └── 5. Discovery Tags on VPC subnets and security groups: karpenter.sh/discovery = <cluster_name>

                                        │
                         ArgoCD Reconciles in EKS
                                        ▼

  [ DEVELOPER PLATFORM: internal-developer-platform (Cluster Services) ]
  └── 4-platform-engineering/2-cluster-services/karpenter/
        ├── Application: Karpenter Helm Chart (v1.x, pinned, wait=true)
        ├── EC2NodeClass:
        │     ├── subnetSelectorTerms: { tags: { "karpenter.sh/discovery": "{{cluster_name}}" } }
        │     ├── securityGroupSelectorTerms: { tags: { "karpenter.sh/discovery": "{{cluster_name}}" } }
        │     └── amiFamily: AL2023
        └── NodePool (default):
              ├── capacityType: [spot, on-demand] # Spot primary, on-demand fallback
              ├── instanceCategory: [c, m, r]     # Diverse instance families
              ├── disruption:
              │     ├── consolidationPolicy: WhenEmptyOrUnderutilized
              │     └── consolidateAfter: 30s
              └── limits: { cpu: 1000, memory: 1000Gi }
========================================================================================================
```

#### Why this split is the enterprise standard:
1. **Zero Terraform-to-K8s API coupling:** Terraform never attempts to call Kubernetes APIs or install Helm
   releases during `plan`, keeping offline validation (`smoke-test.sh`, `-backend=false`) 100% reliable.
2. **True GitOps lifecycle:** Upgrading Karpenter versions and tuning NodePool disruption budgets are PRs
   against this repository that ArgoCD synchronizes declaratively with zero Terraform state churn.

### 18.5 — IAM Boundary Map: Workload, Human SSO, and CI/CD OIDC

To prevent privilege escalation and maintain clear governance, IAM ownership is strictly partitioned:

| Layer / Persona | Identity Mechanism | Owning Repository | Purpose |
|---|---|---|---|
| **EKS Cluster Agent** | `aws_eks_addon` (pod-identity-agent) | `enterprise-aws-infrastructure` (`compute/eks`) | Intercepts in-cluster AWS STS credential requests |
| **Service Workload** | `aws_eks_pod_identity_association` | `internal-developer-platform` (`3-tenant-repos/.../iam.tf`) | Binds microservice Pod to specific IAM permissions |
| **Karpenter Node & Controller** | Pod Identity + Node Instance Role + Access Entry | `enterprise-aws-infrastructure` (`compute/eks`) | Dynamic node provisioning and spot interruption handling |
| **Human Engineers** | AWS IAM Identity Center (SSO Permission Sets) | `enterprise-aws-infrastructure` (`identity/human-access`) | PlatformAdmin, Developer, and Auditor access |
| **CI/CD Automation** | GitHub OIDC Provider (`token.actions.githubusercontent.com`) | `enterprise-aws-infrastructure` (`foundation-live-repo/_bootstrap`) | Zero-key secretless deployment pipeline auth |

---

## Phase 19 — Tenant Repo Topology, Naming Sweep & Repo Boundaries

> **Status (2026-09-21).** Done and verified locally: 19.1, 19.2, 19.3, 19.4, 19.5 (in this repo),
> 19.6, 19.7 gates 1-5, and 19.8 (module pins `*-v2.0.0` at `iac-modules-repo/`, checked with a real
> `terraform init` + `validate`). **Not done:** 19.7 gate 6 (`make setup` on k3d) — the ApplicationSets
> read `github.com/ok-karthik/internal-developer-platform` at `HEAD`, so the new
> `3-tenant-repos/*/gitops-repo/...` paths only resolve after this branch is pushed. Also not done:
> a real `renovate` dry run (the regexes were checked by hand against the files).
> Changes beyond the spec: the Python engine now reads `destinations:` instead of hardcoding
> output paths; `restrict-applicationset` gained a `deny-path-traversal` rule (Kyverno's `*`
> matches across `/`, so `..` passed the tightened pattern); `postgres.tf.tmpl` reads the region
> from `data "aws_region"` rather than a `providers.tf` variable, because `providers.tf` lives in
> a different root module from the per-service directory.

The decisions are settled and written up in
[ADR 0010](docs/adr/0010-tenant-repository-topology.md) (two repos per tenant, split by who
writes the file), [ADR 0011](docs/adr/0011-capability-modules-external-repo.md) (capability
modules move to `enterprise-aws-infrastructure`) and
[ADR 0012](docs/adr/0012-platform-repo-owns-no-cloud-terraform.md) (this repo owns no cloud
Terraform at all). Read all three before starting — this section is the *execution*, not
the reasoning.

**Target layout.** Every tenant becomes two directories, one per real repo, and `apps/`
becomes `services/` everywhere so that `platform/` vs `services/` means the same thing
(platform-owned vs team-owned) in every tree:

```
3-tenant-repos/<tenant>/
├── workloads-repo/              ← humans write it → <org>/<tenant>-workloads
│   ├── CODEOWNERS
│   ├── tenant.yaml
│   ├── services/<app>/
│   └── infra/{platform,services/<app>/<env>}/
└── gitops-repo/                 ← CI writes it, ArgoCD reads it → <org>/<tenant>-gitops
    ├── CODEOWNERS
    ├── platform/{tenancy,applicationsets}/
    └── services/<app>/<env>/{values.yaml,manifests/}
```

**The `-repo` suffix means exactly one thing:** this directory is a separate repository in
production and only looks like a folder because this repo teaches the whole platform from
one checkout. Directories without the suffix are plain directories inside whatever repo
holds them — `1-platform-catalog/` needs no marker, because this repository is itself real.
The tenant directory already carries the tenant name, so `workloads-repo/` is enough to
mean `<org>/<tenant>-workloads`.

**One thing to notice before editing globs:** the gitops tree keeps its *depth*.
`<tenant>/gitops-repo/` sits where `<tenant>/gitops/` always did, so the path-segment
indexes in both ApplicationSet templates and the `awk` field numbers in CI still work
untouched — only the directory *name* changed. Everything below that shifts because of
`apps` → `services` and the new `workloads-repo/` level on the source side.

**19.4 is a live bug, not a cleanup.** ACK capability claims (`s3`, `iam`) are scaffolded
into the gitops tree but never reach the cluster, because ArgoCD only syncs `manifests/`
and CI never writes them there. It is invisible today only because the fixture requests
`postgres` alone. Fix it in this phase even if everything else slips.

**Do 19.1 and 19.2 in one pass.** They touch the same files; doing them separately means
editing the same globs, `awk` field positions and docs twice.

**Directory names inside `1-platform-catalog/` do not change.** The `destinations:` keys are
literal *source* paths (`per-service/apps/runtimes`), so they keep saying `apps`. Only
output paths move. This keeps `REQUIRED_DESTINATIONS` in both engines untouched.

---

### 19.1 — Two-repo layout and the `apps` → `services` rename

**(a) Rename the container directory first — `3-tenant-workloads/` → `3-tenant-repos/`.**
Everything under it is a repository, not a folder, and the old name hid that at the very
level where the confusion starts. Do this before anything else, so every later step edits
the final path once:

```bash
rmdir 3-tenant-repos/tenant-a 2>/dev/null; rmdir 3-tenant-workloads/tenant-a
git add -A 3-tenant-workloads/          # stage the already-deleted tenant-a files
git mv 3-tenant-workloads 3-tenant-repos
git mv .github/workflows/tenant-workloads-ci-cd.yaml \
       .github/workflows/tenant-repos-ci-cd.yaml
```

Then fix every consumer of the old name:

- `2-idp-scaffolder/python/render.py:18` — `TENANT_WORKLOADS_DIR` constant, both the name
  and the `"3-tenant-workloads"` path it builds (used again at lines 151, 158, 160). The Go
  engine takes `--output-root` and hardcodes nothing, so it needs no change.
- `Makefile` — `--output-root "$(REPO_ROOT)/3-tenant-workloads"` in both demo targets.
- `.github/workflows/ci.yaml:10` path filter, `:78` the Python-output `mv`, `:97` the find.
- `.github/workflows/tenant-repos-ci-cd.yaml` — path filters and the `git add` at line 184.
- The two ArgoCD globs and the Kyverno pattern — covered by steps (d) and (e) below, which
  already show the final paths.
- `README.md`, `.agents/AGENTS.md`, `docs/gitops-delivery.md` — covered by the sweep in 19.6.

**(b) `1-platform-catalog/catalog.yaml` — `destinations:` is the only place output paths
live.** Replace the block with:

```yaml
destinations:
  per-tenant/root:   "{tenant}/workloads-repo/"
  per-tenant/infra:  "{tenant}/workloads-repo/infra/"
  per-tenant/gitops: "{tenant}/gitops-repo/"

  per-service/apps/runtimes:      "{tenant}/workloads-repo/services/{app}/"
  per-service/apps/service-meta:  "{tenant}/workloads-repo/services/{app}/"
  per-service/gitops/release:     "{tenant}/gitops-repo/services/{app}/{env}/"

  per-service/infra/capabilities:  "{tenant}/workloads-repo/infra/services/{app}/{env}/"
  per-service/gitops/capabilities: "{tenant}/gitops-repo/services/{app}/{env}/"
```

**(c) Rename `gitops/platform/team/` → `gitops/platform/tenancy/` in the output.** The
catalog already uses `tenancy/`; only the stale committed fixture says `team/`. Step 19.2
regenerates it, so no manual move is needed.

**(d) ArgoCD discovery — both globs, directory name only.**

`4-platform-engineering/2-cluster-services/gitops-orchestration/applicationset-tenant-apps.yaml`:

```yaml
- path: 3-tenant-repos/*/gitops-repo/platform/applicationsets
```

`1-platform-catalog/per-tenant/gitops/platform/applicationsets/[[ .TenantName ]].yaml.tmpl`:

```yaml
- path: 3-tenant-repos/[[ .TenantName ]]/gitops-repo/services/*/*
```

Both templates carry a comment counting path segments. `{{path[1]}}` is still the tenant
and `index .path.segments 4` is still the app. **Update the comments anyway** — they spell
out `apps` — or the next reader will count them wrong.

**(e) `4-platform-engineering/2-cluster-services/security-governance/tenancy/restrict-applicationset.yaml`** —
tighten the pattern so it also pins the gitops repo:

```yaml
- path: "3-tenant-repos/{{request.object.metadata.name}}/gitops-repo/*"
```

**(f) `.github/workflows/tenant-repos-ci-cd.yaml`** (renamed in step (a)) — four changes:

| Line | Old | New |
|---|---|---|
| 25 | `3-tenant-workloads/*/apps/**` | `3-tenant-repos/*/workloads-repo/services/**` |
| 26 | `3-tenant-workloads/*/gitops/apps/**/values.yaml` | `3-tenant-repos/*/gitops-repo/services/**/values.yaml` |
| 92, 143 | `find 3-tenant-workloads/*/gitops/apps/*/*/values.yaml` | `find 3-tenant-repos/*/gitops-repo/services/*/*/values.yaml` |
| 101 | `SRC_DIR="3-tenant-workloads/$TEAM/apps/$APP"` | `SRC_DIR="3-tenant-repos/$TEAM/workloads-repo/services/$APP"` |

The `awk -F'/'` field numbers (`$2` tenant, `$5` app, `$6` env) are **unchanged** — path
depth is the same. Leave them alone and update the comment at line 152.

**(g) `.github/workflows/ci.yaml` line 97** — `find 3-tenant-workloads/*/gitops` becomes
`find 3-tenant-repos/*/gitops-repo`.

**(h) `Makefile`** — the demo targets still call the deprecated verb and flag. Fix while
you are here:

```make
DEMO_TENANT ?= tenant-a
DEMO_OWNER  ?= team-a
DEMO_APP    ?= app-a

demo-onboard-tenant:      # was demo-onboard-team
	... onboard-tenant --tenant-name "$(DEMO_TENANT)" --owner "$(DEMO_OWNER)" ...

demo-add-service:
	... add-service --tenant-name "$(DEMO_TENANT)" --app-name "$(DEMO_APP)" \
	    --golden-path "$(DEMO_PATH)" --capabilities postgres,s3
```

`--capabilities postgres,s3` on purpose: it exercises both provisioner paths and is what
surfaces the bug in 19.4.

**(i) CODEOWNERS templates** — `per-tenant/root/CODEOWNERS.tmpl` and
`per-tenant/gitops/CODEOWNERS.tmpl` both reference "AGENTS.md Decision 15" (there is no
Decision 15) and describe the monorepo root path. Point them at ADR 0010, update the paths
to `3-tenant-repos/<tenant>/workloads-repo/` and `…/gitops-repo/`, and keep the header
line naming the repo each one becomes (`<org>/[[ .TenantName ]]-workloads`,
`<org>/[[ .TenantName ]]-gitops`). Add the extraction command from ADR 0010 to that header
too, so each simulated repo root explains itself without anyone opening a doc.

**(j) New `3-tenant-repos/README.md`** — one table answering "what here is a real repo":
simulated path, real repo name, who writes it, what reads it, and the `-repo` suffix rule.
This is the page a reviewer lands on when they click into the directory.

---

### 19.2 — Fixture: `tenant-a` owned by `team-a`, service `app-a`

The fixture keeps the neutral names `tenant-a` (the isolation boundary), `team-a` (the owning
group) and `app-a` (the service) — deliberately generic so a reader can see the pattern rather
than a business domain. The tenant/team axes are told apart by the prefix in the name itself:
the namespace, AppProject and directory say `tenant-a`; CODEOWNERS and the identity groups say
`team-a`. (An earlier draft renamed these to `payments` / `checkout-squad` / `checkout-api`;
that was dropped in favour of the generic names.)

**Regenerate, do not `git mv`.** `3-tenant-repos/` is generated output, and the old tree
was already stale against the catalog (it still had `gitops/platform/team/`). The `tenant-a`
files are **already deleted in the working tree**; step 19.1(a) removed the empty directory
and staged the deletions. Regenerating proves the tooling works:

```bash
make demo-onboard-tenant
make demo-add-service

# CI normally writes manifests/. Reproduce it locally so the committed tree is complete:
helm template app-a 1-platform-catalog/charts/service \
  --values 3-tenant-repos/tenant-a/gitops-repo/services/app-a/dev/values.yaml \
  --namespace tenant-a \
  > 3-tenant-repos/tenant-a/gitops-repo/services/app-a/dev/manifests/rendered.yaml
```

Result to check: the generated `CODEOWNERS` says `@acme-corp/team-a` (the team)
while the namespace, AppProject and directory all say `tenant-a` (the tenant). Those two
names being different is the point.

Also update the hardcoded `tenant-a` / `app-a` references in `README.md` (roughly lines
282, 350-351, 517-522, 542-544) and in
`4-platform-engineering/2-cluster-services/observability/slo/app-a-*.yaml` plus the runbooks
that name them.

---

### 19.3 — Move capability modules to `enterprise-aws-infrastructure`

> **Superseded paths:** the `infrastructure-modules/` URLs and `*-v1.0.0` pins below are what 19.3 shipped.
> 19.8 moved them to `iac-modules-repo/` and `*-v2.0.0` after the other repo's `-repo` split.

Per ADR 0011. **The other repo's work is done** — the modules exist at the agreed paths
(`data/postgres`, `storage/s3`, `identity/workload-iam`), all 8 modules carry annotated
per-module tags, and release-please manifest mode is wired for future releases.

**The contract is live and was verified end to end**, not just reported: all 8 annotated
tags are on `origin`, `main` is in sync, and a scratch root module using the exact source
URL below completed `terraform init` + `terraform validate` against the remote, passing
only the four inputs `postgres.tf.tmpl` renders today.

```
git::https://github.com/ok-karthik/enterprise-aws-infrastructure.git//infrastructure-modules/data/postgres?ref=postgres-v1.0.0
```

Behaviour was checked against the *fetched* module source, not the migration summary:
`random_string` keeps `upper = false` / `special = false` and the identifier keeps
`substr(local.name_prefix, 0, 56)` — the lowercase-only RDS constraint that shipped broken
once before survived the move — and the master credential is handled by
`manage_master_user_password = true`.

Re-run `git ls-remote --tags origin | wc -l` (expect 8) if anything here stops resolving;
a missing or moved tag is the first thing to suspect.

**Then in this repo:**

4. `1-platform-catalog/catalog.yaml`:
   ```yaml
   capabilities_source_base: "git::https://github.com/ok-karthik/enterprise-aws-infrastructure.git//infrastructure-modules"

   capabilities:
     postgres: { provisioner: terraform, module: data/postgres,         version: postgres-v1.0.0 }
     s3:       { provisioner: ack,       module: storage/s3,            version: s3-v1.0.0,          kind: Bucket, group: s3.services.k8s.aws }
     iam:      { provisioner: ack,       module: identity/workload-iam, version: workload-iam-v1.0.0, kind: Role,  group: iam.services.k8s.aws }
   ```
   `version:` is passed straight through to `?ref=`, so a module-scoped tag needs no
   template or engine change — it is still one opaque string.
5. `1-platform-catalog/per-tenant/infra/platform/team-iam.tf.tmpl` line 21 — this one
   hardcodes its own URL instead of going through `capabilities:`. Point it at
   `…/enterprise-aws-infrastructure.git//infrastructure-modules/identity/workload-iam?ref=workload-iam-v1.0.0`.
6. `renovate.json` — two changes, and the second is the fiddly one:
   - `depNameTemplate` becomes `ok-karthik/enterprise-aws-infrastructure`, and the
     `description` gets rewritten: it currently explains that the pin resolves against this
     repo's own tags, which is no longer true and is the whole reason the move is worth
     doing.
   - **Module-scoped tags need one manager entry per module.** The `github-tags` datasource
     returns every tag in the repo, so without filtering Renovate would offer `eks-v1.2.0`
     as an upgrade for `postgres`. Give each capability its own entry with a regex
     versioning that only matches its own prefix:
     ```json
     "versioningTemplate": "regex:^postgres-v(?<major>\\d+)\\.(?<minor>\\d+)\\.(?<patch>\\d+)$"
     ```
     Tags that do not match are invalid under that versioning and are ignored. Verify with
     `npx renovate-config-validator` and a dry run before trusting it — a silently
     over-broad match here would bump a tenant's database module to a VPC release.
7. `git rm -r 4-platform-engineering/3-capability-modules/` — the whole directory, including
   `azure/postgres` and the README. Azure is deleted, not relocated: an AWS-named repo is
   the wrong home for it, and leaving one unused module behind in an otherwise empty
   directory is a half-finished seam. The portability argument is now stronger without it —
   the provider is the *repository*, so a second cloud means a second infra repo and a
   second `capabilities_source_base` (ADR 0011).
8. Regenerate the Go golden testdata under
   `2-idp-scaffolder/golang/internal/templater/testdata/` — both the file contents (new
   source URL) and the directory path (it mirrors the output tree, so it gains the
   `workloads-repo/` level and `apps` → `services`).
9. **Wire `postgres.tf.tmpl` to the SSM discovery contract.** The migrated module gained
   two inputs the old one did not have:

   | Input | Default | What the default means |
   |---|---|---|
   | `vpc_id` | `""` | `count = 0` on the security group; `vpc_security_group_ids = null` |
   | `subnet_ids` | `[]` | `count = 0` on the DB subnet group; `db_subnet_group_name = null` |

   Both have defaults, so the template still passes `terraform validate` unchanged — that
   is confirmed, and it is also the trap. A database scaffolded today would come up with no subnet group and no
   security group, landing in whatever default VPC the account has. It fails at `apply`,
   not at `validate`, which is the expensive place to find out.

   The foundation repo now publishes exactly what is needed (Phase 18.1, Pattern A), so
   read it at plan time rather than hardcoding:

   ```hcl
   data "aws_ssm_parameter" "vpc_id" {
     name = "/platform/[[ .Env ]]/${var.region}/vpc/id"
   }

   data "aws_ssm_parameter" "database_subnets" {
     name = "/platform/[[ .Env ]]/${var.region}/vpc/database_subnets"
   }

   module "postgres" {
     # …
     vpc_id     = data.aws_ssm_parameter.vpc_id.value
     subnet_ids = split(",", data.aws_ssm_parameter.database_subnets.value)
   }
   ```

   Two things to settle while doing it: where `region` comes from (a `providers.tf`
   variable in `per-tenant/infra/platform/`, not a per-capability input), and that a
   capability template still declares **no `locals`** — every capability renders into the
   same root module, so a second capability declaring `locals` is a hard duplicate-name
   error. Multiple `data` blocks with distinct names are fine.

   This is the first real use of the discovery contract, so it is also the test of whether
   Phase 18.1 is a working design or just a document.

---

### 19.4 — Bug: ACK capability claims never reach the cluster

Found while writing ADR 0010. **This is a real defect, not a cleanup.**

An ACK capability (`s3`, `iam`) renders to
`<tenant>/gitops-repo/services/<app>/<env>/s3.yaml`. But the tenant ApplicationSet syncs
`{{ .path.path }}/manifests` — only the `manifests/` subdirectory — and the CI render step
wipes `manifests/` and writes nothing into it except `rendered.yaml` from the Helm chart.
So the Bucket CR is scaffolded, is validated by the Kyverno job in `ci.yaml`, and is then
never applied by ArgoCD.

It is invisible today only because the committed fixture requests `postgres` alone.

**Fix:** in `.github/workflows/tenant-workloads-ci-cd.yaml`, after the `helm template` call
and before the commit, copy the ACK claims into the synced directory:

```bash
# ACK claims are desired state authored by hand; manifests/ is the only path ArgoCD syncs.
find "$(dirname "$VALUES")" -maxdepth 1 -name '*.yaml' ! -name 'values.yaml' \
  -exec cp {} "$OUT_DIR/" \;
```

This keeps the invariant that `manifests/` is the one directory ArgoCD reads. Do not point
the ApplicationSet at the environment directory instead — ArgoCD would try to apply
`values.yaml` as a manifest.

**Acceptance:** scaffold with `--capabilities postgres,s3` and confirm the rendered
`Bucket` appears under `manifests/`.

---

### 19.5 — Move the cloud foundation to `enterprise-aws-infrastructure`

Per [ADR 0012](docs/adr/0012-platform-repo-owns-no-cloud-terraform.md). This repo ends up
owning **no cloud Terraform at all**.

The two repos currently build the same things twice. `enterprise-aws-infrastructure`
already has `infrastructure-modules/{network/vpc,compute/eks,identity/human-access}`, live
Terragrunt stacks for `dev` and `prod` in `eu-central-1`, and an `infrastructure-bootstrap/`
day-0 stack with the GitHub OIDC provider and S3 state bucket. This repo has plan-clean
copies of the same ideas that have never been applied. Only one of them can be the real
one.

**(a) Move each stack, merging rather than pasting.** The target repo's versions are the
ones that run, so where both exist, keep its structure and fold in anything this repo's
version does better (the EKS file here has private-endpoint, IMDSv2 and control-plane
logging hardening worth checking against `compute/eks`):

| From `4-platform-engineering/1-cloud-foundation/aws/` | To |
|---|---|
| `network/` | merge into `infrastructure-modules/network/vpc` + live stacks |
| `cluster/` | merge into `infrastructure-modules/compute/eks` + live stacks |
| `cluster-access/` | `infrastructure-modules/identity/human-access` |
| `workload-identity/` | `infrastructure-modules/identity/workload-identity` (new) |
| `organization/` (incl. `ack-cross-account.tf`) | `infrastructure-modules/governance/organization` (new) + a `_global` live stack |
| `bootstrap/` | **delete** — `infrastructure-bootstrap/` already does this |

Each new module follows that repo's conventions: `main.tf` / `variables.tf` / `outputs.tf`,
no `provider` blocks inside modules, a `terraform-docs` entry in its `docs` make target,
and its own release tag (`vpc-v1.0.0`, `eks-v1.2.0`, …) like every other module there.
Then `make fmt && make lint && make validate && make test && make security`.

**(b) Replace `1-cloud-foundation/aws/` with the contract.** Delete the directory and write
`4-platform-engineering/1-cloud-foundation/README.md` stating what this platform needs from
the foundation and how it reads it:

- cluster name, OIDC provider ARN, VPC ID, database subnet IDs, ACK cross-account role ARN
- read via the SSM parameter service catalog already specified in **Phase 18.1**
  (`/platform/${env}/${region}/...`) — this is what makes 18.1 load-bearing instead of
  aspirational
- who publishes each parameter (the foundation repo, on every stack apply) and what breaks
  if it is missing

**(c) `1-cloud-foundation/local/` stays.** The k3d harness is a test rig for this repo, not
cloud infrastructure. After this move it is the only foundation this repo can stand up on
its own, which makes `make setup` staying green a hard gate rather than a nice-to-have.

**(d) Renumber.** `4-platform-engineering/` loses `3-capability-modules/` (19.3) and keeps
`1-cloud-foundation/` as a contract directory. Rename `4-platform-apis/` → `3-platform-apis/`
so the numbering is contiguous again. Five files reference it: `README.md`, `PLAN.md`,
`.agents/AGENTS.md`, `2-cluster-services/composition/crossplane.yaml`,
`docs/adr/0001-tools-evaluated.md`.

**(e) Fix the references to the foundation.** Eleven files mention `1-cloud-foundation`;
each needs to describe it as *consumed* rather than *owned*: `README.md`,
`.agents/AGENTS.md`, `PLAN.md`, `docs/runbooks/cluster-upgrade.md`,
`2-cluster-services/README.md`,
`2-cluster-services/security-governance/external-secrets.yaml`,
`2-idp-scaffolder/python/TODO.md`, and the generated tenant tree (regenerated in 19.2).

**(f) PLAN.md phase history.** Phases 5, 7.2b and 7.2c describe work that now lives in the
other repo. Do not rewrite the history — it records what was done. Add one line to each
saying where the work lives now.

---

### 19.6 — Stale documentation sweep

Several documents still describe the three-repo model, and two of them contradict each
other today. All of these are now settled by ADR 0010 and should point at it rather than
re-argue it.

| File | What's wrong |
|---|---|
| `.agents/AGENTS.md:146` | "each of those three maps to a standalone repo (`-apps`, `-infra`, `-gitops`)" — contradicted by its own Decision 14 twelve lines later |
| `.agents/AGENTS.md:301` (Decision 9) | says use `<team>` as the ownership boundary; Phase 13 reversed this to `{tenant}` |
| `.agents/AGENTS.md:14-16, 44-47` | tree shows `gitops/platform/team/`; the catalog says `tenancy/` |
| `.agents/AGENTS.md:227-229` | module source URLs still point into this repo |
| `.agents/AGENTS.md:310` (Decision 14) | keep a short summary, move the reasoning to ADR 0010 and link it. Both CODEOWNERS templates cite it as "Decision 15" — fix the reference |
| `.agents/AGENTS.md` §"Resolved CLI Design Decisions" | add ADR 0010 and 0011 to the list |
| `docs/gitops-delivery.md` | whole doc describes three subfolders → three repos, and shows `git subtree split` for `apps/`, which cannot produce the merged repo. Rewrite around the two-repo split and the `git filter-repo` command in ADR 0010 |
| `2-idp-scaffolder/golang/CODE_WALKTHROUGH.md:106` | "Root of `<team>-apps` repo" |
| `README.md:52, 83, 162, 221` | repo-layout tour and output-contract table |
| `README.md:388, 416, 421, 479, 528` | capability module paths, now in another repo. Line 421 links `azure/postgres` as the portability proof — that module is deleted (ADR 0011), so the claim must be rewritten around the repo-per-provider seam |
| `README.md:83-88` and `.agents/AGENTS.md:48-77` | the `4-platform-engineering/` tree: four numbered directories become three, and the cloud foundation becomes a contract (ADR 0012) |
| `docs/adr/0001-tools-evaluated.md` | references `4-platform-apis`, renumbered in 19.5(d) |
| `docs/adr/0006-single-platform-chart.md:11` | names `3-tenant-workloads/`; renamed to `3-tenant-repos/` in 19.1(a). ADRs 0001-0009 are historical, but a path a reader will try to open should still resolve |
| everything else naming `3-tenant-workloads` | `git grep -l 3-tenant-workloads` after 19.1(a) and fix what it finds — the rename touches docs far beyond the list above |
| `PLAN.md:7-8` | says module sources resolve against `?ref=v2.2.0` (this repo's tag); after 19.3 they resolve against per-module tags in `enterprise-aws-infrastructure` (`postgres-v1.0.0`) |
| `PLAN.md:152` (Phase 18) | names the infra repo `enterprise-aws-platform-terragrunt` throughout; it was renamed to `enterprise-aws-infrastructure` — **done** |

`tenant-repos-layout-change.md` at the repo root was the scratch file behind these
decisions. It is superseded by ADR 0010 and has been deleted.

---

### 19.7 — Verification gate

Nothing here is done until all of it passes:

```bash
# 1. Both engines still agree, byte for byte
cd 2-idp-scaffolder/golang && go build ./... && go vet ./... && gofmt -l . && go test ./...
./idp-cli onboard-tenant --tenant-name test-tenant --owner test-owner \
  --catalog-root ./1-platform-catalog --output-root /tmp/go-out
./idp-cli add-service --tenant-name test-tenant --app-name test-app \
  --golden-path go-service-postgres --capabilities postgres,s3 \
  --catalog-root ./1-platform-catalog --output-root /tmp/go-out
# same two commands with the Python CLI, then:
diff -r /tmp/go-out/test-tenant /tmp/python-out/test-tenant

# 2. Generated Terraform resolves against the NEW repo and parses
cd /tmp/go-out/test-tenant/workloads-repo/infra/services/test-app/dev
terraform init -backend=false && terraform validate && terraform fmt -check
grep -r 'enterprise-aws-infrastructure.*?ref=postgres-v' .   # right repo, right tag scheme
# init proves the tag is PUSHED — a local-only tag fails here, which is the whole point

# 3. Tenancy policies still pass over the new paths
kyverno apply 4-platform-engineering/2-cluster-services/security-governance/ \
  $(find 3-tenant-repos/*/gitops-repo -type f -name '*.yaml' ! -name 'values.yaml' \
    | sed 's/^/--resource /')

# 4. Nothing anywhere still points at the old layout
git grep -n "gitops/apps\|/apps/app-a\|tenant-a\|team-a\|3-capability-modules\|1-cloud-foundation/aws\|4-platform-apis" \
  -- . ':!PLAN.md' ':!docs/adr'

# 5. No cloud Terraform is left in this repo (ADR 0012)
find 4-platform-engineering -name '*.tf' -not -path '*/local/*'    # expect: nothing

# 6. End to end on k3d — now the only foundation this repo can stand up alone
make setup && make wait-for-apps
#    then confirm ArgoCD created Applications for tenant-a, and the Bucket CR
#    from the s3 capability appears under manifests/ and syncs (19.4)
```

Steps 4 and 5 should return nothing. Step 3 is the one most likely to fail quietly —
Kyverno wildcards match across `/`, so confirm the tightened pattern from 19.1(d) actually
rejects a tenant pointing its ApplicationSet at another tenant's gitops repo.

In the other repo those gates already pass (`fmt-check`, `lint`, `test`, `security`, and
offline `terraform validate` on all 8 modules). **Done (2026-09-21):** the module tags are
pushed. The pins now in use are the `*-v2.0.0` tags at `iac-modules-repo/` (see 19.8); the
older `*-v1.0.0` tags point at the pre-split `infrastructure-modules/` path and no longer resolve here.

### 19.8 — Follow the infra repo's `-repo` split (its Task 1.7)

`enterprise-aws-infrastructure` Phase 1 renamed `infrastructure-modules/` to
`iac-modules-repo/` (plus `foundation-live-repo/`, `workloads-live-repo/`,
`policy-library-repo/`). The old `*-v1.0.0` tags still point at the old path, so
`//iac-modules-repo/...?ref=postgres-v1.0.0` cannot resolve. The modules were released
again at the new path as **2.0.0** (breaking: the source path changed).

- `catalog.yaml`: `capabilities_source_base` → `…//iac-modules-repo`; pins
  `postgres-v2.0.0`, `s3-v2.0.0`, `workload-iam-v2.0.0`.
- `per-tenant/infra/platform/team-iam.tf.tmpl`: `…//iac-modules-repo/identity/workload-iam?ref=workload-iam-v2.0.0`.
- `renovate.json`: the `team-iam.tf.tmpl` match string follows the new path.
- `3-tenant-repos/tenant-a/` fixture, Go golden testdata, `.agents/AGENTS.md`,
  ADR 0011/0012, `1-cloud-foundation/README.md` and the cluster-upgrade runbook use the new paths.

**Done and verified (2026-09-21).** `postgres-v2.0.0`, `s3-v2.0.0` and `workload-iam-v2.0.0` are on
`origin` (commit `b15f32e`) and hold the modules under `iac-modules-repo/`. On a fresh `tenant-a`
render, `terraform init` + `validate` pass for `infra/services/app-a/dev` and `infra/platform`.
Go tests pass, the Go and Python engines still match byte for byte, and the committed fixture
equals a fresh render (apart from CI-written `manifests/`).

Also fixed here: `team-iam.tf.tmpl` indented its `module` block with 4 spaces, so the rendered
`team-iam.tf` failed `terraform fmt -check`. It now uses 2 spaces and `fmt -check` passes.
