# 🏛️ Golden Path Scaffolder (IDP Control Plane)

A production-ready blueprint engine for zero-touch microservice onboarding, GitOps bootstrapping, and declarative Infrastructure-as-Code (IaC) scaffolding. 

This component acts as the **Control Plane / Developer Experience Layer (Layer 1)** of our Internal Developer Platform (IDP) reference architecture.

---

## 💡 Core Design Patterns & Engineering Architecture

### 1. Two-Pass Template Rendering Pattern
Instead of single-shot text replacement, the scaffolder splits scaffolding into two discrete passes:
* **Pass 1: Common Platform Layout**: Orchestrated by [Copier](https://copier.readthedocs.io/) using `templates/tenant-template/`. This establishes the team's shared workspaces, GitOps repositories, GitHub Action CD pipelines, and environmental directories (`dev/` and `prod/` Terraform configurations).
* **Pass 2: Language-Specific App Blueprint**: Evaluated from `templates/apps-source/` (e.g. Python, Go, Node.js, Spring Boot) to generate the starter microservice code itself under the team's `apps-source/` directory.

### 2. Idempotent Infrastructure Scaffolding
To ensure that product teams can safely add manual configurations (e.g., custom S3 buckets, SQS queues) without the scaffolder overwriting their work on subsequent runs:
* Infrastructure is split into shared `team-base.tf` and app-specific `{{app_name}}.tf` files.
* Copier's `_skip_if_exists` rule protects the shared base files, allowing safe, multi-app scaffolding within the same repository without merge conflicts.

### 3. Automated Dependency Management (Renovate)
The entire platform relies on **RenovateBot** to stay secure and up to date:
* Natively bumps Python (`pyproject.toml`, `uv.lock`) and deployed Terraform workloads.
* Utilizes **Custom Regex Managers** (`renovate.json`) to dynamically scan and update the `vX.Y.Z` Git module references embedded deeply within `.tf.jinja` template files and python scaffolding logic.

### 4. Strictly Validated API/CLI Contracts (Pydantic Layer)
To ensure platform stability and prevent invalid Kubernetes/cloud resource creation, all inputs are parsed and validated by a robust Pydantic data layer (`schemas.py`):
* **Strict Port Validation**: Microservice ports must be within standard non-system ranges (`1024-65535`).
* **RFC-Compliant Naming Rules**: Team and application names must conform to lowercase alphanumeric characters and hyphens (DNS compliance).
* **Type Safety & Cloud Enums**: Restricts selected cloud services to supported platform-managed modules (e.g. `aws-s3`, `aws-postgres`).

### 5. State-Preserving IPAM System
To enable seamless multi-tenant cloud architectures, the scaffolder features a **Deterministic IP Address Management (IPAM)** engine:
* Automatically reads, updates, and saves tenant allocations in a central state file (`3-tenant-workloads/cloud_vpcs_allocated.yaml`).
* Assigns non-overlapping `/16` CIDR blocks (starting from `10.0.0.0/16`) to each new tenant VPC.
* Guarantees network safety and prevents peer-routing collisions during AWS/multi-cloud VPC peering.

### 6. GitOps Separation of Concerns (Helm Source vs Rendered Manifests)
To optimize ArgoCD performance and enhance auditability, the generated repository structure strictly separates source Helm charts from rendered Kubernetes configurations:
* **Source (`helm-charts/`)**: Holds the parameterized Helm templates, `values.yaml`, and `Chart.yaml` for developer modifications.
* **Rendered (`rendered-manifests/`)**: Used by the GitOps agent (ArgoCD). During a CI/CD run, the GitHub Action automatically executes `helm template` against the source and writes flat, raw Kubernetes manifests into the rendered directory, committing them back to Git.

---

## 🛠️ Technology Stack

* **CLI Framework**: [Typer](https://typer.tiangolo.com/) for building self-documenting, type-hinted developer CLIs.
* **API Framework**: [FastAPI](https://fastapi.tiangolo.com/) for exposing REST endpoints (fully documented with OpenAPI/Swagger), allowing easy integration with developer portals like Spotify Backstage.
* **Template Engine**: [Copier](https://copier.readthedocs.io/) to enable Jinja-powered templating with built-in Day-2 upgrade capabilities (declarative updates to downstream scaffolded files when platform templates evolve).
* **Validation Layer**: [Pydantic v2](https://docs.pydantic.dev/) for type safety, validation, and data serialization.

---

## 📂 Directory Structure

```text
1-idp-scaffolder/
├── api.py                    # FastAPI server exposing REST endpoints
├── cli.py                    # Typer command-line interface implementation
├── main.py                   # Package entrypoint (delegates execution to Typer CLI)
├── schemas.py                # Pydantic data models & schema validation rules
├── utils.py                  # Helper functions for filesystem and IPAM calculations
├── pyproject.toml            # Package metadata and CLI entrypoint registrations
├── uv.lock                   # Deterministic package dependency lockfile
└── templates/
    ├── apps-source/          # Language-specific starter microservices
    ├── cloud-services/       # Terraform modules for AWS services
    └── tenant-template/      # Common platform templates (Terraform, GitOps workflows, Helm charts)
```

---

## 🚀 Getting Started & Execution

### 1. Local Installation
Install the scaffolder package locally in your active virtual environment in editable mode. This registers the `scaffolder` command globally within your shell:

```bash
# Install CLI via pip or uv (run from project root)
make install-scaffolder
```

### 2. Scaffold a Service via CLI
To scaffold a new microservice for a team (including dedicated VPC CIDR block, Terraform code, microservice template, Helm charts, and CI/CD pipelines):

```bash
# Provide multiple --cloud-services (or -cs) flags for each service
scaffolder create \
  --app-name my-python-app \
  --app-type python \
  --app-port 8080 \
  --team-name team-a \
  --cloud-services aws-s3 \
  --cloud-services aws-postgres
```

*Note: Typer parses list options by repeating the flag (e.g., `-cs aws-s3 -cs aws-postgres`). Passing comma-separated lists (e.g., `aws-s3,aws-postgres`) is not supported.*

### 3. Alternative: Run as a REST API (FastAPI)
If you prefer exposing the scaffolder as a web service (e.g., to integrate it with a developer portal UI like Spotify Backstage), you can run the FastAPI application:

```bash
# Starts the local FastAPI web server
make run-api
```
Once started, you can access the interactive OpenAPI Documentation at **[http://127.0.0.1:8000/docs](http://127.0.0.1:8000/docs)** to trigger microservice creation via API requests.