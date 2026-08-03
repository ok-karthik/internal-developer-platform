# Plan: Finish the Scaffolder Refactor (Plan-then-Write + Golden Tests)

## Context

The catalog reorg (commit `6875cf9`) moved `1-idp-scaffolder-templates/` → `1-platform-catalog/`, introduced a
`destinations:` ABI table, and renamed the output tree to `<team>-services` / `<team>-infra` / `<team>-gitops`.
That was the right direction. But the commit shipped with:

- **Four files corrupted by shell escaping** — the `python3 -c` / heredoc edits wrote literal `\"` into
  `catalog.yaml` and all three capability templates. The CLI cannot render a single `.tf` file today.
- **Two downstream consumers left pointing at the old layout** — the cluster bootstrap ApplicationSet and the
  GitHub Actions CI both glob directories that no longer exist. The GitOps chain is dead end-to-end.
- **An Application name collision** introduced by the AppSet glob fix.
- **Incomplete committed output** — `3-tenant-workloads/` was regenerated from a run that produced no
  capabilities and no `catalog-info.yaml`, then committed as if it were the reference example.
- **A Go implementation guide with four design bugs in it**, which you are about to code from.

Outcome: a scaffolder that runs from any directory, renders correctly, proves itself with golden-file tests,
and whose output ArgoCD can actually discover and sync.

**Working agreement:** you write all Go yourself. This plan gives signatures, design rationale, and the traps
— not function bodies. `2-idp-scaffolder/golang/IMPLEMENTATION_PLAN.md` is superseded by this file; Phase 2
below lists exactly where it is wrong so you don't code the bugs in.

---

## Phase 0 — Unbreak what's committed (no Go, ~15 min)

**0.1 `1-platform-catalog/catalog.yaml:6`** — the value is a YAML *plain scalar* containing literal
backslashes:

```yaml
capabilities_source_base: \"git::https://...cloud-services-terraform-modules\"
```

YAML does not process escapes in plain scalars, so the parsed string starts with `\"` and ends with `\"`.
Every generated Terraform `source` would be malformed. Replace with a proper double-quoted scalar (no
backslashes).

**0.2 `building-blocks/capabilities/{postgres,s3,iam}.tf.tmpl`** — same artifact, worse consequence:

```
source = "[[ .CapabilitiesSourceBase ]]/[[ (index .Capabilities \"postgres\").Module ]]?ref=..."
```

Inside a `text/template` action a string literal is `"postgres"`. A backslash is not a valid token there, so
this fails at **Parse** time, not render time. These three files will be rewritten anyway in Phase 2 (see 2.3)
— when you do, they become `[[ .Module ]]` / `[[ .Version ]]` / `[[ .SourceBase ]]` with no `index` and no
quoting at all.

**0.3 `blueprints/system/gitops/applicationset.yaml.tmpl:16`** — the glob was correctly deepened to `*/*`
(app + env), but the Application name was not updated:

```yaml
name: '[[ .TeamName ]]-{{path.basename}}'
```

`path.basename` is now the **env** directory, so every app in a system generates an Application named
`team-a-dev`. All apps in a system collide onto one resource. For path
`3-tenant-workloads/team-a-gitops/systems/product-system/app-a/dev`, segments are
`0=3-tenant-workloads 1=team-a-gitops 2=systems 3=product-system 4=app-a 5=dev`. Use
`'[[ .TeamName ]]-{{path[4]}}-{{path.basename}}'` (this file uses fasttemplate syntax; the goTemplate
equivalent would be `{{index .path.segments 4}}`).

---

## Phase 1 — Make the CLI location-independent (Go, small)

This is a prerequisite for testing *and* a real bug: today the CLI only works if you `cd 2-idp-scaffolder/golang`
first. A binary on a developer's laptop is broken.

In `cmd/cli/root.go`, add two persistent flags — `--catalog-root` and `--output-root` — defaulting to a
discovery walk: from `os.Getwd()`, walk up until you find a directory containing `1-platform-catalog/catalog.yaml`;
that's the repo root. Resolve both roots once in a `PersistentPreRunE` and hand them to the `Renderer`.

Rationale: root resolution is a *CLI concern*, not a renderer concern. Once the renderer takes its roots as
inputs, it becomes trivially testable — which is the whole point of Phase 2.

---

## Phase 2 — Plan-then-Write (Go, the main event)

### 2.1 Where `IMPLEMENTATION_PLAN.md` is wrong

Read these before you start; four of them will cost you an hour each if you find them the hard way.

| § | Its instruction | Why it's wrong |
|---|---|---|
| 2d + 3 | Keep `resolveDestination` prepending `../../3-tenant-workloads`, then call `plan.WriteTo("../../3-tenant-workloads")` | Double-prefix. `WriteTo` joins root + key, so you'd get `../../3-tenant-workloads/../../3-tenant-workloads/...`. **`Plan.Files` keys must be output-root-relative** — `resolveDestination` returns a bare relative path and prepends nothing. |
| 2b | `Config.Capabilities` becomes `map[string]catalog.Capability` | Breaks golden-path seeding (`gp.Capabilities` is `[]string`); forces each template to hardcode its own key via `index`; map iteration is unordered, which fights determinism. See 2.3. |
| 3 | `CatalogFS: os.DirFS("../../1-platform-catalog")` | Re-introduces the CWD dependence the refactor was meant to remove. Use the roots from Phase 1. |
| — | Silent on `{env}` | `resolveDestination` still hardcodes `"dev"`. Make it `Config.Env`, defaulting to `dev`, so the promotion model (AGENTS decisions #3/#4) is expressible and testable. |

### 2.2 Types

```go
type Renderer struct {
    CatalogFS fs.FS             // rooted AT 1-platform-catalog
    Spec      *catalog.Catalog
}

// Plan is every file a verb would produce. Keys are output-root-relative. Nothing has touched disk.
type Plan struct {
    Files map[string][]byte
}

func (r *Renderer) PlanTeam(cfg Config) (*Plan, error)
func (r *Renderer) PlanSystem(cfg Config) (*Plan, error)
func (r *Renderer) PlanService(cfg Config) (*Plan, error)

func (p *Plan) WriteTo(root string) error
func (p *Plan) DiffAgainst(root string) (string, error)  // powers --dry-run
```

### 2.3 The capability data-shape decision (the one real design lesson here)

Don't give a template the whole world and make it dig its own key out. Give each capability template a view
scoped to exactly what it renders:

```go
type CapabilityView struct {
    Config            // embedded — TeamName, AppName, SystemName, Env
    Name       string
    Module     string
    Version    string
    SourceBase string
}
```

`Config.Capabilities` stays `[]string` (ordered, deduped). `PlanService` looks each name up in
`r.Spec.Capabilities`, errors clearly on unknown names, and renders with a `CapabilityView`.

The templates then collapse to:

```
source = "[[ .SourceBase ]]/[[ .Module ]]?ref=[[ .Version ]]"
```

Identical in all three files, no `index`, no nested quoting, no escaping hazard. **Principle: scope the data to
the template.** Templates that need to know their own name in the catalog are a smell.

### 2.4 `io/fs` path rules — the trap that will bite you

`io/fs` paths are **slash-separated, unrooted, no leading `/`, no `..`**, and `.` means the root. `filepath` on
macOS is *also* slash-separated, so mistakes here compile, run, and silently produce wrong keys.

Rule: **`path` package for anything inside the FS; `filepath` only inside `WriteTo`.**

- `filepath.WalkDir` → `fs.WalkDir(r.CatalogFS, sourceDir, …)`
- `filepath.Rel` → `strings.TrimPrefix` + `strings.TrimPrefix(…, "/")`
- `filepath.Join` → `path.Join`
- `os.ReadFile` → `fs.ReadFile(r.CatalogFS, srcPath)`
- source dirs become plain relative strings: `"building-blocks/runtimes/go"`, not `filepath.Join("..","..", …)`

Also drop the `d.Name() == "copier.yml"` check in the walk — those files no longer exist.

**Template-name trap:** `template.ParseFiles`/`ParseFS` register the template under its *basename*, so
`Execute` silently fails unless `template.New(name)` matches it. Reading bytes with `fs.ReadFile` and calling
`template.New(name).Delims("[[","]]").Parse(string(b))` sidesteps it entirely. Do that. (Note `.Delims()` must
be called *before* `.Parse()`.)

### 2.5 Fix the orphaned `catalog-info.yaml.tmpl`

It sits at `building-blocks/runtimes/catalog-info.yaml.tmpl` — a *sibling* of `go/`, `python/` — so a walk
rooted at `building-blocks/runtimes/<runtime>/` never sees it. Confirmed missing from the committed output:
`3-tenant-workloads/team-a-services/product-system/app-a/` has only `go.mod` and `main.go`.

Prefer a structural fix over a special case: move it to `building-blocks/service-meta/catalog-info.yaml.tmpl`
and give it its own `destinations:` entry pointing at `{team}-services/{system}/{app}/`. It is not a runtime,
so it should not live under `runtimes/`. A one-off "also render this file" line in `PlanService` is the thing
you'd have to remember forever.

### 2.6 Be precise about what Plan-then-Write guarantees

`.agents/AGENTS.md` #11 currently claims it "structurally prevents half-scaffolded outputs." That oversells it.
What you actually get: **render failures happen before any write**. If write 5 of 10 fails, four files are
already on disk. Real atomicity needs write-to-temp-dir + `os.Rename`.

Fix the wording in AGENTS.md #11 to say "no partial output from render failures," and note the temp-dir
option as a possible follow-up. Being exact about your own guarantees is a senior habit; overstating them in
an architecture doc is precisely what gets picked apart in a design review.

---

## Phase 3 — `--dry-run` and golden tests

**3.1 `--dry-run`** is `Plan` + `DiffAgainst` instead of `WriteTo`. Not a parallel branch — the same code path
minus the last step. That's what keeps it from drifting.

**3.2 Golden tests** in `internal/templater/render_test.go`, fixtures under `testdata/golden/<golden-path-name>/`
(Go tooling ignores any dir named `testdata`).

- `var update = flag.Bool("update", false, "rewrite golden files")` → `go test ./... -update` regenerates.
- **Derive cases from the catalog, never hardcode them** — range over `r.Spec.GoldenPaths`. Add a golden path
  to `catalog.yaml` without fixtures and the suite fails immediately, so the catalog can't drift from its tests.
- Assert sorted key sets first (clean "missing/extra file" message), then per-file contents. Use
  `github.com/google/go-cmp`; whole-tree diffs in one shot are unreadable.
- Renderer under test: `os.DirFS("../../../1-platform-catalog")`.

The payoff worth understanding: **the golden tree is a regression harness for your platform's public API.**
Bump a module `version:` in `catalog.yaml`, run `-update`, and `git diff testdata/` shows every service that
changes. That's your "git diff = the PR" idea turned inward on the platform team's own changes.

---

## Phase 4 — Close the GitOps loop (currently dead)

Neither of these was touched by the reorg; both point at directories that no longer exist. Until they're
fixed, nothing you generate is discoverable or deployable.

**4.1 `4-platform-engineering/argocd-apps/gitops-orchestration/applicationset-tenant-apps.yaml:12`**
globs `3-tenant-workloads/*/gitops-repo/systems/*`. New layout is `3-tenant-workloads/*-gitops/platform/systems/*`.
Also `{{path[1]}}` in the name template was `<team>` and is now `<team>-gitops`.

**4.2 `.github/workflows/tenant-workloads-ci-cd.yaml:47`** loops
`3-tenant-workloads/*/gitops-repo/helm-charts/*` — gone entirely. This is the missing half of AGENTS decision
#6, so rewrite it to match the new model rather than patching paths:

- discover `3-tenant-workloads/*-gitops/systems/*/*/*/values.yaml` (team, system, app, env)
- source is `3-tenant-workloads/<team>-services/<system>/<app>/`
- render **one platform-owned chart** — `1-platform-catalog/building-blocks/delivery/chart/` — with that
  `values.yaml`, output to `<same dir>/manifests/`
- that path is exactly what the per-system AppSet syncs (`{{path}}/manifests`), which is what makes the
  contract closed

**4.3 Regenerate `3-tenant-workloads/` once Phases 0–2 pass.** The committed tree is from a run that emitted
no capabilities and no `catalog-info.yaml` — `team-a-infra/` contains only `platform/`. Delete and regenerate
so the reference output is actually reference-quality.

---

## Phase 5 — Docs

**5.1 `.agents/AGENTS.md` is now self-contradictory** and it's the first file every agent reads. Items 10–11
were appended, but the body still documents `1-idp-scaffolder-templates/`, `tenant-foundation/`, `components/`,
`golden-paths.yaml`, `copier.yml`, `apps-source/infra-repo/gitops-repo`, decision #6 pointing at
`.../components/delivery/standard-helm`, and a "CLI status (mid-migration)" note about a `create` command that
no longer exists. Rewrite the directory tree, the verb table, and decisions #6 and #9 against reality.

**5.2 `README.md`** still shows `go run . create --app-name …` and the old layer descriptions.

**5.3 Python scaffolder — deferred by decision.** `2-idp-scaffolder/python/` lost its `templates/` tree in the
reorg and cannot run. You're repointing it at `1-platform-catalog` in a few weeks. Until then, add one line to
its README and the AGENTS entry marking it non-functional pending repoint, so the docs don't lie in the
interim.

---

## Verification

```bash
# Phase 0
python3 -c "import yaml;print(repr(yaml.safe_load(open('1-platform-catalog/catalog.yaml'))['capabilities_source_base']))"
# must print a clean git:: URL — no backslashes, no embedded quotes

# Phase 1-2 — from a directory that is NOT 2-idp-scaffolder/golang, to prove root discovery
cd /tmp && go run <repo>/2-idp-scaffolder/golang onboard-team --team-name payments --dry-run

# full happy path
scaffolder onboard-team  --team-name payments
scaffolder create-system --team-name payments --system-name checkout
scaffolder add-service   --team-name payments --system-name checkout --app-name checkout-api \
                         --golden-path go-service-postgres
# expect, among others:
#   3-tenant-workloads/payments-infra/checkout/checkout-api/dev/postgres.tf
#     with source = "git::https://...cloud-services-terraform-modules/aws-postgres?ref=v1.0.2"
#   3-tenant-workloads/payments-services/checkout/checkout-api/catalog-info.yaml
#   3-tenant-workloads/payments-gitops/systems/checkout/checkout-api/dev/values.yaml

# error paths
scaffolder add-service --team-name payments --system-name nope --app-name x --golden-path go-service-postgres
#   → should name create-system in the error, not stack-trace
scaffolder add-service ... --capabilities bogus
#   → "unknown capability: bogus"

# Phase 3
cd 2-idp-scaffolder/golang && go test ./... -update && git diff --stat testdata/   # review, then commit
go test ./...                                                                       # must pass clean

# Phase 4
make setup && argocd app list     # expect one Application per app/env, names NOT colliding on "-dev"
```

## Explicitly out of scope

- Repointing the Python scaffolder (deferred by decision — Phase 5.3 only adds a "non-functional" note)
- `huh`/`lipgloss` interactive UI (revisit after golden tests pass)
- Real atomic writes via temp-dir + rename (noted in 2.6 as a follow-up)
- Merging `create-system` into `add-service` — decided against; the verbs sit on opposite sides of a
  permission boundary, and `os.Create` truncation would make `add-service` silently revert hand-tuned
  ApplicationSets
