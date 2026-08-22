# 🐍 Python Scaffolder Engine & REST API

The Python implementation of the IDP scaffolder: a Typer CLI and a FastAPI service,
both driven by `1-platform-catalog/catalog.yaml`.

It is a **second engine, not a legacy one.** The Go CLI in `../golang/` is definitive;
this exists so the catalog is a *falsifiable* contract rather than an asserted one — run
both with the same inputs, diff the trees, and any difference is a real finding. See
[The Acceptance Test](#-the-acceptance-test).

What Python adds on top of parity: pydantic validation of every input, and a REST surface
a developer portal can call.

---

## 💡 Design

### 1. The catalog is the only source of truth

`catalog.py` is the Python twin of `../golang/internal/catalog/catalog.go`. It loads and
validates `catalog.yaml` with pydantic, and its `REQUIRED_DESTINATIONS` list is kept
**textually identical** to Go's `requiredDestinations`. If the two drift, one engine
accepts a catalog the other rejects and the contract claim dies quietly.

Validation happens at the load boundary, so a bad catalog fails with the offending key
named, before anything is written — not halfway through, with half a tree on disk.

### 2. Jinja2 configured to behave like Go's `text/template`

Templates in `1-platform-catalog/` are written in Go syntax and shared by both engines,
so Jinja is configured to match:

| Setting | Why |
|---|---|
| `variable_start_string="[["` | leaves Helm's `{{ }}` untouched, so charts pass through verbatim |
| `block_start_string="[%"` | Go's `[[- if ]]` is rewritten to this by regex at render time |
| `keep_trailing_newline=True` | otherwise every rendered file loses its final `\n` |
| `undefined=StrictUndefined` | a typo'd variable must fail, not silently render `""` — Go's default |

**Copier was removed.** It renders *to a directory* and cannot hand back rendered bytes,
which is incompatible with the Plan-then-Write architecture both engines are moving
toward. Dropping it also gave up Copier's `_skip_if_exists` idempotency — an honest cost,
tracked as skip-if-exists plus `--force` in [`TODO.md`](TODO.md).

### 3. Strictly validated CLI and API contracts (pydantic)

`schemas.py` validates every input before any work happens:

* **RFC-compliant naming** — team and app names must be lowercase alphanumeric with
  hyphens (DNS-safe, since they become Kubernetes and AWS resource names).
* **`extra: "forbid"`** — an unknown field is an error, not silently ignored.

Go has no equivalent, so this is a genuine Python-side advantage.

### 4. No per-tenant VPC allocation (removed)

An earlier version of this engine allocated a unique `/16` VPC CIDR per team into
`3-tenant-workloads/cloud_vpcs_allocated.yaml`, on the assumption that each team would get
its own VPC (and implicitly, its own cluster). That's not the design that shipped — the
platform runs one shared EKS cluster with namespace-per-tenant as the soft isolation
boundary, and one AWS account per *environment* (not per team) as the hard one. The
allocator was also already dead in practice: the CIDR it computed was never substituted
into any template (`team-iam.tf.tmpl` hardcoded `10.0.0.0/16` regardless of team), so it
computed and persisted a value nothing ever read. Removed rather than fixed.

---

## 📂 Layout

```text
2-idp-scaffolder/python/
├── main.py          # entrypoint, delegates to the Typer app
├── cli.py           # the two verbs: onboard-team, add-service
├── api.py           # FastAPI endpoints over the same engine
├── catalog.py       # loads + validates catalog.yaml (twin of Go's internal/catalog)
├── render.py        # Jinja2 environment, path roots
├── schemas.py       # pydantic input models
├── pyproject.toml   # package metadata, dependencies, CLI entrypoint
├── uv.lock          # deterministic lockfile
└── TODO.md          # remaining work, phased, with an answer key
```

There is no `templates/` directory — templates live in `1-platform-catalog/`, shared with
the Go engine.

---

## 🚀 Usage

```bash
uv sync

uv run python main.py onboard-team --team-name payments

uv run python main.py add-service --team-name payments --app-name checkout-api \
    --golden-path go-service-postgres

# --golden-path seeds runtime + capabilities from catalog.yaml;
# --runtime and --capabilities override or extend that seed.
uv run python main.py add-service --team-name payments --app-name checkout-api \
    --runtime go --capabilities postgres,s3
```

Verbs and flags match the Go CLI exactly. Two vocabularies for one platform is the thing
that makes a reference architecture look unconsidered.

### As a REST API

```bash
make run-api        # then open http://127.0.0.1:8000/docs
```

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/meta/capabilities` | capabilities and their pinned module versions |
| `GET /api/v1/meta/golden-paths` | paved roads — what a portal renders as a picker |
| `GET /api/v1/meta/runtimes` | runtimes with a scaffold template |
| `GET /api/v1/teams` | onboarded teams |
| `GET /api/v1/teams/{team}/repositories` | repo kinds generated for a team |
| `POST /api/v1/teams` | the `onboard-team` verb |
| `POST /api/v1/applications` | the `add-service` verb |
| `POST /api/v1/applications/plan` | **501** — needs Plan-then-Write (`TODO.md` Phase 2) |

Every metadata endpoint reads the catalog, so the API and CLI cannot disagree about what
the platform offers. Bad requests return `400` with the same message the CLI prints.

---

## 🔬 The Acceptance Test

```bash
# Go
cd ../golang
go run . onboard-team --catalog-root ../../1-platform-catalog --output-root /tmp/go-out -t acc
go run . add-service  --catalog-root ../../1-platform-catalog --output-root /tmp/go-out \
    -t acc -a checkout --golden-path go-service-postgres

# Python
cd ../python
uv run python main.py onboard-team --team-name acc
uv run python main.py add-service --team-name acc --app-name checkout \
    --golden-path go-service-postgres

diff -r /tmp/go-out/3-tenant-workloads/acc ../../3-tenant-workloads/acc
```

**This currently produces no output** — the trees are byte-identical, both verbs, every
file. Keeping it that way depends on `trim_blocks`/`lstrip_blocks` in `create_jinja_env`:
the regex that rewrites `[[- if .X ]]` to `[% if X %]` cannot carry Go's `-` trim markers
across, so without those flags a false conditional leaves an indented blank line behind.

> Python has no `--output-root` yet (`TODO.md` Phase 3), so it always writes into the real
> `3-tenant-workloads/`. Remove the scratch team afterwards, or use a team name you do not
> mind deleting.

---

## ⚠️ Known limitations

Full detail, with fixes, in [`TODO.md`](TODO.md).

* **Not idempotent** — writes truncate, so re-running `add-service` overwrites edits to a
  previously scaffolded file.
* **No `--dry-run`** and no `--catalog-root` / `--output-root`.
* **`catalog.destinations` is ignored** — output paths are currently hardcoded in
  `cli.py`. They happen to match Go today; change a `destinations:` value and only Go
  follows it.
* **Silent divergences from Go** — an unknown golden path, a missing runtime, and an
  unknown capability are all errors in Go but are ignored or defaulted here.
* **No tests.**
