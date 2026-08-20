# PLAN.md — Platform Hardening & Roadmap

Execution plan for the next phase of this IDP. Written to be picked up cold by a
fresh session. Each task states **what**, **why**, **exact files**, and **how to verify**.

Work the phases in order. Phase 0 and 1 are the ones that make the platform *true*;
everything after that makes it *more capable*. Do not skip ahead — Phase 3 depends on
the tenancy work in Phase 1 (the ArgoCD `AppProject` whitelist gates what ACK can deploy).

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

## Phase 4 — Backstage (highest market value)

Rationale, briefly: Backstage/Humanitec/Port are the most-adopted IDP frameworks in DACH,
and Zalando's platform *Sunrise* — the flagship Berlin platform-engineering story, at a
company that hires in English — is built on Backstage.

The repo is closer to this than it looks: `add-service` already emits
`catalog-info.yaml` per service, which is Backstage's ingestion format.

1. Add a Backstage instance to the local stack (a `Makefile` target alongside
   `install-argocd`).
2. Point its catalog at `3-tenant-workloads/*/apps/*/catalog-info.yaml`.
3. Wrap the scaffolder in a Backstage Software Template so the golden paths are
   selectable from a UI. **The template calls the existing CLI — do not reimplement
   scaffolding logic inside Backstage.**
4. Add the ArgoCD and Kubernetes plugins so a service's deployment state is visible from
   its catalog entry.

Update the README roadmap: Backstage moves from "future" to "done", and the
"migrate the Python CLI into Backstage" framing is wrong — Backstage *calls* the CLI, it
does not replace it.

---

## Phase 5 — Composition engine (optional, do last)

Only after Phases 0–4 are genuinely finished.

Pick **one**. Crossplane by a narrow margin: it graduated CNCF in October 2025, shipped
v2.0, and has enterprise adopters including SAP and IBM, versus kro which is still not
GA. But the margin is thin enough that **"whichever you will actually finish" is the
better tiebreak** — two half-built composition engines are worth less than one working
loop.

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

One commit per phase, or per task for Phases 3–5. Work on a branch —
`main` is the default branch here and the owner has separate work in flight on the Go
scaffolder. Do not commit anything under `2-idp-scaffolder/golang/`.
