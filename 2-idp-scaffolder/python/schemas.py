from pydantic import BaseModel, Field

class OnboardTenantInput(BaseModel):
    """Validation schema for onboard-tenant command"""
    model_config = {"extra": "forbid"}
    tenant_name: str = Field(min_length=2, max_length=40, pattern=r"^[a-z0-9][-a-z0-9]*[a-z0-9]$", description="Team/Tenant name")
    owner: list[str] = Field(default_factory=list, description="Owners of the tenant")

class AddServiceInput(BaseModel):
    """Validation schema for add-service command"""
    model_config = {"extra": "forbid"}
    tenant_name: str = Field(min_length=2, max_length=40, pattern=r"^[a-z0-9][-a-z0-9]*[a-z0-9]$", description="Team/Tenant name")
    app_name: str = Field(min_length=2, max_length=40, pattern=r"^[a-z0-9][-a-z0-9]*[a-z0-9]$", description="Name of the application")
    golden_path: str = Field(default="", description="Named golden path to seed defaults from")
    runtime: str = Field(default="", description="Runtime language/framework override")
    capabilities: list[str] = Field(default_factory=list, description="List of capabilities")
    system: str = Field(default="", description="Backstage system metadata")
    env: str = Field(default="dev", description="Target environment")

