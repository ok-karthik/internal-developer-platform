import typer
import yaml
from pathlib import Path
import subprocess
import shutil
import tempfile
from jinja2 import Environment, FileSystemLoader, StrictUndefined


# Absolute path to repository root (2 levels up from 2-idp-scaffolder/python/)
REPO_ROOT: Path = Path(__file__).resolve().parents[2]

# Platform catalog containing golden paths and capabilities
CATALOG_DIR: Path = REPO_ROOT / "1-platform-catalog"

# Target output directory for tenant workloads
TENANT_WORKLOADS_DIR: Path = REPO_ROOT / "3-tenant-workloads"

env = Environment(
    loader=FileSystemLoader(CATALOG_DIR),
    variable_start_string="[[", variable_end_string="]]",
    block_start_string="[%",   block_end_string="%]",
    keep_trailing_newline=True,   # preserve trailing newlines
    undefined=StrictUndefined,    # fail on unhandled variables (matches Go behavior)
)

DEFAULT_CLOUD_SERVICES = ["aws-networking", "aws-iam"]

# IPAM registry file for tracking VPC CIDR allocations
IPAM_REGISTRY_FILE = TENANT_WORKLOADS_DIR / "cloud_vpcs_allocated.yaml"



REMOTE_TEMPLATE_REPO = "" # "https://github.com/ok-karthik/internal-developer-platform@version=feature/python-scaffolder"

def create_jinja_env(catalog_root: Path) -> Environment:
    """Configures Jinja2 with Go-compatible delimiters [[ ]] and strict erroring."""
    return Environment(
        loader=FileSystemLoader(str(catalog_root)),
        variable_start_string="[[",
        variable_end_string="]]",
        block_start_string="[%",
        block_end_string="%]",
        keep_trailing_newline=True,
        undefined=StrictUndefined,
    )

def render_template_string(env: Environment, tmpl_content: str, data: dict) -> str:
    """Renders an in-memory template string with Go delimiters."""
    template = env.from_string(tmpl_content)
    return template.render(**data)

def get_template_base_dir() -> Path:
    """Gets the path to the template base directory.
    
    If REMOTE_TEMPLATE_REPO is a remote git URL, it clones it to a temporary directory
    (or uses a local cached version) and returns that path.
    Otherwise, it returns the local SCAFFOLDER_PKG_ROOT.
    """
    if REMOTE_TEMPLATE_REPO and REMOTE_TEMPLATE_REPO.startswith(("http://", "https://", "git@", "ssh://")):
        repo_url = REMOTE_TEMPLATE_REPO
        version = None
        if "@" in repo_url:
            parts = repo_url.rsplit("@", 1)
            repo_url = parts[0]
            version_part = parts[1]
            if version_part.startswith("version="):
                version = version_part.split("version=")[1]
            else:
                version = version_part

        # Use a deterministic cache path in the temp directory
        cache_dir = Path(tempfile.gettempdir()) / "idp-scaffolder-templates"
        
        try:
            # Clone or checkout the repo
            if cache_dir.exists():
                try:
                    shutil.rmtree(cache_dir)
                except Exception:
                    pass
                
            cmd = ["git", "clone", "--depth", "1", "--branch", version, repo_url, str(cache_dir)]
                
            subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            
            return cache_dir / "1-platform-catalog"
        except Exception as e:
            print(f"Warning: Failed to retrieve remote templates from {repo_url} (Error: {e}). Falling back to local templates.")
            return CATALOG_DIR
    else:
        return CATALOG_DIR

def list_available_capabilities() -> list[str]:
    """List all capabilities defined in catalog.yaml"""
    catalog_path = CATALOG_DIR / "catalog.yaml"
    if catalog_path.exists():
        with open(catalog_path, "r") as f:
            data = yaml.safe_load(f) or {}
            return list(data.get("capabilities", {}).keys())
    return ["postgres", "s3", "iam"]


def list_available_cloud_services() -> list[str]:
    """List all cloud service templates except the default ones"""
    # Since these are now remote modules, we return the list of supported ones
    return ["aws-postgres", "aws-s3"]

def list_tenant_repositories(team_name: str) -> list[str]:
    """List all repos under a team"""
    team_dir = Path(TENANT_WORKLOADS_DIR / team_name)
    if not team_dir.exists():
        return []
    return [template.name for template in team_dir.iterdir() if template.is_dir()]

def list_onboarded_teams() -> list[str]:
    """List all onboarded teams"""
    if not Path(TENANT_WORKLOADS_DIR).exists():
        return []
    return [template.name for template in Path(TENANT_WORKLOADS_DIR).iterdir() if template.is_dir()]

def allocate_vpc_cidr_block(team_name: str) -> str:
    """Gets the existing CIDR block for a team, or allocates the next available /16 range."""
    
    # 1. Load the existing allocations
    cloud_vpcs_allocated = {}
    if IPAM_REGISTRY_FILE.exists() and IPAM_REGISTRY_FILE.stat().st_size > 0:
        with open(IPAM_REGISTRY_FILE, "r") as f:
            cloud_vpcs_allocated = yaml.safe_load(f) or {}

    # 2. Get or calculate allocation
    if team_name in cloud_vpcs_allocated:
        return cloud_vpcs_allocated[team_name]
    
    # Determine the next second octet (10.x.0.0/16)
    if cloud_vpcs_allocated:
        max_index = max([int(cidr.split(".")[1]) for cidr in cloud_vpcs_allocated.values()])
        next_index = max_index + 1
    else:
        next_index = 0  # Start at 10.0.0.0/16
        
    vpc_cidr = f"10.{next_index}.0.0/16"
    
    # 3. Save the new allocation back to the YAML file
    cloud_vpcs_allocated[team_name] = vpc_cidr
    TENANT_WORKLOADS_DIR.mkdir(parents=True, exist_ok=True)
    with open(IPAM_REGISTRY_FILE, "w") as f:
        yaml.safe_dump(cloud_vpcs_allocated, f)
        typer.echo(f"Allocated VPC CIDR: {vpc_cidr}")
        
    return vpc_cidr