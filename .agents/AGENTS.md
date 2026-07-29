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
├── 2-idp-scaffolder-golang/            # Go implementation of the IDP Scaffolder CLI (Cobra)
│   ├── cmd/cli/                        # Cobra commands (`root.go`, `create.go`)
│   └── internal/templater/             # Template rendering engine (`render.go`)
├── 2-idp-scaffolder-python/            # Python implementation of the IDP Scaffolder CLI & REST API
│   ├── cli.py / api.py                 # Typer CLI and FastAPI REST endpoints
│   └── utils.py                        # IPAM CIDR allocation & helper functions
├── 3-tenant-workloads/                 # Simulated tenant monorepo target directory (Monitored by ArgoCD)
│   └── <team_name>/                    # Per-team workloads (GitOps helm charts & Terraform infra)
└── 4-platform-engineering/             # Platform Control Plane Infrastructure
    ├── cloud-services-terraform-modules/ # Reusable AWS Terraform modules (networking, iam, s3, postgres)
    ├── cluster-gitops-argocd-apps/     # ArgoCD App-of-Apps declarations
    ├── otel-setup/                     # OpenTelemetry collector setup
    └── traefik-setup/                  # Traefik ingress controller setup
```

---

## 🛠️ Code Conventions & Scaffolder Standards

### 1. Templating Engine Rules
- **Template Delimiters**: Use Go `text/template` double square brackets `[[ .FieldName ]]` across shared templates in `1-idp-scaffolder-templates/`.
- **File Extensions**: Use `.tmpl` for template files (do not use `.jinja` or `.jinja2`).
- **File Names**:
  - `[[ .TeamName ]]` for team folders.
  - `[[ .AppName ]]` for app folders / files.
- **Ignored Files**: `copier.yml` / `copier.yaml` are exclusively for Python Copier execution; the Go scaffolder explicitly skips them during `filepath.WalkDir`.

### 2. Go Scaffolder Conventions (`2-idp-scaffolder-golang/`)
- **CLI Framework**: [Cobra](https://github.com/spf13/cobra) (`cmd/cli/`).
- **Templating Package**: `text/template` (not `html/template`).
- **Path Handling**: Always compute relative paths using `filepath.Rel(sourceDir, srcPath)` before running `renderPath` on variable directory/file names.
- **Error Handling**: Use explicit `if err != nil` return guards inside `filepath.WalkDir` callbacks to avoid `nil` pointer dereferences on `d.IsDir()`.

### 3. Python Scaffolder Conventions (`2-idp-scaffolder-python/`)
- **CLI Framework**: [Typer](https://typer.tiangolo.com/).
- **REST API**: [FastAPI](https://fastapi.tiangolo.com/).
- **IPAM Engine**: `utils.py` handles deterministic `/16` VPC CIDR allocations saved in `3-tenant-workloads/cloud_vpcs_allocated.yaml`.

### 4. Terraform Cloud Modules Location
- Reusable Terraform modules reside under `4-platform-engineering/cloud-services-terraform-modules/`.
- Module git source URLs:
  `git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//4-platform-engineering/cloud-services-terraform-modules/<module_name>?ref=v1.0.2`

---

## 🚀 Execution & Verification Commands

### Go Scaffolder (`2-idp-scaffolder-golang/`)
```bash
# Run CLI create command without building binary
go run . create --app-name my-app --team-name payments --cloud-services s3,rds
```

### Python Scaffolder (`2-idp-scaffolder-python/`)
```bash
# Run Python CLI
python main.py create --app-name my-app --team-name payments
```
