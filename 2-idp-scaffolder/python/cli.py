import typer, copier
from typing import Annotated
import schemas, utils

from pydantic import ValidationError

DEFAULT_CLOUD_SERVICES = ["aws-vpc", "aws-iam"]

def scaffold_tenant_workload(app_name: str, app_type: str, app_port: int, team_name: str, cloud_services: list[str] = list()) -> bool:
    """Scaffolds the workspace directory, helm charts, and starting code for a tenant application.
    
    Args:
        app_name: Name of the application to be generated
        app_type: Type of the application (e.g. python, golang)
        app_port: Port the application listens on
        team_name: Tenant/Team namespace
        cloud_services: Supported cloud service modules to enable
        
    Returns:
        True if the scaffolding was completed successfully
    """
    # IPAM Network Allocation
    vpc_cidr = utils.allocate_vpc_cidr_block(team_name)

    # Resolve the template base directory (supports local fallback or cloned remote repository)
    template_base_dir = utils.get_template_base_dir()

    # Pass 1: Scaffold common team infrastructure & GitOps workflows
    copier.run_copy(
        str(template_base_dir / "templates" / "tenant-template"),
        str(utils.TENANT_WORKLOADS_DIR),
        data={
            "team_name": team_name,
            "tenant_name": team_name,
            "app_name": app_name,
            "app_type": app_type,
            "app_port": app_port,
            "cloud_services": cloud_services,
            "vpc_cidr": vpc_cidr
        },
        overwrite=True,
        defaults=True
    )

    # Pass 2: Inject the language starter application files
    copier.run_copy(
        str(template_base_dir / "templates" / "apps-source" / app_type),
        str(utils.TENANT_WORKLOADS_DIR / team_name / "apps-source" / app_name),
        data={
            "team_name": team_name,
            "tenant_name": team_name,
            "app_name": app_name,
            "app_port": app_port
        },
        overwrite=True,
        defaults=True
    )
    return True

def onboard_team_workload(team_name: str) -> bool:
    catalog_dir = utils.CATALOG_DIR
    vpc_cidr = utils.allocate_vpc_cidr_block(team_name)
    env = utils.create_jinja_env(catalog_dir)
    data = {"TeamName": team_name, "VpcCidr": vpc_cidr}

    # Render team blueprints (apps, infra, gitops)
    for blueprint_kind in ["apps", "infra", "gitops"]:
        src_dir = catalog_dir / "blueprints" / "team" / blueprint_kind
        dst_dir = utils.TENANT_WORKLOADS_DIR / team_name / blueprint_kind
        
        if not src_dir.exists():
            continue
            
        for path in src_dir.rglob("*"):
            if path.is_file():
                rel_path = path.relative_to(src_dir)
                out_path = dst_dir / str(rel_path).replace(".tmpl", "").replace("[[ .TeamName ]]", team_name)
                out_path.parent.mkdir(parents=True, exist_ok=True)
                
                content = path.read_text(encoding="utf-8")
                if path.suffix == ".tmpl":
                    content = utils.render_template_string(env, content, data)
                
                out_path.write_text(content, encoding="utf-8")
                
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

    # TODO: render runtime, service-meta, delivery release values & capability modules into 3-tenant-workloads
    typer.echo(f"Scaffolded service '{app_name}' for team '{team_name}' (env: {env}) successfully.")
    return True
