# 2-idp-scaffolder

The self-service CLI for the platform. It is written in Go with Cobra and has two commands:

- `onboard-tenant` — run once per tenant. Writes the tenant's namespace, network policy, RBAC and ApplicationSet.
- `add-service` — run once per service. Writes the app source, delivery values and infra claims for a golden path.

Who runs it: developers directly, Backstage through the `idp:run-cli` custom action, and CI.

```bash
cd 2-idp-scaffolder

go run . onboard-tenant --catalog-root ../1-platform-catalog \
                        --tenant-name tenant-a --owner team-a

go run . add-service --catalog-root ../1-platform-catalog \
                     --tenant-name tenant-a --app-name app-a \
                     --golden-path go-service-postgres
```

Pass `--output-root /tmp/out` to write somewhere other than the current directory.

Tests: `go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./...`

More: [CODE_WALKTHROUGH.md](CODE_WALKTHROUGH.md), [ADR 0015](../docs/adr/0015-go-only-scaffolder.md).
