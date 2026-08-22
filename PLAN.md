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
