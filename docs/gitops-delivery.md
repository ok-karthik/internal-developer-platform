# Monorepo Authoring, Polyrepo Delivery

`3-tenant-workloads/` in this repo is a **simulation**. In a real company, each team would
have its own repos, not a shared folder. This doc explains how the simulated layout turns
into real repos, and why that conversion needed no extra tooling.

## Why one folder simulates many repos

Everything a team owns lives under `3-tenant-workloads/<team>/{apps,infra,gitops}/` in this
repo, so the whole platform can be demonstrated from one checkout. In production, each of
those three subfolders — `apps/`, `infra/`, `gitops/` — would be split out into its own
real repository (source code, Terraform, and deployment manifests are usually owned,
reviewed, and access-controlled separately).

## The conversion is a built-in git feature, not custom code

Turning `3-tenant-workloads/team-a/apps/` into a standalone `team-a-apps` repo doesn't need
a script — `git subtree split` already does exactly this, and the folder layout was chosen
specifically so the split works cleanly:

```console
$ git subtree split --prefix=3-tenant-workloads/team-a/apps
1f8a94e38604a7ff63e3e2d909fab5ecca8279af

$ git ls-tree -r --name-only 1f8a94e3
CODEOWNERS
app-a/catalog-info.yaml
app-a/go.mod
app-a/main.go
```

Run it yourself — these are real commits carrying the full history of that folder, not a
preview. (The commit hash will differ once anything new is added under `team-a/apps/`.)

## What the split removes, on purpose

Inside the monorepo, a file's path is `team-a/apps/app-a/main.go` — the `team-a/` and
`apps/` prefixes exist so you can tell teams and content types apart while looking at the
whole platform at once. Once split into `team-a-apps`, that same file is just
`app-a/main.go` — a repo that's already scoped to one team's application code doesn't need
to repeat that in every path.

`CODEOWNERS` ends up at the root of the split repo, which is the only place GitHub actually
reads it from.

## Why there's no `--layout` flag on the scaffolder CLI

A flag to pick a different output layout would mean maintaining a second code path — one
more thing that can drift out of sync, and one more decision a user can get wrong. Since
`git subtree split` already produces the polyrepo layout for free from the existing
structure, adding a flag would solve a problem that doesn't exist.
