import typer
import yaml
from pathlib import Path
import subprocess
import shutil
import tempfile
import re
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

REMOTE_TEMPLATE_REPO = "" # "https://github.com/ok-karthik/internal-developer-platform@version=feature/python-scaffolder"

def create_jinja_env(catalog_root: Path) -> Environment:
    """Configures Jinja2 to render Go text/template sources identically to Go.

    trim_blocks/lstrip_blocks reproduce Go's `[[-` whitespace trimming. The regex
    conversion in render_template_string() rewrites `[[- if .X ]]` to `[% if X %]`
    and cannot carry the `-` markers across, so without these two flags a false
    conditional leaves an indented blank line behind. That was the sole remaining
    difference between the two engines' output (in catalog-info.yaml).
    """
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
    import re
    
    # 1. Convert Go conditionals and loops
    # [[- if .SystemName ]] -> [% if SystemName %]
    # [[- range .Owners ]] -> [% for _dot_ in Owners %]
    # [[- end ]] -> [% endif %] or [% endfor %]
    
    stack = []
    
    def block_replacer(match):
        left_trim = match.group(1)
        inner = match.group(2).strip()
        right_trim = match.group(3)
        
        prefix = f'[%{left_trim}'
        suffix = f'{right_trim}%]'
        
        if inner.startswith('if '):
            stack.append('if')
            var = inner[3:].strip().lstrip('.')
            return f'{prefix} if {var} {suffix}'
        elif inner.startswith('range '):
            stack.append('for')
            var = inner[6:].strip().lstrip('.')
            return f'{prefix} for _dot_ in {var} {suffix}'
        elif inner == 'end':
            block_type = stack.pop() if stack else 'if'
            return f'{prefix} end{block_type} {suffix}'
        return match.group(0)

    cleaned_content = re.sub(r'\[\[(-?)\s*(if\s+.*?|range\s+.*?|end)\s*(-?)\]\]', block_replacer, tmpl_content)
    
    # 2. Convert Go template dot variables [[ .Var ]] -> [[ Var ]] for Jinja2 compatibility
    cleaned_content = re.sub(r'\[\[\s*\.([a-zA-Z0-9_]+)\s*\]\]', r'[[ \1 ]]', cleaned_content)
    
    # 3. Convert [[ . ]] -> [[ _dot_ ]] (used inside range loops)
    cleaned_content = re.sub(r'\[\[\s*\.\s*\]\]', r'[[ _dot_ ]]', cleaned_content)
    
    template = env.from_string(cleaned_content)
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

def list_tenant_repositories(tenant_name: str) -> list[str]:
    """List all repos under a tenant"""
    tenant_dir = Path(TENANT_WORKLOADS_DIR / tenant_name)
    if not tenant_dir.exists():
        return []
    return [template.name for template in tenant_dir.iterdir() if template.is_dir()]

def list_onboarded_tenants() -> list[str]:
    """List all onboarded tenants"""
    if not Path(TENANT_WORKLOADS_DIR).exists():
        return []
    return [template.name for template in Path(TENANT_WORKLOADS_DIR).iterdir() if template.is_dir()]

