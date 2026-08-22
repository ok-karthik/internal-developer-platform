# 3. Capability Modules

Terraform modules **tenants** consume, resolved from `1-platform-catalog/catalog.yaml`'s
`capabilities:` block. Nobody in the platform team applies these directly — they are
rendered into tenant repos (`3-tenant-workloads/<team>/infra/apps/<app>/<env>/`) by the
scaffolder, and the tenant's own CI/pipeline applies them there.

```
3-capability-modules/
└── aws/
    ├── postgres/    # was aws-postgres/ — the aws- prefix is redundant once the
    ├── s3/          #   parent directory is already `aws/`
    ├── iam/
    └── networking/
```

Nested by provider for the same reason as `../1-cloud-foundation/aws/`: this is where the
Phase 10 portability seam would add `azure/` or `gcp/` siblings, and the diff between them
would be the portability story made visible. `catalog.yaml`'s `capabilities_source_base`
points a `git::` source at this directory; `module:` in each capability entry is a path
segment underneath it (e.g. `aws/postgres`), not a name prefix.
