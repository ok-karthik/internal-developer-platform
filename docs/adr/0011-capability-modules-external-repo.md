# ADR 0011: Capability Modules Live in the Infrastructure Repo

**Date:** 2026-09-20
**Status:** Accepted

> **Path note (2026-09-21):** `enterprise-aws-infrastructure` later split into `-repo` folders (its ADR 0001): `infrastructure-modules/` is now `iac-modules-repo/`, `infrastructure-live/` is `workloads-live-repo/` (plus `foundation-live-repo/`), and `infrastructure-bootstrap/` is `foundation-live-repo/_bootstrap/`. Paths below use the new names.

## Context

`catalog.yaml` maps a capability a developer asks for (`postgres`) to a version-pinned
Terraform module (`aws/postgres@v2.2.0`). Until now those modules lived in this same
repository, under `4-platform-engineering/3-capability-modules/`, and
`capabilities_source_base` pointed back at this repo's own Git tags.

That works, but it hides the part that matters. In a real company the Terraform modules a
tenant consumes are owned by an infrastructure team in a different repository, on their own
release cadence. The platform team consumes them the same way any other customer does: by
URL and version. Keeping the modules in-repo means the version pin in `catalog.yaml` is
pinning this repo to itself, which proves nothing.

`enterprise-aws-infrastructure` already exists and already holds the company's Terraform
modules under `iac-modules-repo/`, grouped by domain (`compute/eks`, `network/vpc`,
`identity/human-access`) and consumed by its own `workloads-live-repo/` Terragrunt stacks.

## Decision

**Move the AWS capability modules into `enterprise-aws-infrastructure`, and consume them
from this repo by URL and semantic version tag.**

They follow that repo's existing domain-first grouping:

| Capability | New location | `catalog.yaml` `module:` |
|---|---|---|
| `postgres` | `iac-modules-repo/data/postgres/` | `data/postgres` |
| `s3` | `iac-modules-repo/storage/s3/` | `storage/s3` |
| `iam` | `iac-modules-repo/identity/workload-iam/` | `identity/workload-iam` |

`identity/workload-iam` rather than `identity/iam`, because that directory already holds
`human-access`. The two names then explain themselves: one is for people signing in, one
is for pods getting AWS permissions.

`capabilities_source_base` becomes:

```yaml
capabilities_source_base: "git::https://github.com/ok-karthik/enterprise-aws-infrastructure.git//iac-modules-repo"
```

The version pin now resolves against **that** repo's tags, and Renovate tracks a genuinely
external dependency.

**Versions are per module, not per repo.** That repo tags each module separately —
`postgres-v1.0.0`, `eks-v1.2.0`, `vpc-v1.0.0` — so a VPC release does not force a version
bump on a database module that did not change. `catalog.yaml`'s `version:` is passed
through to `?ref=` unmodified, so this costs no template or engine change on this side. It
does cost something in `renovate.json`: the `github-tags` datasource returns every tag in
the repo, so each capability needs its own manager entry with a regex versioning that
matches only its prefix. Without that, Renovate would offer `eks-v1.2.0` as an upgrade for
`postgres`.

## Rationale

1. **The version pin becomes a real contract.** A module bump is now a change in someone
   else's repository that arrives here as a Renovate pull request against `catalog.yaml`.
   That is the actual day-to-day relationship between a platform team and an infrastructure
   team, and it is only demonstrable across a repo boundary.
2. **Provider becomes the repo, not a path segment.** The cloud-portability seam used to be
   the `aws/` vs `azure/` prefix in the module path. With one repository per cloud
   (`enterprise-aws-infrastructure`), switching providers means pointing
   `capabilities_source_base` at a different repository — which is how it actually works
   when infrastructure is owned by a cloud team.
3. **Each repo keeps one job.** This repo is the developer platform: the catalog, the
   scaffolder, the cluster, the tenancy model. The infrastructure repo owns Terraform
   modules and their release process, including `terraform validate`, Checkov, tflint and
   Infracost, which it already runs.

## Consequences

- **`enterprise-aws-infrastructure` must carry version tags.** It has none today, so
  nothing resolves until the first per-module tags are cut and pushed. Without a tag,
  `terraform init` fails on every generated file. Producing `<module>-vX.Y.Z` tags from a
  monorepo is what release-please's manifest mode (or semantic-release with a per-package
  config) is for; whichever is chosen, the consumer side here only sees tags.
- **Renovate's custom manager in this repo changes `depName`** from
  `ok-karthik/internal-developer-platform` to `ok-karthik/enterprise-aws-infrastructure`,
  and gains a per-module regex versioning (see above). The file patterns stay the same:
  `catalog.yaml` and `team-iam.tf.tmpl` are still the only hand-maintained pins in the tree.
- **`terraform validate` coverage for these modules moves to the other repo's CI.** This
  repo keeps validating the *generated* Terraform — the rendered `module` block, its inputs
  and the resolved source URL — which is the part this repo is responsible for.
- **`4-platform-engineering/3-capability-modules/` is deleted entirely.** See below.

## `azure/postgres` is Deleted

`azure/postgres` existed to prove that a second cloud could satisfy the same module
contract (Phase 10). It is removed along with the rest of the directory.

An AWS-named repository is the wrong home for it, and `capabilities_source_base` is a
single string, so it cannot point at two repositories at once. Keeping one unused Azure
module behind in an otherwise empty directory would leave a half-finished seam in the tree
that nothing exercises.

The portability seam itself is unchanged and is now clearer than it was: the provider is
the *repository*, so a second cloud means a second infrastructure repo
(`enterprise-azure-infrastructure`) and a second `capabilities_source_base`. That is how
it works when a cloud team owns the Terraform, and it is recorded here rather than proven
by a module nothing consumes.

**Cost, stated plainly:** this repo loses a runnable demonstration that two providers can
satisfy one contract. Restoring it means standing up a real second repo, which is the
honest version of that demonstration anyway.
