# 3. Capability Modules

Terraform modules **tenants** consume, resolved from `1-platform-catalog/catalog.yaml`'s
`capabilities:` block. Nobody in the platform team applies these directly — they are
rendered into tenant repos (`3-tenant-workloads/<team>/infra/apps/<app>/<env>/`) by the
scaffolder, and the tenant's own CI/pipeline applies them there.

```
3-capability-modules/
├── aws/
│   ├── postgres/    # was aws-postgres/ — the aws- prefix is redundant once the
│   ├── s3/          #   parent directory is already `aws/`
│   ├── iam/
│   └── networking/
└── azure/
    └── postgres/    # Phase 10.2 — the portability seam, proved on ONE capability
                      #   and stopped there. Same team_name/app_name/env/tags input
                      #   contract and db_identifier/db_endpoint output contract as
                      #   aws/postgres — diff the two main.tf files to verify.
```

Nested by provider for the same reason as `../1-cloud-foundation/aws/`: the diff between
`aws/` and `azure/` **is** the portability story made visible, and its relative size is
the honest measure of how much of this platform is actually cloud-agnostic (most of
`2-cluster-services/` — nothing here). `catalog.yaml`'s `capabilities_source_base` points
a `git::` source at this directory; `module:` in each capability entry is a path segment
underneath it (e.g. `aws/postgres`), not a name prefix — which is what makes
`azure/postgres` existing alongside `aws/postgres` a one-line `catalog.yaml` change away
from being live, without touching either scaffolder engine. `catalog.yaml` deliberately
still points `postgres` at `aws/postgres`; see the comment there for why the seam is
proved by the module existing, not by flipping the platform's live target.

**What this does not prove:** ACK (`2-cluster-services/aws-controllers/`) is AWS-only —
there is no ACK for Azure or STACKIT, so `s3`/`iam` do not have the same seam `postgres`
does. That is a real, named ceiling, not an oversight — see `README.md`'s "What
cloud-agnostic means here" section.
