# Phase 10.2: proves the portability seam with exactly ONE capability, on
# ONE second provider, and stops there — this is not "port the platform,"
# it is "port postgres." Azure chosen over STACKIT per PLAN.md Phase 10.3's
# revised ordering: the Target Environment section names AKS as a stated
# secondary target, and Azure Workload Identity is the direct analogue of
# EKS Pod Identity (Phase 7.2c), which makes that seam concrete rather than
# theoretical. STACKIT remains the differentiator to add *after* this one.
#
# THE CONTRACT THIS MUST MATCH (PLAN.md Phase 10.2): "the module must expose
# the same variable and output contract as the AWS one (team_name, app_name,
# env, tags in; connection details out)." If this contract diverges from
# ../../aws/postgres/main.tf's, the portability demo proves the opposite of
# what it intends — that switching `module: aws/postgres` to
# `module: azure/postgres` in catalog.yaml is NOT actually a one-line change.
# Diff the two files' variable/output blocks to verify parity directly
# rather than trusting this comment.
terraform {
  required_version = ">= 1.5"
  required_providers {
    azurerm = { source = "hashicorp/azurerm", version = "~> 5.0" }
    random  = { source = "hashicorp/random", version = "~> 3.6" }
  }
}

# --- Shared contract (identical names to ../../aws/postgres/main.tf) --------

variable "team_name" {
  type        = string
  description = "Team/tenant that owns this database"
}

variable "app_name" {
  type        = string
  description = "Application the database belongs to"
}

variable "env" {
  type        = string
  description = "Target environment (dev, staging, prod)"
  default     = "dev"
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to every resource, supplied by the scaffolder"
  default     = {}
}

# --- Provider-specific extras (Azure has no VPC/subnet_ids concept; a
#     resource group and location are the Azure-native equivalent of "which
#     account/region does this land in" that the AWS module gets for free
#     from its provider block) -------------------------------------------

variable "resource_group_name" {
  type        = string
  description = "Azure resource group this database is created in"
}

variable "location" {
  type        = string
  default     = "germanywestcentral"
  description = "Azure region. Defaults to an EU region for the same data-residency reasoning as the eu-central-1 SCP in 1-cloud-foundation/aws/organization/."
}

variable "subnet_id" {
  type        = string
  description = "Delegated subnet for VNet integration. Empty means public network access instead — the Azure analogue of the AWS module's optional vpc_id/subnet_ids (empty = no security group created)."
  default     = ""
}

variable "sku_name" {
  type        = string
  description = "Flexible Server compute tier, e.g. B_Standard_B1ms. Azure's rough equivalent of the AWS module's instance_class."
  default     = "B_Standard_B1ms"
}

variable "postgres_version" {
  type        = string
  description = "Major PostgreSQL version. Pinned for the same reason as the AWS module's engine_version — do not let the provider pick a new default under you."
  default     = "16"
}

variable "backup_retention_days" {
  type        = number
  description = "Automated backup retention. Azure Flexible Server defaults to 7 already (unlike AWS RDS, which defaults to 0) but this is set explicitly anyway so both modules read the same either way."
  default     = 7
}

# random_string, not random_password: this suffix goes into the server NAME,
# which (like the AWS module's RDS identifier) must be globally unique and
# lowercase. upper must be explicitly false — the same shipped-broken lesson
# AGENTS.md records for the AWS modules applies here too.
resource "random_string" "server_suffix" {
  length  = 6
  upper   = false
  special = false
}

locals {
  name_prefix = lower("${var.team_name}-${var.app_name}-${var.env}")
  server_name = "${substr(local.name_prefix, 0, 56)}-${random_string.server_suffix.result}"
}

resource "azurerm_postgresql_flexible_server" "this" {
  name                = local.server_name
  resource_group_name = var.resource_group_name
  location            = var.location

  version    = var.postgres_version
  sku_name   = var.sku_name
  storage_mb = 32768

  administrator_login = "dbadmin"
  # Azure has no "manage_master_user_password" analogue that hands rotation
  # to a managed secret store the way AWS's manage_master_user_password
  # does — Key Vault + a rotation policy is the equivalent, and wiring it is
  # a documented follow-up rather than built here, same spirit as the AWS
  # module's own TODO on skip_final_snapshot.
  administrator_password = random_password.admin.result

  # Guardrails a tenant cannot switch off from the scaffolder — same
  # non-negotiable posture as the AWS module.
  public_network_access_enabled = var.subnet_id == ""
  backup_retention_days         = var.backup_retention_days
  geo_redundant_backup_enabled  = false # cost choice for a demo; production would flip this per env, same as the AWS module's skip_final_snapshot TODO

  tags = merge(var.tags, {
    Name  = local.server_name
    Owner = var.team_name
  })

  lifecycle {
    ignore_changes = [zone]
  }
}

resource "random_password" "admin" {
  length  = 20
  special = true
  # Azure Flexible Server rejects a narrower set of special characters than
  # RDS does; override_special avoids the ones it disallows in admin
  # passwords rather than guessing at apply time.
  override_special = "!#$%&*()-_=+[]{}<>:?"
}

# --- Shared contract, output side --------------------------------------

output "db_identifier" {
  description = "Generated server name, including the uniqueness suffix — same meaning as the AWS module's db_identifier"
  value       = azurerm_postgresql_flexible_server.this.name
}

output "db_endpoint" {
  description = "Connection endpoint — same meaning as the AWS module's db_endpoint"
  value       = azurerm_postgresql_flexible_server.this.fqdn
}

# Named differently from the AWS module's master_secret_arn on purpose:
# Azure's credential-rotation primitive is a Key Vault secret ID, not an
# ARN, and calling it an "arn" here would be the wrong vocabulary for the
# platform it actually runs on. Same INFORMATION (where to go get the
# password), different provider-native name for it — this is the same
# "analogue, not equivalent" point Phase 10.4 makes about workload identity,
# extended one level down to a single output's naming.
output "admin_credentials_note" {
  description = "Azure has no manage_master_user_password equivalent wired here yet (see the resource comment) — this documents the gap rather than pointing at a Key Vault secret that does not exist."
  value       = "unmanaged: administrator_password is in Terraform state only; wire Key Vault rotation before using this for anything beyond a portability demo"
}
