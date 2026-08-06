# Implementation Plan — Python scaffolder, remaining work

**Headline: the acceptance test almost passes.** Both engines, same inputs, same catalog:

```
diff -r <go-out>/zz-accept <py-out>/zz-accept
Only in py: catalog-info.yaml line 13 — a stray line containing two spaces
```

Every other file — `CODEOWNERS` ×3, the four gitops platform manifests, the
ApplicationSet, `providers.tf`, `team-iam.tf`, `go.mod`, `main.go`, `values.yaml`,
`postgres.tf` — is **byte-identical**. The "one catalog, two engines" claim is one
whitespace fix from being true and testable.

That is the good news, and it is genuinely good. The rest of this file is what stands
between here and being able to run that diff in CI.

---

## Done since the last plan

| Item | Where | Notes |
|---|---|---|
| `catalog.py` — the whole data layer | `catalog.py` | pydantic models, `Field(alias="golden-paths")`, `@model_validator` mirroring Go's `validate()`, `REQUIRED_DESTINATIONS` textually matching Go |
| Copier → Jinja2 | `render.py:34-44` | Go delimiters `[[ ]]` / `[% %]`, `StrictUndefined`, `keep_trailing_newline` |
| Go-template conversion | `render.py:46-56` | regex rewrites `[[ .Var ]]` → `[[ Var ]]`, `[[- if .X ]]` → `[% if X %]` |
| Verb parity with Go | `cli.py` | `onboard-team` / `add-service`, `--golden-path` / `--capabilities` / `--system` / `--env` |
| Import-time enum removed | `schemas.py` | plain pydantic models; the package imports with no filesystem present |
| Phase 0 paths | `Makefile:194,197`, `pyproject.toml:2` | both repointed to `2-idp-scaffolder/python` |

---

## Phase 0 — Blockers (about an hour, do before merging anything else)

### 0a. `api.py` does not import

```
AttributeError: module 'schemas' has no attribute 'AppDetails'
```

`api.py` still references four names that no longer exist: `schemas.AppDetails`,
`schemas.CloudServices`, `render.list_available_app_types`, and
`cli.scaffold_tenant_workload`. The module fails at **import**, not at request time,
because `schemas.AppDetails` is in a function signature evaluated at def time.

`make dev-api` (Makefile:197) is therefore broken. Either repoint the four names at
what now exists, or comment the file out with a TODO — but do not leave a module in the
tree that cannot be imported.

### 0b. `import copier` breaks any fresh checkout

`cli.py:1` does `import typer, copier`. **`copier` appears in neither `pyproject.toml`
nor `uv.lock`** — verified: `uv tree` has zero matches, and an isolated environment with
the four declared dependencies raises `ModuleNotFoundError: No module named 'copier'`.
It works on this machine only because it is stale in the local `.venv`.

So the Python CLI is broken for anyone who clones the repo. The import is also unused —
`copier` appears on line 1 and nowhere else. Delete it.

**Related, lower priority:** `pyyaml` is imported by `catalog.py:18` and `render.py:2`
and is not declared either — but unlike copier it *is* present transitively, via
`fastapi[standard]` → `uvicorn[standard]` → `pyyaml`. Nothing breaks today. Still worth
declaring: depending on an extra of an unrelated package is a bug waiting for someone
else's upgrade, exactly as `catalog.py:21-23` notes.

**Verify:** `rm -rf .venv && uv sync && uv run python -c "import cli, api"` must succeed
from clean.

### 0c. The Renovate manager is dead again

`renovate.json:9` was repointed to `2-idp-scaffolder/python/render.py` — correct. But it
matches `REMOTE_TEMPLATE_REPO = "https://github.com/…@version=v…"`, and `render.py:32`
now reads `REMOTE_TEMPLATE_REPO = ""` with the URL commented out. **The regex matches
zero strings**, exactly as before, for a new reason.

The catalog manager (`renovate.json:19`) is correct and will match
`version: "v1.0.2"` in `catalog.yaml`.

Since Phase 3 deletes remote fetching from Python entirely, the right fix is to **delete
the first custom manager**, not repair it. A manager matching zero files is
indistinguishable from a healthy one until someone looks — which is the whole lesson
from the last round.

---

## Phase 1 — Close the contract gap (the important one)

### 1a. The whitespace diff — one line, verified

`render.py:34-44` builds the Jinja `Environment` without whitespace control, so Go's
`[[-` trim markers are lost in conversion and a false conditional leaves an indented
blank line.

```python
trim_blocks=True,     # drop the first newline after a block tag
lstrip_blocks=True,   # drop leading whitespace before a block tag
```

Verified against the real template: with these two kwargs the rendered tail is
`...not supplied.\n` — byte-identical to Go. Without them it is `...not supplied.\n  \n`.

### 1b. Python ignores `catalog.destinations` entirely

This is the real architectural gap. Go derives every output path from the catalog:

```go
target := r.resolveDestination(r.Spec.Destinations[item.destKey], cfg)
```

Python hardcodes the same paths literally:

```python
app_dst    = TENANT_WORKLOADS_DIR / team_name / "apps" / app_name          # cli.py:34
gitops_dst = TENANT_WORKLOADS_DIR / team_name / "gitops" / "apps" / app_name / env_name  # cli.py:53
infra_dst  = TENANT_WORKLOADS_DIR / team_name / "infra" / "apps" / app_name / env_name   # cli.py:67
```

They agree **today**. Change one `destinations:` value in `catalog.yaml` and Go follows
it while Python does not — silently, with no error, producing two different trees from
one contract. The catalog stops being a contract the moment one engine stops reading it.

Implement the Go twin:

```python
def resolve_destination(catalog: Catalog, key: str, cfg: Config, output_root: Path) -> Path:
    """Substitute {team}/{system}/{app}/{env} into destinations[key]."""
```

and route all four write sites through it. Mirror Go's `blueprint` table too — a list of
`(src, dest_key)` pairs looped over once, rather than three copy-pasted blocks.

**Trap:** raise if a `{placeholder}` survives substitution. A typo'd key in
`catalog.yaml` should fail loudly, not create a directory literally named `{app}`.

### 1c. Error parity — three silent divergences

Go fails; Python shrugs. Any of these makes the engines non-interchangeable:

| Case | Go | Python | Where |
|---|---|---|---|
| Unknown golden path | `Error: golden path 'x' not found`, exit 1 | `if gp:` with no `else` — ignored | `cli.py:22` |
| No runtime, no golden path | `Error: a runtime is required`, exit 1 | silently defaults to `"go"` | `cli.py:28-29` |
| Unknown capability | `Error: unknown capability: bogus`, exit 1 | `if cap_template.exists() and …` — skipped | `cli.py:70` |

The last is the worst: `--capabilities bogus` produces a service with no database and a
zero exit code. Match Go — raise `typer.BadParameter` or `typer.Exit(1)` in all three.

### 1d. Non-deterministic capability order

```python
capabilities = list(set(gp.capabilities + capabilities))   # cli.py:26
```

Python randomises `str` hashing per process (`PYTHONHASHSEED`), so this ordering varies
**between runs of the same command**. Go sorts. Use `sorted(set(...))` — cheap, and it
makes the acceptance diff stable.

### 1e. Path templating by string replace

`cli.py:43` and `cli.py:104` do
`str(rel).replace("[[ .AppName ]]", app_name)`. Go renders path segments as templates
(`renderPath`), so `[[ .TeamName ]].yaml.tmpl` works for any variable. The string-replace
version silently fails for any placeholder nobody thought to add. Render the relative
path through Jinja, the same way file contents already are.

---

## Phase 2 — Plan-then-Write and `--dry-run`

Still not started, and it is Python's chance to lead rather than follow — Go has this
ahead of it too (`golang/IMPLEMENTATION_PLAN.md` Phase 3).

```python
Plan = dict[str, bytes]        # keys OUTPUT-ROOT-RELATIVE, posix-style, sorted

def plan_team(cfg: Config) -> Plan: ...
def plan_service(cfg: Config) -> Plan: ...
def write_plan(plan: Plan, output_root: Path, force: bool = False) -> None: ...
def diff_plan(plan: Plan, output_root: Path) -> str: ...   # powers --dry-run
```

Everything currently interleaves rendering with `out_path.write_text(...)`. Separating
them gives `--dry-run`, the API's plan endpoint (Phase 4), and testability in one move.

**Also fix here:** `write_plan` should skip-if-exists unless `--force`. Python has the
same truncation problem Go does — re-running `add-service` overwrites a team's edited
`main.go`. Dropping Copier gave up `_skip_if_exists`; this is the debt coming due, and
the old plan said to note it as a trade-off rather than pretend otherwise.

**Traps:** sort at the walk (`Path.rglob` is not ordered), strip `.tmpl` in exactly one
place, use `PurePosixPath` for plan keys and `Path` only when writing, and write with
explicit `encoding="utf-8"` / `newline="\n"` so no CRLF slip makes every file differ
from Go's.

---

## Phase 3 — Roots and flags

`render.py:12` still computes `REPO_ROOT = Path(__file__).resolve().parents[2]` at
module scope — wrong the moment the package moves or is installed as a wheel, and
untestable because it cannot be overridden. The old plan called for `find_repo_root()`
resolved once in the CLI and passed down; still outstanding.

Add `--catalog-root` and `--output-root` to both verbs, same names as Go's. **The
acceptance test in CI needs `--output-root`** — without it Python can only write into
the real `3-tenant-workloads/`, which is why running the comparison above required
backing up and restoring `cloud_vpcs_allocated.yaml`.

**Delete while here** — all dead since the Copier removal:

- `render.py:59-98` `get_template_base_dir()` — `REMOTE_TEMPLATE_REPO` is `""`, so it is unreachable; the old plan already said delete
- `render.py:20-26` the module-level `env` — shadowed by `create_jinja_env()`, never used
- `render.py:110-113` `list_available_cloud_services()` — still returns hardcoded `["aws-postgres","aws-s3"]`; this was finding #4 in the *previous* plan
- `render.py:107` the `["postgres","s3","iam"]` fallback in `list_available_capabilities()` — a missing catalog should raise, not invent a plausible answer

---

## Phase 4 — `api.py`

Once Phase 0a makes it importable and Phase 2 gives you plans:

1. **`POST /api/v1/applications/plan`** — return the plan without writing. Still the most
   demo-able thing in the project: a portal preview of exactly what the PR will contain.
2. **No module-scope mutable state.** Build a `Config` per request; share only the
   immutable loaded `Catalog` via a FastAPI dependency. (Go has this bug too — its
   `cfg`/`renderer` globals are tracked in its Phase 5c. Do not import it here.)
3. **`GET /api/v1/meta/capabilities`** from `catalog.capabilities`, replacing the
   hardcoded metadata endpoints.

---

## Phase 5 — IPAM hardening

`allocate_vpc_cidr_block()` (`render.py:128-157`) is the one genuinely interesting thing
Python has that Go does not — deterministic, stateful, prevents overlapping VPC CIDRs
that break peering irreversibly. Both bugs from the last plan are still open:

- **No bounds check.** `next_index = max_index + 1` (`render.py:144`) emits
  `10.256.0.0/16` past 255 teams. Use `ipaddress.ip_network` and let the stdlib reject it.
- **Read-modify-write with no locking** (`render.py:133-154`). Two concurrent API
  requests allocate the same block. A file lock, or a documented note that the registry
  is the source of truth and CI serialises writes.

Also: `typer.echo` sits *inside* the `with open(...)` block at `render.py:155`, so the
log line is emitted mid-write. Cosmetic, but move it out.

**Worth deciding explicitly:** IPAM makes the engines *not* interchangeable — Python
writes `cloud_vpcs_allocated.yaml`, Go does not, and Go has no VPC concept at all. Either
port it to Go, or document that `onboard-team` is Python-only in production and Go is the
service-scaffolding path. Right now it is an accident rather than a decision.

---

## Phase 6 — Tests and the CI gate

No tests exist. The Go side now has `internal/templater/resolve_test.go` as a model.

1. **`test_catalog.py`** — validation rejects a bad catalog naming the offending key;
   `find_golden_path` hit and miss. `catalog.py` is pure and takes a `Path`, so this is
   the cheapest test to write.
2. **`test_resolve.py`** — once golden-path seeding is extracted from `cli.py` into a
   pure function (mirroring Go's `templater.Resolve`), table-test it with `pytest.mark.parametrize`.
   Cases: runtime-only, golden-path-only, explicit runtime wins, duplicates collapse,
   unknown golden path raises.
3. **The acceptance test, in CI.** This is the one that matters:

```bash
scaffolder      onboard-team -t acc --output-root /tmp/go-out
uv run python main.py onboard-team -t acc --output-root /tmp/py-out
scaffolder      add-service -t acc -a checkout --golden-path go-service-postgres --output-root /tmp/go-out
uv run python main.py add-service -t acc -a checkout --golden-path go-service-postgres --output-root /tmp/py-out
diff -r /tmp/go-out /tmp/py-out
```

Gate the PR on it. Any difference is a real finding — either the engines disagree, or
the catalog is under-specified. Blocked only on Phase 3's `--output-root`.

---

## Verification

```bash
cd 2-idp-scaffolder/python

# Phase 0 — clean install must work
rm -rf .venv && uv sync && uv run python -c "import cli, api; print('imports clean')"

# Phase 1 — error parity, all must exit 1
uv run python main.py add-service -t acc -a x --golden-path nope
uv run python main.py add-service -t acc -a x                       # no runtime
uv run python main.py add-service -t acc -a x -r go -c bogus        # unknown capability

# Phase 1d — determinism: same output across runs
for i in 1 2 3; do uv run python main.py add-service -t acc -a x \
  --golden-path go-service-postgres -c iam,postgres --dry-run | sha256sum; done  # 3 identical hashes

# Phase 2 — dry run writes nothing
uv run python main.py add-service -t acc -a x --golden-path go-service-postgres --dry-run
git status --short ../../3-tenant-workloads/    # must be empty
```

---

## Out of scope

- Porting IPAM to Go — decide the ownership question in Phase 5 first
- Copier's Day-2 `update` flow — dropped with Copier; revisit if regeneration proves painful
- Publishing to PyPI
- Backstage integration beyond emitting `catalog-info.yaml`

---

# Answer Key

## Phase 1a — the whitespace fix

```python
def create_jinja_env(catalog_root: Path) -> Environment:
    """Jinja2 configured to match Go's text/template semantics.

    trim_blocks/lstrip_blocks reproduce Go's `[[-` whitespace trimming, which the
    regex conversion in render_template_string() cannot carry over on its own.
    Without them a false `[[- if ]]` leaves an indented blank line and the
    acceptance diff against the Go engine fails on catalog-info.yaml.
    """
    return Environment(
        loader=FileSystemLoader(str(catalog_root)),
        variable_start_string="[[", variable_end_string="]]",
        block_start_string="[%",   block_end_string="%]",
        keep_trailing_newline=True,
        trim_blocks=True,
        lstrip_blocks=True,
        undefined=StrictUndefined,
    )
```

## Phase 1b — destination resolution

```python
import re
from pathlib import Path, PurePosixPath

_PLACEHOLDER = re.compile(r"\{(\w+)\}")

def resolve_destination(catalog: Catalog, key: str, cfg: Config) -> PurePosixPath:
    """Python twin of Go's resolveDestination.

    The destinations key is also the source directory inside the catalog, which is
    what lets one string drive both halves of the walk — same contract as Go.
    """
    template = catalog.destinations[key]          # KeyError here is a catalog bug
    dest = (template
            .replace("{team}", cfg.team_name)
            .replace("{system}", cfg.system)
            .replace("{app}", cfg.app_name)
            .replace("{env}", cfg.env or "dev"))

    # Fail loudly rather than creating a directory literally named "{app}".
    if leftover := _PLACEHOLDER.search(dest):
        raise ValueError(
            f"destinations[{key!r}] = {template!r} has unsubstituted "
            f"placeholder {leftover.group(0)!r}"
        )
    return PurePosixPath(dest)
```

## Phase 1c/1d — resolution with Go's semantics

```python
def resolve(catalog: Catalog, golden_path: str, cfg: Config) -> Config:
    """Merge a golden path with explicit overrides. Mirrors Go's templater.Resolve.

    Pure: no filesystem, no globals, no module-scope constants — so it is directly
    unit-testable and reusable by the FastAPI layer.
    """
    out = replace(cfg)                       # dataclasses.replace — a shallow copy
    out.capabilities = list(cfg.capabilities)  # do not alias the caller's list

    if golden_path:
        gp = find_golden_path(catalog, golden_path)
        if gp is None:
            raise ValueError(f"golden path {golden_path!r} not found in catalog")
        if not out.runtime:                  # explicit --runtime wins
            out.runtime = gp.runtime
        out.capabilities += gp.capabilities

    if not out.runtime:
        raise ValueError("a runtime is required: pass --runtime or --golden-path")

    for cap in out.capabilities:
        if cap not in catalog.capabilities:
            raise ValueError(
                f"unknown capability {cap!r} "
                f"(available: {', '.join(sorted(catalog.capabilities))})"
            )

    # sorted(set(...)), never list(set(...)): str hashing is randomised per process,
    # so list(set(...)) reorders between runs of the same command.
    out.capabilities = sorted(set(out.capabilities))
    return out
```

## Phase 5 — bounded CIDR allocation

```python
import ipaddress

VPC_SUPERNET = ipaddress.ip_network("10.0.0.0/8")
VPC_PREFIX = 16

def _next_cidr(allocated: dict[str, str]) -> str:
    """Allocate the next free /16 inside 10.0.0.0/8, refusing to run off the end.

    The previous version did `max_index + 1` unchecked, which silently emits the
    invalid 10.256.0.0/16 past 255 teams.
    """
    used = {ipaddress.ip_network(c) for c in allocated.values()}
    for candidate in VPC_SUPERNET.subnets(new_prefix=VPC_PREFIX):
        if candidate not in used:
            return str(candidate)
    raise RuntimeError(
        f"no free /{VPC_PREFIX} left in {VPC_SUPERNET} ({len(used)} allocated)"
    )
```
