from enum import Enum
from pydantic import BaseModel, Field, ValidationError
import utils

AppType = Enum("AppType", {t: t for t in utils.list_available_app_types()}, type=str)

CloudServices = Enum("CloudService", {t: t for t in utils.list_available_cloud_services()}, type=str)

# Validate the input details datatypes and ensure the data is sent via JSON body rather than URL parameters
class AppDetails(BaseModel):
    """App Model request body validations"""
    model_config = {
        "extra": "forbid"
    }
    app_name: str = Field(min_length=2, max_length=40, pattern=r"^[a-z0-9][-a-z0-9]*[a-z0-9]$", description="Name of the application to be generated")
    app_type: AppType
    app_port: int = Field(default=8080, ge=0, le=65535, description="Port number of the application to be generated")
    team_name: str = Field(min_length=2, max_length=40, pattern=r"^[a-z0-9][-a-z0-9]*[a-z0-9]$", description="Team/Tenant name of the application to be generated")
    cloud_services: list[CloudServices] = []
