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
| Vault / secrets management | 3.5% | — | Phase 7 |
| Backstage | **1.2%** (7) | — | **Phase 8** (demoted) |
| kro / ACK / Kargo / Crossplane | ≈0–1% | — | Phase 9 (optional) |

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

### 5.1 — Model the two accounts

Add `4-platform-engineering/clusters/` documentation (or Terraform, if budget allows) for:

- **Hub account** — EKS cluster, ArgoCD, ACK controllers, the observability stack
- **Spoke account** — tenant AWS resources only; no cluster required to make the point

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

## Phase 7 — Secrets and identity (small, high leverage)

### 7.1 — External Secrets Operator

The repo ships `addons/security-governance/sealed-secrets.yaml`. Sealed Secrets is fine
but it is not what AWS shops run — **External Secrets Operator + AWS Secrets Manager** is,
and it is roughly one addon manifest plus a `ClusterSecretStore`.

Add ESO alongside Sealed Secrets, add `external-secrets.io/ExternalSecret` to the
`AppProject` whitelist (Phase 1.5), and write two paragraphs in the README on the
trade-off: Sealed Secrets keeps the ciphertext in git (auditable, works offline, rotation
is a re-seal); ESO keeps only a *reference* in git and resolves at runtime via IRSA
(rotation is free, but the cluster now depends on AWS being reachable). Holding both and
explaining when each wins is a better answer than having picked one.

### 7.2 — Keycloak (only worth it for this reason)

Phase 1.1 binds the developer Role to `Group: oidc:<team>`. **That group exists nowhere.**
The RBAC is currently aspirational.

Keycloak as an OIDC provider for the API server makes it real: a group claim, mapped to
the RoleBinding, demonstrable with `kubectl auth can-i --as-group=oidc:team-a`. Okta is
1.7% and Keycloak is unmeasured, so do not do this for the keyword — do it because it
closes a loop this plan already opened.

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

1. Backstage instance in the local stack (a `Makefile` target beside `install-argocd`)
2. Point the catalog at `3-tenant-workloads/*/apps/*/catalog-info.yaml`
3. Wrap the scaffolder in a Software Template. **The template calls the existing CLI —
   do not reimplement scaffolding logic inside Backstage.**
4. Add the ArgoCD and Kubernetes plugins so deployment state shows on the catalog entry

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

**Report honestly.** If a phase is partly done, say which tasks were skipped and why.
Do not mark a phase complete because the mechanical edits landed if the verification
did not run.

---

## Commit strategy

One commit per phase, or per task for Phases 3–9. Work on a branch —
`main` is the default branch here and the owner has separate work in flight on the Go
scaffolder. Do not commit anything under `2-idp-scaffolder/golang/`.
