# Implementation Plan — Python scaffolder as a second engine

**Goal:** make `catalog.yaml` a genuine contract by having two independent implementations produce
byte-identical output. That claim is worth being able to make, and it is falsifiable — diff the two
trees.

Companion docs:
- `../golang/CODE_WALKTHROUGH.md` — how the Go engine works, line by line. The Python design mirrors
  it deliberately, so this is the reference for *what* to implement.
- `../golang/IMPLEMENTATION_PLAN_CLAUDE.md` — the Go side's remaining work (Phases 1–3).

**Working agreement: you write the Python.** This file gives signatures, rationale and traps, not
function bodies.

---

## 1. Diagnosis — it is not "broken", it is silently wrong

`utils.py:15` pins `REMOTE_TEMPLATE_REPO` to tag `v1.0.2`. That tag **still contains
`1-idp-scaffolder/`**, so `get_template_base_dir()` clones successfully, returns a real directory,
and Copier renders the v1.0.2 layout (`2-tenant-workloads/`, `apps-source/`, `gitops-repo/`) — an
architecture two refactors old — into the wrong output root. It never raises. Failing loudly would
be a better outcome than this.

### Why it drifted, precisely

`renovate.json:9` matches `^1-idp-scaffolder/utils\.py$`. That path died in the reorg, so the custom
manager that exists **specifically to bump `REMOTE_TEMPLATE_REPO`** has never fired since. The
automation designed to prevent this exact drift was itself pointed at a dead path. `renovate.json:17`
has the same problem: it matches `\.tf\.jinja$`, but capability templates are `.tf.tmpl` now and the
version pin has moved into `catalog.yaml` anyway.

Worth internalising: **a version pin is only as good as the automation that moves it, and that
automation needs its own test.** A Renovate manager matching zero files is indistinguishable from a
healthy one until you look.

### Findings

| # | Location | Problem |
|---|---|---|
| 1 | `utils.py:15` | Pinned to `v1.0.2`; renders an architecture that no longer exists |
| 2 | `utils.py:11` | `TENANT_WORKLOADS_DIR = SCAFFOLDER_PKG_ROOT.parent / "3-tenant-workloads"` resolves to `2-idp-scaffolder/3-tenant-workloads`. The comment still says `1-idp-scaffolder`, from when the file lived one level up |
| 3 | `utils.py:57` | Returns `cache_dir / "1-idp-scaffolder"`; gone from `main` |
| 4 | `utils.py:74` | `list_available_cloud_services()` hardcodes `["aws-postgres","aws-s3"]` — belongs in `catalog.yaml` `capabilities:` |
| 5 | `utils.py:41-45` | Deletes and re-clones the whole repo on **every invocation**; the `--depth 1` is discarded whenever a version is set |
| 6 | `schemas.py:5,7` | Builds `AppType`/`CloudServices` Enums **at import time** from a filesystem scan. A missing directory becomes an ImportError three modules away, and the CLI cannot even print `--help` |
| 7 | `cli.py:30,47` | Copies `templates/tenant-template` and `templates/apps-source/<type>`; writes `<team>/apps-source/<app>/` |
| 8 | `cli.py:63` | One verb `create`, with `--app-type` / `--cloud-services`. Go has `onboard-team` / `add-service` with `--golden-path` / `--capabilities`. Two vocabularies for one platform |
| 9 | `pyproject.toml:2` | `name = "1-idp-scaffolder"` |
| 10 | `Makefile:193,196` | Both targets `cd 1-idp-scaffolder` |
| 11 | `renovate.json:9,17` | Both custom managers match zero files |
| 12 | `README.md` | Documents `templates/`, two-pass Copier rendering, `helm-charts/` vs `rendered-manifests/` — none of which exist |
| 13 | — | No awareness of `catalog.yaml`: no golden paths, no destinations, no capability versions |

---

## 2. The one decision to make first: drop Copier

**Recommendation: replace Copier with plain Jinja2.** The reason is structural, not preference.

Copier's `run_copy(src, dst, …)` renders **to a destination directory**. It cannot return the
rendered files. But the whole point of adopting Plan-then-Write here (see §5) is that
`plan_service(cfg)` returns `dict[str, bytes]` with nothing touching disk — that is what makes
`--dry-run` honest and what lets the FastAPI endpoint return a plan for review instead of mutating
the repo on a GET-adjacent request. Those two requirements are incompatible: **Copier cannot do
Plan-then-Write.**

Jinja2 is already a transitive dependency of Copier, so this removes a direct dependency rather than
adding one. Configure it to match the catalog's Go delimiters:

```python
Environment(
    loader=FileSystemLoader(catalog_root),
    variable_start_string="[[", variable_end_string="]]",
    block_start_string="[%",   block_end_string="%]",
    keep_trailing_newline=True,   # otherwise every rendered file loses its final \n
    undefined=StrictUndefined,    # a typo'd variable must fail, not render as ""
)
```

> `StrictUndefined` is the important one. Jinja's default renders an unknown variable as an empty
> string, so `[[ .AppNam ]]` would silently produce `name: ` in a Kubernetes manifest. Go's
> `text/template` errors by default; this makes Python match.

**What you give up, honestly:** Copier's `_skip_if_exists` idempotency and its Day-2 `copier update`
flow. Both are genuinely nice. But the Go engine has neither, and this repo's model is that output
is regenerated from the catalog rather than updated in place — so keeping Copier for features the
contract doesn't use would be paying a structural cost for nothing. Note the trade-off in the README
rather than pretending it isn't one.

`copier.yml` gets deleted with it.

### Template syntax caveat

The blocks delimiter matters. `catalog-info.yaml.tmpl` already contains Go's
`[[- if .SystemName ]] … [[- end ]]`, which Jinja **cannot** parse — Jinja needs
`[% if … %] … [% endif %]`. Two options:

- **(a) Keep templates Go-flavoured, special-case the conditional.** Only one template uses a
  conditional today. Move `spec.system` out of the template and into the renderer's data (pass a
  pre-formatted string or omit the key), so both engines read a conditional-free template.
- **(b) Accept two dialects.** Rejected — it breaks the "one catalog, two engines" claim at the first
  file.

**Go with (a).** It also improves the Go side: templates that contain logic are harder to validate
than templates that only substitute.

---

## 3. Phase 0 — Stop the bleeding (no new features, ~20 min)

Mechanical, and worth doing first so nothing downstream is built on a lie.

| File | Change |
|---|---|
| `renovate.json:9` | `^1-idp-scaffolder/utils\.py$` → `^2-idp-scaffolder/python/utils\.py$` |
| `renovate.json:17` | `\.tf\.jinja$` → a manager for `^1-platform-catalog/catalog\.yaml$`, matching `version: (?<currentValue>v[0-9.]+)` under `capabilities:`, datasource `github-tags`. That is where module pins live now |
| `Makefile:193,196` | `cd 1-idp-scaffolder` → `cd 2-idp-scaffolder/python` |
| `pyproject.toml:2` | `name = "idp-scaffolder"` |
| `pyproject.toml:22` | `packages` list — update once module names settle in Phase 4 |

**Verify Renovate actually matches something** rather than assuming:

```bash
npx --yes renovate-config-validator renovate.json
```

(A validator confirms the config parses; it does not prove the regex matches. Grep the target files
by hand once — that is the check that would have caught this three months ago.)

---

## 4. Phase 1 — `catalog.py` (mirror `internal/catalog/catalog.go`)

New module. The data layer, with validation at the load boundary.

```python
class Capability(BaseModel):
    module: str
    version: str

class GoldenPath(BaseModel):
    name: str
    description: str = ""
    runtime: str
    capabilities: list[str] = []
    delivery: str = ""

class Catalog(BaseModel):
    capabilities_source_base: str
    golden_paths: list[GoldenPath] = Field(alias="golden-paths")
    capabilities: dict[str, Capability]
    destinations: dict[str, str]

REQUIRED_DESTINATIONS: list[str] = [...]   # byte-identical to Go's requiredDestinations

def load_catalog(path: Path) -> Catalog: ...
def find_golden_path(catalog: Catalog, name: str) -> GoldenPath | None: ...
```

> **Why pydantic here rather than plain dataclasses:** it is already a dependency, `Field(alias=...)`
> handles the `golden-paths` hyphen that is not a valid Python identifier, and a
> `@model_validator(mode="after")` gives you exactly Go's `validate()` — fail at load, where the
> error can still name the offending key, instead of mid-render with half the tree on disk.

The validator must check the same three things Go does, so the engines reject the same catalogs:

1. `capabilities_source_base` is non-empty.
2. Every key in `REQUIRED_DESTINATIONS` is present in `destinations`.
3. Every golden path has a name and runtime, and references only capabilities that exist.

**Trap:** keep `REQUIRED_DESTINATIONS` textually identical to Go's list. If the two drift, one engine
accepts a catalog the other rejects and the "contract" claim is dead. Consider a test that reads
`../golang/internal/catalog/catalog.go` and asserts the two lists match — ugly, but it is the only
thing that actually holds them together.

---

## 5. Phase 2 — `utils.py` rewrite (roots and IPAM)

**Delete** `get_template_base_dir()` entirely along with the clone logic. Replace with the same
model Go is moving to in its Phase 1:

```python
def find_repo_root(start: Path | None = None) -> Path:
    """Walk up from `start` until a directory contains 1-platform-catalog/catalog.yaml."""

# Resolved once, in the CLI, and passed down. Never read from module scope.
```

> **Why root discovery beats both alternatives:** the current module-scope constant is wrong whenever
> the package moves, and the git clone is wrong whenever the branch differs. Discovery is correct
> from any CWD, needs no network, and makes tests possible. Keep `--catalog-root` / `--output-root`
> flags as overrides — same names as Go's, so muscle memory transfers.

**Fix in place:**

- `list_available_app_types()` → list `building-blocks/runtimes/` from the catalog root.
- `list_available_cloud_services()` → `sorted(catalog.capabilities)`. Never hardcode again.
- `list_onboarded_teams()` / `list_tenant_repositories()` → take the output root as an argument.

**Keep `allocate_vpc_cidr_block()`.** It is the one genuinely interesting thing Python has that Go
does not, and it is the right kind of platform feature — deterministic, stateful, prevents a class of
outage (overlapping VPC CIDRs break peering irreversibly). Two real bugs to fix while you are there:

- **No bounds check.** `next_index = max_index + 1` will happily emit `10.256.0.0/16` past 255 teams.
  Raise instead. Better: use `ipaddress.ip_network` and let the stdlib reject it.
- **Read-modify-write with no locking.** Two concurrent API requests can allocate the same block.
  Single-process today, but the FastAPI surface makes it reachable. A file lock, or at minimum a
  documented note that the registry is the source of truth and CI serialises writes.

---

## 6. Phase 3 — `renderer.py` (mirror `internal/templater/render.go`, but Plan-first)

Go still has Plan-then-Write ahead of it. **Python should be built that way from the start** — it
costs nothing at this stage and gives `--dry-run` and the API plan endpoint for free.

```python
Plan = dict[str, bytes]          # keys are OUTPUT-ROOT-RELATIVE, posix-style

@dataclass
class Config:
    team_name: str
    app_name: str = ""
    system: str = ""             # Backstage metadata only; creates no directory
    runtime: str = ""
    capabilities: list[str] = field(default_factory=list)
    env: str = "dev"

class Renderer:
    def __init__(self, catalog_root: Path, catalog: Catalog): ...

    def plan_team(self, cfg: Config) -> Plan: ...
    def plan_service(self, cfg: Config) -> Plan: ...

def write_plan(plan: Plan, output_root: Path) -> None: ...
def diff_plan(plan: Plan, output_root: Path) -> str: ...   # powers --dry-run
```

Internals, mirroring Go one-to-one so the walkthrough applies to both:

| Python | Go equivalent |
|---|---|
| `_resolve_destination(key, cfg) -> str` | `resolveDestination` |
| `_render_template(rel_path, data) -> bytes` | `processSingleTemplate` (render half) |
| `_walk_and_plan(source_dir, dest_dir, cfg, plan)` | `walkAndRender` |
| `TEAM_BLUEPRINTS: list[str]` | `teamBlueprints` |

`plan_team` is a loop over the same three keys. `plan_service` does runtime → service-meta →
delivery/release → capabilities, with a `CapabilityView`-equivalent dict scoped to each capability.

### Traps

- **Sort the keys.** `sorted(plan)` wherever order is observable. Python dicts are insertion-ordered,
  which is *more* stable than Go maps — but `Path.iterdir()` is **not** sorted, so walk order still
  varies by filesystem. Sort at the walk, not just at output.
- **Strip `.tmpl` once,** in the same place Go does, so the two agree on output filenames.
- **`PurePosixPath` for plan keys, `Path` only when writing.** Same discipline as Go's
  `path` vs `filepath` split. Windows is not the concern; *consistency between the two engines* is.
- **Binary-safe.** Read templates as text, write as bytes with an explicit `encoding="utf-8"` and
  `newline="\n"`. A CRLF slip makes every file differ from Go's output and the acceptance test
  becomes useless noise.
- **`_resolve_destination` must reject leftovers.** If `{` survives substitution, raise naming the
  placeholder. Go needs this guard too — it is listed in its Phase 2.

---

## 7. Phase 4 — `cli.py` and `schemas.py`

**Verbs must match Go exactly.** Two vocabularies for one platform is the thing that makes a
reference architecture look unconsidered.

```
scaffolder onboard-team --team-name payments
scaffolder add-service  --team-name payments --app-name checkout-api \
                        --golden-path go-service-postgres [--system checkout] [--dry-run]
```

Drop `create`, `--app-type` and `--cloud-services` (→ `--runtime`, `--capabilities`). `--app-port` has
no consumer in the current templates — drop it rather than carrying a flag that does nothing.

Match Go's seed-and-override semantics, and **do not reproduce its bug**: the golden path seeds,
explicit flags win.

```python
if golden_path:
    gp = find_golden_path(catalog, golden_path)
    if runtime == "":                     # NOT unconditional — see Go bug 2
        runtime = gp.runtime
    capabilities = dedupe([*gp.capabilities, *capabilities])   # order-preserving
```

**`schemas.py`: kill the import-time enum.** `AppType = Enum(..., utils.list_available_app_types())`
at module scope is why the package cannot be imported without a populated templates directory. Build
validators from the *loaded catalog*, at call time:

```python
def app_details_model(catalog: Catalog) -> type[BaseModel]:
    """Build the request model against a loaded catalog, so validation tracks the catalog
    and importing this module never touches the filesystem."""
```

Keep the existing field constraints — the DNS-compliant name patterns and `extra: "forbid"` are good
and Go has no equivalent. Worth calling out in the README as a real Python-side advantage.

---

## 8. Phase 5 — `api.py`

Endpoints mostly survive. Three changes:

1. **Add `POST /api/v1/applications/plan`** returning the plan without writing. This is the payoff
   from Phase 3 and the most demo-able thing in the whole project — a portal preview of exactly what
   the PR will contain.
2. **No module-scope mutable state.** Go has this bug too (`cfg` and `renderer` are package globals,
   so concurrent requests stomp each other's `team_name`). Do not import it into Python: build a
   `Config` per request; share only the immutable loaded `Catalog` via a FastAPI dependency.
3. `GET /api/v1/meta/capabilities` from `catalog.capabilities`, replacing the hardcoded list.

---

## 9. Phase 6 — Docs

`README.md` is currently fiction — it documents `templates/`, two-pass Copier rendering, and a
`helm-charts/` vs `rendered-manifests/` split that no longer exists. Rewrite around: one catalog, two
engines; what Python adds (pydantic validation, IPAM, the REST surface); and the Copier trade-off
from §2, stated as a trade-off.

Update `.agents/AGENTS.md` — the Python section and the "non-functional pending repoint" note.

---

## 10. Verification

```bash
cd 2-idp-scaffolder/python && uv sync

# Phase 0 — confirm the diagnosis before changing anything, so you see it fail
uv run python -c "import utils; print(utils.TENANT_WORKLOADS_DIR)"     # wrong root
uv run python -c "import utils; print(utils.get_template_base_dir())"  # v1.0.2 clone

# Phase 1 — validation rejects a bad catalog at load, naming the key
uv run python -c "
from catalog import load_catalog; from pathlib import Path
c = load_catalog(Path('../../1-platform-catalog/catalog.yaml'))
print(len(c.golden_paths), 'golden paths;', sorted(c.capabilities))"

# Phase 4 — importable with no filesystem present (the schemas.py fix)
uv run python -c "import schemas; print('imports clean')"

# Phase 3/4 — dry run writes nothing
uv run python main.py add-service --team-name tmp --app-name x \
  --golden-path go-service-postgres --dry-run
git status --short 3-tenant-workloads/     # must be empty
```

### The acceptance test — this is the whole point

```bash
# Same inputs, both engines, into scratch output roots
scaffolder      onboard-team --team-name payments --output-root /tmp/go-out
uv run python main.py onboard-team --team-name payments --output-root /tmp/py-out
scaffolder      add-service --team-name payments --app-name checkout-api \
                --golden-path go-service-postgres --output-root /tmp/go-out
uv run python main.py add-service --team-name payments --app-name checkout-api \
                --golden-path go-service-postgres --output-root /tmp/py-out

diff -r /tmp/go-out /tmp/py-out && echo "CONTRACT HOLDS"
```

Any difference is a real finding, not test noise: either the two engines disagree about the catalog,
or the catalog is under-specified. Both are worth knowing. Note this needs Go's Phase 1
(`--output-root`) done first — so **do the Go roots phase before this final check**, or compare
against the committed `3-tenant-workloads/team-a/` instead.

---

## Out of scope

- Go Phases 1–3 — tracked in `../golang/IMPLEMENTATION_PLAN_CLAUDE.md`. Only Go's `--output-root`
  blocks anything here (the acceptance test).
- Copier's Day-2 `update` flow — dropped with Copier; revisit only if regeneration proves painful.
- Packaging/publishing the CLI to PyPI.
- Backstage integration beyond emitting `catalog-info.yaml`.
