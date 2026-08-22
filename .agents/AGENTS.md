# AGENTS.md - Platform Engineering IDP GitOps Reference Architecture

This repository is an enterprise-grade **Internal Developer Platform (IDP)** blueprint for zero-touch microservice onboarding, GitOps continuous delivery, and infrastructure management.

---

## 🏛️ Repository Architecture & Directory Structure

```
platform-engineering-idp-gitops-reference-architecture/
├── 1-platform-catalog/                 # The platform's offering (Go text/template syntax, [[ ]] delims)
│   ├── catalog.yaml                    # Golden paths, capabilities, and the destinations output contract
│   ├── per-team/<kind>/                # Rendered ONCE per team by `onboard-team`; MIRRORS its output tree
│   │   ├── apps/CODEOWNERS.tmpl        # kind = apps | infra | gitops, one destination each
│   │   ├── infra/{CODEOWNERS,platform/*.tf}.tmpl
│   │   └── gitops/{CODEOWNERS.tmpl,platform/{team,applicationsets}/}
│   ├── per-service/                    # Composed per service by `add-service` — ALL of it
│   │   │                               # has a destinations key and lands in a tenant repo
│   │   ├── apps/runtimes/<lang>/       # Language scaffolds; only those declared in runtimes: are offered
│   │   ├── apps/service-meta/          # Runtime-agnostic Backstage catalog-info.yaml
│   │   ├── infra/capabilities/<cap>.tf.tmpl    # provisioner: terraform — Terraform module claims
│   │   ├── gitops/capabilities/<cap>.yaml.tmpl # provisioner: ack — ACK CRD claims
│   │   └── gitops/release/             # Per-env values.yaml scaffolded by the CLI
│   └── charts/service/                 # Platform-owned Helm chart. NEVER scaffolded — no
│                                       # destinations key; CI renders it and only the output
│                                       # reaches 3-tenant-workloads/
├── 2-idp-scaffolder/                   # IDP Scaffolder Implementations
│   ├── golang/                         # Go implementation of the IDP Scaffolder CLI (Cobra)
│   │   ├── cmd/cli/                    # Cobra commands (`root.go`, `onboard_team.go`, etc)
│   │   └── internal/templater/         # Template rendering engine (`render.go`)
│   └── python/                         # Python implementation of the IDP Scaffolder CLI & REST API
│       ├── cli.py / api.py             # Typer CLI and FastAPI REST endpoints
│       ├── catalog.py                  # pydantic twin of internal/catalog — loads + validates catalog.yaml
│       ├── render.py                   # Jinja2 engine, roots, IPAM CIDR allocation (was utils.py)
│       └── TODO.md                     # Remaining Python work, phased, with an answer key
├── 3-tenant-workloads/                 # Simulated tenant monorepo target directory (Monitored by ArgoCD)
│   └── <team>/                         # Tenant-first: one directory per team
│       ├── apps/<app>/                 # Application source + catalog-info.yaml
│       │   └── (CODEOWNERS at apps/ root)
│       ├── infra/
│       │   ├── platform/               # Team-wide Terraform (providers, team IAM) — platform-owned
│       │   └── apps/<app>/<env>/       # Per-service capability modules — team-owned
│       └── gitops/
│           ├── platform/               # Platform-owned, CODEOWNERS-protected
│           │   ├── team/               # AppProject, Namespace, NetworkPolicy, PolicyException
│           │   └── applicationsets/    # One <team>.yaml ApplicationSet per team
│           └── apps/<app>/<env>/       # Team-owned values.yaml + CI-rendered manifests/
└── 4-platform-engineering/             # Platform Control Plane Infrastructure — numbered
    │                                     # in order of operations (Phase 3.8). Each of the
    │                                     # four gets its own README.md.
    ├── bootstrap.yaml                   # Root of the App-of-Apps pattern — stays put
    ├── 1-cloud-foundation/              # Terraform the PLATFORM TEAM applies, BEFORE a
    │   │                                 # cluster exists.
    │   ├── aws/                          #   Provider-specific → nested by provider. Only
    │   │   │                             #   the subdirs below are real; the rest are the
    │   │   │                             #   Phase 5/7 target layout, documented not built.
    │   │   ├── organization/             #   Organizations, OUs, SCPs      (Phase 5.4)
    │   │   ├── network/                  #   VPC, subnets, NAT            (Phase 5.1)
    │   │   ├── cluster/                  #   EKS. Held to `plan`-clean.   (Phase 5.1)
    │   │   ├── cluster-access/           #   Access Entries + Identity Ctr(Phase 7.2b)
    │   │   └── workload-identity/        #   Pod Identity / IRSA seam     (Phase 7.2c)
    │   └── local/                        #   k3d. A TEST HARNESS — never the reference.
    ├── 2-cluster-services/               # ArgoCD App-of-Apps declarations (App manifests
    │                                     # per addon) merged with the raw resources some
    │                                     # of them deploy, e.g. ingress-routing/middlewares.yaml,
    │                                     # observability/otel-instrumentation.yaml — one
    │                                     # concept, one directory. Portable Kubernetes →
    │                                     # NOT nested by provider.
    ├── 3-capability-modules/             # Terraform modules TENANTS consume, resolved
    │   └── aws/                          # from catalog.yaml `capabilities:`. Nobody here
    │       ├── postgres/                 # applies these — they are rendered into tenant
    │       ├── s3/                       # repos and applied there.
    │       ├── iam/
    │       └── networking/
    └── 4-platform-apis/                 # Platform API definitions (KRO RGDs / Crossplane
                                          # XRDs). Empty with a README until Phase 5.
```

---

## 🗺️ Render Map — what each catalog directory produces

Answers "what does this directory turn into?" without opening `catalog.yaml`.
`catalog.yaml`'s `destinations:` block remains the **authority**; this table is the
readable projection of it. If they disagree, `catalog.yaml` is right and this table is
stale — fix the table.

Every path below is relative to `1-platform-catalog/` on the left and
`3-tenant-workloads/` on the right.

| You edit this | It renders to | Rendered by | How often |
|---|---|---|---|
| `per-team/apps/` | `{team}/apps/` | `onboard-team` | once per team |
| `per-team/infra/` | `{team}/infra/` | `onboard-team` | once per team |
| `per-team/gitops/` | `{team}/gitops/` | `onboard-team` | once per team |
| `per-service/apps/runtimes/<lang>/` | `{team}/apps/{app}/` | `add-service` | once per service — **one** `<lang>` picked by `--runtime` / golden path |
| `per-service/apps/service-meta/` | `{team}/apps/{app}/` | `add-service` | once per service, always |
| `per-service/infra/capabilities/<cap>.tf.tmpl` | `{team}/infra/apps/{app}/{env}/` | `add-service` | one file per requested capability with `provisioner: terraform` |
| `per-service/gitops/capabilities/<cap>.yaml.tmpl` | `{team}/gitops/apps/{app}/{env}/` | `add-service` | one file per requested capability with `provisioner: ack` |
| `per-service/gitops/release/` | `{team}/gitops/apps/{app}/{env}/` | `add-service` | once per service per env |
| `charts/service/` | **nothing** — never scaffolded | CI, via `helm template` | output only, into `{team}/gitops/apps/{app}/{env}/manifests/` |

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
ACK), `1` = remaining installers (loki, tempo, promtail, argo-rollouts, sealed-secrets),
`2` = namespaced config (ingresses, middlewares, instrumentation, grafana-datasources),
`3` = policies and the tenant `ApplicationSet` — last, so they never gate the platform's
own boot.

**Resolved:** this table used to name two gaps — a `building-blocks/capabilities-ack`
destinations key with no matching directory, and `blueprints/`/`building-blocks/` names
that encoded nothing about *once per team* vs *once per service*. Both are fixed now:
`per-team/` and `per-service/` are the real directory names (the cardinality is in the
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
| `onboard-team` | `per-team/{apps,infra,gitops}/` | once per team | tenancy boundary — AppProject, Namespace + ResourceQuota + LimitRange, default-deny NetworkPolicy, CODEOWNERS, Kyverno PolicyException, team Terraform providers + IAM — plus the team ApplicationSet |
| `add-service` | `per-service/**` | repeatable | a golden path — runtime + service-meta + delivery values + capabilities |

**Golden paths** (`catalog.yaml`) compose three pieces: a runtime (`per-service/apps/runtimes/<lang>/`), infra **capabilities** (`per-service/infra/capabilities/<cap>.tf.tmpl` or `per-service/gitops/capabilities/<cap>.yaml.tmpl`, by `provisioner:`), and delivery (`per-service/gitops/release/`). Capabilities are declarative claims mapped to blessed, version-pinned Terraform modules — e.g. `postgres → aws/postgres@v2.0.0`, `s3 → aws/s3`, `iam → aws/iam`. `per-service/apps/service-meta/` is runtime-agnostic and rendered for every service, which is why it is a sibling of `runtimes/` rather than living inside it.

Key decisions:
- **Go is the definitive scaffolder; Python is a second engine, not a legacy one.** Both implement `onboard-team` and `add-service` against the same `catalog.yaml`. The point of keeping two is that it makes the catalog a *falsifiable* contract: run both with the same inputs and `diff -r` the trees. **The two trees are currently byte-identical**, both verbs, every file. If a change makes them disagree, either the engines drifted or the catalog is under-specified — both are findings, and neither should be papered over. Matching Go's whitespace depends on `trim_blocks`/`lstrip_blocks` in the Python Jinja environment, because the regex that converts `[[- if ]]` to `[% if %]` cannot carry Go's `-` trim markers across.
- **git-as-PR.** `add-service` writes into the git-tracked `3-tenant-workloads/` tree; the resulting `git diff` simulates the PR that would be opened against a real tenant repo.
- **Monorepo output, polyrepo mapping.** Everything lands under `3-tenant-workloads/<team>/{apps,infra,gitops}/`; in production each of those three maps to a standalone repo (`<org>/<team>-apps`, `-infra`, `-gitops`) under a department subgroup. The mapping is documented here rather than encoded in the directory name, so the monorepo stays tenant-first and `git subtree split --prefix=3-tenant-workloads/<team>/apps` remains the split path.
- **Two naming axes, kept deliberately distinct.** `apps` / `infra` / `gitops` is the **repo kind** (what sort of artifact; each becomes a real repo). `platform` / `apps` inside `infra/` and `gitops/` is **ownership** (`platform/` is platform-owned and CODEOWNERS-protected; `apps/` belongs to the team). `apps` is reused on purpose — it always means team-owned per-service content, and the enclosing repo kind says whether that is source code, Terraform, or Helm values. Ownership therefore reduces to two glob lines.
- **IRSA / Pod Identity is the missing fourth wall of the tenancy model.** `onboard-team` builds three walls per namespace: what ArgoCD may deploy (`AppProject`), what pods may talk to (`NetworkPolicy`), and what a human may do with `kubectl` (RBAC `Role`/`RoleBinding`). None of that constrains what AWS a pod's *own* credentials can reach. ACK controllers (`4-platform-engineering/2-cluster-services/aws-controllers/`) grant AWS permissions **per namespace** via IRSA (EKS) or Pod Identity — a `Bucket`/`Role` CR reconciled in `team-a`'s namespace only ever gets `team-a`'s AWS permissions, because the trust policy is scoped to the namespace/service-account pair, not to the controller process as a whole. That is the mechanism that makes ACK safe to run multi-tenant, and it is the reason `AppProject` + `Namespace` + `NetworkPolicy` + RBAC is not yet the complete tenancy boundary — this is the fourth control, expressed in AWS IAM rather than Kubernetes RBAC. It is not yet real in this repo: `per-team/infra/platform/team-iam.tf.tmpl` calls the `aws-iam` module, which is a documented stub (`4-platform-engineering/3-capability-modules/aws/iam/main.tf`) that provisions no identity. Phase 5.1 added real, `terraform validate`-clean EKS Terraform (`1-cloud-foundation/aws/cluster/`) that outputs the OIDC issuer URL IRSA needs — but that Terraform has not been applied to a real account, so the issuer URL does not exist yet either. Wiring real IRSA is therefore gated on an actual `terraform apply`, not on missing code.

- **The full chain, once a real hub cluster exists (Phase 5.1-5.3).** `namespace` (Phase 1) → `IRSA/Pod Identity role` (Phase 7.2c, `1-cloud-foundation/aws/workload-identity/`) → `assumed spoke-account role` (Phase 5.2, `1-cloud-foundation/aws/organization/ack-cross-account.tf`) → blast radius is **one tenant's AWS account**, not just one tenant's IAM policy. This is the fourth wall with a real account boundary behind it: a compromised ACK controller reconciling a `team-a` `Bucket` CR can only ever assume `team-a`'s spoke role, in `team-a`'s spoke account — it has no path to `team-b`'s resources even if `team-b`'s Bucket CR sits in the same hub cluster, because the trust chain (namespace → IRSA role → spoke role) never crosses. The direction matters: **the spoke trusts the hub, never the reverse** — see the comment in `ack-cross-account.tf` for why getting this backwards reopens the confused-deputy problem the `ExternalId` condition exists to close.
- **Ownership is enforceable, not just documented.** `onboard-team` writes a CODEOWNERS at each of the three would-be repo roots (`<team>/{apps,infra,gitops}/`), because GitHub honours CODEOWNERS only at a repo root, `.github/`, or `docs/` — nesting it under `platform/` would make it decorative. Each file is two rules: the team owns `*`, then `/platform/` reverts to the platform team (last match wins); the gitops one adds security review on `policy-exceptions.yaml`.
- **`per-team/` mirrors its output.** `per-team/<kind>/` is laid out exactly like the tree it produces, so the nesting *is* the path logic and no file needs its own destination rule. Three keys — one per repo kind — replace what would otherwise be one key per output directory.
- **Helm only** for delivery (no Kustomize); **`1-platform-catalog/catalog.yaml`** is the source of truth for golden paths and the capability → module mapping.
- **Output paths live in data, not code.** The `destinations:` table in `catalog.yaml` maps each catalog source directory to its output path template (`{team}`, `{app}`, `{env}`). No Go file contains a hardcoded output path; restructuring `3-tenant-workloads/` is a YAML edit. `LoadCatalog` validates that every required key is present and fails before writing anything.
- **ArgoCD discovery is convention-based, two levels:** a cluster-wide bootstrap ApplicationSet globs `3-tenant-workloads/*/gitops/platform/applicationsets` (one Application per team, applying that team's AppSet); each team AppSet then globs `3-tenant-workloads/<team>/gitops/apps/*/*` (app × env). `add-service` never edits a root app-of-apps file.

> **CLI status:** both verbs are implemented in **both** engines and produce matching output.
>
> The Go CLI defaults to fetching the catalog from GitHub via `go-getter`, so local edits to `1-platform-catalog/` do not take effect until pushed — **pass `--catalog-root ../../1-platform-catalog` when working locally.** `--output-root` likewise redirects the generated tree, which is what makes the two-engine `diff` possible without writing into the repo. The fetched ref is still hardcoded to a branch (`root.go`); pinning it is Phase 4 of the Go TODO.
>
> The Python CLI has no root flags yet, so it always writes into the real `3-tenant-workloads/` — Phase 3 of the Python TODO. It also allocates a VPC CIDR into `3-tenant-workloads/cloud_vpcs_allocated.yaml` on `onboard-team`, which Go does not do; the engines are therefore not interchangeable for that verb.

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
- **IPAM Engine**: `render.py` handles deterministic `/16` VPC CIDR allocations saved in `3-tenant-workloads/cloud_vpcs_allocated.yaml`. Python-only; Go has no VPC concept.
- **Declare your dependencies.** `copier` was imported while absent from `pyproject.toml` and `uv.lock`, working only from a stale local `.venv` — every fresh checkout had a CLI that could not start. Verify with `rm -rf .venv && uv sync && uv run python -c "import cli, api"`.

### 4. Terraform Cloud Modules (`4-platform-engineering/3-capability-modules/aws/`)
- Module git source URLs:
  `git::https://github.com/ok-karthik/internal-developer-platform.git//4-platform-engineering/3-capability-modules/aws/<module_name>?ref=v2.0.0`
- **`random_string` flags mean "include this class", not "restrict to it".** `upper` defaults to `true`, so setting only `lower = true` does nothing. S3 bucket names and RDS identifiers are lowercase-only — always set `upper = false` for a name suffix. This shipped broken and would have failed at `apply` roughly half the time.
- **Never declare `provider "..." {}` inside a module.** It blocks `count`/`for_each` on the module and prevents clean removal, because Terraform requires the provider config to outlive the resources. Use `required_providers` in the `terraform {}` block; providers are configured once in the root module.
- **Guardrails are not knobs.** Encryption, public-access blocks, versioning, and backup retention are set by the module and not exposed to tenants — that is the argument for a platform module over raw resources. Note `aws_db_instance.backup_retention_period` defaults to `0` in Terraform, i.e. backups off, so it must be set explicitly.
- **Verify generated Terraform, not just generated text.** Render a service with 2+ capabilities, point the module sources at local paths, and run `terraform init -backend=false && terraform validate && terraform fmt -check`.

---

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
(Python TODO Phase 3), so today this needs backing up `3-tenant-workloads/cloud_vpcs_allocated.yaml`
and removing the scratch team afterwards.

```bash
diff -r /tmp/go-out/3-tenant-workloads/<team> 3-tenant-workloads/<team>
```


## ✅ Resolved CLI Design Decisions & Refinements

1. **Golden-Path as Seed, Capabilities as Override:** The CLI uses a single unified pipeline rather than two competing modes. 
   - A golden path (`--golden-path`) seeds a default runtime and capability array from `catalog.yaml`.
   - Explicit flags (`--capabilities`) can override or extend that seed array. 
   - This provides the "paved road" defaults while retaining the à la carte "menu" flexibility as an escape hatch.
2. **Unified Vocabulary:** The CLI and templates will standardize on the term **capabilities** (e.g., the `--capabilities` flag) rather than mixing in legacy terms like `--cloud-services`, keeping the CLI perfectly aligned with `catalog.yaml`.
3. **Dev-Only Scaffolding & Gated Promotion:** When scaffolding infrastructure (e.g., `postgres.tf`), the CLI writes **only** to the `dev/` directory. Production infrastructure never appears automatically; it requires a deliberate, gated promotion step (such as a PR that copies `dev/` to `prod/`).

4. **GitOps Environment Boundary (Dev/Prod):** The GitOps layer will strictly mirror the Terraform infrastructure layer by adopting an "Environment-as-Folders" promotion model. The CLI will scaffold GitOps manifests exclusively into a `dev/` directory (e.g., `gitops/apps/<app>/dev/`). Production manifests never appear automatically; they require a deliberate PR to promote from `dev/` to `prod/`. This physical separation prevents a single shared `Chart.yaml` from triggering premature deployments and perfectly aligns the deployment promotion gate with the infrastructure promotion gate.
5. **System is Metadata, Not a Directory (reversed):** An earlier revision carried a `<system>/` directory level under `apps/`, justified as an ArgoCD anchor and a Terraform blast-radius boundary. Neither held up: Terraform blast radius is set by where `apply` runs (`<app>/<env>/`), and a team-level AppSet globbing `apps/*/*` discovers apps perfectly well. Backstage models System as a *relation*, so it now lives only in `catalog-info.yaml` (`spec.system`, via the optional `--system` flag) where it cannot drift from a second encoding in the path. Reintroduce a grouping level only when a single team outgrows a flat app list — and because `destinations:` is data, that is a YAML edit.
6. **Tri-Repo Rendered Manifests & Golden Path Delivery:** ONE platform-owned chart at `1-platform-catalog/charts/service/` serves every service, so a chart fix ships fleet-wide instead of being copy-pasted into N repos. It sits in `charts/` rather than `per-service/` because it is the one catalog artefact that is never *copied* into a tenant repo — it has no `destinations` key, and only its rendered output reaches `3-tenant-workloads/`. That keeps one rule true: everything under `per-team/` and `per-service/` gets scaffolded, nothing else does. It is therefore a plain chart, not a `.tmpl` — per-app identity comes from the Helm release name plus `nameOverride` in the release values, and its `values.yaml` holds only genuinely universal defaults (non-root, read-only rootfs, dropped capabilities). The CLI scaffolds only per-env `values.yaml` into `<team>/gitops/apps/<app>/<env>/`; no Helm packaging leaks into `<team>/apps/`. CI (`.github/workflows/tenant-workloads-ci-cd.yaml`) discovers work by globbing `3-tenant-workloads/*/gitops/apps/*/*/values.yaml` — a values file existing at that path IS the declaration that the app deploys to that env — builds one image per app, then renders the chart into a sibling `manifests/`. **CI passes no `--set` overrides:** rendered output is a pure function of (chart, values.yaml), so the committed manifests are an honest record of what git says should be running. ArgoCD syncs ONLY `manifests/`.
7. **Promotion Surface (Values vs Rendered):** Promotion is defined as a PR copying `<team>/gitops/apps/<app>/dev/values.yaml` to `prod/values.yaml`. CI reacts to this commit, re-renders the prod plain YAML into `manifests/`, and ArgoCD syncs it.
8. **Platform as a Product Philosophy:** The platform is treated as a product where development teams are the customers. This is enacted through platform behavior, not just folder names: Golden Paths are the UX, the Scaffolder CLI is the self-service portal, and version-pinned Terraform modules are the stable API.
9. **Taxonomy (Team vs. Tenant):** We explicitly use `<team>` as the ownership boundary (e.g., `3-tenant-workloads/<team>`) because teams are the customers of our product. We restrict the word "tenant" to the isolation control plane (`per-team/`, which renders the AppProject/Namespace/NetworkPolicy boundary) rather than using it as the owner name, demonstrating precise usage of infrastructure vs. business vocabulary.

10. **Data-Driven Scaffolder (Catalog Destinations):** The CLI avoids hardcoded output paths. Instead, a `destinations:` ABI mapping block in `1-platform-catalog/catalog.yaml` defines the precise target directories for team blueprints, ApplicationSets, runtimes, service metadata, delivery values, and capabilities. Every key is the literal source directory inside the catalog, so the renderer derives the source path from the key rather than hardcoding both sides. The Go scaffolder substitutes `{team}`, `{app}`, `{env}` before writing (`{system}` was removed along with the system directory level — see decision 5), and `LoadCatalog` validates that every required key exists so a mismatch fails at load rather than mid-render.
11. **Plan-then-Write is the intended architecture, NOT the current one.** Today both engines render and write file-by-file. Go buffers each template in memory before writing it, so a *single* template failure leaves no truncated file — but a failure on file 5 of 10 still leaves four on disk. `--dry-run` is declared in `root.go` and **never read**, so passing it performs a full silent write. Neither engine has an in-memory `Plan` map yet. Getting there (`plan_service(cfg) -> dict[str, bytes]`, then `write_plan`) is Phase 3 in the Go TODO and Phase 2 in the Python TODO, and it is the prerequisite for an honest `--dry-run`, the API's plan endpoint, and in-memory golden tests. Do not describe this as done.
13. **Runtimes are declared, and an undeclared directory is deliberately invisible.** `catalog.yaml` carries a `runtimes:` map alongside `capabilities:`, and `validate()` checks it in both useful directions: a golden path may not name a runtime that is not declared, and a declared runtime must have a directory under `per-service/apps/runtimes/`. `Resolve` applies the same check to an explicit `--runtime`, which never passes through golden-path validation. What is **not** checked — on purpose — is the reverse: a directory that exists but is not listed in `runtimes:` is simply not offered, which is what lets a half-built runtime sit in the tree without being scaffoldable. "Supported" is a platform decision, not a consequence of what happens to be on disk. `TestLoadCatalog_UndeclaredRuntimeDirectoryIsIgnored` guards this; do not "fix" it by adding a reverse check or by auto-discovering directories into `c.Runtimes`. Note the asymmetry with capabilities is principled rather than accidental: a capability entry carries `module` + `version` that the template cannot get from a directory name (the module is remote and independently versioned), whereas a runtime directory is local and self-contained. When runtimes acquire real metadata — base image, default port, deprecation status — the natural next step is a co-located `runtime.yaml` per directory (the Backstage model), not more central YAML.
14. **Generated output is not yet idempotent.** Both engines use truncating writes, so re-running `add-service` overwrites a team's edits to a scaffolded file. Skip-if-exists plus `--force` is planned alongside Plan-then-Write. Copier used to provide `_skip_if_exists` on the Python side; that guarantee was given up when Copier was dropped (Copier renders *to a directory* and cannot return rendered bytes, which is incompatible with Plan-then-Write).

---

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
thing standing between team-a and team-b's directory.

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
  admins get a group that bypasses it, for `onboard-team`, which by definition cannot be
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
("team-a may request postgres but not s3"). Entitlement belongs in `catalog.yaml` as
data if it is ever wanted, in the same spirit as `destinations:` and `provisioner:` — not
in code, and not in a second policy engine.
