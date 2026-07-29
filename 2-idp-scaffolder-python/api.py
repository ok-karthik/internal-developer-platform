import schemas, utils
import cli
from fastapi import FastAPI, HTTPException

api = FastAPI(
    title="Platform Control Plane Scaffolder API",
    description="REST API interface for Golden Path application scaffolding and tenant provisioning."
)

@api.get("/api/v1/meta/cloud-services", tags=["Metadata"])
async def get_cloud_services():
    """List all cloud services supported by the platform."""
    return [cs.value for cs in schemas.CloudServices]

@api.get("/api/v1/meta/app-types", tags=["Metadata"])
async def get_app_types():
    """List all application runtimes supported by the platform templates."""
    return {"app_types": utils.list_available_app_types()}

@api.get("/api/v1/teams", tags=["Tenancy"])
async def get_teams():
    """List all currently onboarded teams."""
    return {"teams": utils.list_onboarded_teams()}

@api.get("/api/v1/teams/{team_name}/repositories", tags=["Tenancy"])
async def get_repositories(team_name: str):
    """List all repositories generated for a specific team."""
    return {"repositories": utils.list_tenant_repositories(team_name)}

@api.post("/api/v1/applications", status_code=201, tags=["Scaffolding"])
def scaffold_application(app: schemas.AppDetails):
    """Scaffold a new microservice workload for a tenant."""
    try:
        cli.scaffold_tenant_workload(**app.model_dump(mode='json'))
        return {"message": "Application scaffolded and provisioned successfully"}
    except HTTPException as he:
        raise he
    except Exception as e:
        exc_name = e.__class__.__name__
        print(f"Error during application scaffolding: {exc_name}: {e}")
        raise HTTPException(status_code=500, detail=f"{exc_name}: {e}")