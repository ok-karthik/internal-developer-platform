"""REST surface over the same engine the CLI drives.

Every endpoint reads the catalog rather than a hardcoded list, so the API and the
CLI can never disagree about what the platform offers.

Run it:

    make dev-api          # or: fastapi dev api.py
"""

from functools import lru_cache
from pathlib import Path
from typing import Annotated

from fastapi import Depends, FastAPI, HTTPException

import catalog as catalog_mod
import cli
import render
import schemas

api = FastAPI(
    title="Platform Control Plane Scaffolder API",
    description=(
        "REST interface for Golden Path application scaffolding and tenant "
        "provisioning. Backed by 1-platform-catalog/catalog.yaml."
    ),
)


@lru_cache(maxsize=1)
def _load_catalog() -> catalog_mod.Catalog:
    """Load and validate the catalog once per process.

    Cached because a Catalog is immutable once built, so sharing one across
    concurrent requests is safe — unlike the per-request Config, which must never
    become module-scope state.
    """
    return catalog_mod.load_catalog(render.CATALOG_DIR / "catalog.yaml")


def get_catalog() -> catalog_mod.Catalog:
    """FastAPI dependency. Turns a catalog authoring error into a 503 rather than
    a 500 with a stack trace, since the request itself was not at fault."""
    try:
        return _load_catalog()
    except Exception as e:
        raise HTTPException(503, f"catalog unavailable: {e.__class__.__name__}: {e}")


CatalogDep = Annotated[catalog_mod.Catalog, Depends(get_catalog)]


# ---------------------------------------------------------------- Metadata


@api.get("/api/v1/meta/capabilities", tags=["Metadata"])
def get_capabilities(cat: CatalogDep):
    """Capabilities the platform can provision, with their pinned module versions."""
    return {
        name: {"module": c.module, "version": c.version}
        for name, c in sorted(cat.capabilities.items())
    }


@api.get("/api/v1/meta/golden-paths", tags=["Metadata"])
def get_golden_paths(cat: CatalogDep):
    """Paved roads on offer. This is what a portal would render as a picker."""
    return [gp.model_dump() for gp in cat.golden_paths]


@api.get("/api/v1/meta/runtimes", tags=["Metadata"])
def get_runtimes():
    """Runtimes with a scaffold template in the catalog."""
    runtimes_dir = render.CATALOG_DIR / "building-blocks" / "runtimes"
    if not runtimes_dir.is_dir():
        raise HTTPException(503, f"runtimes directory not found: {runtimes_dir}")
    return {"runtimes": sorted(p.name for p in runtimes_dir.iterdir() if p.is_dir())}


# ---------------------------------------------------------------- Tenancy


@api.get("/api/v1/teams", tags=["Tenancy"])
def get_teams():
    """Teams that have been onboarded."""
    return {"teams": sorted(render.list_onboarded_teams())}


@api.get("/api/v1/teams/{team_name}/repositories", tags=["Tenancy"])
def get_repositories(team_name: str):
    """The repo-kind directories (apps, infra, gitops) generated for a team."""
    repos = render.list_tenant_repositories(team_name)
    if not repos:
        raise HTTPException(404, f"team {team_name!r} has no scaffolded repositories")
    return {"repositories": sorted(repos)}


@api.post("/api/v1/teams", status_code=201, tags=["Tenancy"])
def onboard_team(body: schemas.OnboardTeamInput):
    """Scaffold the tenancy boundary for a new team (the `onboard-team` verb)."""
    try:
        cli.onboard_team_workload(body.team_name)
    except Exception as e:
        raise HTTPException(500, f"{e.__class__.__name__}: {e}")
    return {"team": body.team_name, "message": "team onboarded"}


# ---------------------------------------------------------------- Scaffolding


@api.post("/api/v1/applications", status_code=201, tags=["Scaffolding"])
def scaffold_application(body: schemas.AddServiceInput, cat: CatalogDep):
    """Scaffold a service (the `add-service` verb).

    Validation happens here rather than inside the workload so a bad request is a
    400, not a 500. The CLI performs the same checks — see TODO.md Phase 1c, which
    tracks extracting them into a shared `resolve()` both callers use.
    """
    if body.golden_path and catalog_mod.find_golden_path(cat, body.golden_path) is None:
        raise HTTPException(400, f"golden path {body.golden_path!r} not found in catalog")

    unknown = sorted(set(body.capabilities) - set(cat.capabilities))
    if unknown:
        raise HTTPException(
            400,
            f"unknown capabilities: {', '.join(unknown)} "
            f"(available: {', '.join(sorted(cat.capabilities))})",
        )

    if not body.golden_path and not body.runtime:
        raise HTTPException(400, "either golden_path or runtime is required")

    try:
        cli.add_service_workload(
            team_name=body.team_name,
            app_name=body.app_name,
            golden_path=body.golden_path,
            runtime=body.runtime,
            capabilities=body.capabilities,
            system=body.system,
            env_name=body.env,
        )
    except Exception as e:
        raise HTTPException(500, f"{e.__class__.__name__}: {e}")

    return {
        "team": body.team_name,
        "app": body.app_name,
        "message": "application scaffolded",
    }


@api.post("/api/v1/applications/plan", tags=["Scaffolding"])
def plan_application(body: schemas.AddServiceInput):
    """Return what `add-service` *would* write, without touching disk.

    Not implemented yet: it needs the Plan-then-Write split tracked in TODO.md
    Phase 2 (`plan_service(cfg) -> dict[str, bytes]`). Declared here because it is
    the endpoint a portal actually wants — a preview of the PR contents — and
    leaving it undeclared hides that from anyone reading the API surface.
    """
    raise HTTPException(
        501,
        "plan endpoint requires Plan-then-Write rendering (see TODO.md Phase 2)",
    )
