terraform {
  required_version = ">= 1.5"

  # Declared here, NOT as a `provider "random" {}` block — see the note in
  # ../s3/main.tf. Providers are configured in the root module.
  required_providers {
    aws    = { source = "hashicorp/aws", version = "~> 5.0" }
    random = { source = "hashicorp/random", version = "~> 3.6" }
  }
}

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

variable "vpc_id" {
  type        = string
  description = "VPC to place the database in. Empty means no security group is created."
  default     = ""
}

variable "subnet_ids" {
  type        = list(string)
  description = "Subnets for the DB subnet group. Empty means none is created."
  default     = []
}

variable "engine_version" {
  type        = string
  description = "Major PostgreSQL version. Pinned so AWS cannot pick a new default under you."
  default     = "16"
}

variable "instance_class" {
  type        = string
  description = "RDS instance size"
  default     = "db.t3.micro"
}

variable "backup_retention_days" {
  type        = number
  description = "Automated backup retention. Terraform's default is 0, which disables backups entirely."
  default     = 7
}

# RDS identifiers are lowercase-only, so `upper` must be false here for the same
# reason as the S3 bucket suffix — random_string defaults it to true, and setting
# only `lower = true` does not exclude uppercase.
resource "random_string" "db_suffix" {
  length  = 6
  upper   = false
  special = false
}

locals {
  name_prefix = lower("${var.team_name}-${var.app_name}-${var.env}")
  identifier  = "${substr(local.name_prefix, 0, 56)}-${random_string.db_suffix.result}"
}

resource "aws_db_subnet_group" "this" {
  count      = length(var.subnet_ids) > 0 ? 1 : 0
  name       = "${local.identifier}-subnets"
  subnet_ids = var.subnet_ids

  tags = merge(var.tags, { Name = "${local.identifier}-subnets" })
}

resource "aws_security_group" "this" {
  count       = var.vpc_id != "" ? 1 : 0
  name        = "${local.identifier}-sg"
  description = "PostgreSQL access for ${var.team_name}/${var.app_name}"
  vpc_id      = var.vpc_id

  ingress {
    description = "PostgreSQL from within the private supernet"
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/8"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(var.tags, { Name = "${local.identifier}-sg" })
}

resource "aws_db_instance" "this" {
  identifier     = local.identifier
  engine         = "postgres"
  engine_version = var.engine_version
  instance_class = var.instance_class

  allocated_storage = 20
  username          = "dbadmin"

  # manage_master_user_password hands the credential to AWS Secrets Manager and
  # rotates it there. The previous version generated it with random_password,
  # which writes the plaintext secret into Terraform state — readable by anyone
  # with state access, and impossible to rotate without a Terraform run.
  #
  # It also removes a latent bug: the old override_special set included "@",
  # which RDS rejects in master passwords (as it does "/", '"', and space).
  manage_master_user_password = true

  db_subnet_group_name   = length(var.subnet_ids) > 0 ? aws_db_subnet_group.this[0].name : null
  vpc_security_group_ids = var.vpc_id != "" ? [aws_security_group.this[0].id] : null

  # Guardrails a tenant cannot switch off from the scaffolder.
  publicly_accessible     = false
  storage_encrypted       = true
  backup_retention_period = var.backup_retention_days

  # TODO(platform): skip_final_snapshot is convenient for dev and wrong for prod.
  # Drive it from var.env once a prod environment exists.
  skip_final_snapshot = true

  tags = merge(var.tags, {
    Name  = local.identifier
    Owner = var.team_name
  })
}

output "db_identifier" {
  description = "Generated RDS identifier, including the uniqueness suffix"
  value       = aws_db_instance.this.identifier
}

output "db_endpoint" {
  description = "Connection endpoint"
  value       = aws_db_instance.this.endpoint
}

output "master_secret_arn" {
  description = "Secrets Manager ARN holding the master credential"
  value       = aws_db_instance.this.master_user_secret[0].secret_arn
}
