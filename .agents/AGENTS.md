# AGENTS.md - Platform Engineering IDP GitOps Reference Architecture

This repository is an enterprise-grade **Internal Developer Platform (IDP)** blueprint for zero-touch microservice onboarding, GitOps continuous delivery, and infrastructure management.

---

## 🏛️ Repository Architecture & Directory Structure

```
platform-engineering-idp-gitops-reference-architecture/
├── 1-idp-scaffolder-templates/         # Capability-driven template catalog (Go text/template syntax)
│   ├── tenant-foundation/              # Tenancy boundaries (AppProject, namespaces, team IAM)
│   ├── systems/                        # Logical groupings & auto-discovery (ApplicationSets)
│   ├── components/                     # Microservice capabilities (runtimes, infra, delivery)
│   ├── golden-paths.yaml               # Paved-road definitions mapping runtimes & capabilities
│   └── copier.yml                      # Copier configuration (skipped by Go scaffolder)
├── 2-idp-scaffolder/                   # IDP Scaffolder Implementations
│   ├── golang/                         # Go implementation of the IDP Scaffolder CLI (Cobra)
│   │   ├── cmd/cli/                    # Cobra commands (`root.go`, `onboard_team.go`, etc)
│   │   └── internal/templater/         # Template rendering engine (`render.go`)
│   └── python/                         # Python implementation of the IDP Scaffolder CLI & REST API
│       ├── cli.py / api.py             # Typer CLI and FastAPI REST endpoints
│       └── utils.py                    # IPAM CIDR allocation & helper functions
├── 3-tenant-workloads/                 # Simulated tenant monorepo target directory (Monitored by ArgoCD)
│   └── <team_name>/                    # Per-team workloads (GitOps helm charts & Terraform infra)
└── 4-platform-engineering/             # Platform Control Plane Infrastructure
    ├── cloud-services-terraform-modules/ # Reusable AWS Terraform modules (networking, iam, s3, postgres)
    ├── argocd-apps/     # ArgoCD App-of-Apps declarations
    ├── otel/                     # OpenTelemetry collector setup
    └── traefik/                  # Traefik ingress controller setup
```

---

## 🧭 Platform Model & Key Decisions

The scaffolder templates are organised around **platform lifecycle verbs**, not file type. Each verb maps to a `1-idp-scaffolder-templates/` domain:

| Verb | Template domain | Idempotency | Produces |
|---|---|---|---|
| `onboard-team` | `tenant-foundation/` | once per team | tenancy boundary — AppProject, Namespace + ResourceQuota + LimitRange, default-deny NetworkPolicy, CODEOWNERS, Kyverno PolicyException |
| `create-system` | `systems/` | once per system | per-team ArgoCD ApplicationSet (Git-generator auto-discovery) |
| `add-service` | `components/` | repeatable | a golden path — runtime + capabilities + delivery — plus a Backstage `catalog-info` |

**Golden paths** (`golden-paths.yaml`) compose three pieces: a runtime (`components/runtimes/<lang>`), infra **capabilities** (`components/infra/<cap>.tf.tmpl`), and delivery (`components/delivery/`). Capabilities are declarative claims mapped to blessed Terraform modules — e.g. `postgres → aws-postgres`, `s3 → aws-s3`, `iam → aws-iam`.

Key decisions:
- **Go is the definitive scaffolder.** The three verbs are implemented in `2-idp-scaffolder/golang/`; the Python scaffolder stays as a legacy/reference implementation in `2-idp-scaffolder/python/`.
- **git-as-PR.** `add-service` writes into the git-tracked `3-tenant-workloads/` tree; the resulting `git diff` simulates the PR that would be opened against a real tenant repo.
- **Monorepo output, polyrepo mapping.** Everything lands under `3-tenant-workloads/<team>/{apps-source,infra-repo,gitops-repo}/`; in production each maps to a standalone repo under a department subgroup.
- **Helm only** for delivery (no Kustomize); **`golden-paths.yaml`** is the source of truth for the capability → module mapping.
- **ArgoCD discovery is convention-based:** a per-team ApplicationSet globs `apps/<system>/*`, and a cluster-wide app-of-appsets bootstrap globs `3-tenant-workloads/*/gitops-repo/systems/*`. `add-service` never edits a root app-of-apps file.

> **CLI status (mid-migration):** the current Go `create` command still targets the pre-restructure template layout. The `onboard-team` / `create-system` / `add-service` verbs above are the planned CLI surface, not yet implemented.

---

## 🛠️ Code Conventions & Scaffolder Standards

### 1. Templating Engine Rules
- **Template Delimiters**: Use Go `text/template` double square brackets `[[ .FieldName ]]` across shared templates in `1-idp-scaffolder-templates/`.
- **File Extensions**: Use `.tmpl` for template files (do not use `.jinja` or `.jinja2`).
- **File Names**:
  - `[[ .TeamName ]]` for team folders.
  - `[[ .SystemName ]]` for logical-system folders.
  - `[[ .AppName ]]` for app folders / files.
- **Ignored Files**: `copier.yml` / `copier.yaml` are exclusively for Python Copier execution; the Go scaffolder explicitly skips them during `filepath.WalkDir`.

### 2. Go Scaffolder Conventions (`2-idp-scaffolder/golang/`)
- **CLI Framework**: [Cobra](https://github.com/spf13/cobra) (`cmd/cli/`).
- **Templating Package**: `text/template` (not `html/template`).
- **Path Handling**: Always compute relative paths using `filepath.Rel(sourceDir, srcPath)` before running `renderPath` on variable directory/file names.
- **Error Handling**: Use explicit `if err != nil` return guards inside `filepath.WalkDir` callbacks to avoid `nil` pointer dereferences on `d.IsDir()`.

### 3. Python Scaffolder Conventions (`2-idp-scaffolder/python/`)
- **CLI Framework**: [Typer](https://typer.tiangolo.com/).
- **REST API**: [FastAPI](https://fastapi.tiangolo.com/).
- **IPAM Engine**: `utils.py` handles deterministic `/16` VPC CIDR allocations saved in `3-tenant-workloads/cloud_vpcs_allocated.yaml`.

### 4. Terraform Cloud Modules Location
- Reusable Terraform modules reside under `4-platform-engineering/cloud-services-terraform-modules/`.
- Module git source URLs:
  `git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//4-platform-engineering/cloud-services-terraform-modules/<module_name>?ref=v1.0.2`

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
   - A golden path (`--golden-path`) seeds a default array of capabilities from `golden-paths.yaml`.
   - Explicit flags (`--capabilities`) can override or extend that seed array. 
   - This provides the "paved road" defaults while retaining the à la carte "menu" flexibility as an escape hatch.
2. **Unified Vocabulary:** The CLI and templates will standardize on the term **capabilities** (e.g., the `--capabilities` flag) rather than mixing in legacy terms like `--cloud-services`, keeping the CLI perfectly aligned with `golden-paths.yaml`.
3. **Dev-Only Scaffolding & Gated Promotion:** When scaffolding infrastructure (e.g., `postgres.tf`), the CLI writes **only** to the `dev/` directory. Production infrastructure never appears automatically; it requires a deliberate, gated promotion step (such as a PR that copies `dev/` to `prod/`).

4. **GitOps Environment Boundary (Dev/Prod):** The GitOps layer will strictly mirror the Terraform infrastructure layer by adopting an "Environment-as-Folders" promotion model. The CLI will scaffold GitOps manifests exclusively into a `dev/` directory (e.g., `systems/<system>/<app>/dev/`). Production manifests never appear automatically; they require a deliberate PR to promote from `dev/` to `prod/`. This physical separation prevents a single shared `Chart.yaml` from triggering premature deployments and perfectly aligns the deployment promotion gate with the infrastructure promotion gate.
5. **System Grouping for C4 Modeling:** The `systems/<system>/` grouping is maintained strictly to demonstrate Backstage C4 modeling (Component -> System) for the showcase. It is *not* technically required by Backstage (which relies on `catalog-info.yaml`) or ArgoCD (which could use labels or file generators), but provides a highly structured enterprise topology.
6. **Tri-Repo Rendered Manifests & Golden Path Delivery:** The Helm chart is strictly platform-owned and lives in `1-idp-scaffolder-templates/components/delivery/standard-helm`. No Helm packaging (`Chart.yaml`) leaks into `apps-source`. The CLI scaffolds only per-env `values.yaml` into the `gitops-repo`. CI then renders the standard chart with these values into raw Kubernetes YAML (`manifests/`). ArgoCD syncs ONLY the rendered `manifests/` directory.
7. **Promotion Surface (Values vs Rendered):** Promotion is defined as a PR copying `gitops-repo/systems/<system>/<app>/dev/values.yaml` to `prod/values.yaml`. CI reacts to this commit, re-renders the prod plain YAML into `manifests/`, and ArgoCD syncs it.
