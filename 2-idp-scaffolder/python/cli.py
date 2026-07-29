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

app = typer.Typer()

@app.command()
def create(
    app_name: Annotated[str, typer.Option("--app-name", "-a", help="<app-name> - Name of the application to be generated")],
    app_type: Annotated[str, typer.Option("--app-type", "-t", help="<app-type> - Type of the application to be generated")],
    app_port: Annotated[int, typer.Option("--app-port", "-p", help="<app-port> - Port of the application to be generated")],
    team_name: Annotated[str, typer.Option("--team-name", "-team", help="<team-name> - Team/Tenant name")],
    cloud_services: Annotated[list[str], typer.Option("--cloud-services", "-cs", help="<cloud-services> - List of Cloud Services to be included in the workload")]=[]
) -> bool:
    try:
        validated_data = schemas.AppDetails(
            app_name=app_name,
            app_type=app_type,
            app_port=app_port,
            team_name=team_name,
            cloud_services=cloud_services
        )
    except ValidationError as e:
        typer.echo("Error: Validation failed.")
        for error in e.errors():
            loc = " -> ".join(str(x) for x in error["loc"])
            typer.echo(f"  - {loc}: {error['msg']}")
        raise typer.Exit(code=1)

    scaffold_tenant_workload(**validated_data.model_dump(mode="json"))