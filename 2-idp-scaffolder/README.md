# 2-idp-scaffolder

The self-service CLI for the platform. It is written in Go with Cobra and has two commands:

- `onboard-tenant` — run once per tenant. Writes the tenant's namespace, network policy, RBAC and ApplicationSet.
- `add-service` — run once per service. Writes the app source, delivery values and infra claims for a golden path.

Who runs it: developers directly, Backstage through the `idp:run-cli` custom action, and CI.

```bash
cd 2-idp-scaffolder

go run . onboard-tenant --catalog-root ../1-platform-catalog --output-root /tmp/idp-out \
                        --tenant-name tenant-a --owner team-a

go run . add-service --catalog-root ../1-platform-catalog --output-root /tmp/idp-out \
                     --tenant-name tenant-a --app-name app-a \
                     --golden-path go-service-postgres
```

`--output-root` is where files land (default: the current directory). `make demo-onboard-tenant` /
`make demo-add-service` write the real demo fixture into `3-tenant-repos/` instead.

## Name rules

Tenant, app, env, system and owner names become folder paths and Kubernetes names, so the CLI
checks them before it writes anything. A name must be 1-40 characters of lowercase letters, digits
and dashes, start with a letter, and not end with a dash:

```
^[a-z]([-a-z0-9]{0,38}[a-z0-9])?$
```

It starts with a letter because Kubernetes Service names require it, and it is capped at 40 so
`<tenant>-<app>-<env>-<cluster>` and Helm's 53-character release limit still fit. A bad name exits 1
and writes no files:

```
Error: app-name "Bad Name: x": must be 1-40 chars of lowercase letters, digits and dashes, start with a letter, and not end with a dash
```

The rule lives in `internal/templater/names.go`.

Tests: `go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./...`

More: [CODE_WALKTHROUGH.md](CODE_WALKTHROUGH.md), [ADR 0015](../docs/adr/0015-go-only-scaffolder.md).
