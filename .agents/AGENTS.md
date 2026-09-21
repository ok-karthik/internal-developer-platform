# AGENTS.md - Platform Engineering IDP GitOps Reference Architecture

This repository is an enterprise-grade **Internal Developer Platform (IDP)** blueprint for zero-touch microservice onboarding, GitOps continuous delivery, and infrastructure management.

---

## 🏛️ Repository Architecture & Directory Structure

```
platform-engineering-idp-gitops-reference-architecture/
├── 1-platform-catalog/                 # The platform's offering (Go text/template syntax, [[ ]] delims)
│   ├── catalog.yaml                    # Golden paths, capabilities, and the destinations output contract
│   ├── per-tenant/<kind>/              # Rendered ONCE per tenant by `onboard-tenant`; MIRRORS its output tree
│   │   ├── root/{CODEOWNERS,tenant.yaml}.tmpl  # kind = root | infra | gitops, one destination each
│   │   ├── infra/platform/*.tf.tmpl    # providers, backend, tenant IAM — platform-owned
│   │   └── gitops/{CODEOWNERS.tmpl,platform/{tenancy,applicationsets}/}
│   ├── per-service/                    # Composed per service by `add-service` — ALL of it
│   │   │                               # has a destinations key and lands in a tenant repo
│   │   ├── apps/runtimes/<lang>/       # Language scaffolds; only those declared in runtimes: are offered
│   │   ├── apps/service-meta/          # Runtime-agnostic Backstage catalog-info.yaml
│   │   ├── infra/capabilities/<cap>.tf.tmpl    # provisioner: terraform — Terraform module claims
│   │   ├── gitops/capabilities/<cap>.yaml.tmpl # provisioner: ack — ACK CRD claims
│   │   └── gitops/release/             # Per-env values.yaml scaffolded by the CLI
│   └── charts/                         # Platform-owned Helm charts. NEVER scaffolded — no
│       │                               # destinations key.
│       ├── service/                    # CI renders it; only the output reaches 3-tenant-repos/
│       └── karpenter-nodes/            # EC2NodeClass + NodePool, applied per cluster by ArgoCD
├── 2-idp-scaffolder/                   # IDP Scaffolder Implementations
│   ├── golang/                         # Go implementation of the IDP Scaffolder CLI (Cobra)
│   │   ├── cmd/cli/                    # Cobra commands (`root.go`, `onboard_tenant.go`, etc)
│   │   └── internal/templater/         # Template rendering engine (`render.go`)
│   └── python/                         # Python implementation of the IDP Scaffolder CLI & REST API
│       ├── cli.py / api.py             # Typer CLI and FastAPI REST endpoints
│       ├── catalog.py                  # pydantic twin of internal/catalog — loads + validates catalog.yaml
│       ├── render.py                   # Jinja2 engine, path roots (was utils.py)
│       └── TODO.md                     # Remaining Python work, phased, with an answer key
├── 3-tenant-repos/                     # Container for simulated tenant REPOSITORIES (ADR 0010)
│   └── <tenant>/                       # One directory per tenant (the isolation boundary)
│       ├── workloads-repo/             # → <org>/<tenant>-workloads. Humans write it.
│       │   ├── CODEOWNERS, tenant.yaml
│       │   ├── services/<app>/         # Application source + catalog-info.yaml
│       │   └── infra/
│       │       ├── platform/           # Providers, tenant IAM — platform-owned
│       │       └── services/<app>/<env>/  # Terraform capability claims — team-owned
│       └── gitops-repo/                # → <org>/<tenant>-gitops. CI writes it, ArgoCD reads it.
│           ├── CODEOWNERS
│           ├── platform/               # Platform-owned, CODEOWNERS-protected
│           │   ├── tenancy/            # AppProject, Namespace, NetworkPolicy, RBAC, PolicyException
│           │   └── applicationsets/    # One <tenant>.yaml ApplicationSet per tenant
│           └── services/<app>/<env>/   # Team-owned values.yaml + CI-rendered manifests/
└── 4-platform-engineering/             # Platform Control Plane — numbered in order of operations.
    │                                     # Each directory gets its own README.md.
    ├── bootstrap.yaml                   # Root of the App-of-Apps pattern — stays put
    ├── 1-cloud-foundation/              # The CONTRACT with the cloud foundation, which lives in
    │   │                                 # enterprise-aws-infrastructure (ADR 0012). This repo
    │   │                                 # owns no cloud Terraform.
    │   ├── README.md                     #   What the platform needs: SSM params + the cluster-
    │   │                                 #   registration Secret contract (labels/annotations)
    │   └── local/                        #   k3d. A TEST HARNESS — the only foundation this
    │       └── cluster-secret.yaml       #   repo can stand up alone. Registers k3d as env=dev.
    ├── 2-cluster-services/               # ArgoCD App-of-Apps declarations (App manifests
    │                                     # per addon) merged with the raw resources some
    │                                     # of them deploy, e.g. ingress-routing/middlewares.yaml,
    │                                     # observability/otel-instrumentation.yaml — one
    │                                     # concept, one directory. Portable Kubernetes →
    │                                     # NOT nested by provider. Includes karpenter/
    │                                     # (per-cluster ApplicationSets), observability/opencost.yaml
    │                                     # and security-governance/require-cost-tags.yaml.
    └── 3-platform-apis/                 # Platform API definitions (Crossplane XRDs).
```

---

## 🗺️ Render Map — what each catalog directory produces

Answers "what does this directory turn into?" without opening `catalog.yaml`.
`catalog.yaml`'s `destinations:` block remains the **authority**; this table is the
readable projection of it. If they disagree, `catalog.yaml` is right and this table is
stale — fix the table.

Every path below is relative to `1-platform-catalog/` on the left and
`3-tenant-repos/` on the right.

| You edit this | It renders to | Rendered by | How often |
|---|---|---|---|
| `per-tenant/root/` | `{tenant}/workloads-repo/` | `onboard-tenant` | once per tenant |
| `per-tenant/infra/` | `{tenant}/workloads-repo/infra/` | `onboard-tenant` | once per tenant |
| `per-tenant/gitops/` | `{tenant}/gitops-repo/` | `onboard-tenant` | once per tenant |
| `per-service/apps/runtimes/<lang>/` | `{tenant}/workloads-repo/services/{app}/` | `add-service` | once per service — **one** `<lang>` picked by `--runtime` / golden path |
| `per-service/apps/service-meta/` | `{tenant}/workloads-repo/services/{app}/` | `add-service` | once per service, always |
| `per-service/infra/capabilities/<cap>.tf.tmpl` | `{tenant}/workloads-repo/infra/services/{app}/{env}/` | `add-service` | one file per requested capability with `provisioner: terraform` |
| `per-service/gitops/capabilities/<cap>.yaml.tmpl` | `{tenant}/gitops-repo/services/{app}/{env}/` | `add-service` | one file per requested capability with `provisioner: ack` |
| `per-service/gitops/release/` | `{tenant}/gitops-repo/services/{app}/{env}/` | `add-service` | once per service per env |
| `charts/service/` | **nothing** — never scaffolded | CI, via `helm template` | output only, into `{tenant}/gitops-repo/services/{app}/{env}/manifests/` |

**Addon namespace rule.** `4-platform-engineering/2-cluster-services/` (bootstrap.yaml's App-of-Apps
root) is applied with `directory.recurse: true` and `destination.namespace: argocd`. Every
namespaced resource under that tree — `ConfigMap`, `Ingress`, `Middleware`,
`Instrumentation`, and any future kind — **must set `metadata.namespace` explicitly**.
Only ArgoCD `Application`/`ApplicationSet` objects may rely on the bootstrap default,
because `argocd` genuinely is where those belong. A file that omits the namespace is
silently created in `argocd` instead — nothing reports an error, and whatever depends on
it (a `PrometheusRule` never selected by Prometheus, for instance) fails quietly with
every manifest still showing Synced/Healthy. Every file in the addon tree carries an explicit `argocd.argoproj.io/sync-wave`
too, so ordering is fully specified rather than half-implied by `SkipDryRunOnMissingResource`:
`0` = CRD-providing installers (kyverno, cert-manager, traefik, opentelemetry, prometheus,
ACK, karpenter), `1` = remaining installers (loki, tempo, promtail, argo-rollouts, sealed-secrets),
`2` = namespaced config (ingresses, middlewares, instrumentation, grafana-datasources),
`3` = policies and the tenant `ApplicationSet` — last, so they never gate the platform's
own boot.

**Resolved:** this table used to name two gaps — a `building-blocks/capabilities-ack`
destinations key with no matching directory, and `blueprints/`/`building-blocks/` names
that encoded nothing about *once per team* vs *once per service*. Both are fixed now:
`per-tenant/` and `per-service/` are the real directory names (the cardinality is in the
name), and `per-service/` splits `infra/capabilities/` (provisioner: terraform) from
`gitops/capabilities/` (provisioner: ack) into two real directories, so the directory
itself is the router — no file-extension dispatch, no phantom key. Both scaffolder engines
(`golang/internal/catalog/catalog.go`, `golang/internal/templater/render.go`, `python/catalog.py`,
`python/cli.py`, `python/api.py`) implement the split; `provisioner:` in `catalog.yaml` is a
data fact the renderer acts on, not a label nobody reads.

---

## 🧭 Platform Model & Key Decisions

The scaffolder templates are organised around **platform lifecycle verbs**, not file type. Each verb maps to a `1-platform-catalog/` domain:

| Verb | Catalog source | Idempotency | Produces |
|---|---|---|---|
| `onboard-tenant` | `per-tenant/{root,infra,gitops}/` | once per tenant | tenancy boundary — AppProject, Namespace + ResourceQuota + LimitRange, default-deny NetworkPolicy, CODEOWNERS, Kyverno PolicyException, team Terraform providers + IAM — plus the team ApplicationSet |
| `add-service` | `per-service/**` | repeatable | a golden path — runtime + service-meta + delivery values + capabilities |

**Golden paths** (`catalog.yaml`) compose three pieces: a runtime (`per-service/apps/runtimes/<lang>/`), infra **capabilities** (`per-service/infra/capabilities/<cap>.tf.tmpl` or `per-service/gitops/capabilities/<cap>.yaml.tmpl`, by `provisioner:`), and delivery (`per-service/gitops/release/`). Capabilities are declarative claims mapped to blessed, version-pinned modules (ADR 0011) — e.g. `postgres → data/postgres@postgres-v2.0.0`, `s3 → storage/s3`, `iam → identity/workload-iam`, all in `enterprise-aws-infrastructure` (ADR 0011). `per-service/apps/service-meta/` is runtime-agnostic and rendered for every service, which is why it is a sibling of `runtimes/` rather than living inside it.

Key decisions:
- **Go is the definitive scaffolder; Python is a second engine, not a legacy one.** Both implement `onboard-tenant` and `add-service` against the same `catalog.yaml`. The point of keeping two is that it makes the catalog a *falsifiable* contract: run both with the same inputs and `diff -r` the trees. **The two trees are currently byte-identical**, both verbs, every file. If a change makes them disagree, either the engines drifted or the catalog is under-specified — both are findings, and neither should be papered over. Matching Go's whitespace depends on `trim_blocks`/`lstrip_blocks` in the Python Jinja environment, because the regex that converts `[[- if ]]` to `[% if %]` cannot carry Go's `-` trim markers across.
- **git-as-PR.** `add-service` writes into the git-tracked `3-tenant-repos/` tree; the resulting `git diff` simulates the PR that would be opened against a real tenant repo.
- **Monorepo output, polyrepo mapping.** Everything lands under `3-tenant-repos/<tenant>/{workloads-repo,gitops-repo}/`; in production each of those two directories is a standalone repo (`<org>/<tenant>-workloads`, `<org>/<tenant>-gitops`). The `-repo` suffix means exactly that and nothing else. See [ADR 0010](../docs/adr/0010-tenant-repository-topology.md) for the reasoning and the extraction commands.
- **`platform/` vs `services/` means the same thing in every tree.** `platform/` is platform-owned and CODEOWNERS-protected; `services/` belongs to the tenant's team. The repo directory (`workloads-repo` / `gitops-repo`) says what kind of artifact it is; the split is on who *writes* a file, not on the technology in it. Ownership reduces to two glob lines per repo.
- **IRSA / Pod Identity is the missing fourth wall of the tenancy model.** `onboard-tenant` builds three walls per namespace: what ArgoCD may deploy (`AppProject`), what pods may talk to (`NetworkPolicy`), and what a human may do with `kubectl` (RBAC `Role`/`RoleBinding`). None of that constrains what AWS a pod's *own* credentials can reach. ACK controllers (`4-platform-engineering/2-cluster-services/aws-controllers/`) grant AWS permissions **per namespace** via IRSA (EKS) or Pod Identity — a `Bucket`/`Role` CR reconciled in the `tenant-a` namespace only ever gets `tenant-a`'s AWS permissions, because the trust policy is scoped to the namespace/service-account pair, not to the controller process as a whole. That is the mechanism that makes ACK safe to run multi-tenant, and it is the reason `AppProject` + `Namespace` + `NetworkPolicy` + RBAC is not yet the complete tenancy boundary — this is the fourth control, expressed in AWS IAM rather than Kubernetes RBAC. It is not yet real in this repo: `per-tenant/infra/platform/team-iam.tf.tmpl` calls the `aws-iam` module, which is a documented stub (`identity/workload-iam` in `enterprise-aws-infrastructure`) that provisions no identity. That repo has real, `terraform validate`-clean EKS Terraform that publishes the OIDC provider ARN IRSA needs (`/platform/<env>/<region>/eks/oidc_provider_arn`) — but it has not been applied to a real account, so the ARN does not exist yet either. Wiring real IRSA is therefore gated on an actual `terraform apply`, not on missing code.

- **The full chain, once a real hub cluster exists (Phase 5.1-5.3).** `namespace` (Phase 1) → `IRSA/Pod Identity role` (Phase 7.2c, `identity/workload-identity` in `enterprise-aws-infrastructure`) → `assumed spoke-account role` (Phase 5.2, `governance/organization` there, published as `/platform/<env>/<region>/ack/cross_account_role_arn`) → blast radius is **one tenant's AWS account**, not just one tenant's IAM policy. This is the fourth wall with a real account boundary behind it: a compromised ACK controller reconciling a `tenant-a` `Bucket` CR can only ever assume `tenant-a`'s spoke role, in `tenant-a`'s spoke account — it has no path to another tenant's resources even if that tenant's Bucket CR sits in the same hub cluster, because the trust chain (namespace → IRSA role → spoke role) never crosses. The direction matters: **the spoke trusts the hub, never the reverse** — see the comment in the organization module's ACK trust for why getting this backwards reopens the confused-deputy problem the `ExternalId` condition exists to close.
- **Ownership is enforceable, not just documented.** `onboard-tenant` writes a CODEOWNERS at each repo root (`<tenant>/workloads-repo/`, `<tenant>/gitops-repo/`), because GitHub honours CODEOWNERS only at a repo root, `.github/`, or `docs/` — nesting it under `platform/` would make it decorative. Each file is two rules: the team owns `*`, then `platform/` reverts to the platform team (last match wins); the workloads one also protects `.github/workflows/`, and the gitops one adds security review on `policy-exceptions.yaml`.
- **`per-tenant/` mirrors its output.** `per-tenant/<kind>/` is laid out exactly like the tree it produces, so the nesting *is* the path logic and no file needs its own destination rule. Three keys replace what would otherwise be one key per output directory.
- **Helm only** for delivery (no Kustomize); **`1-platform-catalog/catalog.yaml`** is the source of truth for golden paths and the capability → module mapping.
- **Output paths live in data, not code.** The `destinations:` table in `catalog.yaml` maps each catalog source directory to its output path template (`{tenant}`, `{app}`, `{env}`). No Go file contains a hardcoded output path; restructuring `3-tenant-repos/` is a YAML edit. `LoadCatalog` validates that every required key is present and fails before writing anything.
- **ArgoCD discovery is convention-based, two levels:** a cluster-wide bootstrap ApplicationSet globs `3-tenant-repos/*/gitops-repo/platform/applicationsets` (one Application per tenant, applying that tenant's AppSet); each tenant AppSet then globs `3-tenant-repos/<tenant>/gitops-repo/services/*/*` (app × env) and **matrix-joins it with the ArgoCD cluster registry**: an app whose env directory is `dev` deploys to every registered cluster labelled `environment: dev` (Phase 18.2). The scaffolder never holds a cluster endpoint; registering a cluster is what routes apps to it, and a cluster with no matching label receives nothing — that is the gated promotion of ADR 0003 made mechanical. The tenant `AppProject` allows any server but only the tenant's own namespace. The Kyverno `restrict-applicationset` policy pins each tenant's AppSet to its own `gitops-repo`. `add-service` never edits a root app-of-apps file.

> **CLI status:** both verbs are implemented in **both** engines and produce matching output.
>
> The Go CLI defaults to fetching the catalog from GitHub via `go-getter`, so local edits to `1-platform-catalog/` do not take effect until pushed — **pass `--catalog-root ../../1-platform-catalog` when working locally.** `--output-root` likewise redirects the generated tree, which is what makes the two-engine `diff` possible without writing into the repo. The fetched ref is still hardcoded to a branch (`root.go`); pinning it is Phase 4 of the Go TODO.
>
> The Python CLI has no root flags yet, so it always writes into the real `3-tenant-repos/` — Phase 3 of the Python TODO. It reads its output paths from `destinations:` in `catalog.yaml`, exactly as Go does.

---

## 🔐 Identity & Authorization — The Four Planes

Phase 1.1 binds the developer `Role` to `Group: oidc:<team>`. **That group exists nowhere**
— nothing issues it, nothing validates it, and no human has ever authenticated to this
platform. Phase 7 is what makes it real. Design this before writing any YAML: authorization
is not one decision, it is four, each with its own policy engine, and a rule granted in one
plane grants nothing in another.

| Plane | Question it answers | Enforced by |
|---|---|---|
| **Kubernetes API** | what can this human do with `kubectl`? | RBAC `Role`/`RoleBinding` (Phase 1.1), or `ClusterRole` + per-namespace `RoleBinding` once a team owns more than one namespace (Phase 7.4) |
| **ArgoCD** | who may sync/rollback which app? | `argocd-rbac-cm` `policy.csv` (Phase 7.3) — **not** the same thing as the plane below |
| **ArgoCD (deploy surface)** | what *kinds* may be deployed, and where? | `AppProject` (Phase 1.5) |
| **Workload → cloud** | what may the *pod itself* do in AWS? | IRSA / Pod Identity (Phase 3.5, 7.2c) |

**The last plane is not a human plane at all.** Conflating workload identity with user
identity is the single most common error here — Pod Identity answers "what can this pod do
in AWS," a completely different question from "what can this person do with `kubectl`,"
which is why Phase 7.2's OIDC wiring and Phase 7.2c's Pod Identity work are not
alternatives to each other, they are two different planes that happen to share the word
"identity."

**The group name is the contract across every plane, so it is chosen once and never
varies:** `platform:<team>:<tier>` — e.g. `platform:team-a:developer`,
`platform:team-a:oncall`, `platform:admin`. Tier is part of the group string, not a
separate attribute, because every consumer below (Kubernetes RBAC, `argocd-rbac-cm`,
Grafana's `role_attribute_path`, Backstage entity ownership, IAM Identity Center
permission sets) can only match on the group string it receives — see Phase 7.8's table in
`README.md` for all five wired at once.

---

## 🛠️ Code Conventions & Scaffolder Standards

### 1. Templating Engine Rules
- **Template Delimiters**: Use Go `text/template` double square brackets `[[ .FieldName ]]` across shared templates in `1-platform-catalog/`. This leaves Helm's `{{ }}` untouched, so chart templates pass through verbatim. `.Delims()` must be called *before* `.Parse()` — chaining it after silently does nothing.
- **File Extensions**: Use `.tmpl` for template files (do not use `.jinja` or `.jinja2`).
- **File Names**:
  - `[[ .TeamName ]]` for team folders.
  - `[[ .SystemName ]]` for logical-system folders.
  - `[[ .AppName ]]` for app folders / files.
- **Scoped Template Data**: Give each template a view containing exactly what it renders (e.g. `CapabilityView` carries `.Module`/`.Version` for one capability). A template that has to look itself up with `index .Capabilities "postgres"` is a smell — it cannot be reused and a YAML rename breaks it silently.
- **Capability templates must NOT declare `locals`.** Every capability a service requests renders into the *same* Terraform root module directory, and local value names must be unique within a module. Two capability files each declaring `locals { tags = ... }` is a hard `Duplicate local value definition` error, so any service with 2+ capabilities produces Terraform that will not parse. Inline values into the `module` call instead. (This shipped once; `--capabilities postgres,s3` was broken and nothing caught it because the generated Terraform was never run through `terraform validate`.)
- **Both engines must receive the same template fields.** Go builds `CapabilityView` by embedding `Config`, so adding `[[ .Env ]]` to a template works automatically; Python builds an explicit dict in `cli.py` and will raise on `StrictUndefined`. Adding a field to any shared template means updating the Python render data in the same commit.

### 2. Go Scaffolder Conventions (`2-idp-scaffolder/golang/`)
- **CLI Framework**: [Cobra](https://github.com/spf13/cobra) (`cmd/cli/`).
- **Templating Package**: `text/template` (not `html/template`).
- **Catalog Access**: The renderer holds an `fs.FS` (`Renderer.CatalogFS`), not a directory path — that single field is the testability seam that allows `fstest.MapFS` in tests and `//go:embed` later.
- **Path Handling**: Use the `path` package for anything inside the `fs.FS` (slash-separated, unrooted, no `..`); reserve `path/filepath` for real disk writes. Both are slash-separated on macOS, so mixing them compiles, runs, and silently produces wrong paths.
- **Error Handling**: Use explicit `if err != nil` return guards inside `fs.WalkDir` callbacks to avoid `nil` pointer dereferences on `d.IsDir()`. Never let a second `:=` overwrite an unchecked `err`. **`go vet` does not flag discarded error returns** — a bare `r.renderDestinations(...)` compiles clean and swallows the failure. `golangci-lint` with `errcheck` is the tool for this; wiring it up is Phase 1 of the Go TODO.
- **Rendering is table-driven.** `blueprint{src, destKey}` slices drive both verbs through one `renderDestinations` helper. Adding a building block is a table row, not a new `if` block — the previous copy-pasted loops had already drifted apart and produced a real bug.
- **Keep resolution pure.** `templater.Resolve(spec, goldenPath, in Config) (Config, error)` reads nothing outside its parameters. Policy belongs there, not in a cobra `RunE` closure, so `cmd/api/` can reuse it. It clones the incoming capabilities slice — a struct copy shares a slice's backing array, so appending without a copy writes through into the caller's data.
- **Exported methods validate their own inputs.** `RenderService` guards an empty `Runtime` even though the CLI already does: `path.Join` drops empty segments, so the walk would silently target `per-service/apps/runtimes` and render *every* runtime into one directory.

### 3. Python Scaffolder Conventions (`2-idp-scaffolder/python/`)
- **CLI Framework**: [Typer](https://typer.tiangolo.com/). **REST API**: [FastAPI](https://fastapi.tiangolo.com/).
- **Templating**: Jinja2, **not Copier** (dropped — it renders to a directory and cannot return rendered bytes, which blocks Plan-then-Write). Configure it to match Go: `variable_start_string="[["`, `block_start_string="[%"`, `keep_trailing_newline=True`, and `undefined=StrictUndefined` so a typo'd variable fails instead of rendering an empty string.
- **Go→Jinja conversion**: `render.render_template_string()` rewrites `[[ .Var ]]` → `[[ Var ]]` and `[[- if .X ]]` → `[% if X %]` by regex. The `-` trim markers are lost in that conversion, which is why the environment needs `trim_blocks`/`lstrip_blocks` to match Go's whitespace behaviour.
- **Validation at the load boundary**: `catalog.py` mirrors `internal/catalog/catalog.go`, including a `REQUIRED_DESTINATIONS` list that **must stay textually identical to Go's `requiredDestinations`** — if they drift, one engine accepts a catalog the other rejects.
- **Declare your dependencies.** `copier` was imported while absent from `pyproject.toml` and `uv.lock`, working only from a stale local `.venv` — every fresh checkout had a CLI that could not start. Verify with `rm -rf .venv && uv sync && uv run python -c "import cli, api"`.

### 4. Terraform Capability Modules (`enterprise-aws-infrastructure`, `iac-modules-repo/`)
- The modules live in their own repository (ADR 0011), not here. Each is released with its own annotated tag (`<module>-vX.Y.Z`, e.g. `postgres-v2.0.0`). Module git source URLs:
  `git::https://github.com/ok-karthik/enterprise-aws-infrastructure.git//iac-modules-repo/<category>/<module>?ref=<module>-vX.Y.Z`
- Renovate has one manager entry per module, each with a regex versioning that only accepts its own tag prefix, so a `postgres` pin can never be offered an `eks-v1.2.0` upgrade.
- A capability template reads network placement from the SSM discovery contract (`data "aws_ssm_parameter"`), never a hardcoded ID. It declares **no `locals`** and names every `data` block after the capability, because all capability files render into the same root module.
- **`random_string` flags mean "include this class", not "restrict to it".** `upper` defaults to `true`, so setting only `lower = true` does nothing. S3 bucket names and RDS identifiers are lowercase-only — always set `upper = false` for a name suffix. This shipped broken and would have failed at `apply` roughly half the time.
- **Never declare `provider "..." {}` inside a module.** It blocks `count`/`for_each` on the module and prevents clean removal, because Terraform requires the provider config to outlive the resources. Use `required_providers` in the `terraform {}` block; providers are configured once in the root module.
- **Guardrails are not knobs.** Encryption, public-access blocks, versioning, and backup retention are set by the module and not exposed to tenants — that is the argument for a platform module over raw resources. Note `aws_db_instance.backup_retention_period` defaults to `0` in Terraform, i.e. backups off, so it must be set explicitly.
- **Verify generated Terraform, not just generated text.** Render a service with 2+ capabilities, point the module sources at local paths, and run `terraform init -backend=false && terraform validate && terraform fmt -check`.

---

## 🌐 Fleet Routing & Cross-Repo Contract (Phase 18)

- **Clusters are routed by label, never by endpoint.** A cluster is registered as an ArgoCD
  cluster Secret. The tenant ApplicationSet matrix-joins app×env directories with clusters
  whose `environment` label equals the env directory. No matching cluster → the app deploys
  nowhere (locally k3d is `environment: dev`, so `prod` apps do not deploy). Application
  names end in `-<cluster>`. The full label/annotation table is in
  `4-platform-engineering/1-cloud-foundation/README.md` — keep that the single source.
- **The SSM contract has ONE writer:** `governance/discovery-publisher` in
  `enterprise-aws-infrastructure`. The `publish_ssm_parameters` switches on `compute/eks` and
  `network/vpc` are off in live. Never assume a module that owns a value also publishes it.
  Names: `/platform/<env>/<region>/{vpc/id, vpc/database_subnets, eks/cluster_name,
  eks/oidc_provider_arn, eks/cluster_endpoint, eks/cluster_ca_data, eks/karpenter_node_role,
  eks/karpenter_queue_name, ack/cross_account_role_arn}`.
- **Terraform in the foundation repo never calls the Kubernetes API** (keeps offline
  `validate` reliable). So the ArgoCD cluster Secret cannot be written by Terraform; it is
  meant to be assembled in-cluster from SSM via External Secrets.
- **Open, do not describe as done:** (1) the four new SSM keys are being added to
  `discovery-publisher` (its PLAN 2.10); (2) `network/vpc` does not tag subnets
  `karpenter.sh/discovery`; (3) the ExternalSecret that builds the cluster Secret is **not
  written** — it needs a Parameter Store `ClusterSecretStore` (the only store here reads
  Secrets Manager) and the hub→spoke auth (likely ArgoCD `awsAuthConfig`) is unverified.
  Do not put that ExternalSecret under `2-cluster-services/`: it would fail on k3d and break
  `make setup`.
- **Cross-repo wording.** A prompt for the other repo must use *that repo's* plan numbers
  (its phases are its own) and gates that run offline (`make validate` needs AWS creds).
- **Nothing in Phase 18 has run on a cluster.** Verified offline only: both engines
  byte-identical, Go tests, Kyverno, `helm template` of both charts.

## 🚀 Execution & Verification Commands

### Go Scaffolder (`2-idp-scaffolder/golang/`)
```bash
CAT=../../1-platform-catalog        # without this the CLI fetches the catalog from GitHub

go build ./... && go vet ./... && gofmt -l . && go test ./...

go run . onboard-team --catalog-root $CAT --output-root /tmp/out -t payments
go run . add-service  --catalog-root $CAT --output-root /tmp/out -t payments -a checkout \
    --golden-path go-service-postgres --capabilities postgres,s3

# Error paths — every one must exit 1 AND write nothing
go run . add-service ... --golden-path nope          # unknown golden path
go run . add-service ... --runtime doesnotexist      # no such runtime directory
go run . add-service ... --capabilities bogus        # unknown capability
go run . add-service ...                             # neither --runtime nor --golden-path
```

### Python Scaffolder (`2-idp-scaffolder/python/`)
```bash
uv sync
uv run python main.py onboard-team --team-name payments
uv run python main.py add-service --team-name payments --app-name checkout \
    --golden-path go-service-postgres --capabilities postgres,s3

make run-api        # FastAPI on the same engine; /docs for the OpenAPI UI
```

### Verifying a change did not alter output

Check **exit codes and the file tree**, never just stdout — a success message can print
immediately before the failure that matters.

```bash
# Regression check against any commit, without writing into the repo
git worktree add /tmp/wt <ref>
(cd /tmp/wt/2-idp-scaffolder/golang && go run . add-service ... --output-root /tmp/base)
go run . add-service ... --output-root /tmp/new
diff -r /tmp/base /tmp/new
git worktree remove --force /tmp/wt
```

### The two-engine acceptance test

The catalog is only a contract if both engines agree. Python has no `--output-root` yet
(Python TODO Phase 3), so today this needs removing the scratch team from
`3-tenant-repos/` afterwards.

```bash
diff -r /tmp/go-out/<tenant> 3-tenant-repos/<tenant>
```


### Kyverno policies

```bash
kyverno apply 4-platform-engineering/2-cluster-services/security-governance/ \
  $(find 3-tenant-repos/*/gitops-repo -type f -name '*.yaml' ! -name 'values.yaml' | sed 's/^/--resource /')
```
- **Use the CLI version CI pins (v1.11.4).** Homebrew's 1.19.x panics on this tree even
  before any change. Download the release tarball rather than trusting `brew install kyverno`.
- **A policy is not tested until a bad input has failed it.** After changing one, craft a
  violating resource (other tenant's path, `..` traversal, missing tag) and confirm `fail: 1`.
  A pass count that did not rise after adding a rule means the rule matched nothing.
- `restrict-applicationset` uses `foreach` over both the top-level and the `matrix` generator
  shapes; a plain `pattern` cannot see inside a matrix, and Kyverno's `*` matches across `/`.
- `sed -i` on macOS needs `-i ''`; prefer a small Python edit for scripted file changes.
- Go tests can report `(cached)` and hide a stale golden: use `go test -count=1`. There is no
  golden update flag; edit the file under `internal/templater/testdata/`.

## ✅ Resolved CLI Design Decisions & Refinements

1. **Golden-Path as Seed, Capabilities as Override:** (See [ADR 0002](docs/adr/0002-golden-path-and-capabilities.md))
2. **Unified Vocabulary:** (See [ADR 0002](docs/adr/0002-golden-path-and-capabilities.md))
3. **Dev-Only Scaffolding & Gated Promotion:** (See [ADR 0003](docs/adr/0003-dev-only-scaffolding.md))
4. **GitOps Environment Boundary (Dev/Prod):** (See [ADR 0004](docs/adr/0004-gitops-environment-boundary.md))
5. **System is Metadata, Not a Directory (reversed):** (See [ADR 0005](docs/adr/0005-system-as-metadata.md))
6. **Rendered Manifests & Golden Path Delivery:** (See [ADR 0006](docs/adr/0006-single-platform-chart.md))
7. **Promotion Surface (Values vs Rendered):** (See [ADR 0007](docs/adr/0007-values-vs-rendered-promotion.md))
8. **Platform as a Product Philosophy:** (See [ADR 0008](docs/adr/0008-platform-as-a-product.md))
9. **Taxonomy (Team vs. Tenant):** `{tenant}` is the isolation boundary and the directory name (`3-tenant-repos/<tenant>`): namespace, AppProject, quota, and both repos are named after it. A *team* is the human group that owns a tenant (`--owner`), and appears only in CODEOWNERS and the `platform:<team>:<tier>` identity groups. Phase 13 reversed the earlier rule that used `<team>` as the directory. The fixture makes the difference visible: tenant `tenant-a`, owned by `team-a`.

10. **Data-Driven Scaffolder (Catalog Destinations):** The CLI avoids hardcoded output paths. Instead, a `destinations:` ABI mapping block in `1-platform-catalog/catalog.yaml` defines the precise target directories for team blueprints, ApplicationSets, runtimes, service metadata, delivery values, and capabilities. Every key is the literal source directory inside the catalog, so the renderer derives the source path from the key rather than hardcoding both sides. Both scaffolders substitute `{tenant}`, `{app}`, `{env}` before writing (`{system}` was removed along with the system directory level — see decision 5), and `LoadCatalog` validates that every required key exists so a mismatch fails at load rather than mid-render.
11. **Plan-then-Write is the intended architecture, NOT the current one.** Today both engines render and write file-by-file. Go buffers each template in memory before writing it, so a *single* template failure leaves no truncated file — but a failure on file 5 of 10 still leaves four on disk. `--dry-run` is declared in `root.go` and **never read**, so passing it performs a full silent write. Neither engine has an in-memory `Plan` map yet. Getting there (`plan_service(cfg) -> dict[str, bytes]`, then `write_plan`) is Phase 3 in the Go TODO and Phase 2 in the Python TODO, and it is the prerequisite for an honest `--dry-run`, the API's plan endpoint, and in-memory golden tests. Do not describe this as done.
12. **Runtimes are declared, and an undeclared directory is deliberately invisible.** `catalog.yaml` carries a `runtimes:` map alongside `capabilities:`, and `validate()` checks it in both useful directions: a golden path may not name a runtime that is not declared, and a declared runtime must have a directory under `per-service/apps/runtimes/`. `Resolve` applies the same check to an explicit `--runtime`, which never passes through golden-path validation. What is **not** checked — on purpose — is the reverse: a directory that exists but is not listed in `runtimes:` is simply not offered, which is what lets a half-built runtime sit in the tree without being scaffoldable. "Supported" is a platform decision, not a consequence of what happens to be on disk. `TestLoadCatalog_UndeclaredRuntimeDirectoryIsIgnored` guards this; do not "fix" it by adding a reverse check or by auto-discovering directories into `c.Runtimes`. Note the asymmetry with capabilities is principled rather than accidental: a capability entry carries `module` + `version` that the template cannot get from a directory name (the module is remote and independently versioned), whereas a runtime directory is local and self-contained. When runtimes acquire real metadata — base image, default port, deprecation status — the natural next step is a co-located `runtime.yaml` per directory (the Backstage model), not more central YAML.
13. **Generated output is not yet idempotent.** Both engines use truncating writes, so re-running `add-service` overwrites a team's edits to a scaffolded file. Skip-if-exists plus `--force` is planned alongside Plan-then-Write. Copier used to provide `_skip_if_exists` on the Python side; that guarantee was given up when Copier was dropped (Copier renders *to a directory* and cannot return rendered bytes, which is incompatible with Plan-then-Write).

---

14. **Two tenant repos, split on who *writes* a file, not on the technology in it.** `3-tenant-repos/<tenant>/` holds `workloads-repo/` (humans write it: service source plus the Terraform for the infrastructure those services claim) and `gitops-repo/` (CI writes it, ArgoCD reads it). A service and the infrastructure it claims are one unit of change; the gitops repo is separate so ArgoCD never reads source, no bot can write to the repo humans author, and machine commits stay out of source history. Permission separation inside a repo is a `CODEOWNERS` path rule, not another repository. Full reasoning, alternatives, the honest cost, the revisit trigger, and the extraction commands (`git filter-repo` for the workloads repo, `git subtree split` for gitops) are in [ADR 0010](../docs/adr/0010-tenant-repository-topology.md).
15. **Capability modules live in their own repository, pinned by module-scoped tags.** (See [ADR 0011](../docs/adr/0011-capability-modules-external-repo.md))
16. **This repository owns no cloud Terraform.** The cloud foundation is consumed through the SSM parameter contract, not built here. (See [ADR 0012](../docs/adr/0012-platform-repo-owns-no-cloud-terraform.md))
17. **Karpenter is split across the repo boundary.** AWS half (IAM, SQS, EventBridge, discovery tags) in `enterprise-aws-infrastructure`; controller + `EC2NodeClass`/`NodePool` here, per cluster labelled `karpenter: enabled`. Supersedes ADR 0009. (See [ADR 0013](../docs/adr/0013-karpenter-two-halves.md))
18. **Cost attribution is enforced at admission, not reported afterwards.** Every ACK claim must carry `Tenant`, `Service` and `CostCenter` tags and `Tenant` must equal its namespace (`require-cost-tags` Kyverno policy); OpenCost attributes in-cluster spend by namespace. `CostCenter` currently defaults to the tenant name — there is no real cost-centre mapping yet. Terraform claims are gated in `enterprise-aws-infrastructure`.
19. **`make setup` was broken until Phase 18:** `bootstrap`/`clean` referenced a root `bootstrap.yaml` that lives at `4-platform-engineering/bootstrap.yaml`. Fixed, and `bootstrap` now also applies `local/cluster-secret.yaml`. **The full `make setup` has still never been run end to end** (Phase 14 is blocked on Docker, a push, and `asciinema`).

## 🔭 Roadmap — Scaffolder identity (not planned work; direction only)

Nothing here is scheduled. It is recorded so that the *shape* of the answer is decided
before someone reaches for the easy wrong version of it.

### The gap

**The scaffolder has no notion of who is running it.** `--team` is a string, and both
engines trust it. Anyone who can run the binary, or reach the FastAPI endpoint in
`python/api.py`, can scaffold into any team's directory. The repo's own multi-tenancy
story stops at the cluster boundary and does not extend to the tool that *creates*
tenants — which is a gap worth naming out loud, because a reviewer will spot it.

Today that is defensible: the scaffolder writes to a local working tree, and the real
gate is the pull request plus `CODEOWNERS`. **Git review is the authorisation plane.**
That stops being true the moment the API is hosted for more than one person, or a portal
(Backstage) calls it on a user's behalf — at that point the caller's identity is the only
thing standing between one tenant's directory and another's.

### The shape of the fix

The platform will already have an identity provider — Keycloak, per PLAN.md Phase 7 —
issuing group claims that drive Kubernetes RBAC, the ArgoCD `policy.csv`, and
`AppProject` scope. **The scaffolder should be a consumer of that same identity, not a
second one.** Two group systems is the failure mode to avoid; the value of the design is
that one group membership governs every plane.

- **CLI:** OIDC device-authorisation flow (`oauth2 device_code`) against Keycloak. It is
  the right grant type for a terminal — no browser redirect URI, no client secret on
  disk, works over SSH. Cache the token under `~/.config/idp/`, honour `IDP_TOKEN` for
  CI. This is the same flow `argocd login --sso` and `gh auth login` use, which makes it
  a familiar thing to explain rather than a bespoke one.
- **API:** validate the bearer JWT against Keycloak's JWKS endpoint. Do not invent
  sessions.
- **Authorisation, both:** one rule — *the caller's groups must contain the team they are
  scaffolding into.* `--team payments` succeeds only for a member of `payments`. Platform
  admins get a group that bypasses it, for `onboard-tenant`, which by definition cannot be
  authorised by membership in a team that does not exist yet.

### Where it belongs in the code

The CLI is a thin adapter over `internal/catalog` and `internal/templater` — commands in
`cmd/cli/` are ~30–50 lines and hold no logic. That structure is what makes this cheap:
**authentication is a Cobra `PersistentPreRunE` and an `internal/auth` package; it does
not touch the renderer at all.** Authorisation is one comparison between the token's
groups claim and `cfg.Team`, applied at the same boundary. Neither concern belongs
inside `render.go`, and pushing them there would undo the separation that the TODO.md
refactor phases were about.

Ordering: this comes **after** Plan-then-Write (Go TODO Phase 3 / Python TODO Phase 2).
Adding an auth layer on top of an engine that still does partial writes on failure fixes
the less important problem first — an unauthorised caller is a hypothetical today, a
half-written tenant directory is reproducible right now.

### Non-goals

Do not build a user database, a permissions UI, or per-capability entitlements
("tenant-a may request postgres but not s3"). Entitlement belongs in `catalog.yaml` as
data if it is ever wanted, in the same spirit as `destinations:` and `provisioner:` — not
in code, and not in a second policy engine.
