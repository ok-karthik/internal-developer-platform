# Implementation Plan — what's left

Companion docs:
- **`CODE_WALKTHROUGH.md`** — what the code does *today*, line by line, with real captured I/O.
  Read that first; this file assumes it.
- **`IMPLEMENTATION_PLAN_ANSWERS.md`** — full reference code for the phases below, if you get stuck.

Working agreement: **you write the Go.** This file gives signatures, rationale, and traps — not
bodies.

---

## Already done (was Phases 0, 4, 5)

Listed so the history stays legible; don't redo these.

| Was | Outcome |
|---|---|
| Phase 0 — shell-corrupted `catalog.yaml` + three `.tf.tmpl` | Fixed; capability templates now use `[[ .Module ]]` / `[[ .Version ]]` via `CapabilityView` |
| Phase 0.5 — normalize destinations keys | Done. Every key is now the **literal catalog source directory**, and `LoadCatalog` calls `validate()` so a mismatch fails at load |
| Phase 4.1 — bootstrap AppSet | Globs `3-tenant-workloads/*/gitops/platform/applicationsets`, one Application per team |
| Phase 4.2 — CI rewrite | Discovers `*/gitops/apps/*/*/values.yaml`, builds one image per app, renders the single platform chart into a sibling `manifests/`. No `--set` overrides |
| Phase 4.3 — regenerate `3-tenant-workloads/` | Regenerated from the local catalog; `manifests/` verified against the AppSet's sync path |
| Phase 5 — docs | AGENTS.md, README.md, and this file rewritten against reality |

Structural decisions taken along the way, each recorded in AGENTS.md:

- **Tenant-first layout** — `3-tenant-workloads/<team>/{apps,infra,gitops}/`, one repo kind each.
- **`platform/` vs `apps/`** inside `infra/` and `gitops/` — ownership, not taxonomy.
- **No `<system>/` directory.** Backstage models System as a *relation*; it lives in
  `catalog-info.yaml` via an optional `--system` flag. `create-system` folded into `onboard-team`,
  so the CLI is **two verbs**.
- **Blueprints mirror their output**, so `RenderTenantFoundation` is a loop over three keys with no
  path logic in it.
- **CODEOWNERS at the three would-be repo roots**, where GitHub actually reads it.
- **One platform-owned Helm chart**, a plain chart rather than per-app templates.

---

## Phase 1 — Make the CLI location-independent (Go, small)

Two live bugs, one fix. See `CODE_WALKTHROUGH.md` §12 bug 1 and the §3.2 note.

**1a. `--output-root` is a dead flag.** It's assigned in `PersistentPreRunE` and read nowhere;
`resolveDestination` hardcodes `filepath.Join("../../3-tenant-workloads", dest)`, which resolves
against the **process CWD**. Run the binary from `/tmp` and it writes to `/tmp/3-tenant-workloads`.

**1b. There is no `--catalog-root`.** The catalog is fetched from GitHub by `go-getter`, so local
edits to `1-platform-catalog/` do nothing until pushed, and tests cannot run without network.

Do both together:

```go
rootCmd.PersistentFlags().StringVar(&catalogRoot, "catalog-root", "", "local catalog dir (default: fetch from git)")
rootCmd.PersistentFlags().StringVar(&outputRoot,  "output-root",  "", "where to write (default: <repo>/3-tenant-workloads)")
```

Default both by walking up from `os.Getwd()` until you find a directory containing
`1-platform-catalog/catalog.yaml` — that's the repo root. Resolve once in `PersistentPreRunE`, then:

- `CatalogFS: os.DirFS(catalogRoot)` when the flag is set, else the `go-getter` cache path.
- Store `outputRoot` on the `Renderer` and have `resolveDestination` return a **bare relative path**,
  prepending nothing. Joining happens once, at write time.

> **Why root resolution belongs in the CLI, not the renderer:** once the renderer takes its roots as
> inputs it becomes trivially testable, which is the whole point of Phase 2. A renderer that calls
> `os.Getwd()` can never be tested in parallel.

Also worth two minutes: `rootCmd.SilenceUsage = true`. Right now a catalog validation error dumps the
full usage block, making a runtime failure look like a typo (§12 bug 8).

**Verify** — run from a directory that is *not* the module, and confirm output lands in the repo:

```bash
cd /tmp && go run <repo>/2-idp-scaffolder/golang onboard-team --team-name payments \
  --catalog-root <repo>/1-platform-catalog
```

---

## Phase 2 — Plan-then-Write (Go, the main event)

Today `processSingleTemplate` calls `os.Create` and writes immediately. Two consequences, both real:
a template that fails to parse leaves a zero-byte file behind (§12 bug 7), and `--dry-run` cannot
exist because there is no representation of "what would be written."

### Types

```go
type Renderer struct {
    CatalogFS fs.FS             // rooted AT 1-platform-catalog
    Spec      *catalog.Catalog
}

// Plan is every file a verb would produce. Keys are output-root-relative.
// Nothing has touched disk.
type Plan struct {
    Files map[string][]byte
}

func (r *Renderer) PlanTeam(cfg Config) (*Plan, error)
func (r *Renderer) PlanService(cfg Config) (*Plan, error)

func (p *Plan) WriteTo(root string) error
func (p *Plan) DiffAgainst(root string) (string, error)   // powers --dry-run
func (p *Plan) Paths() []string                            // SORTED — see below
```

`PlanSystem` is gone along with `create-system`.

### The four traps

**Sorted `Paths()`.** Go randomizes map iteration deliberately. Anything that ranges over
`Plan.Files` directly will make golden tests flap intermittently — the worst kind of failure to
debug. Sort once, in one place.

**Collision guard in `add`.** Two templates resolving to the same destination currently means one
silently wins. Make it an error naming both.

**`path` vs `filepath`.** `io/fs` paths are slash-separated, unrooted, no leading `/`, no `..`. On
macOS `filepath` is *also* slash-separated, so mixing them compiles, runs, and produces subtly wrong
keys. Rule: `path` for anything inside the FS, `filepath` only inside `WriteTo`.

**`.Delims()` before `.Parse()`.** Chainable, so calling it after also compiles — and silently does
nothing, leaving literal `[[ .AppName ]]` in the output.

### Fix while you're in there

`fs.ReadFile`'s error is discarded — the next line's `:=` overwrites `err` before it's checked
(§12 bug 6). On a read failure you get an empty template, which parses fine, and a silently empty
output file.

Also: `resolveDestination` still substitutes `{system}`, which no destination uses any more. Delete
it. And add a guard — if the resolved path still contains `{`, return an error naming the
placeholder, so a typo'd destination fails loudly instead of creating a directory called `{regoin}`.

### Be precise about what this buys you

AGENTS.md #11 is already worded correctly, and it's worth keeping that way: Plan-then-Write gives you
**no partial output from render failures**. It is *not* atomic — if write 5 of 10 fails, four files
are on disk. Real atomicity needs write-to-temp-dir plus `os.Rename`. Being exact about your own
guarantees is the habit; overstating them in an architecture doc is what gets picked apart in a
design review.

---

## Phase 3 — `--dry-run` and golden tests

**3a. `--dry-run`** is `Plan` + `DiffAgainst` instead of `WriteTo`. Not a parallel code path — the
same path minus the last step. That's what stops it drifting from real behaviour.

**3b. Golden tests** in `internal/templater/render_test.go`, fixtures under
`testdata/golden/<golden-path-name>/` (Go tooling ignores any directory named `testdata`).

- `var update = flag.Bool("update", false, "rewrite golden files")` → `go test ./... -update`.
- **Derive cases from the catalog, never hardcode them** — range over `r.Spec.GoldenPaths`. Add a
  golden path to `catalog.yaml` without fixtures and the suite fails immediately, so the catalog
  cannot drift from its tests.
- Assert the sorted key set first (clean "missing/extra file" message), then per-file contents. Use
  `github.com/google/go-cmp`; whole-tree diffs in one shot are unreadable.
- Renderer under test: `os.DirFS("../../../1-platform-catalog")`, or `fstest.MapFS` for unit cases.

The payoff worth internalising: **the golden tree is a regression harness for your platform's public
API.** Bump a module `version:` in `catalog.yaml`, run `-update`, and `git diff testdata/` shows
every service that would change. That's "git diff = the PR" turned inward on the platform team.

---

## Remaining known bugs (from `CODE_WALKTHROUGH.md` §12)

Fix these as you pass through; none needs its own phase.

| # | Bug | Fix |
|---|---|---|
| 2 | `--runtime python --golden-path go-…` silently produces Go | `if cfg.Runtime == "" { cfg.Runtime = gp.Runtime }` |
| 3 | `cfg.Capabilities = gp.Capabilities` shares the catalog's backing array | `append([]string(nil), gp.Capabilities...)` |
| 4 | Capability order randomized by map iteration | `sort.Strings` before use — required before golden tests |
| 5 | `{env}` hardcoded to `"dev"` | Add `Config.Env`, flag default `dev` |
| 6 | Unchecked `fs.ReadFile` error | Check it before the `Parse` line |
| 7 | `os.Create` before `Parse` leaves zero-byte files | Removed by Phase 2 |
| 8 | Usage block printed on runtime errors | `rootCmd.SilenceUsage = true` |

---

## Verification

```bash
# Phase 1 — from a directory that is NOT the module, to prove root discovery
cd /tmp && go run <repo>/2-idp-scaffolder/golang onboard-team --team-name payments --dry-run

# Full happy path
scaffolder onboard-team --team-name payments
scaffolder add-service  --team-name payments --app-name checkout-api \
                        --golden-path go-service-postgres --system checkout

# Expect, among others:
#   3-tenant-workloads/payments/{apps,infra,gitops}/CODEOWNERS
#   3-tenant-workloads/payments/gitops/platform/applicationsets/payments.yaml
#   3-tenant-workloads/payments/apps/checkout-api/{go.mod,main.go,catalog-info.yaml}
#   3-tenant-workloads/payments/infra/apps/checkout-api/dev/postgres.tf
#     source = "git::https://…/aws-postgres?ref=v1.0.2"
#   3-tenant-workloads/payments/gitops/apps/checkout-api/dev/values.yaml

# Error paths
scaffolder add-service --team-name payments --app-name x --capabilities bogus
#   → "unknown capability: bogus", and with SilenceUsage, no usage dump

# Phase 3
go test ./... -update && git diff --stat testdata/   # review, then commit
go test ./...                                        # must pass clean
go test -race ./...                                  # catches shared-state bugs early

# The delivery half, end to end
helm lint 1-platform-catalog/building-blocks/delivery/chart
helm template checkout-api 1-platform-catalog/building-blocks/delivery/chart \
  -f 3-tenant-workloads/payments/gitops/apps/checkout-api/dev/values.yaml
```

---

## Explicitly out of scope

- Repointing the Python scaffolder (deferred by your decision; it is marked non-functional in the docs)
- `huh`/`lipgloss` interactive UI — revisit after golden tests pass
- Real atomic writes via temp-dir + rename (noted in Phase 2 as a follow-up)
- The `net/http` API. Note the ordering though: **Plan-then-Write is its prerequisite**, not polish.
  An HTTP handler cannot call code that writes to a CWD-relative path. Once `PlanService(cfg)`
  returns `map[string][]byte`, the handler is trivial. Second: `cfg` and `renderer` are package-level
  mutable globals — concurrent requests would stomp each other's `TeamName`. `renderer` is safe to
  share (read-only after construction); `Config` must become per-request. `go test -race` will prove
  it.
