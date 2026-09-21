# Monorepo Authoring, Polyrepo Delivery

`3-tenant-repos/` in this repo is a **simulation**. In a real company, each tenant would
have its own repositories, not a shared folder. This doc explains how the simulated layout
turns into real repos, and why that conversion needs no custom tooling. The decision itself
(two repos, split by who writes the file) is [ADR 0010](adr/0010-tenant-repository-topology.md);
this page is the practical "how do I actually split it".

## Why one checkout simulates many repos

Everything a tenant owns lives under `3-tenant-repos/<tenant>/` in this repo, so the whole
platform can be demonstrated from one checkout. That directory holds **two** more
directories, and each one is a real repository in production:

```
3-tenant-repos/tenant-a/
├── workloads-repo/   →  <org>/tenant-a-workloads   humans write it: service source + Terraform claims
└── gitops-repo/      →  <org>/tenant-a-gitops      CI writes it, ArgoCD reads it
```

The `-repo` suffix means exactly one thing: *this directory is a separate repository in
production.* Directories without the suffix are plain directories inside whatever repo
contains them.

The split is on **who writes a file**, not on what technology is in it. A service and the
infrastructure it claims are one unit of change, so they share a repo. The gitops repo is
separate so ArgoCD never gets read access to source code, and so the CI token that pushes
rendered manifests cannot touch code or IAM.

## The conversion uses standard git tooling

Both extractions keep history. They are documented, not run: `3-tenant-repos/` stays an
authoring simulation, and the tree is identical either way.

Each repo is a single directory prefix, so the built-in `git subtree split` produces either one:

```bash
git subtree split --prefix=3-tenant-repos/tenant-a/gitops-repo    -b tenant-a-gitops
git subtree split --prefix=3-tenant-repos/tenant-a/workloads-repo -b tenant-a-workloads
```

If you would rather get a whole new repository than a branch, `git filter-repo` (run on a
fresh clone) does the same with one path and one rename:

```bash
git filter-repo \
  --path 3-tenant-repos/tenant-a/workloads-repo \
  --path-rename 3-tenant-repos/tenant-a/workloads-repo/:
```

These need committed history under those paths to have anything to carry over.

## What the split removes, on purpose

Inside the monorepo, a service's source file is
`3-tenant-repos/tenant-a/workloads-repo/services/app-a/main.go`. The tenant and repo
prefixes exist so you can tell tenants and repo roles apart while looking at the whole
platform at once. Once split into `tenant-a-workloads`, that same file is just
`services/app-a/main.go` — a repo already scoped to one tenant does not repeat that in
every path.

`CODEOWNERS` ends up at the root of each split repo, which is the only place GitHub reads
it from (besides `.github/` and `docs/`). Each generated `CODEOWNERS` header names the repo
it becomes and carries its own extraction command.

## What ArgoCD reads

ArgoCD only ever sees the gitops repo:

- `platform/applicationsets/` — discovered by the cluster-wide bootstrap ApplicationSet
  (`3-tenant-repos/*/gitops-repo/platform/applicationsets`).
- `services/<app>/<env>/manifests/` — synced by the tenant's own ApplicationSet. CI writes
  `manifests/`: the Helm-rendered `rendered.yaml`, plus any ACK capability claims
  (`s3.yaml`, ...) copied from the environment directory. `manifests/` is the only
  directory ArgoCD applies, so `values.yaml` is never treated as a manifest.

## Why there's no `--layout` flag on the scaffolder CLI

A flag to pick a different output layout would mean maintaining a second code path — one
more thing that can drift out of sync, and one more decision a user can get wrong. Output
paths already live in data (`destinations:` in `catalog.yaml`), and standard git tooling
produces the polyrepo layout from the existing structure, so a flag would solve a problem
that doesn't exist.
