# PLAN.md — Phase History

What each phase of this platform's build-out actually did. Kept short on purpose — this is
a changelog, not the design doc. The original 1,780-line planning document (with the full
reasoning behind each decision) is retrievable from git history: `git log --all -- PLAN.md`.

Phases 0 through 10 were tagged **`v2.0.0`**; Phases 11 through 13 were tagged **`v2.2.0`**
(the git tag Terraform module sources in `catalog.yaml` now resolve against: `?ref=v2.2.0`);
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

Defines the contract boundaries between the Cloud Foundation (`enterprise-aws-platform-terragrunt`)
and this Developer Platform (`internal-developer-platform`), and formalizes how tenant workloads
dynamically discover infrastructure and fan out across multi-cluster fleets.

### 18.1 — The Discovery Contract: How Tenants Discover VPC & Cluster Context

Tenant capability modules in `3-tenant-workloads/<tenant>/infra/apps/<app>/<env>/` must never
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
`enterprise-aws-platform-terragrunt`, while the runtime controller and node CRDs live in this repo
under `4-platform-engineering/2-cluster-services/karpenter/` managed by ArgoCD.

```text
========================================================================================================
                                 KARPENTER CROSS-REPO ARCHITECTURE SEAM
========================================================================================================

  [ CLOUD FOUNDATION: enterprise-aws-platform-terragrunt ]
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
| **EKS Cluster Agent** | `aws_eks_addon` (pod-identity-agent) | `enterprise-aws-platform-terragrunt` (`compute/eks`) | Intercepts in-cluster AWS STS credential requests |
| **Service Workload** | `aws_eks_pod_identity_association` | `internal-developer-platform` (`3-tenant-workloads/.../iam.tf`) | Binds microservice Pod to specific IAM permissions |
| **Karpenter Node & Controller** | Pod Identity + Node Instance Role + Access Entry | `enterprise-aws-platform-terragrunt` (`compute/eks`) | Dynamic node provisioning and spot interruption handling |
| **Human Engineers** | AWS IAM Identity Center (SSO Permission Sets) | `enterprise-aws-platform-terragrunt` (`identity/human-access`) | PlatformAdmin, Developer, and Auditor access |
| **CI/CD Automation** | GitHub OIDC Provider (`token.actions.githubusercontent.com`) | `enterprise-aws-platform-terragrunt` (`infrastructure-bootstrap`) | Zero-key secretless deployment pipeline auth |
