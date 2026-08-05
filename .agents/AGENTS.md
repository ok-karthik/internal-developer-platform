# AGENTS.md - Platform Engineering IDP GitOps Reference Architecture

This repository is an enterprise-grade **Internal Developer Platform (IDP)** blueprint for zero-touch microservice onboarding, GitOps continuous delivery, and infrastructure management.

---

## 🏛️ Repository Architecture & Directory Structure

```
platform-engineering-idp-gitops-reference-architecture/
├── 1-platform-catalog/                 # The platform's offering (Go text/template syntax, [[ ]] delims)
│   ├── catalog.yaml                    # Golden paths, capabilities, and the destinations output contract
│   ├── blueprints/team/<kind>/         # Rendered ONCE per team; MIRRORS its output tree
│   │   ├── apps/CODEOWNERS.tmpl        # kind = apps | infra | gitops, one destination each
│   │   ├── infra/{CODEOWNERS,platform/*.tf}.tmpl
│   │   └── gitops/{CODEOWNERS.tmpl,platform/{team,applicationsets}/}
│   └── building-blocks/                # Composed per service by `add-service`
│       ├── runtimes/<lang>/            # Language scaffolds (go, python, nodejs, java-springboot)
│       ├── service-meta/               # Runtime-agnostic Backstage catalog-info.yaml
│       ├── capabilities/<cap>.tf.tmpl  # Version-pinned Terraform module claims
│       └── delivery/
│           ├── chart/                  # Platform-owned Helm chart, rendered by CI (not the CLI)
│           └── release/                # Per-env values.yaml scaffolded by the CLI
├── 2-idp-scaffolder/                   # IDP Scaffolder Implementations
│   ├── golang/                         # Go implementation of the IDP Scaffolder CLI (Cobra)
│   │   ├── cmd/cli/                    # Cobra commands (`root.go`, `onboard_team.go`, etc)
│   │   └── internal/templater/         # Template rendering engine (`render.go`)
│   └── python/                         # Python implementation of the IDP Scaffolder CLI & REST API
│       ├── cli.py / api.py             # Typer CLI and FastAPI REST endpoints
│       └── utils.py                    # IPAM CIDR allocation & helper functions
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
└── 4-platform-engineering/             # Platform Control Plane Infrastructure
    ├── cloud-services-terraform-modules/ # Reusable AWS Terraform modules (networking, iam, s3, postgres)
    ├── argocd-apps/     # ArgoCD App-of-Apps declarations
    ├── otel/                     # OpenTelemetry collector setup
    └── traefik/                  # Traefik ingress controller setup
```

---

## 🧭 Platform Model & Key Decisions

The scaffolder templates are organised around **platform lifecycle verbs**, not file type. Each verb maps to a `1-platform-catalog/` domain:

| Verb | Catalog source | Idempotency | Produces |
|---|---|---|---|
| `onboard-team` | `blueprints/team/{apps,infra,gitops}/` | once per team | tenancy boundary — AppProject, Namespace + ResourceQuota + LimitRange, default-deny NetworkPolicy, CODEOWNERS, Kyverno PolicyException, team Terraform providers + IAM — plus the team ApplicationSet |
| `add-service` | `building-blocks/**` | repeatable | a golden path — runtime + service-meta + delivery values + capabilities |

**Golden paths** (`catalog.yaml`) compose three pieces: a runtime (`building-blocks/runtimes/<lang>/`), infra **capabilities** (`building-blocks/capabilities/<cap>.tf.tmpl`), and delivery (`building-blocks/delivery/release/`). Capabilities are declarative claims mapped to blessed, version-pinned Terraform modules — e.g. `postgres → aws-postgres@v1.0.2`, `s3 → aws-s3`, `iam → aws-iam`. `building-blocks/service-meta/` is runtime-agnostic and rendered for every service, which is why it is a sibling of `runtimes/` rather than living inside it.

Key decisions:
- **Go is the definitive scaffolder.** The three verbs are implemented in `2-idp-scaffolder/golang/`; the Python scaffolder stays as a legacy/reference implementation in `2-idp-scaffolder/python/`.
- **git-as-PR.** `add-service` writes into the git-tracked `3-tenant-workloads/` tree; the resulting `git diff` simulates the PR that would be opened against a real tenant repo.
- **Monorepo output, polyrepo mapping.** Everything lands under `3-tenant-workloads/<team>/{apps,infra,gitops}/`; in production each of those three maps to a standalone repo (`<org>/<team>-apps`, `-infra`, `-gitops`) under a department subgroup. The mapping is documented here rather than encoded in the directory name, so the monorepo stays tenant-first and `git subtree split --prefix=3-tenant-workloads/<team>/apps` remains the split path.
- **Two naming axes, kept deliberately distinct.** `apps` / `infra` / `gitops` is the **repo kind** (what sort of artifact; each becomes a real repo). `platform` / `apps` inside `infra/` and `gitops/` is **ownership** (`platform/` is platform-owned and CODEOWNERS-protected; `apps/` belongs to the team). `apps` is reused on purpose — it always means team-owned per-service content, and the enclosing repo kind says whether that is source code, Terraform, or Helm values. Ownership therefore reduces to two glob lines.
- **Ownership is enforceable, not just documented.** `onboard-team` writes a CODEOWNERS at each of the three would-be repo roots (`<team>/{apps,infra,gitops}/`), because GitHub honours CODEOWNERS only at a repo root, `.github/`, or `docs/` — nesting it under `platform/` would make it decorative. Each file is two rules: the team owns `*`, then `/platform/` reverts to the platform team (last match wins); the gitops one adds security review on `policy-exceptions.yaml`.
- **Blueprints mirror their output.** `blueprints/team/<kind>/` is laid out exactly like the tree it produces, so the nesting inside a blueprint *is* the path logic and no file needs its own destination rule. Three keys — one per repo kind — replace what would otherwise be one key per output directory.
- **Helm only** for delivery (no Kustomize); **`1-platform-catalog/catalog.yaml`** is the source of truth for golden paths and the capability → module mapping.
- **Output paths live in data, not code.** The `destinations:` table in `catalog.yaml` maps each catalog source directory to its output path template (`{team}`, `{app}`, `{env}`). No Go file contains a hardcoded output path; restructuring `3-tenant-workloads/` is a YAML edit. `LoadCatalog` validates that every required key is present and fails before writing anything.
- **ArgoCD discovery is convention-based, two levels:** a cluster-wide bootstrap ApplicationSet globs `3-tenant-workloads/*/gitops/platform/applicationsets` (one Application per team, applying that team's AppSet); each team AppSet then globs `3-tenant-workloads/<team>/gitops/apps/*/*` (app × env). `add-service` never edits a root app-of-apps file.

> **CLI status:** both verbs (`onboard-team`, `add-service`) are implemented in `2-idp-scaffolder/golang/`. The CLI currently fetches the catalog from GitHub via `go-getter`, so local edits to `1-platform-catalog/` do not take effect until pushed; a `--catalog-root` override is planned. The Python scaffolder is **non-functional** pending a repoint at `1-platform-catalog/`.

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

### 2. Go Scaffolder Conventions (`2-idp-scaffolder/golang/`)
- **CLI Framework**: [Cobra](https://github.com/spf13/cobra) (`cmd/cli/`).
- **Templating Package**: `text/template` (not `html/template`).
- **Catalog Access**: The renderer holds an `fs.FS` (`Renderer.CatalogFS`), not a directory path — that single field is the testability seam that allows `fstest.MapFS` in tests and `//go:embed` later.
- **Path Handling**: Use the `path` package for anything inside the `fs.FS` (slash-separated, unrooted, no `..`); reserve `path/filepath` for real disk writes. Both are slash-separated on macOS, so mixing them compiles, runs, and silently produces wrong paths.
- **Error Handling**: Use explicit `if err != nil` return guards inside `fs.WalkDir` callbacks to avoid `nil` pointer dereferences on `d.IsDir()`. Never let a second `:=` overwrite an unchecked `err`.

### 3. Python Scaffolder Conventions (`2-idp-scaffolder/python/`)
- **CLI Framework**: [Typer](https://typer.tiangolo.com/).
- **REST API**: [FastAPI](https://fastapi.tiangolo.com/).
- **IPAM Engine**: `utils.py` handles deterministic `/16` VPC CIDR allocations saved in `3-tenant-workloads/cloud_vpcs_allocated.yaml`.

### 4. Terraform Cloud Modules Location
- Reusable Terraform modules reside under `4-platform-engineering/cloud-services-terraform-modules/`.
- Module git source URLs:
  `git::https://github.com/ok-karthik/internal-developer-platform.git//4-platform-engineering/cloud-services-terraform-modules/<module_name>?ref=v1.0.2`

---

## 🚀 Execution & Verification Commands

### Go Scaffolder (`2-idp-scaffolder/golang/`)
```bash
# Run CLI onboard-team command
go run . onboard-team --team-name payments
```

### Python Scaffolder (`2-idp-scaffolder/python/`)
```bash
# Run Python CLI
python main.py create --app-name my-app --team-name payments
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
6. **Tri-Repo Rendered Manifests & Golden Path Delivery:** ONE platform-owned chart at `1-platform-catalog/building-blocks/delivery/chart/` serves every service, so a chart fix ships fleet-wide instead of being copy-pasted into N repos. It is therefore a plain chart, not a `.tmpl` — per-app identity comes from the Helm release name plus `nameOverride` in the release values, and its `values.yaml` holds only genuinely universal defaults (non-root, read-only rootfs, dropped capabilities). The CLI scaffolds only per-env `values.yaml` into `<team>/gitops/apps/<app>/<env>/`; no Helm packaging leaks into `<team>/apps/`. CI (`.github/workflows/tenant-workloads-ci-cd.yaml`) discovers work by globbing `3-tenant-workloads/*/gitops/apps/*/*/values.yaml` — a values file existing at that path IS the declaration that the app deploys to that env — builds one image per app, then renders the chart into a sibling `manifests/`. **CI passes no `--set` overrides:** rendered output is a pure function of (chart, values.yaml), so the committed manifests are an honest record of what git says should be running. ArgoCD syncs ONLY `manifests/`.
7. **Promotion Surface (Values vs Rendered):** Promotion is defined as a PR copying `<team>/gitops/apps/<app>/dev/values.yaml` to `prod/values.yaml`. CI reacts to this commit, re-renders the prod plain YAML into `manifests/`, and ArgoCD syncs it.
8. **Platform as a Product Philosophy:** The platform is treated as a product where development teams are the customers. This is enacted through platform behavior, not just folder names: Golden Paths are the UX, the Scaffolder CLI is the self-service portal, and version-pinned Terraform modules are the stable API.
9. **Taxonomy (Team vs. Tenant):** We explicitly use `<team>` as the ownership boundary (e.g., `3-tenant-workloads/<team>`) because teams are the customers of our product. We restrict the word "tenant" to the isolation control plane (`blueprints/team/`, which renders the AppProject/Namespace/NetworkPolicy boundary) rather than using it as the owner name, demonstrating precise usage of infrastructure vs. business vocabulary.

10. **Data-Driven Scaffolder (Catalog Destinations):** The CLI avoids hardcoded output paths. Instead, a `destinations:` ABI mapping block in `1-platform-catalog/catalog.yaml` defines the precise target directories for team blueprints, ApplicationSets, runtimes, service metadata, delivery values, and capabilities. Every key is the literal source directory inside the catalog, so the renderer derives the source path from the key rather than hardcoding both sides. The Go scaffolder substitutes `{team}`, `{app}`, `{env}` before writing (`{system}` was removed along with the system directory level — see decision 5), and `LoadCatalog` validates that every required key exists so a mismatch fails at load rather than mid-render.
11. **Plan-then-Write CLI Architecture:** The Go scaffolder operates on a `Plan-then-Write` architecture (using `fs.FS` and an in-memory `Plan` map). This prevents partial output from *render* failures — nothing is written until every template has rendered successfully. (It is not atomic: if write 5 of 10 fails, four files remain on disk. Real atomicity would need write-to-temp-dir plus `os.Rename`.) It also strictly aligns `--dry-run` with actual execution, and allows unit tests to run entirely in-memory using golden files without touching the local disk.
