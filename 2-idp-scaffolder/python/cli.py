import typer
from typing import Annotated
import schemas, render, catalog

from pydantic import ValidationError

def add_service_workload(
    team_name: str,
    app_name: str,
    golden_path: str = "",
    runtime: str = "",
    capabilities: list[str] = None,
    system: str = "",
    env_name: str = "dev"
) -> bool:
    capabilities = capabilities or []
    cat_data = catalog.load_catalog(render.CATALOG_DIR / "catalog.yaml")

    # 1. Seed from Golden Path if specified
    if golden_path:
        gp = catalog.find_golden_path(cat_data, golden_path)
        if gp:
            if not runtime:
                runtime = gp.runtime
            # Combine seeded capabilities with explicit capability overrides
            capabilities = list(set(gp.capabilities + capabilities))

    if not runtime:
        runtime = "go"  # default runtime fallback

    env = render.create_jinja_env(render.CATALOG_DIR)
    
    # 2. Render Runtime Source + Service Meta into apps/<app>
    app_dst = render.TENANT_WORKLOADS_DIR / team_name / "apps" / app_name
    svc_data = {"TeamName": team_name, "AppName": app_name, "SystemName": system}

    for block_dir in [render.CATALOG_DIR / "per-service" / "apps" / "runtimes" / runtime,
                      render.CATALOG_DIR / "per-service" / "apps" / "service-meta"]:
        if block_dir.exists():
            for path in block_dir.rglob("*"):
                if path.is_file():
                    rel = path.relative_to(block_dir)
                    out_path = app_dst / str(rel).replace(".tmpl", "").replace("[[ .AppName ]]", app_name)
                    out_path.parent.mkdir(parents=True, exist_ok=True)
                    content = path.read_text(encoding="utf-8")
                    if path.suffix == ".tmpl":
                        content = render.render_template_string(env, content, svc_data)
                    out_path.write_text(content, encoding="utf-8")
                    typer.echo(f"  [WROTE] {out_path.relative_to(render.REPO_ROOT)}")

    # 3. Render Delivery Release values into gitops/apps/<app>/<env>/
    gitops_src = render.CATALOG_DIR / "per-service" / "gitops" / "release"
    gitops_dst = render.TENANT_WORKLOADS_DIR / team_name / "gitops" / "apps" / app_name / env_name
    if gitops_src.exists():
        for path in gitops_src.rglob("*"):
            if path.is_file():
                rel = path.relative_to(gitops_src)
                out_path = gitops_dst / str(rel).replace(".tmpl", "")
                out_path.parent.mkdir(parents=True, exist_ok=True)
                content = path.read_text(encoding="utf-8")
                if path.suffix == ".tmpl":
                    content = render.render_template_string(env, content, svc_data)
                out_path.write_text(content, encoding="utf-8")
                typer.echo(f"  [WROTE] {out_path.relative_to(render.REPO_ROOT)}")

    # 4. Render capability claims. Phase 3.9: provisioner decides which ONE of
    # the two capability directories a capability's template lives in and
    # which destination it lands in — mirrors Go's internal/templater
    # RenderService dispatch exactly, so the two engines cannot silently
    # diverge on which capabilities land where.
    infra_dst = render.TENANT_WORKLOADS_DIR / team_name / "infra" / "apps" / app_name / env_name
    gitops_caps_dst = render.TENANT_WORKLOADS_DIR / team_name / "gitops" / "apps" / app_name / env_name
    for cap_name in capabilities:
        if cap_name not in cat_data.capabilities:
            continue
        cap_info = cat_data.capabilities[cap_name]
        cap_data = {
            "TeamName": team_name,
            "AppName": app_name,
            # Env must be present: the capability templates tag resources with
            # it, and StrictUndefined turns a missing key into an error rather
            # than a silently empty value. Go supplies it via CapabilityView
            # embedding Config, so both engines render the same fields.
            "Env": env_name,
            "CapabilitiesSourceBase": cat_data.capabilities_source_base,
            "Module": cap_info.module,
            "Version": cap_info.version,
        }

        if cap_info.provisioner == "terraform":
            cap_template = render.CATALOG_DIR / "per-service" / "infra" / "capabilities" / f"{cap_name}.tf.tmpl"
            out_path = infra_dst / f"{cap_name}.tf"
        else:  # "ack" — catalog.py's validator already rejected anything else
            cap_template = render.CATALOG_DIR / "per-service" / "gitops" / "capabilities" / f"{cap_name}.yaml.tmpl"
            out_path = gitops_caps_dst / f"{cap_name}.yaml"

        if not cap_template.exists():
            continue

        out_path.parent.mkdir(parents=True, exist_ok=True)
        content = cap_template.read_text(encoding="utf-8")
        content = render.render_template_string(env, content, cap_data)
        out_path.write_text(content, encoding="utf-8")
        typer.echo(f"  [WROTE] {out_path.relative_to(render.REPO_ROOT)}")


def onboard_team_workload(team_name: str) -> bool:
    catalog_dir = render.CATALOG_DIR
    env = render.create_jinja_env(catalog_dir)
    data = {"TeamName": team_name}

    cat_data = catalog.load_catalog(catalog_dir / "catalog.yaml")

    # Render the per-team tree (apps, infra, gitops) — rendered ONCE per team
    for dest_key in ["per-team/root", "per-team/infra", "per-team/gitops"]:
        src_dir = catalog_dir / dest_key
        dst_dir = render.TENANT_WORKLOADS_DIR / cat_data.destinations[dest_key].format(team=team_name)
        
        if not src_dir.exists():
            continue
            
        for path in src_dir.rglob("*"):
            if path.is_file():
                rel_path = path.relative_to(src_dir)
                out_path = dst_dir / str(rel_path).replace(".tmpl", "").replace("[[ .TeamName ]]", team_name)
                out_path.parent.mkdir(parents=True, exist_ok=True)
                
                content = path.read_text(encoding="utf-8")
                if path.suffix == ".tmpl":
                    content = render.render_template_string(env, content, data)
                
                out_path.write_text(content, encoding="utf-8")
                typer.echo(f"  [WROTE] {out_path.relative_to(render.REPO_ROOT)}")

                
    typer.echo(f"Successfully onboarded team '{team_name}'")
    return True


app = typer.Typer()

@app.command()
def onboard_team(
    team_name: Annotated[str, typer.Option("--team-name", "-t", help="Team/Tenant name")],
) -> bool:
    try:
        validated_data = schemas.OnboardTeamInput(team_name=team_name)
    except ValidationError as e:
        typer.echo("Error: Validation failed.")
        for error in e.errors():
            loc = " -> ".join(str(x) for x in error["loc"])
            typer.echo(f"  - {loc}: {error['msg']}")
        raise typer.Exit(code=1)

    return onboard_team_workload(**validated_data.model_dump())

@app.command()
def add_service(
    team_name: Annotated[str, typer.Option("--team-name", "-t", help="Name of the team/tenant")],
    app_name: Annotated[str, typer.Option("--app-name", "-a", help="Name of the application")],
    golden_path: Annotated[str, typer.Option("--golden-path", "-g", help="Seed configuration from a named golden path")] = "",
    runtime: Annotated[str, typer.Option("--runtime", "-r", help="Application runtime (e.g. go, python)")] = "",
    capabilities: Annotated[str, typer.Option("--capabilities", "-c", help="Comma-separated list of capabilities (e.g. postgres,s3)")] = "",
    system: Annotated[str, typer.Option("--system", "-s", help="Backstage system metadata")] = "",
    env: Annotated[str, typer.Option("--env", "-e", help="Target environment for scaffolding")] = "dev",
) -> bool:
    caps_list = [c.strip() for c in capabilities.split(",") if c.strip()] if capabilities else []
    try:
        validated_data = schemas.AddServiceInput(
            team_name=team_name,
            app_name=app_name,
            golden_path=golden_path,
            runtime=runtime,
            capabilities=caps_list,
            system=system,
            env=env,
        )
    except ValidationError as e:
        typer.echo("Error: Validation failed.")
        for error in e.errors():
            loc = " -> ".join(str(x) for x in error["loc"])
            typer.echo(f"  - {loc}: {error['msg']}")
        raise typer.Exit(code=1)

    add_service_workload(
        team_name=validated_data.team_name,
        app_name=validated_data.app_name,
        golden_path=validated_data.golden_path,
        runtime=validated_data.runtime,
        capabilities=validated_data.capabilities,
        system=validated_data.system,
        env_name=validated_data.env,
    )
    return True

