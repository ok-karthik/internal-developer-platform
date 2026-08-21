# PLAN.md — Platform Hardening & Roadmap

Execution plan for the next phase of this IDP. Written to be picked up cold by a
fresh session. Each task states **what**, **why**, **exact files**, and **how to verify**.

Work the phases in order. Phase 0 and 1 are the ones that make the platform *true*;
everything after that makes it *more capable*. Do not skip ahead — Phase 3 depends on
the tenancy work in Phase 1 (the ArgoCD `AppProject` whitelist gates what ACK can deploy).

Phases 0–3 are complete as of commit `020cfe1`. **Phases 4 onward were re-ranked on
2026-08-21 against measured job-market demand — read the Interlude before Phase 4 for
why the order changed and why Backstage moved from Phase 4 to Phase 8.**

---

## Scope

### Out of scope — do not touch

- **`2-idp-scaffolder/golang/`** — all of it. There is uncommitted work in flight here
  (`internal/templater/errors.go` is untracked, `TODO.md` is modified). The owner is
  handling this on a separate branch against `TODO.md`. Do not edit, refactor, or
  "fix" anything under this directory, and do not commit those pending changes.
- **`2-idp-scaffolder/python/`** — no code changes needed by this plan (see the
  invariant note in Appendix A explaining why the new templates need none).

### In scope

- `1-platform-catalog/` — blueprints, chart, `catalog.yaml`
- `3-tenant-workloads/` — the rendered output tree
- `4-platform-engineering/` — addons, modules, ArgoCD apps
- `README.md`, `.agents/AGENTS.md`
- `archived/`

### Running the scaffolder

You may **run** the Go CLI to regenerate output, but do not modify it. If it fails to
build because of the in-flight uncommitted changes, **do not fix it** — instead
hand-write the rendered output file to mirror its template, and note in your summary
that a scaffolder run is still needed to confirm byte-parity. Keeping
`1-platform-catalog/` and `3-tenant-workloads/` consistent is required either way.

---

## Target environment — read this before any design decision

**The target is EKS. k3d is a test harness, not the platform.** Azure AKS and GCP GKE are
secondary targets via the Phase 10 seam.

This governs every choice in this plan, and it overrides a specific failure mode: *"pick
the option k3d can demonstrate."* Local reproducibility is a development convenience. It
is **not** an architectural constraint, and letting it become one teaches the wrong
lessons — which is the opposite of the point of this repo.

**The rule: model what production does, then note how the local harness approximates it.**
Never the reverse. Where the two genuinely differ, the EKS mechanism is the one that goes
in the platform, and the k3d one is the one that goes in a comment.

### What the local loop does *not* test

Name these in the README. False confidence from a green local run is a worse outcome than
no local run at all:

| Concern | k3d / k3s | EKS | Consequence |
|---|---|---|---|
| Workload identity | none — SA tokens go nowhere | Pod Identity / IRSA | **Nothing IAM-related is exercised locally** |
| `type: LoadBalancer` | klipper-lb, a host port | a real NLB/ALB with a bill and a security group | LB quota in Phase 1.3 is untested locally |
| Storage | local-path | EBS CSI, zone-bound volumes | zonal scheduling failures are invisible locally |
| Cluster auth | flags on the API server | Access Entries / IAM Identity Center | Phase 7's *production* path is untested locally |
| IMDS | absent | `169.254.169.254` is live | the Phase 0.3 SSRF egress rule is unexercised locally |
| Node isolation | one Docker host | real nodes, AZs, taints | anti-affinity and spread rules never actually apply |
| NetworkPolicy | enforced (k3s ships a controller) | enforced (VPC CNI / Calico) | ✅ this one *does* transfer |

### How to be EKS-ready without paying for EKS

Three tiers. Do the first two always; the third once.

1. **`terraform validate` + `tflint` in CI** — catches syntax and obvious type errors.
   Free, no credentials. This is table stakes, not evidence.
2. **`terraform plan` against a real AWS account, in CI, via GitHub OIDC — never `apply`.**
   A plan creates nothing and costs nothing, but it authenticates against real AWS APIs,
   resolves real data sources, and validates AMI IDs, instance types, IAM policy documents
   and AZ availability. **This is the tier that separates "I wrote HCL" from "I wrote HCL
   that works,"** and it is the single highest-value item in this section. Wire it into
   Phase 3.6's runner.
3. **One timed end-to-end run, destroyed the same session.** Worth doing once, because the
   arithmetic is better than most people assume: an EKS control plane is ~$0.10/hour, two
   `t3.medium` nodes ~$0.08/hour, a NAT gateway ~$0.045/hour — roughly **$0.25/hour**, so
   a three-hour session that ends in `terraform destroy` costs **under $1**.

   Guard it: `terraform destroy` in a CI job with `if: always()`, a budget alarm, and a
   single NAT gateway rather than one per AZ. Do this **after** Phase 5 so one run
   exercises the whole platform, and write the result up — "I stood this up on real EKS,
   here is what broke that k3d hid" is a genuinely strong interview story, and the honest
   answer to "have you run this for real?" becomes yes.

---

## Phase 0 — Truth-up (do this first, ~1 hour)

The repo currently describes an architecture it does not have, and ships two
NetworkPolicy rules that break traffic. Fix the lies before adding features.

### 0.1 — Fix the ingress NetworkPolicy bug

**Problem.** `3-tenant-workloads/team-a/gitops/platform/team/networkpolicy.yaml` allows
ingress only from namespaces labelled `team: team-a`. Traefik runs in namespace
`traefik` (`4-platform-engineering/argocd-apps/ingress-routing/traefik.yaml:24`). The
service chart renders an Ingress with `ingressClassName: traefik`. **Traefik cannot
reach tenant pods** — every ingress-enabled service is unreachable. k3s ships a
NetworkPolicy controller by default, so this bites locally too.

### 0.2 — Fix the egress NetworkPolicy bug

**Problem.** Egress is allowed only to the team's own namespace and kube-system DNS.
RDS lives *outside* the cluster. So `add-service --capabilities postgres` provisions a
database the pod is firewalled away from. Same for every AWS API call and external
dependency.

### 0.3 — Rewrite the NetworkPolicy as named policies

Replace the single `default-deny-and-dns` blob with five named policies. One blob is
hard to review; named policies document intent and can be changed independently.

**Edit both:**
- `1-platform-catalog/blueprints/team/gitops/platform/team/networkpolicy.yaml.tmpl` (use `[[ .TeamName ]]`)
- `3-tenant-workloads/team-a/gitops/platform/team/networkpolicy.yaml` (rendered, `team-a`)

```yaml
# 1. Default deny everything
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: { name: default-deny-all, namespace: [[ .TeamName ]] }
spec:
  podSelector: {}
  policyTypes: [Ingress, Egress]
---
# 2. DNS — always required
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: { name: allow-dns, namespace: [[ .TeamName ]] }
spec:
  podSelector: {}
  policyTypes: [Egress]
  egress:
    - to:
        - namespaceSelector:
            matchLabels: { kubernetes.io/metadata.name: kube-system }
      ports: [{ protocol: UDP, port: 53 }, { protocol: TCP, port: 53 }]
---
# 3. Same-team east-west traffic
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: { name: allow-same-team, namespace: [[ .TeamName ]] }
spec:
  podSelector: {}
  policyTypes: [Ingress, Egress]
  ingress: [{ from: [{ namespaceSelector: { matchLabels: { team: [[ .TeamName ]] } } }] }]
  egress:  [{ to:   [{ namespaceSelector: { matchLabels: { team: [[ .TeamName ]] } } }] }]
---
# 4. FIX 0.1 — the ingress controller must be able to reach tenant pods
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: { name: allow-ingress-controller, namespace: [[ .TeamName ]] }
spec:
  podSelector: {}
  policyTypes: [Ingress]
  ingress:
    - from:
        - namespaceSelector:
            matchLabels: { kubernetes.io/metadata.name: traefik }
---
# 5. FIX 0.2 — egress to external services, EXCEPT the cloud metadata endpoint.
#    169.254.169.254 is the SSRF -> IAM credential theft path (Capital One breach).
#    Blocking it at the tenant boundary is the reason this policy is not just
#    "allow all egress".
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: { name: allow-egress-external, namespace: [[ .TeamName ]] }
spec:
  podSelector: {}
  policyTypes: [Egress]
  egress:
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
            except:
              - 169.254.169.254/32   # IMDS
              - 10.0.0.0/8           # force intra-VPC through explicit rules
```

**Verify:** `kubectl apply --dry-run=client -f` both files. Confirm the rendered file
contains `team-a` everywhere the template contains `[[ .TeamName ]]`, and nowhere else.

### 0.4 — Fix the Component Matrix in `README.md`

**Problem.** `README.md:172` lists **Crossplane** as the Infra-as-Code layer. Crossplane
is archived (`archived/crossplane/`) and nothing in `4-platform-engineering/` deploys it.
The flagship architecture table misstates the actual architecture.

Replace that row with the truth as of today: Terraform modules are the IaC layer,
version-pinned via `catalog.yaml`. Add a Crossplane/kro row only in Phase 5, if that
phase is done.

### 0.5 — Delete `archived/`

`git rm -r archived/`. Git history retains it. A directory named `archived` is a
standing invitation to ask "is this live?" Mention the deletion in the commit message
so the Crossplane experiment stays discoverable via `git log`.

### 0.6 — Document the open loop in `README.md`

**Problem.** The scaffolder writes `3-tenant-workloads/<team>/infra/apps/<app>/<env>/postgres.tf`.
Nothing in this repo ever runs `terraform apply`. CI
(`.github/workflows/tenant-workloads-ci-cd.yaml`) builds images and runs `helm template`;
there is no Terraform runner, no state backend, no plan gate.

So: **app delivery is a closed loop; infrastructure is an open loop.** A developer asks
for a database and receives a text file.

Add a short, explicit note to `README.md` naming this gap and pointing at Phase 3 as the
fix. Naming a known gap reads as engineering maturity. Leaving a reviewer to discover it
does not. Do **not** quietly fix the wording to imply it works.

---

## Phase 1 — Complete the multi-tenancy controls (~half a day)

The design premise: **namespace-per-team in a shared cluster**, which is what almost all
companies below hyperscale actually run. The skeleton is right; three controls are
incomplete and one is entirely missing.

### 1.1 — Add RBAC (currently missing entirely)

**Problem.** `blueprints/team/gitops/platform/team/` contains `appproject`, `namespace`,
`networkpolicy`, `policy-exceptions` — and **no RBAC**. The `AppProject` governs what
*ArgoCD* may deploy. Nothing governs what a *human* may do with `kubectl`.

**Design opinion to implement, and it is defensible:** in a GitOps platform developers
get **no write access** to their namespace. Writes go through git — that is the entire
point of the reconciliation loop. What developers need is read plus debug.

**Create:**
- `1-platform-catalog/blueprints/team/gitops/platform/team/rbac.yaml.tmpl`
- `3-tenant-workloads/team-a/gitops/platform/team/rbac.yaml`

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: { name: [[ .TeamName ]]-developer, namespace: [[ .TeamName ]] }
rules:
  - apiGroups: ["", "apps", "batch", "networking.k8s.io", "argoproj.io"]
    resources: ["*"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods/log", "pods/portforward"]
    verbs: ["get", "list", "create"]
  # Deliberately absent: pods/exec, secrets read, and every write verb.
  # pods/exec bypasses every guardrail above — it can read mounted secrets,
  # mutate running state, and leaves no git trail. Break-glass belongs in a
  # separate, time-bound, audited role, not in the default developer role.
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata: { name: [[ .TeamName ]]-developer, namespace: [[ .TeamName ]] }
subjects:
  # Bind to a GROUP from the IdP, never to individual users — otherwise
  # onboarding a person becomes a platform-team ticket, which is exactly the
  # toil an IDP exists to remove.
  - kind: Group
    name: "oidc:[[ .TeamName ]]"
    apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: [[ .TeamName ]]-developer
  apiGroup: rbac.authorization.k8s.io
```

Keep the comments — they carry the reasoning, which is the point of the file.

### 1.2 — Add the Pod Security Admission label to the Namespace

**Problem.** The namespace carries only `team: <name>`. PSA is built into the API server,
costs nothing, needs no admission webhook round-trip, and blocks privileged pods, host
mounts, and host networking outright. It is the cheapest tenancy control available and
it is unused. Kyverno then handles what PSA cannot express.

**Edit** `blueprints/team/gitops/platform/team/namespace.yaml.tmpl` and the rendered
`3-tenant-workloads/team-a/.../namespace.yaml`:

```yaml
metadata:
  name: [[ .TeamName ]]
  labels:
    team: [[ .TeamName ]]
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/enforce-version: latest
```

Check this against the chart's `podSecurityContext` in
`1-platform-catalog/charts/service/values.yaml` — it already sets `runAsNonRoot`,
`seccompProfile: RuntimeDefault`, `allowPrivilegeEscalation: false`, and
`capabilities.drop: [ALL]`, which satisfies `restricted`. They should agree. If they
do not, the chart is wrong, not the label.

### 1.3 — Extend the ResourceQuota

**Problem.** The quota caps only compute. The dimensions that cost real money or break
the cluster are unbounded — most importantly `count/services.loadbalancers`, since each
LoadBalancer Service is a real ELB and a real recurring bill.

Add to the existing `hard:` block in both template and rendered file:

```yaml
    count/services.loadbalancers: "2"    # each = a real ELB = a real bill
    count/services.nodeports: "0"        # forces traffic through the ingress
    persistentvolumeclaims: "10"
    requests.storage: 100Gi
    count/secrets: "100"                 # etcd pressure
```

### 1.4 — Add `max` / `min` to the LimitRange

**Problem.** The LimitRange sets `default` and `defaultRequest` but no ceiling. A tenant
can request 10 CPU for one container and consume the entire namespace quota in a single
pod. The quota is defeatable without this.

```yaml
  limits:
    - type: Container
      default:        { cpu: "500m", memory: "512Mi" }
      defaultRequest: { cpu: "100m", memory: "256Mi" }
      max:            { cpu: "2",    memory: "4Gi"   }
      min:            { cpu: "10m",  memory: "32Mi"  }
      maxLimitRequestRatio: { cpu: "4" }   # limits overcommit gaming
```

**Do not remove the `default`/`defaultRequest` block.** The service chart ships
`resources: {}`, so pods carry no explicit requests. Once a ResourceQuota exists, the
quota admission controller *rejects* pods without requests unless a LimitRange supplies
defaults. Deleting those defaults breaks every deployment on the platform.

### 1.5 — Widen the AppProject whitelist for what is coming

**Problem.** `appproject.yaml` `namespaceResourceWhitelist` has no `argoproj.io/Rollout`.
Argo Rollouts is already installed as an addon
(`4-platform-engineering/argocd-apps/gitops-orchestration/argo-rollouts.yaml`), so the
moment anything renders a `Rollout`, ArgoCD refuses to sync it. The same will apply to
ACK kinds in Phase 3.

Add now:

```yaml
    - group: "argoproj.io"
      kind: "Rollout"
    - group: "argoproj.io"
      kind: "AnalysisTemplate"
```

Add a comment above the list recording that **this whitelist is the tenant-facing API
surface** — every capability the platform offers must appear here. That is a feature,
not a chore.

Leave `clusterResourceWhitelist: []` as is. It is correct and it is a good answer to
"how do you stop a tenant creating cluster-scoped resources?"

### 1.6 — Document the ceiling in `README.md`

Add a short subsection under the multi-tenancy material stating plainly that namespaces
are a **cooperative** boundary, not a security boundary — shared kernel, shared control
plane, shared node. The controls above are correct and sufficient for *internal teams
who are not adversaries*, which is the realistic case.

Then name the escalation ladder without implementing any of it: node pools with taints
per tenant → gVisor/Kata for kernel isolation → vCluster for control-plane isolation →
separate clusters. Add the judgement: none of it is justified below roughly 1000
engineers.

---

## Phase 2 — Structural cleanup (~2 hours)

Keep the numbered top-level directories. `1-` / `2-` / `3-` / `4-` encode a *reading
order*, which is the right call for a reference repo. Do not restructure to imitate
other repos.

### 2.1 — Reshape `4-platform-engineering/`

> **Done, and partly superseded.** This task was completed with escape hatch **(b)** —
> the `modules/` rename was skipped to avoid breaking version pins. **Phase 3.8 now
> replaces this target layout entirely**, including that rename, and does it with the
> sequencing and version bump that make it safe. Read 3.8, not this block, for the
> current target. Kept here for the history of why the first attempt stopped short.

It currently mixes AWS Terraform modules with cluster addons, and is missing the cluster
itself. Target:

```
4-platform-engineering/
├── clusters/     # NEW — k3d config today, EKS Terraform later. The cluster is infra too.
├── addons/       # merge of argocd-apps/ + cluster-addons/ — they are one concept
├── apis/         # NEW — platform API definitions (KRO RGDs / Crossplane XRDs). Empty
│                 #       with a README until Phase 5.
└── modules/      # was cloud-services-terraform-modules/
```

**Careful — `modules/` is a breaking rename.** The old path is baked into Terraform
module source URLs in `catalog.yaml` (`capabilities_source_base`) and in
`1-platform-catalog/building-blocks/capabilities/*.tf.tmpl`. Those URLs are
version-pinned by git ref, so existing pinned refs keep resolving against old tags — but
the next tag will move the path. Either:

- **(a)** do the rename and update `capabilities_source_base` in `catalog.yaml` plus the
  module block in every capability template, **and** bump the capability versions; or
- **(b)** skip the rename, keep `cloud-services-terraform-modules/`, and do the rest.

**(b) is the safe default.** Only do (a) if you also update `renovate.json`'s custom
regex manager, which tracks those pins. Whichever you choose, state it in the summary.

Update the directory tree in `.agents/AGENTS.md` and the reading guide in `README.md`
to match whatever you actually do.

### 2.2 — Make Argo Rollouts real, or remove it

**Problem.** The Rollouts controller is installed. `charts/service/templates/deployment.yaml`
emits a plain `Deployment`. Nothing ever creates a `Rollout`. It is a claim with no
implementation.

Pick one:

- **(a)** Add `charts/service/templates/rollout.yaml` gated on `.Values.rollout.enabled`
  (default `false`), rendering an `argoproj.io/v1alpha1 Rollout` with a canary strategy,
  and make `deployment.yaml` render only when `rollout.enabled` is false. Requires 1.5.
- **(b)** Remove the addon and the Progressive Delivery row from the Component Matrix.

**(a) is preferred** — progressive delivery is a genuine platform capability and the
controller is already there. If time is short, (b) is honest and takes five minutes.

### 2.3 — Demonstrate the promotion gate

**Problem.** README documents promotion as "a PR copying `dev/values.yaml` to
`prod/values.yaml`", but only `dev/` exists anywhere. The gate is described, never shown.

Create `3-tenant-workloads/team-a/gitops/apps/app-a/prod/values.yaml` as a copy of the
`dev/` one with production-appropriate values (higher `replicaCount`, explicit
`resources`, a prod host). Do **not** hand-write `prod/manifests/` — that is CI's job,
and the whole point is that rendered output is a pure function of (chart, values).

Confirm the team ApplicationSet at
`3-tenant-workloads/team-a/gitops/platform/applicationsets/team-a.yaml` picks it up: it
globs `apps/*/*`, so `prod` is discovered automatically with no edit. Verify that claim
rather than assuming it.

### 2.4 — Trim the roadmap

Remove the **"Cross-Cloud Capabilities: expand to Azure AKS and GCP GKE"** item from the
README roadmap. The repo is AWS-based, and the target job market (English-speaking German
scaleups) is heavily AWS. Multi-cloud here is cost without return, and it dilutes an
otherwise focused story.

---

## Phase 3 — Close the infrastructure loop (the credibility fix)

This is the highest-value functional change in the plan. Right now the platform
provisions text files, not infrastructure.

### 3.1 — Add a `provisioner` field to capabilities

Make provisioning strategy a **platform decision expressed in data**, not a fork in the
road. `catalog.yaml` already stores capabilities as data; extend the shape:

```yaml
capabilities:
  postgres:
    provisioner: terraform          # stateful, expensive, wants a plan step
    module: aws-postgres
    version: v1.1.0
  s3:
    provisioner: ack                # cheap, recreatable, per-app
    kind: Bucket
    group: s3.services.k8s.aws
  iam:
    provisioner: ack
    kind: Role
    group: iam.services.k8s.aws
```

**The reasoning, which belongs in a comment in the file:** ACK reconciles continuously
with no plan step — deleting a CR can delete a live resource with no review gate.
That is fine for a bucket and unacceptable for a production database. So: **ACK for
cheap, recreatable, per-app resources; Terraform for stateful and expensive ones.**
The developer requesting the capability never learns which they got, which is the
abstraction working correctly.

> **Note:** consuming this new field requires scaffolder changes, and
> `2-idp-scaffolder/golang/` is **out of scope for this plan**. Add the field, the
> comment, and the ACK templates; leave wiring the `provisioner` switch into the
> renderer as a documented follow-up in the summary. The Python engine is also out of
> scope. Do not edit either engine to make this work.

### 3.2 — Install ACK controllers as addons

Add ArgoCD Application manifests for the ACK S3 and IAM controllers under
`4-platform-engineering/addons/` (or `argocd-apps/` if 2.1(b) was chosen). Start with
those two only — they are the cheapest to reason about and the least dangerous to
delete.

### 3.3 — Add ACK capability templates

Create `1-platform-catalog/building-blocks/capabilities/s3.yaml.tmpl` producing an ACK
`Bucket`. It must land in the **gitops** tree, not `infra/`, so ArgoCD applies it — which
means a new `destinations:` entry. Keep the existing `s3.tf.tmpl` until the scaffolder
can choose between them.

Apply the platform guardrails the Terraform module already enforces — encryption,
public-access block, versioning. Per `.agents/AGENTS.md`, guardrails are not tenant
knobs; they are the argument for a platform abstraction over raw resources.

### 3.4 — Add the ACK kinds to the AppProject whitelist

Building on 1.5. Without this, ArgoCD will refuse to sync tenant ACK resources. This is
the dependency that makes Phase 1 a prerequisite rather than a nice-to-have.

### 3.5 — Document the IRSA / Pod Identity boundary

ACK grants AWS permissions **per namespace**. That is the mechanism making ACK
multi-tenant-safe, and it maps directly onto the `AppProject` + `Namespace` +
`NetworkPolicy` boundary that `onboard-team` already creates. It is the missing fourth
wall of the tenancy model.

Document it in `.agents/AGENTS.md`, and add the IAM role scoping to
`blueprints/team/infra/platform/team-iam.tf.tmpl` if it can be done without a scaffolder
change.

### 3.6 — Terraform runner (only if 3.1–3.5 are done)

For whatever stays on Terraform, add plan-on-PR / apply-on-merge via GitHub Actions with
OIDC, plus a remote state backend keyed per app/env. If this is out of budget, say so
explicitly in the README instead of leaving the loop half-open and undescribed.

### 3.7 — Addon hygiene (do before Phase 4 — Phase 4 depends on it)

Four defects found by auditing all 20 files under `addons/`. Fix them before adding the
alerting rules, because the first one silently breaks those rules.

**(a) Namespace defaulting is a live footgun.** `bootstrap.yaml` points ArgoCD at the
whole directory with `recurse: true` and `destination.namespace: argocd`. Twelve files
are ArgoCD `Application`/`ApplicationSet` objects, where `argocd` is correct. **Eight are
ordinary namespaced resources** — `Instrumentation`, `ConfigMap`, two `Ingress`,
`Middleware` — and they work *only* because each hardcodes `metadata.namespace`.

A new file that omits it is created in `argocd`, and **nothing reports an error**. The
Phase 4.2 `PrometheusRule` is exactly this shape: without an explicit namespace it lands
in `argocd`, Prometheus never selects it, no alert ever fires, and every manifest shows
as Synced/Healthy.

Record the rule in `.agents/AGENTS.md`: *every namespaced resource under this directory
must set `metadata.namespace` explicitly; only ArgoCD `Application`/`ApplicationSet`
objects may rely on the bootstrap default.*

**(b) Sync ordering is half-specified.** `argocd.argoproj.io/sync-wave` appears on 7 of
20 files. The `SkipDryRunOnMissingResource=true` option in `bootstrap.yaml` is the tell —
it exists because `ClusterPolicy`, `Instrumentation` and `Middleware` reference CRDs that
do not exist until Kyverno, the OTel operator and Traefik have installed. That option
suppresses the symptom of a missing wave.

Put a wave on **every** file:

| Wave | Contents |
|---|---|
| `0` | CRD-providing installers — kyverno, cert-manager, traefik, opentelemetry, prometheus, ACK |
| `1` | remaining installers — loki, tempo, promtail, argo-rollouts, sealed-secrets |
| `2` | namespaced config — ingresses, middlewares, instrumentation, grafana-datasources |
| `3` | policies and the tenant `ApplicationSet` — last, so they never gate the platform's own boot |

Then try removing `SkipDryRunOnMissingResource=true` and confirm a cold `make setup`
still converges. If it does, delete the option and say so in the commit — the waves now
carry the ordering honestly. If it does not, keep it and write a comment explaining which
resource needs it.

**(c) Two naming axes are fighting.** The same concern is split across a function-named
folder and a tool-named folder, twice:

- `observability/opentelemetry.yaml` (installer) vs `otel/otel-instrumentation.yaml` +
  `otel/grafana-datasources.yaml` (its config)
- `ingress-routing/traefik.yaml` (installer) vs `traefik/middlewares.yaml` (its config)

Merge `otel/` into `observability/` and `traefik/` into `ingress-routing/`. Do **not**
replace this with an `installers/` + `config/` split — sync-waves already enforce that
ordering, and a directory layout that merely *implies* ordering without enforcing it is
worse than one that does not claim to.

**(d) A version pin has drifted.** `1-platform-catalog/blueprints/team/infra/platform/team-iam.tf.tmpl`
pins `?ref=v1.0.2` for both `aws-networking` and `aws-iam`, while its own rendered output
`3-tenant-workloads/team-a/infra/platform/team-iam.tf` pins `v1.1.0`. Template and output
disagree — Appendix A invariant 4 is broken.

Worse, `renovate.json` states this is *"the ONLY hand-maintained version pin in the tree"*
and its `managerFilePatterns` matches only `^1-platform-catalog/catalog\.yaml$`. Those two
`.tmpl` pins are therefore invisible to Renovate and will silently rot.

Fix both halves: bring the template to `v1.1.0`, and widen `managerFilePatterns` to
include `blueprints/team/infra/platform/team-iam\.tf\.tmpl`. Correct the description
string — it is now factually wrong. Also add a one-line comment to `catalog.yaml`
explaining that `s3` and `iam` deliberately carry **both** `module:` and `kind:`/`group:`
while the scaffolder cannot yet dispatch on `provisioner:` (Phase 3.1), so a reader does
not assume one is vestigial.

**Verify:** `git grep -n 'ref=v' -- '*.tf' '*.tmpl' 1-platform-catalog/catalog.yaml` —
every pin should read `v1.1.0` or later, and every one should be covered by a Renovate
pattern.

### 3.8 — Rename for a taxonomy that survives a second cloud

**The problem with the current names.** `addons/` collides with **EKS add-ons**, which is
a specific AWS product (vpc-cni, coredns, kube-proxy, ebs-csi). A reader lands on that
directory expecting managed EKS components and finds ArgoCD Applications.
`cloud-services-terraform-modules/` names the *tool* in the directory, which is precisely
the thing that has to change if this platform ever runs somewhere else. `clusters/` is
too narrow for what belongs there — an account and a VPC are not a cluster.

**Rename on one principle: name the layer, never the tool.** Tool-named directories are
what make a platform look AWS-shaped even when its abstractions are not.

```
4-platform-engineering/
├── foundation/         # was clusters/ — the substrate the platform sits ON:
│   ├── aws/            #   cluster, cloud account, network. Terraform.
│   │   ├── eks/        #   ← THE TARGET. Real EKS Terraform, held to `plan`-clean.
│   │   ├── network/    #   VPC, subnets, NAT
│   │   ├── access/     #   Access Entries + Identity Center (Phase 7.2b)
│   │   ├── identity/   #   Pod Identity / IRSA seam (Phase 7.2c)
│   │   └── org/        #   Organizations, OUs, SCPs (Phase 5.4)
│   └── local/          #   k3d config. A TEST HARNESS — never the reference.
├── infrastructure/     # was cloud-services-terraform-modules/ — reusable
│   └── aws/            #   cloud-resource modules, now nested BY PROVIDER
│       ├── postgres/   #   (was aws-postgres/ — prefix is redundant under aws/)
│       ├── s3/
│       ├── iam/
│       └── networking/
├── cluster-services/   # was addons/ — what runs IN the cluster to make it a
│                       #   platform. Reconciled by ArgoCD. Cloud-agnostic.
└── platform-apis/      # was apis/ — the custom APIs tenants consume (Phase 9)
```

Note the asymmetry, and write it in the directory's README: **`foundation/` is nested by
provider because it is provider-specific by nature; `cluster-services/` is not nested
because it is portable Kubernetes.** That split *is* the Phase 10 portability story made
visible in the tree — the size of `foundation/aws/` relative to `cluster-services/` is the
honest measure of how cloud-coupled this platform actually is.

`foundation/local/` sits below `foundation/aws/` alphabetically and in importance. Do not
let the k3d config become the file people read first.

It reads top-to-bottom as a stack, no name collides with a vendor product term, and
`infrastructure/aws/` has an obvious sibling slot for Phase 10.

**The catalog change is data-only — no scaffolder edit needed.** The capability templates
already render `source = "[[ .CapabilitiesSourceBase ]]/[[ .Module ]]?ref=[[ .Version ]]"`,
so a slash inside `module:` just works — `git::` sources support subdirectory paths:

```yaml
capabilities_source_base: "git::https://github.com/ok-karthik/internal-developer-platform.git//4-platform-engineering/infrastructure"

capabilities:
  postgres:
    provisioner: terraform
    module: aws/postgres      # provider is a path segment, not a name prefix
    version: v1.2.0
```

Do **not** add a separate `provider:` key. That would require the renderer to join two
fields, and `2-idp-scaffolder/golang/` is out of scope. Encoding the provider in `module:`
keeps this a pure YAML change — which is the `destinations:` design principle applied one
level deeper.

**This is a breaking change. Sequence it exactly:**

1. `git mv` the four directories and the module subdirectories
2. Update `capabilities_source_base` and every `module:` in `catalog.yaml`
3. Update the two hardcoded sources in `blueprints/team/infra/platform/team-iam.tf.tmpl`
   → `aws/networking`, `aws/iam`
4. Update `bootstrap.yaml`'s `path:` → `4-platform-engineering/cluster-services`
5. Update the comment in `1-platform-catalog/building-blocks/capabilities/s3.yaml.tmpl`
   that references the old `aws-s3/main.tf` path
6. Update `renovate.json` (path in the description, plus 3.7(d)'s widened patterns)
7. Update `README.md:33-34`, `README.md:196`, `README.md:232`, `README.md:285`, and the
   directory tree in `.agents/AGENTS.md`
8. Re-render `3-tenant-workloads/team-a/` so the rendered `.tf` files match
9. **Tag `v1.2.0`** — old pins keep resolving against old tags, so nothing breaks
   retroactively, but nothing new resolves until the tag exists

**Do all nine or none.** A half-applied rename leaves Terraform sources pointing at paths
that exist in no tag, and the failure surfaces at `terraform init` in CI, far from the
cause.

---

## Interlude — how Phases 4+ were ranked (read before picking one)

Phases 0–3 were ranked on *architectural truth*: the repo claimed things it did not do.
That work is done. From here the ranking criterion changes to **demand in the market
this repo is a portfolio piece for** — English-speaking Platform/SRE/DevOps roles in
Germany.

The numbers below all come from one population — NOW-track, **n=577** — in
`~/github/karthik-job-market-radar/stats_and_learning_plan.md`, so they are comparable
to each other. `Req%` counts only postings where the surrounding sentence reads as a
requirement rather than a list item; it is the column that matters.

| Capability | Mentioned | Req% | Where it lands |
|---|---|---|---|
| Incident Response | **29.8%** (172) | — | **Phase 4** — repo has none |
| Observability (operated, not installed) | 34% | **15%** | **Phase 4** |
| IAM | 21.1% (122) | — | **Phase 5** |
| AWS (multi-account + VPC depth) | 53.2% | **27%** | **Phase 5** |
| Okta / OIDC-based access control | 1.7% | — | **Phase 7** — see the note below |
| Vault / secrets management | 3.5% | — | Phase 7 |
| Backstage | **1.2%** (7) | — | **Phase 8** (demoted) |
| kro / ACK / Kargo / Crossplane | ≈0–1% | — | Phase 9 (optional) |
| Azure / GCP | 29.3% / 24.3% | 11% / 7% | Phase 10 — seam only, not breadth |
| STACKIT | **0** (untracked) | — | Phase 10 — conversation asset, not a keyword |

**Two phases are in this plan despite low keyword counts, and the reason is the same in
both cases: they are load-bearing for claims the repo already makes.** Phase 7 (identity)
scores 1.7%, but Phase 1's entire tenancy model binds to an OIDC group that nothing
currently issues — without Phase 7 the multi-tenancy story is unfalsifiable. Phase 6
(DORA) scores 0%, but it is what makes Phases 4–5 legible as a product. Neither is
keyword-chasing; both close loops opened earlier. **Backstage, by contrast, closes no
loop** — which is why it moved to Phase 8.

**Correction recorded deliberately.** An earlier revision of this file called Backstage
"highest market value" on the strength of DACH adoption anecdotes (Zalando, Humanitec,
Port). Measured against the corpus it is **7 postings out of 577**. That is the same
order of magnitude as Crossplane, which this plan tells you to skip. Applying market
data to one tool and vibes to another is how the same corpus previously produced a
recommendation to study Kubeflow — 1 posting out of 577 — at a cost of one to two
months. Backstage stays in the plan because it has real learning value (see Phase 8),
but it is **not** the highest-ROI work and must not displace Phases 4–5.

---

## Phase 4 — Operate the observability stack (highest market value)

**The gap.** `4-platform-engineering/addons/observability/` runs Prometheus, Loki, Tempo
and Promtail; `addons/otel/` wires OpenTelemetry and Grafana datasources; the service
chart emits an `Instrumentation`. And there is **not one `PrometheusRule` in the repo.**

So the platform *collects* telemetry and never *acts* on it. Nobody is ever paged. That
is the difference between having installed observability and having operated it — and it
is precisely the Senior→Staff line an interviewer probes.

This phase is worth more than everything below it combined: Incident Response is the #8
most-mentioned skill in the entire corpus (29.8%), and Observability carries a 15% Req —
the highest of any capability this repo does not yet evidence.

### 4.1 — Define SLOs for the sample service

Create `4-platform-engineering/addons/observability/slo/app-a-slo.yaml`. Two SLIs, both
derivable from what the OTel/Prometheus stack already scrapes:

- **Availability** — `1 - (rate(5xx) / rate(all))`, target 99.9% over 30 days
- **Latency** — proportion of requests under a threshold, target 99% under 500ms

Put the *error budget* in a comment beside each target (99.9% over 30d ≈ 43m 12s). The
budget is what makes the next task non-arbitrary.

### 4.2 — Multi-window, multi-burn-rate alerts

Create `4-platform-engineering/addons/observability/slo/app-a-alerts.yaml` as a
`monitoring.coreos.com/v1 PrometheusRule`.

**Do not write a static threshold alert.** `rate(5xx) > 0.01 for 5m` is the alert every
portfolio repo ships and it is why on-call rotations burn out. Implement burn rate
against the error budget instead, the Google SRE workbook shape:

| Severity | Burn rate | Long window | Short window | Budget consumed |
|---|---|---|---|---|
| `page` | 14.4× | 1h | 5m | 2% in 1h |
| `page` | 6× | 6h | 30m | 5% in 6h |
| `ticket` | 3× | 1d | 2h | 10% in 1d |
| `ticket` | 1× | 3d | 6h | 10% in 3d |

The short window is what stops an alert firing for an incident that already recovered.
Every rule carries `severity`, `runbook_url`, and a `summary` naming the user-visible
symptom — never the cause. **"Checkout is failing for 3% of users"**, not
"CPU is high".

### 4.3 — Alertmanager routing

Add Alertmanager config to the Prometheus addon: `severity: page` and `severity: ticket`
route to different receivers, grouped by `alertname` + `namespace`, with an inhibition
rule so a firing `page` suppresses the matching `ticket`.

A local webhook receiver is fine — the routing tree is the artefact, not the integration.
Add a `Makefile` target that fires a synthetic alert through it so the path is
demonstrable in under a minute.

### 4.4 — A runbook per alert

Create `docs/runbooks/`, one file per alert, each linked from its `runbook_url`. Fixed
structure: **Symptom → Impact → Diagnose (exact commands) → Mitigate → Escalate.**

The diagnostic commands must be real and runnable against this platform — the Loki query
for the service's logs, the Tempo trace lookup, `kubectl` calls scoped to what the
Phase 1.1 developer Role actually permits. An unrunnable runbook is worse than none.

Note the constraint explicitly in the runbooks: the developer Role has **no `pods/exec`**
(Phase 1.1), so mitigation is a git revert plus an ArgoCD sync, not a shell on a pod.
That is the GitOps loop being load-bearing under pressure, which is a strong interview
answer on its own.

### 4.5 — One worked postmortem

Write `docs/incidents/2026-XX-XX-<slug>.md` covering a real failure from building this
repo — the Traefik NetworkPolicy bug from Phase 0.1 is ideal, and it genuinely happened.

Blameless format: timeline, impact, detection, root cause, contributing factors,
**what would have caught this earlier**, action items with owners. Then close the loop
honestly: name which Phase 4 alert would have caught it, or state plainly that none
would and what you added as a result.

**Verify:**
```bash
kubectl apply --dry-run=client -f 4-platform-engineering/addons/observability/slo/
promtool check rules 4-platform-engineering/addons/observability/slo/app-a-alerts.yaml
# every runbook_url resolves to a file that exists
grep -rho 'runbook_url:.*' 4-platform-engineering/addons/observability/slo/
```

---

## Phase 5 — Hub → spoke multi-account (the AWS depth phase)

**Why here.** AWS is 53.2% mentioned and **27% Req** — the hardest gate in the corpus.
IAM alone is 21.1% (122 postings). The learning plan's own verdict on the cloud row is
*"multi-account + VPC depth (NOT the cert)"*. This repo is currently single-account, so
it evidences none of that depth.

This is also the one structurally good idea in the AWS workshop architecture: a **hub**
cluster in a central account provisioning into **spoke** tenant accounts.

### 5.1 — Build the hub cluster for real

**Terraform, not documentation.** Per the Target Environment section, EKS is the target, so
`foundation/aws/eks/` holds real HCL held to a clean `terraform plan` in CI. "Documentation
if budget allows" was the wrong default — a plan costs nothing and proves far more.

- **Hub account** — EKS cluster, ArgoCD, ACK controllers, the observability stack
- **Spoke account** — tenant AWS resources; no cluster needed, the account boundary is
  the point

Write the EKS module the way a production one looks, because these are the details
interviews probe and they cost nothing extra to get right:

- **private API endpoint** plus public CIDR allowlist, not `0.0.0.0/0`
- **managed node group** with a launch template, IMDSv2 enforced with hop limit 1 — which
  is the node-level counterpart to the Phase 0.3 IMDS egress rule
- **control-plane logging** enabled (`api`, `audit`, `authenticator`) — Phase 4 and 7 both
  need the audit log to mean anything
- **EKS add-ons** as managed resources (vpc-cni, coredns, kube-proxy, ebs-csi, and the
  **eks-pod-identity-agent** for 7.2c) — and note in a comment that *these* are what AWS
  calls add-ons, which is exactly the name collision Phase 3.8 renamed away from
- **one NAT gateway**, not one per AZ, with a comment saying it is a deliberate cost
  choice and that production would use one per AZ for availability

Reaching `plan`-clean on this is the deliverable. Applying it is optional and belongs to
the single timed run in the Target Environment section.

### 5.2 — Cross-account IAM for ACK

The real content. ACK controllers run in the hub and must create resources in the spoke:

1. Spoke defines a role trusting the hub account's ACK controller role
2. Hub's ACK controller role is granted `sts:AssumeRole` on it
3. ACK is pointed at it per-namespace via the `services.k8s.aws/owner-account-id`
   annotation and a `CARM` ConfigMap entry

Document the trust-policy direction explicitly — *the spoke trusts the hub, never the
reverse* — and why the `ExternalId` condition belongs there. Being able to draw this on
a whiteboard is the thing 122 postings are asking for.

### 5.3 — Tie it back to the tenancy model

Extend the Phase 3.5 IRSA note: namespace → IRSA role → assumed spoke role → blast radius
is one tenant's account. That is the fourth wall of the tenancy model, now with a real
account boundary behind it rather than an IAM policy condition.

### 5.4 — The second tenancy axis: accounts, not namespaces

**This is the piece the plan has been missing.** Everything in Phase 1 is *soft*
multi-tenancy — one cluster, one namespace per team, boundaries enforced by RBAC, quota,
NetworkPolicy and PSA. Phase 1.6 already records that this is a **cooperative** boundary:
shared kernel, shared control plane, shared node.

Real organisations run **both axes at once**, at different granularity:

| Axis | Unit | Boundary strength | Enforced by | Typical granularity |
|---|---|---|---|---|
| **Soft** | Namespace | cooperative | RBAC, ResourceQuota, NetworkPolicy, PSA | per **team** |
| **Hard** | AWS account | real — separate IAM, separate billing, separate blast radius | SCPs, Organizations | per **environment** or **business unit** |

The common shape is *not* "an account per team" and *not* "a namespace per team" — it is
**an account per environment or domain, with a namespace per team inside it.** Teams share
a cluster; prod does not share an account with dev. Say this explicitly in the README,
because "would you give every team their own cluster?" is a standard interview probe and
"no, and here is the axis I would use instead" is the answer that lands.

Document in `foundation/` (post-3.8 naming):

- **AWS Organizations** is the primitive — accounts, Organizational Units, and **SCPs**.
- **SCPs are permission *ceilings*, not grants.** They cannot give anyone access; they cap
  what any principal in the account may do, including the account root. That inversion is
  the whole mental model and it is what makes the account boundary *hard* where a
  NetworkPolicy is soft. Write one real SCP — deny leaving the org, deny disabling
  CloudTrail, deny regions outside `eu-central-1` — and explain the last one as a data
  residency control, which connects to Phase 10.3.
- **AWS Control Tower** is the opinionated setup on top: a Landing Zone, a **Log Archive**
  and **Audit** account for centralised CloudTrail/Config, and "controls" (guardrails)
  applied per OU. **Alternatives worth naming:** Landing Zone Accelerator, or plain
  Organizations + Terraform if you want no managed abstraction.

You do not need to *own* an AWS Organization to do this task, but write it as real
Terraform in `foundation/aws/org/` — `aws_organizations_organizational_unit` and
`aws_organizations_policy` resources with the SCP documents as first-class files — rather
than as prose. Even without an org to apply against, `terraform validate` proves the policy
JSON is well-formed, and committed HCL is inspectable in a way a diagram is not.

Keep the diagram too. The concept is what 122 IAM postings probe for; the console clicks
are not.

### 5.5 — Account vending is the same pattern as your scaffolder

The point that makes this phase yours rather than a recital of AWS docs:

> `onboard-team` provisions a **namespace** from a blueprint, with quotas and policy
> attached, through git. **Account Factory for Terraform (AFT)** provisions an **AWS
> account** from a blueprint, with SCPs and baselines attached, through git.
>
> Same pattern, same guarantees, different substrate. One is your platform's soft axis,
> the other is its hard axis.

Write that comparison down in the README — a table mapping `onboard-team` steps to AFT
steps (request → validation → baseline → policy attach → registration). It costs a
paragraph and it reframes your scaffolder as an instance of a pattern AWS also implements,
rather than as a script. That is the difference between "I wrote a CLI" and "I understand
tenant provisioning."

---

## Phase 6 — DORA metrics

Not a keyword — **0 postings name it**. It is in the plan because it is the framing that
makes every other phase legible as a *product* rather than a pile of YAML, and because
"how do you know your platform is working?" is a standard staff-level interview question
that most candidates answer with anecdote.

All four metrics are derivable from data this repo already produces:

| Metric | Source |
|---|---|
| Deployment frequency | ArgoCD sync history / git commits to `gitops/apps/**` |
| Lead time for change | commit timestamp → ArgoCD `syncedAt` |
| Change failure rate | Phase 4.2 alerts firing within 1h of a sync |
| MTTR | alert `firing` → `resolved` duration |

Ship it as a Grafana dashboard JSON in `4-platform-engineering/addons/observability/`.
**Change failure rate and MTTR are only computable because Phase 4 exists** — say so in
the README. It is the cleanest justification for why Phase 4 came first.

---

## Phase 7 — Identity: authentication and authorisation

**Why this matters more than its keyword count.** Phase 1.1 binds the developer Role to
`Group: oidc:<team>`. **That group exists nowhere.** Nothing issues it, nothing validates
it, and no human has ever authenticated to this platform. The entire tenancy model in
Phase 1 rests on an identity layer that is currently fictional.

Okta is 1.7% and Keycloak is unmeasured, so this is not keyword work. Do it because
"how do you stop team-a touching team-b?" is asked in most platform interviews, and the
complete answer has **four planes**, not one — which is the part most candidates miss.

### 7.0 — The four planes (design this before writing any YAML)

Authorisation is not one decision. Each plane has its own policy engine, and a rule in
one grants nothing in another:

| Plane | Question it answers | Enforced by |
|---|---|---|
| **Kubernetes API** | what can this human do with `kubectl`? | RBAC `Role`/`RoleBinding` (Phase 1.1) |
| **ArgoCD** | who may sync/rollback which app? | `argocd-rbac-cm` `policy.csv` |
| **ArgoCD (deploy surface)** | what *kinds* may be deployed, and where? | `AppProject` (Phase 1.5) |
| **Workload → cloud** | what may the *pod* do in AWS? | IRSA / Pod Identity (Phase 3.5, 5.2) |

The last is not a human plane at all — conflating workload identity with user identity is
the single most common error here. Document this table in `.agents/AGENTS.md`; it is the
spine the rest of the phase hangs on.

**The group name is the contract.** One IdP group flows into all four planes, so pick the
scheme once and never vary it: `platform:<team>:<tier>` — e.g. `platform:team-a:developer`,
`platform:team-a:oncall`, `platform:admin`. Tier is part of the group, not a separate
attribute, because every consumer below can only match on group strings.

### 7.1 — Keycloak as the IdP

Add Keycloak to `cluster-services/` (post-3.8 naming) with a realm defining:

- one **group per team per tier**, following the scheme above
- an OIDC **client** per consumer: `kubernetes`, `argocd`, `grafana`, `backstage`
- a **group membership mapper** on each client, emitting a `groups` claim — without this
  mapper the token carries no groups and every RoleBinding silently matches nothing

Seed the realm declaratively (a realm-export JSON committed to the repo), not by clicking
through the admin UI. A platform whose identity config only exists in a running container
is not a platform.

### 7.2 — Wire the Kubernetes API server to OIDC

The API server validates the token and maps claims to RBAC subjects:

```
--oidc-issuer-url=https://keycloak.<domain>/realms/platform
--oidc-client-id=kubernetes
--oidc-username-claim=preferred_username
--oidc-username-prefix=oidc:
--oidc-groups-claim=groups
--oidc-groups-prefix=oidc:
```

**`--oidc-groups-prefix=oidc:` is why Phase 1.1 binds `Group: oidc:team-a`.** The prefix
exists so an IdP can never mint a group called `system:masters` and take the cluster.
Write that reason down next to the flag — it is a good answer to "how do you stop your
IdP being a cluster-admin escalation path?"

Three control surfaces, one model. **7.2b is the production path — build that.** This
section exists because the flags are what the concept looks like undisguised:

- **EKS ← the target.** API-server flags are not settable. Either
  `aws eks associate-identity-provider-config` (managed OIDC, same settings) **or** IAM
  via Access Entries — see 7.2b, which is what most organisations run.
- **k3d/local:** pass the flags via `--k3s-arg`. A dev-loop approximation, not the design.
- **AKS / GKE:** both take a managed OIDC issuer, so this model ports (Phase 10.1).

Client side: `kubectl oidc-login` (kubelogin) as a `client-go` credential plugin, so
`kubectl` opens a browser and caches the token. Add a `Makefile` target that prints the
kubeconfig stanza a new developer pastes — that is the onboarding artefact, and it is the
same stanza on every one of the three surfaces above, which is the point.

### 7.2b — The EKS production path (build this, not just document it)

7.2 shows the raw mechanism. **This is the one the platform actually targets**, so it gets
real Terraform in `foundation/aws/`, validated by `terraform plan` per the Target
Environment section — not a diagram.

**On EKS there are two authentication paths, and the IAM one is the default:**

**On EKS there are two authentication paths, and the IAM one is the default:**

| Path | Mechanism | Where it fits |
|---|---|---|
| **OIDC** | `aws eks associate-identity-provider-config` — the managed equivalent of Phase 7.2's flags | Portable; the same model as your local cluster |
| **IAM** ← *the common one* | **EKS Access Entries** map an IAM principal ARN → a Kubernetes username and groups | Default on EKS; integrates with the rest of AWS |

Access Entries replaced the old `aws-auth` ConfigMap, which was a single cluster-wide
ConfigMap where one bad edit locked everyone out. Access Entries are a proper API with
per-entry lifecycle — worth mentioning by name, because knowing that `aws-auth` is legacy
is a decent signal on its own.

**The end-to-end chain most AWS shops run.** This is the answer to "how do organisations
achieve SSO across all of this":

```
Corporate IdP  (Entra ID / Okta / Google)
      │ SAML + SCIM
      ▼
AWS IAM Identity Center   ← the hub; formerly "AWS SSO"
      │ Permission Set → an IAM role vended into each account
      ▼
EKS Access Entry          ← maps that IAM role ARN to K8s group "platform:team-a:developer"
      │
      ▼
RoleBinding               ← Phase 1.1 / 7.4, unchanged
```

And in parallel, the *same* corporate IdP is the OIDC provider for ArgoCD, Grafana and
Backstage. **One identity source, one group taxonomy, many consumers** — that is what
"SSO" means here, and the group name from 7.0 is the thing that ties it together.

**State plainly in the README that Keycloak is a self-hosted stand-in** for Entra/Okta/
Identity Center, chosen so the whole loop runs locally with no corporate tenant. Do not
imply you would deploy Keycloak at an AWS shop that already has Identity Center. Naming
the substitution is the mark of someone who has seen the real thing.

**Deliverable:** real Terraform in `foundation/aws/access/` — an `aws_eks_access_entry` and
`aws_eks_access_policy_association` per team group, plus the Identity Center permission-set
assignment — reaching `terraform plan` cleanly against a real account. Not a diagram. The
plan output *is* the evidence that the ARNs, policy documents and group mappings are
well-formed, and it costs nothing.

### 7.2c — Workload identity: Pod Identity is the EKS default

> **Corrected.** An earlier revision of this task said "pick IRSA, because k3d cannot
> demonstrate Pod Identity." That reasoning is backwards under the Target Environment
> section: it let the test harness pick the production mechanism. The k3d limitation is
> real, but it is a note, not a decision.

| | IRSA (2019) | **EKS Pod Identity** (2023) |
|---|---|---|
| Trust | Role trust policy references *that cluster's* OIDC issuer and `system:serviceaccount:<ns>:<sa>` | Generic trust on `pods.eks.amazonaws.com`; a `PodIdentityAssociation` maps (cluster, ns, SA) → role |
| Per-cluster setup | An IAM OIDC provider registered **per cluster** | none |
| Role reuse | trust policy is cluster-specific → **N clusters × M roles** | one role reusable across clusters |
| Where it runs | anywhere an OIDC discovery document can be published | **EKS only** — and not on Fargate |

**Model Pod Identity as the platform's mechanism on EKS.** It is AWS's recommended path
for new EKS workloads, it removes the per-cluster OIDC provider, and it collapses the N×M
trust-policy problem that IRSA creates the moment there is more than one cluster — which
Phase 5's hub/spoke design guarantees.

**Keep IRSA as the documented fallback**, because it is genuinely required in three cases,
and knowing them is the substance of the answer: EKS Fargate, workloads outside EKS, and
any non-AWS cluster. Its OIDC-federation mechanism is also the portable concept with real
analogues — Azure Workload Identity, GCP Workload Identity Federation — which is exactly
what Phase 10.1 needs.

**So build it as a seam, not a pick.** This is the same shape as `provisioner:` in
`catalog.yaml`: the platform exposes *workload identity* as one concept, and the
implementation is selected per environment.

```
foundation/aws/identity/
├── pod-identity.tf      # EKS target — PodIdentityAssociation per (cluster, ns, SA)
└── irsa.tf              # fallback — OIDC provider + per-cluster trust policy
```

The tenant-facing contract does not change either way: a namespace gets a ServiceAccount
that maps to exactly one role. **The blueprint renders the ServiceAccount; the environment
decides how it is bound.** That is the abstraction doing its job.

**Local note, in a comment and nowhere else:** k3d exercises neither mechanism, so nothing
IAM-related is validated by a green local run. That is the point of the tiered validation
in the Target Environment section — `terraform plan` against a real account is what
actually checks these trust policies.

### 7.3 — ArgoCD RBAC (the plane most people forget)

`AppProject` restricts **what** may be deployed. It says nothing about **who** may press
sync. Without `argocd-rbac-cm`, any authenticated ArgoCD user can sync any application in
any project.

```csv
p, role:team-developer, applications, get,      team-a/*, allow
p, role:team-developer, applications, sync,     team-a/*, allow
p, role:team-developer, logs,         get,      team-a/*, allow
p, role:team-developer, applications, delete,   team-a/*, deny
p, role:team-developer, applications, override, team-a/*, deny
g, platform:team-a:developer, role:team-developer
```

Deny `override` deliberately: it is ArgoCD's "sync something other than what git says",
which is the same guardrail bypass as `pods/exec` in Phase 1.1, one layer up. Set
`policy.default: role:readonly`, never `role:admin`.

Point ArgoCD's `dex.config` (or `oidc.config`) at the Keycloak `argocd` client. Note in
the file that this policy must be regenerated per team — which makes it a scaffolder
concern, and therefore a **documented follow-up**, since the engines are out of scope.

### 7.4 — Teams that own more than one namespace

A team will eventually own `team-a`, `team-a-staging`, `team-a-sandbox`. Do **not** invent
a "namespace group" abstraction — Kubernetes has no such object, and every attempt to fake
one ends in a controller you have to maintain.

The two real options, and the choice is defensible either way:

- **(a)** One `ClusterRole` (`platform-developer`, defined once) plus one `RoleBinding`
  per namespace. The Role is DRY; the bindings are the per-namespace grant. **Preferred** —
  the blueprint already renders one file per namespace it creates, so this needs no new
  machinery.
- **(b)** Bind a `ClusterRoleBinding` and filter with an admission policy. Rejected: it
  grants cluster-wide first and claws back second, which is the wrong default.

Take (a) and convert Phase 1.1's namespaced `Role` into a cluster-scoped `ClusterRole`
plus a per-namespace `RoleBinding`. The rules do not change; only the scope of the
definition does. Update both the blueprint template and `3-tenant-workloads/team-a/`.

**Two limits of the DIY approach you must document rather than discover.** Both are real,
both surprise people, and naming them is worth more than pretending they do not exist:

1. **`kubectl get namespaces` is all-or-nothing.** `Namespace` is a cluster-scoped
   resource, so RBAC can grant "list all namespaces" or "list none" — there is no "list
   only mine". A developer running `kubectl get ns` gets a blanket Forbidden. Everything
   else works, because they always name their namespace with `-n`. Put this in the
   onboarding docs as expected behaviour, not a bug.
2. **ResourceQuota is per-namespace, not per-tenant.** A team with three namespaces gets
   3× the quota. Vanilla Kubernetes has no cross-namespace quota object. If a team's
   *total* footprint must be capped, that is the gap — and it is the main thing an
   operator like Capsule exists to close (see Appendix C).

### 7.5 — Break-glass

Phase 1.1's comment promises a *"separate, time-bound, audited role"* for `pods/exec`.
It does not exist. Create it: a `ClusterRole` `platform-breakglass` carrying `pods/exec`
and secret read, bound to `platform:<team>:oncall` — a group **nobody is a permanent
member of**, granted through the IdP for a shift and revoked after.

The control is not the RBAC object, it is the membership lifecycle plus the audit trail.
State plainly in the README that this repo implements the RBAC half and that the
time-bounding is an IdP workflow. Naming the boundary of what you built is stronger than
implying you built all of it.

### 7.6 — Verify (this is the demo)

```bash
kubectl auth can-i get pods       -n team-a --as=dev --as-group=oidc:platform:team-a:developer  # yes
kubectl auth can-i create pods    -n team-a --as=dev --as-group=oidc:platform:team-a:developer  # no
kubectl auth can-i get pods       -n team-b --as=dev --as-group=oidc:platform:team-a:developer  # no
kubectl auth can-i create pods/exec -n team-a --as=dev --as-group=oidc:platform:team-a:developer  # no
kubectl auth can-i create pods/exec -n team-a --as=sre --as-group=oidc:platform:team-a:oncall     # yes
argocd account can-i sync applications 'team-b/*'   # no
```

Put this block in the README verbatim. Six lines that *prove* the tenancy boundary are
worth more than three paragraphs claiming it.

### 7.7 — External Secrets Operator

The repo ships `sealed-secrets.yaml`. Sealed Secrets is fine but it is not what AWS shops
run — **External Secrets Operator + AWS Secrets Manager** is, and it is roughly one addon
manifest plus a `ClusterSecretStore`.

It belongs in this phase because ESO authenticates to AWS via **IRSA** — the workload
identity plane from 7.0. The `ClusterSecretStore` is scoped per namespace to the team's
own role, so the tenancy boundary extends to secrets without new machinery.

Add ESO alongside Sealed Secrets, add `external-secrets.io/ExternalSecret` to the
`AppProject` whitelist (Phase 1.5), and write two paragraphs on the trade-off: Sealed
Secrets keeps ciphertext in git (auditable, works offline, rotation is a re-seal); ESO
keeps only a *reference* in git and resolves at runtime (rotation is free, but the cluster
now depends on AWS being reachable). Holding both and explaining when each wins is a
better answer than having picked one.

### 7.8 — One group, every tool (the SSO deliverable)

The whole phase collapses into one table. Build it, commit it in the README, and make sure
every row is actually wired — an unwired row is the kind of claim that unravels in an
interview:

| Consumer | Protocol | Where the group is consumed | Grants |
|---|---|---|---|
| Kubernetes API | OIDC (`--oidc-groups-prefix=oidc:`), or EKS Access Entry on AWS | `RoleBinding.subjects[].name` | read + logs in that team's namespaces |
| ArgoCD | OIDC via Dex or direct | `argocd-rbac-cm` `policy.csv` `g,` line | sync/rollback that team's apps only |
| Grafana | OIDC + `role_attribute_path` | org role mapping | Viewer, Editor for platform team |
| Backstage | OIDC (`backstage` client) | entity ownership | sees its own components |
| AWS console/CLI | SAML → IAM Identity Center | Permission Set assignment | scoped to that team's account |

**One group string — `platform:team-a:developer` — appears in all five.** That is the
whole design: identity is issued once and interpreted five times, and adding a sixth tool
means adding a client and a mapping, not a new access model.

The failure mode to guard against is drift — a group renamed in Keycloak but not in
`policy.csv` fails **open or closed silently**, with no error anywhere. Add a check to
Appendix B's verification block that greps every consumer for the group strings the realm
export defines, and fails if any is orphaned.

---

## Phase 8 — Backstage (demoted — read the Interlude first)

**7 of 577 postings, 1.2%.** Do this when Phases 4–6 are finished, or when you want the
visual demo for interviews, and **not before**.

What it genuinely buys you, keyword aside: it is the only phase that makes the platform
*look* like a product to a non-engineer, it forces the service catalog to be real rather
than a `catalog-info.yaml` nobody consumes, and "I built a developer portal" is a
sentence a hiring manager understands without you explaining GitOps first.

The repo is closer than it looks — `add-service` already emits `catalog-info.yaml`, which
is Backstage's native ingestion format.

### 8.0 — Does Backstage replace the CLI? No. It becomes a client of it.

The reasonable-sounding version of this question is *"if Backstage has a UI and a
templating engine, why keep a Go CLI at all?"* The answer is that they are not the same
layer. Catalog validation, golden-path resolution, template rendering and destination
routing are the platform's **domain logic**. Backstage is a **presentation layer**.

Put the logic in Backstage and you have exactly one client, forever, in TypeScript, coupled
to a framework you do not control. Keep it in Go and you have the CLI (platform engineers,
CI, offline, debugging), Backstage (developers), and later a GitHub Action or a Slack
command — all hitting one code path with one set of golden tests.

Also worth stating plainly: a CLI a developer can run and read is a *better* artefact for
your job search than a UI wrapping someone else's framework. The CLI is the part that
demonstrates Go.

Three ways to connect them:

| | Approach | Cost | Verdict |
|---|---|---|---|
| **(a)** | Reimplement scaffolding as Backstage TS actions | high | **Reject** — throws away the Go work and locks logic into the framework |
| **(b)** | Backstage shells out to the CLI binary in its own container | ~0 | **Do this** inside the 2-day timebox |
| **(c)** | CLI logic behind HTTP; Backstage calls `http:backstage:request` | 2–3 days | **The right architecture** — do it after (b) works |

**Good news: the repo already supports (c) with no refactor.** `cmd/cli/` holds 49 and 33
lines for `add_service` and `onboard_team`; all logic lives in `internal/templater/` and
`internal/catalog/`. The directory is even named `cmd/cli/`, leaving `cmd/server/` as an
obvious sibling. That is a thin-adapter layout, and it is why (c) is a day of work rather
than a rewrite.

**The design rule to protect** (this belongs in `2-idp-scaffolder/golang/TODO.md`, which is
the owner's branch — record it in your summary, do not edit the file): **`cmd/` stays a
thin adapter; no business logic ever moves into it.** The day that stops being true is the
day Backstage forces a refactor.

**What (c) actually costs, since it is always underestimated.** The endpoint is trivial;
the surrounding concerns are not:

- **Git write credentials.** The service now holds a token that can push to tenant repos.
  That is a genuine new security boundary — the CLI used the *developer's* credentials, a
  server uses its own. Scope the token per team and tie the check to the Phase 7 group
  claim, or you have built a confused-deputy.
- **Async.** Scaffold plus commit plus push exceeds a comfortable HTTP timeout. Return
  `202` with a job id; Backstage polls.
- **Idempotency.** A retried request must not create the service twice.
- **AuthZ.** Which group may scaffold into which team's repo — Phase 7.0's table again.

So: **(b) in Phase 8. (c) as Phase 8.5, and only if Phases 4–6 are done.** Do not let a
transport-layer refactor consume time that belongs to the alerting work.

### 8.1 — The build

1. Backstage instance in the local stack (a `Makefile` target beside `install-argocd`)
2. Point the catalog at `3-tenant-workloads/*/apps/*/catalog-info.yaml`
3. Wrap the scaffolder in a Software Template via approach **(b)**. **The template calls
   the existing CLI — do not reimplement scaffolding logic inside Backstage.**
4. Add the ArgoCD and Kubernetes plugins so deployment state shows on the catalog entry
5. Point Backstage auth at the Keycloak `backstage` client from Phase 7.1, so the portal
   sits on the same identity plane as everything else rather than inventing a fifth

**Timebox this to two days.** Backstage is a TypeScript monorepo with a plugin system
that will absorb unlimited time; steps 1–2 deliver most of the demo value in a few hours,
and step 3 is where the schedule goes to die. If you hit the box, ship what works.

Update the README roadmap: the "migrate the Python CLI into Backstage" framing is wrong —
Backstage *calls* the CLI, it does not replace it.

---

## Phase 9 — Composition engine (optional, do last)

Only after Phases 0–6 are genuinely finished. Every option here measures ≈0–1% in the
target market; this phase is for your own understanding, not for your CV.

Pick **one**. Crossplane by a narrow margin: it graduated CNCF in October 2025, shipped
v2.0, and has enterprise adopters including SAP and IBM, versus kro which is still not
GA. But the margin is thin enough that **"whichever you will actually finish" is the
better tiebreak** — two half-built composition engines are worth less than one working
loop.

Remember the layering (this is the most common confusion): **ACK is the provider layer**
— one CR, one AWS resource. **kro and Crossplane are the composition layer** — one custom
CR fanning out into many resources. kro composes but cannot talk to AWS, so it needs ACK
underneath. Crossplane spans both, so choosing it makes ACK redundant.

If Crossplane: restore from git history (`git log -- archived/crossplane/`), modernize to
v2, and put the XRDs in `4-platform-engineering/apis/`. If kro: `ResourceGraphDefinition`
in the same directory, defining a `Service` kind that expands to Rollout + Service +
Ingress + ACK resources.

Either way, add the corresponding row back to the Component Matrix — and only then.

---

## Phase 10 — Cloud portability (Azure / GCP / STACKIT)

### 10.0 — What Phase 2.4 removed, and why this is not that

Phase 2.4 deleted the roadmap item *"expand to Azure AKS and GCP GKE."* That was correct
and stays correct. It promised **breadth** — three clouds, three times the surface, three
times the rot, in a market where AWS is 53.2% mentioned and **27% Req** against Azure's
11% and GCP's 7%.

**A portability seam is a different thing from breadth.** Breadth means running everywhere.
A seam means the abstraction is honest and you can name exactly what would change. The
first costs months and dilutes the story; the second costs days and *sharpens* it, because
it converts "cloud-agnostic" from a claim into a demonstrated property.

Build the seam. Do not build the breadth.

### 10.1 — What is already portable, and what is not

Most of this platform does not care which cloud it is on. Be specific about the parts that
do, because "what would you have to change?" is the actual interview question:

| Layer | Portable? | Why |
|---|---|---|
| Helm chart, ArgoCD, Kyverno, Prometheus/Loki/Tempo, Argo Rollouts, ESO | ✅ | Plain Kubernetes — runs on any conformant cluster |
| `catalog.yaml` capability *names* (`postgres`, `s3`) | ✅ | Already provider-neutral — this is the abstraction working |
| Terraform modules under `infrastructure/aws/` | ❌ | Provider-specific by definition — but 3.8 already nested them by provider for exactly this |
| The cluster itself — `foundation/aws/eks/` | ❌ | EKS, AKS and GKE differ in node groups, networking and identity. 3.8 nested `foundation/` by provider for this reason; the *relative size* of `foundation/<cloud>/` versus `cluster-services/` is the honest measure of coupling |
| **ACK** | ❌ **hard blocker** | AWS-only. There is no ACK for Azure or STACKIT |
| IRSA / Pod Identity | ❌ | Azure Workload Identity and GCP Workload Identity Federation are the analogues; the *concept* ports, the config does not |
| Ingress / LoadBalancer, StorageClass | ⚠️ | Traefik ports cleanly; the LB and CSI drivers underneath do not |

**The important consequence: ACK is the thing that blocks portability**, and it decides
Phase 9. kro composes but needs ACK for AWS, so kro keeps you AWS-locked. **Crossplane has
providers for AWS, Azure and GCP**, so if portability matters at all, Crossplane wins the
Phase 9 tiebreak decisively rather than narrowly. Record that link in both phases — a
reader should not have to rediscover it.

### 10.2 — Prove the seam with exactly one capability

Do **not** port the platform. Port `postgres`, on one second provider, and stop.

```
4-platform-engineering/infrastructure/
├── aws/postgres/       # exists
└── stackit/postgres/   # add this one
```

```yaml
capabilities:
  postgres:
    provisioner: terraform
    module: aws/postgres          # switching cloud = editing this line
    version: v1.2.0
```

That is the whole demonstration: the provider is a **path segment in data** (Phase 3.8),
so the platform's cloud is a catalog edit, not a refactor. The tenant's `golden-path`
never mentions a cloud, and the developer requesting `postgres` never learns which one
they got — the same abstraction argument as `provisioner:`, one level up.

The module must expose the **same variable and output contract** as the AWS one
(`team_name`, `app_name`, `env`, `tags` in; connection details out). If the contracts
diverge, the abstraction is fake and the demo proves the opposite of what you intended.

### 10.3 — Why STACKIT rather than Azure or GCP

**Read this honestly before doing it.** STACKIT appears in **zero** of the 577 postings —
it is not even in the tracked skill list. Azure (29.3%) and GCP (24.3%) both measure far
higher. On keyword grounds Azure is the correct pick.

The argument for STACKIT is not keywords, it is **differentiation and conversation**.
STACKIT is Schwarz Group's cloud — Lidl and Kaufland, one of Germany's largest companies —
and it exists because German enterprises and the public sector have EU-data-residency
requirements that Schrems II, the EU Data Act and Gaia-X made procurement-relevant rather
than theoretical. It has a real Terraform provider (`stackitcloud/stackit`), a managed
Kubernetes (SKE) and PostgreSQL Flex, so `stackit/postgres` is genuinely buildable.

Every competing portfolio has AWS and Azure. None has a German sovereign cloud. "I built
my platform so the provider is one line in a catalog, and I proved it against STACKIT
because sovereignty is a real constraint here" is a sentence an interviewer in Germany
remembers.

**Treat it as a conversation asset, not a CV keyword.** It must not displace Phases 4–6,
and it should not appear in your skills list as though it were demanded.

**Revised ordering, since AKS and GKE are now stated targets.** The Target Environment
section names EKS primary with AKS and GKE secondary. That makes **Azure the second
provider and STACKIT an optional third**, reversing the emphasis above:

1. **`azure/postgres` + `foundation/azure/aks/`** — 29.3% mentioned, 11% Req, and an
   explicitly stated target. Azure Database for PostgreSQL Flexible Server maps closely
   enough to RDS that the variable contract survives, and **Azure Workload Identity** is
   the direct analogue of Pod Identity, which makes 7.2c's seam concrete rather than
   theoretical.
2. **`gcp/postgres`** — only if Azure went quickly. GCP is 24.3%/7%; the third provider
   demonstrates nothing the second did not.
3. **`stackit/postgres`** — the differentiator, once the seam is already proven. It is
   cheap *after* Azure, because by then the contract is settled and it is one more module.

Doing Azure first also stress-tests the seam harder than STACKIT would: Azure has genuinely
different identity, networking and resource-group semantics, so if the capability contract
survives Azure it will survive anything.

### 10.4 — Write down the ceiling

Add a README section — three paragraphs, no more — titled *"What 'cloud-agnostic' means
here, and what it does not."* State that `cluster-services/` is genuinely portable, that
one capability is proven on a second provider, that ACK is AWS-only and would have to
become Crossplane, that `foundation/` is provider-specific by nature, and that workload
identity has an **analogue rather than an equivalent** on each cloud — EKS Pod Identity,
Azure Workload Identity, GCP Workload Identity Federation solve one problem three
incompatible ways.

**Do not claim the platform runs on three clouds.** It does not, and one competent
follow-up question ends that conversation badly. Claim the seam, show the seam, name the
ceiling. That reads as engineering judgement; the broader claim reads as a résumé.

---

## Appendix A — Repo invariants not to break

Read `.agents/AGENTS.md` in full before starting. The ones this plan touches:

1. **Template delimiters are `[[ ]]`, not `{{ }}`.** This is what lets Helm's `{{ }}`
   pass through untouched. Every new `.tmpl` in `1-platform-catalog/` uses `[[ ]]`.

2. **Blueprints are walked wholesale.** Adding a file to
   `blueprints/team/gitops/platform/team/` is picked up by *both* scaffolder engines with
   **zero code change** — that is the design. This is why Phase 1.1 needs no scaffolder
   edit despite adding a new file.

3. **New blueprint templates may only use `TeamName`.** Python's `onboard_team` builds an
   explicit dict of `{TeamName, VpcCidr}` (`2-idp-scaffolder/python/cli.py:96`) and runs
   Jinja with `StrictUndefined`. Any other field raises at render time. `TeamName` is
   safe; anything else is not.

4. **Catalog and rendered output must stay consistent.** Every edit to a
   `1-platform-catalog/blueprints/**` template needs the matching edit in
   `3-tenant-workloads/team-a/`. A later scaffolder run should reproduce your
   hand-written file byte-for-byte.

5. **Capability templates must not declare `locals`.** All capabilities for a service
   render into the same Terraform root module; duplicate local names are a hard parse
   error. Inline values into the `module` call.

6. **Output paths live in `catalog.yaml`'s `destinations:` table, never in code.** Any
   new output location is a YAML edit there.

7. **`charts/service/` is never scaffolded.** It has no `destinations` key. CI renders it
   and only the output reaches a tenant repo. Do not add a `destinations` entry for it.

8. **Every namespaced resource in `cluster-services/` sets `metadata.namespace`.**
   `bootstrap.yaml` recurses the directory with `destination.namespace: argocd`, so an
   omission is silently created in `argocd` and ArgoCD still reports Synced/Healthy. Only
   `Application` and `ApplicationSet` objects may rely on the default. (Phase 3.7a.)

9. **Provider is a path segment in `module:`, never a separate field.** `aws/postgres`,
   not `provider: aws` + `module: postgres`. The capability templates render
   `[[ .CapabilitiesSourceBase ]]/[[ .Module ]]?ref=[[ .Version ]]`, so a slash keeps this
   a pure YAML change; a second field would require a renderer edit, and both scaffolder
   engines are out of scope. (Phase 3.8, and the reason Phase 10 is cheap.)

---

## Appendix B — Verification

Per phase:

```bash
# Every manifest still parses
find 3-tenant-workloads 4-platform-engineering -name '*.yaml' \
  -exec kubectl apply --dry-run=client -f {} \; 2>&1 | grep -i error

# The chart still renders and lints
helm lint 1-platform-catalog/charts/service
helm template app-a 1-platform-catalog/charts/service \
  -f 3-tenant-workloads/team-a/gitops/apps/app-a/dev/values.yaml

# Templates and rendered output agree (expect only [[ .TeamName ]] -> team-a)
diff <(sed 's/\[\[ \.TeamName \]\]/team-a/g' \
        1-platform-catalog/blueprints/team/gitops/platform/team/namespace.yaml.tmpl) \
     3-tenant-workloads/team-a/gitops/platform/team/namespace.yaml
```

Full local run once Phase 1 is complete:

```bash
make setup            # cluster + argocd + bootstrap
make wait-for-apps
kubectl auth can-i --list --as=system:serviceaccount:team-a:default -n team-a
kubectl describe resourcequota -n team-a
kubectl get networkpolicy -n team-a          # expect 5 named policies
```

Terraform under `foundation/` and `infrastructure/`, per the Target Environment tiers:

```bash
terraform -chdir=4-platform-engineering/foundation/aws/eks init -backend=false
terraform -chdir=4-platform-engineering/foundation/aws/eks validate
tflint --recursive 4-platform-engineering/

# Tier 2 — the one that actually proves something. Creates nothing.
terraform -chdir=4-platform-engineering/foundation/aws/eks plan
```

**A green `kubectl`/`helm` run on k3d does not mean the platform works.** Re-read the
"What the local loop does not test" table before reporting any phase touching IAM,
load balancers, storage or cluster auth as verified. For those, `terraform plan` against
a real account is the verification — not a local apply.

**Report honestly.** If a phase is partly done, say which tasks were skipped and why.
Do not mark a phase complete because the mechanical edits landed if the verification
did not run.

---

## Appendix C — Tools evaluated and not adopted

Commit this table in the README as well. A "considered and rejected, with the condition
that would change my mind" list is one of the strongest artefacts in a portfolio repo:
it is the thing that separates someone who chose from someone who only ever found one
tutorial. Every row must carry the third column — a rejection without a trigger is just
an opinion.

| Tool | What it actually does | Why not here | What would change my mind |
|---|---|---|---|
| **Capsule** (Clastix) | A `Tenant` CRD that owns *many* namespaces: cross-namespace quota, self-service namespace creation within limits, auto-propagated RBAC/NetworkPolicy/LimitRange, and restrictions on ingress classes, hostnames, storage classes and node selectors. `capsule-proxy` also fixes the `kubectl get namespaces` gap in 7.4. | It is the **productised version of Phases 1 + 7.4**, and its headline feature — tenant self-service namespace creation — is a *second answer to a question this platform already answers*, declaratively, through `onboard-team` and git. Adopting it would put an imperative path beside the GitOps one and a mutating webhook in the admission critical path. | A team needing its **total** footprint capped across several namespaces (the 7.4 gap), or tenants who must create namespaces without a PR. Both are real; neither is true at this size. |
| **HNC** (kubernetes-sigs) | Subnamespaces under a parent, with RBAC and object propagation downward and a `HierarchicalResourceQuota`. | Lighter than Capsule and solves the same quota gap, but it still adds a controller to model a hierarchy this platform expresses as a flat, generated list of namespaces. | Team → squad → service nesting deep enough that flat generation stops being readable. |
| **vCluster** (Loft) | A virtual control plane per tenant — own API server, own CRDs, own cluster-scoped resources. | Genuinely solves what namespaces cannot (cluster-scoped isolation, per-tenant CRDs), at the cost of a control plane per tenant. Phase 1.6 already places it on the escalation ladder. | Tenants needing conflicting CRD versions, or an untrusted/adversarial tenant. Per Phase 1.6, roughly 1000 engineers. |
| **AWS Control Tower** | Landing Zone, Account Factory, OU guardrails, centralised Log Archive and Audit accounts. | **Concepts adopted, product not deployed** — Phase 5.4 documents Organizations, OUs and SCPs, which is where the transferable understanding is. A managed Landing Zone needs a real AWS Organization and adds nothing a reader can inspect in git. | Actually operating a multi-account estate rather than documenting one. |
| **Kargo** | Multi-stage promotion (dev → staging → prod) with automated freight tracking. | **0 of 577 postings.** Phase 2.3 already demonstrates promotion as a PR copying `dev/values.yaml` to `prod/values.yaml` — which is simpler, needs no controller, and is easier to explain. | More than three environments, or promotion gates complex enough that a PR stops being expressive. |
| **kro** | Composition layer — one custom CR expanding into many. | Still not GA; cannot provision AWS on its own, so it needs ACK underneath and keeps you AWS-locked (Phase 10.1). | It reaches GA *and* you have decided to stay on AWS permanently. |
| **Crossplane** | Composition **and** provider, across AWS/Azure/GCP. | Deferred to Phase 9, not rejected — it is the correct choice *if* portability matters, and Phase 10.1 explains why that makes the tiebreak decisive rather than narrow. | Starting Phase 10 for real. Then it stops being optional. |
| **Humanitec / Port** | Commercial IDPs, strong in DACH. | Nothing self-hostable to show in a repo; a trial tenant is not an artefact. | Interviewing somewhere that runs one — then learn *their* model, not a substitute. |

---

## Commit strategy

One commit per phase, or per task for Phases 3–9. Work on a branch —
`main` is the default branch here and the owner has separate work in flight on the Go
scaffolder. Do not commit anything under `2-idp-scaffolder/golang/`.
