# ADR 0010: Tenant Repository Topology — Two Repos, Split by Who Writes

**Date:** 2026-09-20
**Status:** Accepted

## Context

Everything a tenant owns lives under `3-tenant-repos/<tenant>/`. That directory is a
simulation: in a real company each part of it would be a separate Git repository. The
question this ADR settles is *how many repos, split along which line, called what*.

The answer has moved twice. The original design made `apps/`, `infra/` and `gitops/` three
separate repo roots. Phase 11 collapsed that to two. Neither the reasoning nor the final
naming was ever written down outside `.agents/AGENTS.md`, so several documents still
describe the three-repo model and disagree with each other.

This ADR is the single place that answers it.

## Decision

**Two repositories per tenant.**

| Repo | Who writes it | What's in it |
|---|---|---|
| `<tenant>-workloads` | humans only | service source code, and the Terraform for the infrastructure those services claim |
| `<tenant>-gitops` | mostly CI, read by ArgoCD | per-environment `values.yaml`, CI-rendered manifests, the tenancy boundary objects |

**The split is on who writes a file, not on what technology is in it.**

In this repository the two repos are visible as directories, so a reader can see the
boundary without reading a doc:

```
3-tenant-repos/<tenant>/
├── workloads-repo/                  ← repo 1: <org>/<tenant>-workloads
│   ├── CODEOWNERS
│   ├── tenant.yaml
│   ├── services/<app>/              ← application source
│   └── infra/
│       ├── platform/                ← platform-owned (providers, team IAM)
│       └── services/<app>/<env>/    ← team-owned capability claims (Terraform)
└── gitops-repo/                     ← repo 2: <org>/<tenant>-gitops
    ├── CODEOWNERS
    ├── platform/
    │   ├── tenancy/                 ← AppProject, Namespace, NetworkPolicy, RBAC
    │   └── applicationsets/
    └── services/<app>/<env>/
        ├── values.yaml              ← team-owned
        └── manifests/               ← written by CI, never by a human
```

**The `-repo` suffix is a convention, and it means one thing:** this directory is a
separate repository in production and only looks like a folder because this repo teaches
the whole platform from one checkout. The container is named `3-tenant-repos/` for the same
reason — it says at the top level that what is inside are repositories, not folders. A directory without the suffix is a plain directory
inside whatever repo contains it — `1-platform-catalog/` needs no marker, because this
repository *is* a real repository.

The tenant directory already carries the tenant name, so the suffix carries the rest:
`workloads-repo/` becomes `<org>/<tenant>-workloads`. Each root's `CODEOWNERS` header names
the repo it becomes and carries the command that extracts it, so the directory explains
itself without anyone finding this ADR.

Every tree now uses the same two words for ownership: `platform/` is platform-owned and
protected by `CODEOWNERS`, `services/` belongs to the tenant's team.

## Rationale

**Why `services/` and `infra/` live together.** A service and the infrastructure it claims
are one change. "This service now needs a bucket" should be one pull request, one review,
one revert. Split across repos it becomes two pull requests that have to land in the right
order, and a rollback that nobody can do atomically. That ordering problem is where most
"the app and its infra drifted apart" incidents come from.

**Why `gitops/` is its own repo.** Three reasons, in order of weight:

1. **ArgoCD never gets read access to source code.** One ArgoCD instance serves every
   tenant. If manifests sat next to source, ArgoCD would hold read credentials for every
   tenant's source repo. Kept separate, it only ever reads desired state.
2. **No bot holds write access to the repo humans author.** The CI token that pushes
   rendered manifests is scoped to a repo with no source code and no Terraform in it. If
   that token leaks, an attacker can change what is deployed but cannot inject code or
   touch IAM.
3. **Bot commits stay out of the source history.** Roughly half the commits in the gitops
   tree are machine-generated. Merged with source, `git log` on the application becomes
   unreadable. This is the practical reason teams that try a single repo back out of it.

**Permission separation needs a path, not a repository.** One `CODEOWNERS` at each repo
root does the job: the team owns `*`, and `/infra/platform/` reverts to the platform team
(last match wins). GitHub's "require review from Code Owners" branch rule turns that into
an enforced approval. Note this only works at a repo root, `.github/` or `docs/` — a
`CODEOWNERS` nested anywhere else is ignored, which is why the file sits at
`<tenant>/workloads-repo/` and not inside it.

**The honest cost of merging `services/` and `infra/`.** The repo that runs dependency
downloads and test execution is now the same repo whose workflows can request the
Terraform apply role. That is mitigated, not eliminated: OIDC tokens are issued per job,
so a job without `id-token: write` cannot get the role; the apply is gated behind a GitHub
Environment with required reviewers; and `CODEOWNERS` covers `.github/workflows/` so the
workflow definitions are protected too.

## Alternatives Considered

**A. Three repos — `apps`, `infra`, `gitops`** (the original design). Rejected. It costs
three repos per tenant — sixty at twenty tenants — and buys nothing that a `CODEOWNERS`
path rule doesn't already give. Its real cost is the triple pull request: one feature that
needs a database touches three repos in a fixed order.

**B. One repo per app, plus a combined `infra` + `gitops` platform repo.** Rejected. This
is the layout most often recommended, on the grounds that Terraform and the Kubernetes
manifests that consume its outputs are tightly coupled. Three problems:

- It does not fix the coupling it claims to fix. Its own worked example — a feature that
  needs a new database — is still two pull requests in two repos: the code in the app repo,
  the `postgres.tf` in the platform repo. It optimised the wrong pair.
- The Terraform-to-Kubernetes coupling is already automated away here. Capability claims
  are generated from `catalog.yaml` templates, and database credentials reach a pod through
  External Secrets, not through a human copying a Terraform output into `values.yaml`.
  `values.yaml.tmpl` contains no credential wiring at all.
- One repo per app reintroduces the sprawl it set out to remove: twenty tenants with three
  services each is sixty source repos plus twenty platform repos.

It also puts Terraform in the same repo ArgoCD reads and CI bots write to, which is worse
than the shipped model on all three of the gitops reasons above.

**C. One repo per tenant, everything in it.** Rejected. CI would have to commit rendered
manifests back into the repo that triggers CI, which risks loops and needs `[skip ci]`
discipline to stay safe. It also fails all three gitops reasons at once.

## Known Seam

Capability claims are routed by `provisioner:` in `catalog.yaml`. Terraform capabilities
(`postgres`) land in the workloads repo; ACK capabilities (`s3`, `iam`) are Kubernetes
resources, so they land in the gitops repo where ArgoCD can apply them.

This means "one pull request for a service and the infrastructure it claims" is true for
Terraform capabilities and not for ACK ones. That is accepted rather than fixed: an ACK
claim *is* desired state, which is exactly what the gitops repo holds. The gitops repo was
never purely machine-written anyway — `values.yaml` is authored by hand.

## Revisit Trigger

Go back to three repos if a security requirement appears that the cloud apply role must be
assumable **only** from a repository containing no third-party code.

GitHub's OIDC `sub` claim can be scoped to a repository, a ref, an environment or a
workflow — never to a path inside a repository. So within one repo this separation is
defence in depth, while across two repos the IAM trust policy names a different repository
and the boundary is enforced by the token itself. Regulated environments do mandate this.
Until one does, two repos.

## Consequences

- `catalog.yaml` `destinations:` gains the repo directory in every output path. No code
  changes — output paths are data (ADR: Decision 10 in `.agents/AGENTS.md`).
- The gitops tree keeps its depth (`<tenant>/gitops-repo/…`), so the CI `awk` field
  positions survive unchanged; the directory is renamed, so both ArgoCD discovery globs do
  change. The Kyverno
  `restrict-applicationset` policy, both CI workflows and the Makefile demo targets still
  need checking, because they match on tenant paths.
- Extraction still preserves history. The workloads repo needs `git filter-repo` because
  `git subtree split` takes only one prefix:

  ```bash
  # repo 1 of 2 — <org>/payments-workloads: application source + its infra claims
  git filter-repo \
    --path 3-tenant-repos/payments/workloads-repo \
    --path-rename 3-tenant-repos/payments/workloads-repo/:

  # repo 2 of 2 — <org>/payments-gitops: desired state; ArgoCD reads it, CI writes it
  git subtree split \
    --prefix=3-tenant-repos/payments/gitops-repo -b payments-gitops
  ```

  Both preserve history. Neither is run today — `3-tenant-repos/` stays an authoring
  simulation, and the tree is identical either way, so this is a recorded decision rather
  than a refactor.

- On hosting: GitHub is flat, so the hierarchy survives only as a naming convention
  (`<org>/payments-workloads`, `<org>/payments-gitops`). GitLab nests via subgroups, so
  `<org>/tenants/payments/{workloads,gitops}` maps this tree one-to-one and group
  membership becomes the ownership model. GitLab reads `CODEOWNERS` from the same three
  locations, so nothing else changes.
